from __future__ import annotations

import base64
from collections.abc import Mapping
import io
import logging
import os
import shutil
import tempfile
import threading
import time
from typing import Annotated, Any

import numpy as np
from fastapi import FastAPI, HTTPException, Request, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from paddleocr import PaddleOCR
from pdf2image import convert_from_path, pdfinfo_from_path
from PIL import Image
from pydantic import BaseModel, Field
from starlette.datastructures import UploadFile as StarletteUploadFile

logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
logger = logging.getLogger(__name__)


def _get_env_int(name: str, default: int) -> int:
    raw = os.getenv(name)
    if raw is None or raw == "":
        return default
    try:
        value = int(raw)
    except ValueError:
        logger.warning("Invalid %s=%r; using default %d", name, raw, default)
        return default
    if value <= 0:
        logger.warning("Non-positive %s=%r; using default %d", name, raw, default)
        return default
    return value


MAX_PDF_UPLOAD_BYTES = _get_env_int("MAX_PDF_UPLOAD_BYTES", 100 * 1024 * 1024)
MAX_PDF_PAGES = _get_env_int("MAX_PDF_PAGES", 200)
MAX_PDF_DPI = _get_env_int("MAX_PDF_DPI", 300)
OCR_SHARED_STORAGE_DIR = os.path.realpath(os.getenv("OCR_SHARED_STORAGE_DIR", "")) if os.getenv("OCR_SHARED_STORAGE_DIR", "") else ""

app = FastAPI(title="LokLingo OCR Service", version="2.0.0")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

# ---------------------------------------------------------------------------
# Language mapping: frontend lang codes → PaddleOCR engine codes
# "ch" supports Chinese + English and most Latin scripts with mixed content.
# For pure Latin-script languages without a dedicated PaddleOCR model,
# "en" gives the best results.
# ---------------------------------------------------------------------------
LANG_MAP: dict[str, str] = {
    "auto":  "ch",      # ch model handles CJK + Latin well (best for unknown scripts)
    "en":    "en",
    "de":    "german",
    "fr":    "french",
    "es":    "es",
    "hi":    "hi",
    "ur":    "arabic",  # Urdu uses Arabic script
    "ar":    "arabic",
    "bn":    "hi",      # Bengali — closest supported: Hindi (Devanagari family)
    "zh":    "ch",
    "ja":    "japan",
    "ko":    "korean",
    "ru":    "ru",
    "pt":    "pt",
    "it":    "it",
    "nl":    "dutch",
    "tr":    "turkish",
    "pl":    "polish",
    "vi":    "vi",
}

# Cache PaddleOCR instances keyed by paddle_lang to avoid re-loading models
_ocr_cache: dict[str, PaddleOCR] = {}
_cache_lock = threading.Lock()


def _build_ocr_engine(paddle_lang: str) -> PaddleOCR:
    """Construct PaddleOCR with backwards/forwards-compatible kwargs."""
    kwargs = {
        "use_angle_cls": True,  # auto-correct rotated text
        "lang": paddle_lang,
        "show_log": False,
        "use_gpu": False,
        "rec_batch_num": 6,     # process multiple lines at once
    }
    while True:
        try:
            return PaddleOCR(**kwargs)
        except ValueError as exc:
            # PaddleOCR >=3 may reject legacy args (e.g., use_gpu/show_log).
            msg = str(exc)
            if "Unknown argument:" not in msg:
                raise
            bad_arg = msg.split("Unknown argument:", 1)[1].strip().split()[0]
            if bad_arg not in kwargs:
                raise
            logger.warning("PaddleOCR arg %s unsupported; retrying without it", bad_arg)
            kwargs.pop(bad_arg)


def get_ocr(lang: str) -> PaddleOCR:
    """Return a cached PaddleOCR instance for the given language."""
    paddle_lang = LANG_MAP.get(lang.lower(), "ch")
    with _cache_lock:
        if paddle_lang not in _ocr_cache:
            logger.info("Initialising PaddleOCR engine: lang=%s", paddle_lang)
            _ocr_cache[paddle_lang] = _build_ocr_engine(paddle_lang)
    return _ocr_cache[paddle_lang]


