from __future__ import annotations

import base64
import io
import logging
import os
import shutil
import tempfile
import threading
import time
from typing import Annotated

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


def get_ocr(lang: str) -> PaddleOCR:
    """Return a cached PaddleOCR instance for the given language."""
    paddle_lang = LANG_MAP.get(lang.lower(), "ch")
    with _cache_lock:
        if paddle_lang not in _ocr_cache:
            logger.info("Initialising PaddleOCR engine: lang=%s", paddle_lang)
            _ocr_cache[paddle_lang] = PaddleOCR(
                use_angle_cls=True,   # auto-correct rotated text
                lang=paddle_lang,
                show_log=False,
                use_gpu=False,
                rec_batch_num=6,      # process multiple lines at once
            )
    return _ocr_cache[paddle_lang]


# ---------------------------------------------------------------------------
# Pydantic models
# ---------------------------------------------------------------------------

LangField = Annotated[
    str,
    Field(default="auto", description="Language hint — ISO 639-1 code or 'auto'"),
]


class BoundingBox(BaseModel):
    points: list[list[float]] = Field(
        description="4 corner points [[x,y], …] in clockwise order (top-left first)"
    )


class TextBlock(BaseModel):
    text: str
    confidence: float = Field(ge=0.0, le=1.0)
    bbox: BoundingBox


class OCRImageRequest(BaseModel):
    image_b64: str = Field(description="Base64-encoded image (PNG, JPEG, BMP, TIFF, …)")
    mime_type: str = Field(default="image/png")
    lang: LangField = "auto"  # type: ignore[assignment]


class OCRImageResponse(BaseModel):
    text: str = Field(description="Full extracted text (lines joined by newline)")
    confidence: float = Field(description="Average confidence across all detected blocks")
    blocks: list[TextBlock] = Field(description="Per-line text blocks with bounding boxes")


class OCRPdfRequest(BaseModel):
    pdf_b64: str = Field(description="Base64-encoded PDF file")
    lang: LangField = "auto"  # type: ignore[assignment]
    dpi: int = Field(default=200, ge=72, le=400, description="Rendering DPI for each page")


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
    result = ocr.ocr(img_array, cls=True)
    blocks: list[TextBlock] = []
    for page in result or []:
        for line in page or []:
            bbox_raw, (text, conf) = line
            blocks.append(
                TextBlock(
                    text=text,
                    confidence=round(float(conf), 4),
                    bbox=BoundingBox(points=[list(pt) for pt in bbox_raw]),
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
    if dpi < 72 or dpi > 400:
        raise HTTPException(status_code=422, detail="dpi must be between 72 and 400")
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

@app.get("/health")
def health() -> dict:
    return {"status": "ok", "service": "loklingo-ocr", "version": "2.0.0"}


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


@app.post("/ocr/pdf", response_model=OCRPdfResponse)
async def ocr_pdf(request: Request) -> OCRPdfResponse:
    """Extract text and bounding boxes from every page of an uploaded PDF."""
    content_type = request.headers.get("content-type", "")
    if not content_type.startswith("multipart/form-data"):
        raise HTTPException(
            status_code=415,
            detail="Content-Type must be multipart/form-data",
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

