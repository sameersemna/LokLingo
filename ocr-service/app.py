from __future__ import annotations

import base64
import io
import logging
import threading
from typing import Annotated

import numpy as np
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from paddleocr import PaddleOCR
from pdf2image import convert_from_bytes
from PIL import Image
from pydantic import BaseModel, Field

logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")
logger = logging.getLogger(__name__)

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
def ocr_pdf(req: OCRPdfRequest) -> OCRPdfResponse:
    """Extract text and bounding boxes from every page of a PDF."""
    try:
        pdf_bytes = base64.b64decode(req.pdf_b64)
    except Exception:
        raise HTTPException(status_code=400, detail="Invalid base64 PDF data")

    try:
        images = convert_from_bytes(pdf_bytes, dpi=req.dpi)
    except Exception as exc:
        logger.exception("PDF→image conversion error: %s", exc)
        raise HTTPException(
            status_code=422,
            detail="Cannot convert PDF to images — ensure it is a valid PDF file",
        )

    pages: list[PageResult] = []
    all_blocks: list[TextBlock] = []

    for page_num, image in enumerate(images, start=1):
        try:
            blocks = _ocr_pil_image(image, req.lang)
        except Exception as exc:
            logger.exception("OCR engine error on page %d: %s", page_num, exc)
            blocks = []

        page_text, page_conf = _summarise(blocks)
        pages.append(
            PageResult(
                page_number=page_num,
                text=page_text,
                confidence=page_conf,
                blocks=blocks,
            )
        )
        all_blocks.extend(blocks)

    full_text, avg_conf = _summarise(all_blocks)
    return OCRPdfResponse(text=full_text, confidence=avg_conf, pages=pages)