# ---------------------------------------------------------------------------
# Pydantic models
# ---------------------------------------------------------------------------

LangField = Annotated[
    str,
    Field(default="auto", description="Language hint — ISO 639-1 code or 'auto'"),
]


class TextBlock(BaseModel):
    text: str
    confidence: float = Field(ge=0.0, le=1.0)
    bbox: list[float] = Field(
        description="Axis-aligned bounding box [x1, y1, x2, y2]",
        min_length=4,
        max_length=4,
    )


class OCRImageRequest(BaseModel):
    image_b64: str = Field(description="Base64-encoded image (PNG, JPEG, BMP, TIFF, …)")
    mime_type: str = Field(default="image/png")
    lang: LangField = "auto"  # type: ignore[assignment]


class OCRImageResponse(BaseModel):
    text: str = Field(description="Full extracted text (lines joined by newline)")
    confidence: float = Field(description="Average confidence across all detected blocks")
    blocks: list[TextBlock] = Field(description="Per-line text blocks with bounding boxes")


class OCRPdfRequest(BaseModel):
    pdf_b64: str | None = Field(default=None, description="Base64-encoded PDF file")
    file_path: str | None = Field(default=None, description="Absolute path to a shared PDF file")
    lang: LangField = "auto"  # type: ignore[assignment]
    dpi: int = Field(default=200, ge=72, le=400, description="Rendering DPI for each page — clamped to MAX_PDF_DPI at runtime")


class PageResult(BaseModel):
    page_number: int
    text: str
    confidence: float
    blocks: list[TextBlock]


class OCRPdfResponse(BaseModel):
    text: str = Field(description="Full extracted text across all pages")
    confidence: float
    pages: list[PageResult]


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

def _ocr_pil_image(image: Image.Image, lang: str) -> list[TextBlock]:
    """Run PaddleOCR on a PIL Image and return structured TextBlock list."""
    ocr = get_ocr(lang)
    img_array = np.array(image.convert("RGB"))
    try:
        result = ocr.ocr(img_array, cls=True)
    except TypeError as exc:
        if "unexpected keyword argument 'cls'" not in str(exc):
            raise
        logger.warning("PaddleOCR ocr() does not accept cls kwarg; retrying without it")
        result = ocr.ocr(img_array)

    def _to_bbox(poly: Any) -> list[float]:
        xs: list[float] = []
        ys: list[float] = []
        if poly is None:
            return [0.0, 0.0, 0.0, 0.0]
        for pt in poly:
            try:
                xs.append(float(pt[0]))
                ys.append(float(pt[1]))
            except Exception:
                continue
        if not xs or not ys:
            return [0.0, 0.0, 0.0, 0.0]
        return [min(xs), min(ys), max(xs), max(ys)]

    blocks: list[TextBlock] = []
    for page in result or []:
        # PaddleOCR v3 pipeline output shape: dict-like OCRResult with
        # arrays in dt_polys + rec_texts + rec_scores.
        if isinstance(page, Mapping) or (hasattr(page, "get") and hasattr(page, "keys")):
            texts = page.get("rec_texts") or []
            scores = page.get("rec_scores") or []
            polys = page.get("dt_polys") or page.get("rec_polys") or []
            count = min(len(texts), len(scores), len(polys))
            for i in range(count):
                text = str(texts[i])
                conf = float(scores[i])
                bbox = _to_bbox(polys[i])
                blocks.append(
                    TextBlock(
                        text=text,
                        confidence=round(conf, 4),
                        bbox=bbox,
                    )
                )
            continue

        # PaddleOCR v2 legacy output shape: list of line tuples.
        for line in page or []:
            if not isinstance(line, (list, tuple)):
                continue

            bbox_raw: Any
            text: Any
            conf: Any
            if len(line) >= 2 and isinstance(line[1], (list, tuple)) and len(line[1]) >= 2:
                bbox_raw = line[0]
                text = line[1][0]
                conf = line[1][1]
            elif len(line) >= 3:
                bbox_raw = line[0]
                text = line[1]
                conf = line[2]
            else:
                continue

            bbox = _to_bbox(bbox_raw)
            blocks.append(
                TextBlock(
                    text=str(text),
                    confidence=round(float(conf), 4),
                    bbox=bbox,
                )
            )
    return blocks


def _summarise(blocks: list[TextBlock]) -> tuple[str, float]:
    """Return (joined_text, average_confidence) from a block list."""
    if not blocks:
        return "", 0.0
    text = "\n".join(b.text for b in blocks)
    avg_conf = sum(b.confidence for b in blocks) / len(blocks)
    return text, round(avg_conf, 4)


def _parse_lang(raw: object) -> str:
    if not isinstance(raw, str) or not raw.strip():
        return "auto"
    return raw.strip()


def _parse_dpi(raw: object) -> int:
    if raw is None or raw == "":
        return 200
    try:
        dpi = int(raw)
    except (TypeError, ValueError):
        raise HTTPException(status_code=400, detail="Invalid dpi value")
    if dpi < 72:
        raise HTTPException(status_code=422, detail="dpi must be at least 72")
    if dpi > MAX_PDF_DPI:
        logger.warning("Requested dpi=%d exceeds MAX_PDF_DPI=%d; clamping", dpi, MAX_PDF_DPI)
        dpi = MAX_PDF_DPI
    return dpi


def _write_upload_to_temp_pdf(upload: UploadFile, max_bytes: int | None = None) -> str:
    if max_bytes is None:
        max_bytes = MAX_PDF_UPLOAD_BYTES
    suffix = os.path.splitext(upload.filename or "")[1].lower() or ".pdf"
    tmp = tempfile.NamedTemporaryFile(delete=False, suffix=suffix)
    try:
        with tmp:
            bytes_written = 0
            while True:
                chunk = upload.file.read(1024 * 1024)
                if not chunk:
                    break
                bytes_written += len(chunk)
                if bytes_written > max_bytes:
                    raise HTTPException(
                        status_code=413,
                        detail=(
                            f"PDF file exceeds max size of {max_bytes} bytes"
                        ),
                    )
                tmp.write(chunk)
        return tmp.name
    except Exception:
        try:
            os.remove(tmp.name)
        except OSError:
            pass
        raise


def _write_pdf_bytes_to_temp_pdf(pdf_bytes: bytes, max_bytes: int | None = None) -> str:
    if max_bytes is None:
        max_bytes = MAX_PDF_UPLOAD_BYTES
    if len(pdf_bytes) > max_bytes:
        raise HTTPException(
            status_code=413,
            detail=f"PDF file exceeds max size of {max_bytes} bytes",
        )

    tmp = tempfile.NamedTemporaryFile(delete=False, suffix=".pdf")
    try:
        with tmp:
            tmp.write(pdf_bytes)
        return tmp.name
    except Exception:
        try:
            os.remove(tmp.name)
        except OSError:
            pass
        raise


def _resolve_shared_pdf_path(raw_path: object) -> str:
    if not isinstance(raw_path, str) or not raw_path.strip():
        raise HTTPException(status_code=400, detail="file_path is required")
    if not OCR_SHARED_STORAGE_DIR:
        raise HTTPException(status_code=400, detail="shared file access is not enabled")

    resolved = os.path.realpath(raw_path.strip())
    try:
        if os.path.commonpath([resolved, OCR_SHARED_STORAGE_DIR]) != OCR_SHARED_STORAGE_DIR:
            raise HTTPException(status_code=400, detail="file_path must be inside shared storage dir")
    except ValueError:
        raise HTTPException(status_code=400, detail="file_path must be inside shared storage dir")

    if not os.path.exists(resolved):
        raise HTTPException(status_code=404, detail="shared PDF file does not exist")
    if not os.path.isfile(resolved):
        raise HTTPException(status_code=400, detail="file_path must point to a file")

    try:
        size = os.path.getsize(resolved)
    except OSError as exc:
        logger.exception("Shared PDF stat error: %s", exc)
        raise HTTPException(status_code=422, detail="Cannot access shared PDF file")
    if size > MAX_PDF_UPLOAD_BYTES:
        raise HTTPException(
            status_code=413,
            detail=f"PDF file exceeds max size of {MAX_PDF_UPLOAD_BYTES} bytes",
        )

    return resolved


def _ocr_pdf_file(
    file_path: str,
    lang: str,
    dpi: int,
    max_pages: int | None = None,
) -> OCRPdfResponse:
    if max_pages is None:
        max_pages = MAX_PDF_PAGES
    try:
        info = pdfinfo_from_path(file_path)
        page_count = int(info["Pages"])
    except Exception as exc:
        logger.exception("PDF metadata read error: %s", exc)
        raise HTTPException(
            status_code=422,
            detail="Cannot convert PDF to images — ensure it is a valid PDF file",
        )

    if page_count > max_pages:
        logger.warning(
            "Rejecting PDF with too many pages: pages=%d max=%d file=%s",
            page_count,
            max_pages,
            file_path,
        )
        raise HTTPException(
            status_code=413,
            detail=f"PDF exceeds max page count of {max_pages}",
        )

    pages: list[PageResult] = []
    full_text_buffer = io.StringIO()
    wrote_text = False
    confidence_sum = 0.0
    confidence_count = 0

    for page_num in range(1, page_count + 1):
        t_render_start = time.perf_counter()
        try:
            images = convert_from_path(
                file_path,
                dpi=dpi,
                first_page=page_num,
                last_page=page_num,
            )
            if not images:
                raise ValueError(f"no image returned for page {page_num}")
            image = images[0]
            del images  # release list reference before OCR to avoid holding two copies
        except Exception as exc:
            logger.exception("PDF→image conversion error on page %d: %s", page_num, exc)
            raise HTTPException(
                status_code=422,
                detail="Cannot convert PDF to images — ensure it is a valid PDF file",
            )
        render_ms = (time.perf_counter() - t_render_start) * 1000

        t_ocr_start = time.perf_counter()
        try:
            blocks = _ocr_pil_image(image, lang)
        except Exception as exc:
            logger.exception("OCR engine error on page %d: %s", page_num, exc)
            blocks = []
        finally:
            image.close()
        ocr_ms = (time.perf_counter() - t_ocr_start) * 1000

        logger.info(
            "page %d/%d render_ms=%.0f ocr_ms=%.0f blocks=%d",
            page_num, page_count, render_ms, ocr_ms, len(blocks),
        )

        page_text, page_conf = _summarise(blocks)
        pages.append(
            PageResult(
                page_number=page_num,
                text=page_text,
                confidence=page_conf,
                blocks=blocks,
            )
        )

        if page_text:
            if wrote_text:
                full_text_buffer.write("\n")
            full_text_buffer.write(page_text)
            wrote_text = True
        confidence_sum += sum(block.confidence for block in blocks)
        confidence_count += len(blocks)

    full_text = full_text_buffer.getvalue()
    full_text_buffer.close()
    avg_conf = round(confidence_sum / confidence_count, 4) if confidence_count else 0.0
    return OCRPdfResponse(text=full_text, confidence=avg_conf, pages=pages)


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------

def _gpu_available() -> bool:
    """Return True if a CUDA-capable GPU is visible to the process."""
    try:
        import paddle  # type: ignore
        return bool(paddle.device.is_compiled_with_cuda() and paddle.device.cuda.device_count() > 0)
    except Exception:
        return False


@app.on_event("startup")
async def log_startup_config() -> None:
    logger.info(
        "OCR service startup: host=0.0.0.0 port=8000 shared_dir=%s max_pdf_bytes=%d max_pdf_pages=%d max_pdf_dpi=%d",
        OCR_SHARED_STORAGE_DIR or "<disabled>",
        MAX_PDF_UPLOAD_BYTES,
        MAX_PDF_PAGES,
        MAX_PDF_DPI,
    )
    route_paths = sorted({route.path for route in app.routes})
    logger.info("OCR routes: %s", ", ".join(route_paths))


@app.get("/health")
def health() -> dict:
    with _cache_lock:
        warmed_models = list(_ocr_cache.keys())

    return {
        "status": "ok",
        "service": "loklingo-ocr",
        "version": "2.0.0",
        "gpu_available": _gpu_available(),
        "models_warmed": warmed_models,
        "models_warmed_count": len(warmed_models),
    }


@app.post("/api/v1/ocr", response_model=OCRImageResponse)
@app.post("/ocr/image", response_model=OCRImageResponse)
def ocr_image(req: OCRImageRequest) -> OCRImageResponse:
    """Extract text and bounding boxes from a single image."""
    try:
        image_bytes = base64.b64decode(req.image_b64)
    except Exception:
        raise HTTPException(status_code=400, detail="Invalid base64 image data")

    try:
        image = Image.open(io.BytesIO(image_bytes))
    except Exception:
        raise HTTPException(status_code=422, detail="Cannot decode image — unsupported format")

    try:
        blocks = _ocr_pil_image(image, req.lang)
    except Exception as exc:
        logger.exception("OCR engine error on image: %s", exc)
        raise HTTPException(status_code=500, detail="OCR processing failed")

    text, confidence = _summarise(blocks)
    return OCRImageResponse(text=text, confidence=confidence, blocks=blocks)


@app.post("/api/v1/ocr/pdf", response_model=OCRPdfResponse)
@app.post("/ocr/pdf", response_model=OCRPdfResponse)
async def ocr_pdf(request: Request) -> OCRPdfResponse:
    """Extract text and bounding boxes from an uploaded PDF or shared file path."""
    content_type = request.headers.get("content-type", "")
    if content_type.startswith("application/json"):
        try:
            payload = OCRPdfRequest.model_validate(await request.json())
        except Exception as exc:
            logger.exception("Invalid OCR JSON request: %s", exc)
            raise HTTPException(status_code=400, detail="Invalid JSON request body")

        lang = _parse_lang(payload.lang)
        dpi = _parse_dpi(payload.dpi)

        if payload.file_path:
            file_path = _resolve_shared_pdf_path(payload.file_path)
            return _ocr_pdf_file(file_path, lang, dpi)

        if payload.pdf_b64:
            try:
                pdf_bytes = base64.b64decode(payload.pdf_b64)
            except Exception:
                raise HTTPException(status_code=400, detail="Invalid base64 PDF data")

            file_path = ""
            try:
                file_path = _write_pdf_bytes_to_temp_pdf(pdf_bytes)
                return _ocr_pdf_file(file_path, lang, dpi)
            finally:
                if file_path:
                    try:
                        os.remove(file_path)
                    except OSError:
                        logger.warning("Failed to remove temp PDF: %s", file_path)

        raise HTTPException(status_code=400, detail="file_path or pdf_b64 is required")

    if not content_type.startswith("multipart/form-data"):
        raise HTTPException(
            status_code=415,
            detail="Content-Type must be multipart/form-data or application/json",
        )

    form = await request.form()
    upload = form.get("file")
    if not isinstance(upload, (UploadFile, StarletteUploadFile)):
        raise HTTPException(status_code=400, detail="file is required (multipart field: file)")

    lang = _parse_lang(form.get("lang"))
    dpi = _parse_dpi(form.get("dpi"))

    file_path = ""
    try:
        file_path = _write_upload_to_temp_pdf(upload)
        return _ocr_pdf_file(file_path, lang, dpi)
    finally:
        await upload.close()
        if file_path:
            try:
                os.remove(file_path)
            except OSError:
                logger.warning("Failed to remove temp PDF: %s", file_path)

