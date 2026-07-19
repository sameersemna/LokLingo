from __future__ import annotations

import base64
import difflib
import json
from collections.abc import AsyncIterator
from collections.abc import Mapping
from contextlib import asynccontextmanager
from dataclasses import dataclass
import io
import logging
import os
import tempfile
import threading
import time
from typing import Annotated, Any, Protocol
from urllib import error as urllib_error
from urllib import parse as urllib_parse
from urllib import request as urllib_request

import numpy as np
from fastapi import FastAPI, HTTPException, Request, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from paddleocr import PaddleOCR
from pdf2image import convert_from_path, pdfinfo_from_path
from PIL import Image, ImageDraw, ImageFilter, ImageFont, ImageOps
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


def _get_env_float(name: str, default: float) -> float:
    raw = os.getenv(name)
    if raw is None or raw == "":
        return default
    try:
        value = float(raw)
    except ValueError:
        logger.warning("Invalid %s=%r; using default %.3f", name, raw, default)
        return default
    if not (0.0 <= value <= 1.0):
        logger.warning("Out-of-range %s=%r; using default %.3f", name, raw, default)
        return default
    return value


MAX_PDF_UPLOAD_BYTES = _get_env_int("MAX_PDF_UPLOAD_BYTES", 100 * 1024 * 1024)
MAX_PDF_PAGES = _get_env_int("MAX_PDF_PAGES", 200)
MAX_PDF_DPI = _get_env_int("MAX_PDF_DPI", 300)
OCR_TARGET_DPI = _get_env_int("OCR_TARGET_DPI", 300)
OCR_MIN_CONFIDENCE = _get_env_float("OCR_MIN_CONFIDENCE", 0.72)
OCR_CORRECTION_ENABLED = os.getenv("OCR_CORRECTION_ENABLED", "false").strip().lower() in {"1", "true", "yes", "on"}
OCR_CORRECTION_MODEL = os.getenv("OCR_CORRECTION_MODEL", "Keyvan/german-ocr").strip() or "Keyvan/german-ocr"
OCR_CORRECTION_TIMEOUT = _get_env_int("OCR_CORRECTION_TIMEOUT", 25)
OCR_CORRECTION_MAX_RETRIES = _get_env_int("OCR_CORRECTION_MAX_RETRIES", 2)
OCR_CORRECTION_BLOCK_CONFIDENCE_THRESHOLD = _get_env_float("OCR_CORRECTION_BLOCK_CONFIDENCE_THRESHOLD", 0.80)
OCR_CORRECTION_CHUNK_CHARS = _get_env_int("OCR_CORRECTION_CHUNK_CHARS", 1400)
OLLAMA_BASE_URL = os.getenv("OLLAMA_BASE_URL", "http://loklingo-ollama:11434").strip() or "http://loklingo-ollama:11434"
OCR_SHARED_STORAGE_DIR = os.path.realpath(os.getenv("OCR_SHARED_STORAGE_DIR", "")) if os.getenv("OCR_SHARED_STORAGE_DIR", "") else ""

try:
    # Pillow <10: Image.LANCZOS. Pillow >=10: Image.Resampling.LANCZOS enum exists
    # but the value is no longer accepted by Image.rotate/resize — using it raises
    # `ValueError: Image.Resampling.LANCZOS (1) cannot be used`. Fall back to BICUBIC
    # (high quality, valid in both Pillow 9 and 10+).
    _PILLOW_VERSION = tuple(int(x) for x in Image.__version__.split('.')[:2])
except Exception:  # pragma: no cover - defensive default
    _PILLOW_VERSION = (0, 0)

if _PILLOW_VERSION >= (10, 0):
    _RESAMPLE_LANCZOS = Image.Resampling.BICUBIC
else:
    try:
        _RESAMPLE_LANCZOS = Image.Resampling.LANCZOS
    except AttributeError:  # pragma: no cover - older Pillow fallback
        _RESAMPLE_LANCZOS = Image.LANCZOS

@asynccontextmanager
async def _log_startup_config(app: FastAPI) -> AsyncIterator[None]:
    """Log the configured runtime parameters and verify the optional
    OCR-correction model is reachable.

    Replaces the deprecated ``@app.on_event("startup")`` hook. Runs once
    at process start; raises if the model check fails when correction is
    enabled (fail-fast, so the orchestrator can re-route traffic).
    """
    logger.info(
        "OCR service startup: host=0.0.0.0 port=8000 shared_dir=%s max_pdf_bytes=%d max_pdf_pages=%d max_pdf_dpi=%d",
        OCR_SHARED_STORAGE_DIR or "<disabled>",
        MAX_PDF_UPLOAD_BYTES,
        MAX_PDF_PAGES,
        MAX_PDF_DPI,
    )
    route_paths = sorted({route.path for route in app.routes})
    logger.info("OCR routes: %s", ", ".join(route_paths))
    provider = _get_correction_provider(None)
    if provider.name() == "noop":
        logger.info("OCR correction provider disabled")
        yield
        return
    available, reason = _check_model_available_cached(provider, OCR_CORRECTION_MODEL, OCR_CORRECTION_TIMEOUT)
    if available:
        logger.info("OCR correction model is available", extra={"provider": provider.name(), "model": OCR_CORRECTION_MODEL})
    else:
        logger.warning("OCR correction model unavailable", extra={"provider": provider.name(), "model": OCR_CORRECTION_MODEL, "reason": reason})
    yield


app = FastAPI(
    title="LokLingo OCR Service",
    version="2.0.0",
    lifespan=_log_startup_config,
)

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
    reading_order: int = Field(default=0, ge=0, description="1-based reading order within the page")
    bbox: list[float] = Field(
        description="Axis-aligned bounding box [x1, y1, x2, y2]",
        min_length=4,
        max_length=4,
    )
    region_class: str = Field(default="body", description="Layout region type: title|heading|body|caption|table|code|annotation|form_field")
    hierarchy_level: int = Field(default=0, ge=0, description="Heading depth: 0=body, 1=title, 2=heading, 3=subheading")
    column_id: int = Field(default=0, ge=0, description="0-based column index in multi-column layouts")


class OCRImageRequest(BaseModel):
    image_b64: str = Field(description="Base64-encoded image (PNG, JPEG, BMP, TIFF, …)")
    mime_type: str = Field(default="image/png")
    lang: LangField = "auto"  # type: ignore[assignment]
    min_confidence: float = Field(default=OCR_MIN_CONFIDENCE, ge=0.0, le=1.0)
    export_json_path: str | None = Field(default=None, description="Optional path for exporting the OCR JSON payload")
    debug_output_dir: str | None = Field(default=None, description="Optional directory for debug overlay images")
    mode: str = Field(default="overlay", description="Processing mode hint: overlay/layout/ocr_only")
    ocr_correction_enabled: bool | None = Field(default=None, description="Optional override for AI OCR correction")
    ocr_correction_model: str | None = Field(default=None, description="Optional override for correction model")
    visual_diff_mode: bool = Field(default=False, description="When true, include raw and corrected OCR in response")


class OCRImageResponse(BaseModel):
    text: str = Field(description="Full extracted text (lines joined by newline)")
    confidence: float = Field(description="Average confidence across all detected blocks")
    blocks: list[TextBlock] = Field(description="Per-line text blocks with bounding boxes")
    debug_image_path: str | None = Field(default=None, description="Optional path to the rendered debug overlay")
    exported_json_path: str | None = Field(default=None, description="Optional path to the exported JSON payload")
    raw_text: str | None = Field(default=None, description="Raw OCR text before AI correction")
    correction_diff: str | None = Field(default=None, description="Unified diff (raw -> corrected) for visual diff mode")
    correction: dict[str, Any] | None = Field(default=None, description="AI OCR correction metadata")


class OCRPdfRequest(BaseModel):
    pdf_b64: str | None = Field(default=None, description="Base64-encoded PDF file")
    file_path: str | None = Field(default=None, description="Absolute path to a shared PDF file")
    lang: LangField = "auto"  # type: ignore[assignment]
    dpi: int = Field(default=200, ge=72, le=400, description="Rendering DPI for each page — clamped to MAX_PDF_DPI at runtime")
    min_confidence: float = Field(default=OCR_MIN_CONFIDENCE, ge=0.0, le=1.0)
    export_json_path: str | None = Field(default=None, description="Optional path for exporting the OCR JSON payload")
    debug_output_dir: str | None = Field(default=None, description="Optional directory for debug overlay images")
    mode: str = Field(default="overlay", description="Processing mode hint: overlay/layout/ocr_only")
    ocr_correction_enabled: bool | None = Field(default=None, description="Optional override for AI OCR correction")
    ocr_correction_model: str | None = Field(default=None, description="Optional override for correction model")
    visual_diff_mode: bool = Field(default=False, description="When true, include raw and corrected OCR in response")


class PageResult(BaseModel):
    page_number: int
    text: str
    confidence: float
    blocks: list[TextBlock]
    debug_image_path: str | None = Field(default=None, description="Optional path to the rendered debug overlay for this page")
    raw_text: str | None = Field(default=None, description="Raw OCR text before AI correction")
    correction_diff: str | None = Field(default=None, description="Unified diff for this page")
    correction: dict[str, Any] | None = Field(default=None, description="AI OCR correction metadata for this page")


class OCRPdfResponse(BaseModel):
    text: str = Field(description="Full extracted text across all pages")
    confidence: float
    pages: list[PageResult]
    exported_json_path: str | None = Field(default=None, description="Optional path to the exported JSON payload")
    raw_text: str | None = Field(default=None, description="Raw OCR text before AI correction")
    correction_diff: str | None = Field(default=None, description="Unified diff (raw -> corrected) across all pages")
    correction: dict[str, Any] | None = Field(default=None, description="Aggregate AI OCR correction metadata")


@dataclass
class CorrectionStats:
    applied: bool = False
    model: str = ""
    latency_ms: float = 0.0
    changed_characters: int = 0
    confidence_delta: float = 0.0
    corrected_blocks: int = 0
    considered_blocks: int = 0
    retries: int = 0
    reason: str = ""


@dataclass
class CorrectionState:
    calls_total: int = 0
    applied_total: int = 0
    failures_total: int = 0
    latency_ms_total: float = 0.0
    changed_characters_total: int = 0
    confidence_delta_total: float = 0.0


class OCRCorrectionProvider(Protocol):
    def name(self) -> str: ...
    def check_model_available(self, model: str, timeout_sec: int) -> tuple[bool, str]: ...
    def correct_chunk(
        self,
        *,
        model: str,
        prompt: str,
        image: Image.Image,
        timeout_sec: int,
        max_retries: int,
    ) -> tuple[list[str] | None, int, str | None]: ...


_correction_state = CorrectionState()
_correction_state_lock = threading.Lock()


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

def _ocr_pil_image(image: Image.Image, lang: str, source_dpi: int | None = None, min_confidence: float | None = None) -> list[TextBlock]:
    """Run PaddleOCR with preprocessing fallback and return reading-ordered blocks."""
    threshold = OCR_MIN_CONFIDENCE if min_confidence is None else min_confidence
    variants = _preprocess_variants(image.convert("RGB"), source_dpi)

    best_blocks: list[TextBlock] = []
    best_confidence = -1.0
    best_stage = ""

    for stage, candidate in variants:
        blocks = _ocr_pil_image_once(candidate, lang)
        if not blocks:
            continue

        ordered_blocks = _assign_reading_order_with_layout(blocks, image.width, image.height)
        _, confidence = _summarise(ordered_blocks)
        if confidence > best_confidence:
            best_blocks = ordered_blocks
            best_confidence = confidence
            best_stage = stage

        if confidence >= threshold:
            if stage != variants[0][0]:
                logger.info(
                    "ocr_preprocess_fallback_succeeded",
                    extra={"lang": lang, "stage": stage, "confidence": confidence, "threshold": threshold},
                )
            return ordered_blocks

    if best_confidence >= 0:
        if best_confidence < threshold:
            logger.warning(
                "ocr_low_confidence_fallback",
                extra={"lang": lang, "stage": best_stage, "confidence": best_confidence, "threshold": threshold},
            )
        return best_blocks
    return []


def _ocr_pil_image_once(image: Image.Image, lang: str) -> list[TextBlock]:
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


def _normalize_image_dpi(image: Image.Image, source_dpi: int | None, target_dpi: int) -> Image.Image:
    if source_dpi is None or source_dpi <= 0 or target_dpi <= 0:
        return image.copy()
    if source_dpi == target_dpi:
        return image.copy()

    scale = target_dpi / float(source_dpi)
    width = max(1, int(round(image.width * scale)))
    height = max(1, int(round(image.height * scale)))
    if width == image.width and height == image.height:
        return image.copy()
    return image.resize((width, height), _RESAMPLE_LANCZOS)


def _denoise_image(image: Image.Image) -> Image.Image:
    return image.convert("RGB").filter(ImageFilter.MedianFilter(size=3))


def _projection_score(image: Image.Image) -> float:
    gray = image.convert("L")
    pixels = gray.load()
    width, height = gray.size
    if width <= 0 or height <= 0:
        return 0.0

    row_scores: list[float] = []
    for y in range(height):
        dark = 0
        for x in range(width):
            if pixels[x, y] < 200:
                dark += 1
        row_scores.append(float(dark))
    if not row_scores:
        return 0.0

    mean = sum(row_scores) / len(row_scores)
    return sum((value - mean) ** 2 for value in row_scores) / len(row_scores)


def _estimate_skew_angle(image: Image.Image) -> float:
    preview = image.convert("L")
    preview.thumbnail((1200, 1200), _RESAMPLE_LANCZOS)
    best_angle = 0.0
    best_score = 0.0
    for angle in range(-5, 6):
        rotated = preview.rotate(angle, expand=True, fillcolor=255)
        score = _projection_score(rotated)
        if score > best_score:
            best_score = score
            best_angle = float(angle)
    return best_angle


def _deskew_image(image: Image.Image) -> Image.Image:
    angle = _estimate_skew_angle(image)
    if abs(angle) < 0.5:
        return image.copy()
    return image.rotate(-angle, expand=True, fillcolor="white", resample=_RESAMPLE_LANCZOS)


def _adaptive_threshold_image(image: Image.Image, window_size: int = 31, offset: int = 12) -> Image.Image:
    gray = ImageOps.autocontrast(image.convert("L"))
    width, height = gray.size
    if width <= 0 or height <= 0:
        return gray

    window_size = max(3, min(window_size if window_size % 2 == 1 else window_size + 1, min(width, height)))
    if window_size % 2 == 0:
        window_size -= 1
    if window_size < 3:
        return gray.point(lambda value: 255 if value > 127 else 0)

    blurred = gray.filter(ImageFilter.BoxBlur(max(1, window_size // 2)))
    src = gray.load()
    ref = blurred.load()
    output = Image.new("L", gray.size, 255)
    dst = output.load()

    for y in range(height):
        for x in range(width):
            threshold = max(0, min(255, ref[x, y] - offset))
            dst[x, y] = 0 if src[x, y] < threshold else 255
    return output


def _preprocess_variants(image: Image.Image, source_dpi: int | None) -> list[tuple[str, Image.Image]]:
    normalized = _normalize_image_dpi(image, source_dpi, OCR_TARGET_DPI)
    denoised = _denoise_image(normalized)
    deskewed = _deskew_image(denoised)
    thresholded = _adaptive_threshold_image(deskewed)
    return [
        ("normalized", normalized),
        ("denoised", denoised),
        ("deskewed", deskewed),
        ("adaptive_threshold", thresholded),
    ]


def _assign_reading_order(blocks: list[TextBlock]) -> list[TextBlock]:
    ordered: list[TextBlock] = []
    for index, block in enumerate(blocks, start=1):
        ordered.append(block.model_copy(update={"reading_order": index}))
    return ordered


# ---------------------------------------------------------------------------
# Layout analysis: column detection, region classification, reading order
# ---------------------------------------------------------------------------

def _detect_columns(blocks: list[TextBlock], image_width: int = 0) -> list[int]:
    """Detect multi-column layout and assign 0-based column IDs to each block.

    Uses horizontal gap analysis: if a consistent vertical gap exists between
    two groups of blocks, they are treated as separate columns.  Returns a list
    of column IDs aligned with `blocks`.
    """
    if not blocks:
        return []

    # Collect x-center positions
    centers = []
    for b in blocks:
        x1, y1, x2, y2 = b.bbox
        centers.append((x1 + x2) / 2.0)

    if image_width <= 0:
        image_width = max(b.bbox[2] for b in blocks)

    # Sort unique x-center positions to find gaps
    sorted_centers = sorted(set(centers))
    if len(sorted_centers) < 2:
        return [0] * len(blocks)

    # Detect large x-gaps that separate column groups (gap > 8% of image width)
    gap_threshold = max(20.0, image_width * 0.08)
    column_breaks: list[float] = []  # x-values where a new column starts
    for i in range(1, len(sorted_centers)):
        if sorted_centers[i] - sorted_centers[i - 1] >= gap_threshold:
            column_breaks.append((sorted_centers[i - 1] + sorted_centers[i]) / 2.0)

    if not column_breaks:
        return [0] * len(blocks)

    col_ids = []
    for b in blocks:
        x_center = (b.bbox[0] + b.bbox[2]) / 2.0
        col = 0
        for break_x in column_breaks:
            if x_center > break_x:
                col += 1
        col_ids.append(col)
    return col_ids


def _classify_region(block: TextBlock, all_blocks: list[TextBlock], image_width: int = 0, image_height: int = 0) -> tuple[str, int]:
    """Classify a block into a layout region type and hierarchy level.

    Returns (region_class, hierarchy_level).

    region_class: title | heading | body | caption | table | code | annotation | form_field
    hierarchy_level: 0=body, 1=title, 2=heading, 3=subheading
    """
    x1, y1, x2, y2 = block.bbox
    width = x2 - x1
    height = y2 - y1
    text = block.text.strip()
    char_count = len(text)

    # Effective image dimensions from all blocks when not provided
    if image_width <= 0 and all_blocks:
        image_width = max(b.bbox[2] for b in all_blocks)
    if image_height <= 0 and all_blocks:
        image_height = max(b.bbox[3] for b in all_blocks)

    if image_width <= 0:
        image_width = 1000
    if image_height <= 0:
        image_height = 1000

    # Relative metrics
    rel_width = width / image_width if image_width > 0 else 0.0
    rel_height = height / image_height if image_height > 0 else 0.0
    rel_y = y1 / image_height if image_height > 0 else 0.0
    font_size_est = height  # approximate: single-line blocks have height ≈ font size

    # Compute median font size across all blocks for comparison
    if all_blocks:
        single_line_heights = [
            b.bbox[3] - b.bbox[1]
            for b in all_blocks
            if len(b.text.strip()) > 0
        ]
        if single_line_heights:
            sorted_h = sorted(single_line_heights)
            median_font = sorted_h[len(sorted_h) // 2]
        else:
            median_font = font_size_est
    else:
        median_font = font_size_est

    # Monospace / code detection: only digits, punctuation, consistent char widths
    import re as _re
    _code_pattern = _re.compile(r"^[\w\s\.\,\;\:\!\?\(\)\[\]\{\}\=\+\-\*/\\\|#@&%^<>~`\"\']+$")
    if char_count >= 3 and char_count <= 120:
        has_code_tokens = bool(_re.search(r'[=(){}\[\];]', text)) or text.startswith("//") or text.startswith("#")
        if has_code_tokens and _code_pattern.match(text):
            return "code", 0

    # Form field: short text ending in colon or followed by blank space
    if char_count <= 40 and (text.endswith(":") or text.endswith("：")):
        return "form_field", 0

    # Annotation: very small text, bottom or margin area
    if rel_height < 0.015 and rel_y > 0.85:
        return "annotation", 0

    # Caption: short, near bottom, low contrast relative to median
    if char_count <= 80 and rel_y > 0.75 and font_size_est <= median_font * 0.85:
        return "caption", 0

    # Table: short, very wide, or text consists of mostly numbers/delimiters
    if rel_width > 0.7 and char_count <= 60:
        digit_ratio = sum(1 for c in text if c.isdigit() or c in ".,|%") / max(1, char_count)
        if digit_ratio > 0.4:
            return "table", 0

    # Title: large font, spans most of width, near top, short text
    is_large = font_size_est >= median_font * 1.6
    is_near_top = rel_y < 0.25
    is_wide = rel_width > 0.4
    is_short = char_count <= 120

    if is_large and is_near_top and is_wide and is_short:
        return "title", 1

    # Heading: moderately large font, short text, wider than median
    is_heading_size = font_size_est >= median_font * 1.3
    if is_heading_size and is_short and is_wide:
        if is_near_top:
            return "heading", 2
        return "heading", 2

    # Sub-heading: slightly larger than body, shorter than full width
    if font_size_est >= median_font * 1.1 and char_count <= 80:
        return "heading", 3

    # Default: body text
    return "body", 0


def _assign_reading_order_with_layout(blocks: list[TextBlock], image_width: int = 0, image_height: int = 0) -> list[TextBlock]:
    """Assign reading_order, region_class, hierarchy_level and column_id.

    Reading order:
    1. Detect columns using x-gap analysis.
    2. Within each column, sort top-to-bottom then left-to-right.
    3. Columns are read left-to-right across the page.
    """
    if not blocks:
        return []

    # Assign column IDs
    col_ids = _detect_columns(blocks, image_width)

    # Classify regions
    classified: list[tuple[TextBlock, int]] = []
    for b, col_id in zip(blocks, col_ids):
        region_class, hierarchy_level = _classify_region(b, blocks, image_width, image_height)
        updated = b.model_copy(update={
            "region_class": region_class,
            "hierarchy_level": hierarchy_level,
            "column_id": col_id,
        })
        classified.append((updated, col_id))

    # Group by column, sort within each column top-to-bottom then left-to-right
    num_columns = max(col_ids) + 1 if col_ids else 1
    columns: list[list[tuple[TextBlock, int]]] = [[] for _ in range(num_columns)]
    for item, col_id in classified:
        columns[col_id].append((item, col_id))

    # Sort each column by y1 (top) then x1 (left)
    for col in columns:
        col.sort(key=lambda t: (t[0].bbox[1], t[0].bbox[0]))

    # Flatten columns left-to-right to get global reading order
    ordered: list[TextBlock] = []
    order_counter = 1
    for col in columns:
        for item, _ in col:
            ordered.append(item.model_copy(update={"reading_order": order_counter}))
            order_counter += 1

    return ordered


def _extract_image_dpi(image: Image.Image) -> int | None:
    dpi = image.info.get("dpi")
    if isinstance(dpi, tuple) and dpi:
        try:
            candidate = int(round(dpi[0]))
        except Exception:
            return None
        return candidate if candidate > 0 else None
    if isinstance(dpi, (int, float)):
        candidate = int(round(dpi))
        return candidate if candidate > 0 else None
    return None


def _ensure_parent_dir(file_path: str) -> None:
    parent = os.path.dirname(file_path)
    if parent:
        os.makedirs(parent, exist_ok=True)


def _render_debug_overlay(image: Image.Image, blocks: list[TextBlock]) -> Image.Image:
    canvas = image.convert("RGB").copy()
    draw = ImageDraw.Draw(canvas)
    try:
        font = ImageFont.load_default()
    except Exception:
        font = None

    palette = ["#ff4d4d", "#4da6ff", "#00b894", "#e17055", "#6c5ce7"]
    for index, block in enumerate(blocks, start=1):
        if len(block.bbox) != 4:
            continue
        x1, y1, x2, y2 = (int(round(v)) for v in block.bbox)
        color = palette[(index - 1) % len(palette)]
        draw.rectangle([x1, y1, x2, y2], outline=color, width=3)
        label = f"{block.reading_order}:{block.confidence:.2f}"
        label_y = max(0, y1 - 12)
        draw.rectangle([x1, label_y, x1 + max(28, len(label) * 6 + 8), label_y + 12], fill=color)
        if font is not None:
            draw.text((x1 + 3, label_y), label, fill="white", font=font)
    return canvas


def _write_debug_overlay(image: Image.Image, blocks: list[TextBlock], output_path: str) -> str:
    _ensure_parent_dir(output_path)
    overlay = _render_debug_overlay(image, blocks)
    overlay.save(output_path)
    return output_path


def _write_json_export(payload: BaseModel, output_path: str | None) -> str | None:
    if not output_path:
        return None
    _ensure_parent_dir(output_path)
    with open(output_path, "w", encoding="utf-8") as handle:
        handle.write(payload.model_dump_json(indent=2))
        handle.write("\n")
    return output_path


def _is_local_ollama_url(base_url: str) -> bool:
    try:
        parsed = urllib_parse.urlparse(base_url)
    except Exception:
        return False
    host = (parsed.hostname or "").lower()
    if host in {"localhost", "127.0.0.1", "::1", "loklingo-ollama", "host.docker.internal"}:
        return True
    if host.startswith("10.") or host.startswith("192.168."):
        return True
    if host.startswith("172."):
        return True
    return False


class NoopCorrectionProvider:
    def name(self) -> str:
        return "noop"

    def check_model_available(self, model: str, timeout_sec: int) -> tuple[bool, str]:
        return False, "ocr correction disabled"

    def correct_chunk(
        self,
        *,
        model: str,
        prompt: str,
        image: Image.Image,
        timeout_sec: int,
        max_retries: int,
    ) -> tuple[list[str] | None, int, str | None]:
        return None, 0, "ocr correction disabled"


class OllamaCorrectionProvider:
    def __init__(self, base_url: str) -> None:
        self.base_url = base_url.rstrip("/")

    def name(self) -> str:
        return "ollama"

    def check_model_available(self, model: str, timeout_sec: int) -> tuple[bool, str]:
        tags_url = f"{self.base_url}/api/tags"
        req = urllib_request.Request(tags_url, method="GET")
        try:
            with urllib_request.urlopen(req, timeout=max(1, timeout_sec)) as resp:
                payload = json.loads(resp.read().decode("utf-8"))
        except Exception as exc:
            return False, f"failed to query Ollama tags: {exc}"
        models = payload.get("models") if isinstance(payload, dict) else []
        for item in models or []:
            name = ""
            if isinstance(item, dict):
                name = str(item.get("name") or "")
            if name == model or name.split(":", 1)[0] == model:
                return True, "ok"
        return False, f"model {model} not found in local Ollama registry"

    def correct_chunk(
        self,
        *,
        model: str,
        prompt: str,
        image: Image.Image,
        timeout_sec: int,
        max_retries: int,
    ) -> tuple[list[str] | None, int, str | None]:
        image_bytes = io.BytesIO()
        image.convert("RGB").save(image_bytes, format="PNG")
        encoded_image = base64.b64encode(image_bytes.getvalue()).decode("ascii")
        endpoint = f"{self.base_url}/api/generate"

        attempts = 0
        last_error = ""
        while attempts <= max_retries:
            attempts += 1
            payload = {
                "model": model,
                "prompt": prompt,
                "stream": False,
                "images": [encoded_image],
                "options": {
                    "temperature": 0,
                    "top_p": 0.2,
                    "num_predict": 1200,
                },
            }
            req = urllib_request.Request(
                endpoint,
                method="POST",
                headers={"Content-Type": "application/json"},
                data=json.dumps(payload).encode("utf-8"),
            )
            try:
                with urllib_request.urlopen(req, timeout=max(1, timeout_sec)) as resp:
                    raw_body = resp.read().decode("utf-8")
            except (urllib_error.URLError, urllib_error.HTTPError, TimeoutError) as exc:
                last_error = str(exc)
                continue
            except Exception as exc:
                last_error = str(exc)
                continue

            try:
                data = json.loads(raw_body)
            except json.JSONDecodeError as exc:
                last_error = f"invalid ollama json response: {exc}"
                continue

            answer = str(data.get("response") or "").strip()
            if answer == "":
                last_error = "empty response from ollama"
                continue

            cleaned = answer
            if "```" in cleaned:
                parts = cleaned.split("```")
                if len(parts) >= 2:
                    cleaned = parts[1]
                    if cleaned.lower().startswith("json"):
                        cleaned = cleaned[4:]
            cleaned = cleaned.strip()

            try:
                parsed = json.loads(cleaned)
            except json.JSONDecodeError:
                last_error = "ollama response is not valid JSON"
                continue
            lines = parsed.get("lines") if isinstance(parsed, dict) else None
            if not isinstance(lines, list):
                last_error = "ollama JSON response missing lines[]"
                continue
            normalized_lines = [str(line) for line in lines]
            return normalized_lines, attempts - 1, None

        return None, max(0, attempts - 1), last_error or "ollama correction failed"


_noop_correction_provider = NoopCorrectionProvider()
_ollama_correction_provider = OllamaCorrectionProvider(OLLAMA_BASE_URL)
_model_availability_cache: dict[str, tuple[bool, str, float]] = {}


def _get_correction_provider(enabled_override: bool | None) -> OCRCorrectionProvider:
    effective_enabled = OCR_CORRECTION_ENABLED if enabled_override is None else bool(enabled_override)
    if not effective_enabled:
        return _noop_correction_provider
    if not _is_local_ollama_url(OLLAMA_BASE_URL):
        logger.warning("OCR correction disabled because OLLAMA_BASE_URL is not local: %s", OLLAMA_BASE_URL)
        return _noop_correction_provider
    return _ollama_correction_provider


def _check_model_available_cached(provider: OCRCorrectionProvider, model: str, timeout_sec: int) -> tuple[bool, str]:
    now = time.time()
    cached = _model_availability_cache.get(model)
    if cached and (now - cached[2]) < 60:
        return cached[0], cached[1]
    ok, reason = provider.check_model_available(model, timeout_sec)
    _model_availability_cache[model] = (ok, reason, now)
    return ok, reason


def _build_german_cleanup_prompt(blocks: list[TextBlock], line_text: list[str]) -> str:
    lines = []
    for idx, block in enumerate(blocks):
        source = line_text[idx] if idx < len(line_text) else block.text
        lines.append({
            "line_index": idx,
            "reading_order": block.reading_order,
            "bbox": block.bbox,
            "confidence": block.confidence,
            "text": source,
        })
    payload = json.dumps(lines, ensure_ascii=False)
    return (
        "You are an OCR correction specialist for German bureaucracy letters, invoices, forms, and noisy scans.\n"
        "Correct ONLY OCR mistakes in the provided lines.\n"
        "Rules:\n"
        "1) Preserve line count exactly.\n"
        "2) Preserve table/form alignment and separators, including colon alignment and checkbox markers.\n"
        "3) Preserve line breaks exactly per line.\n"
        "4) Do not hallucinate new entities, dates, amounts, or addresses.\n"
        "5) If uncertain, keep the original text unchanged.\n"
        "Return strictly JSON: {\"lines\":[\"...\", ...]} with exactly the same number of lines as input.\n"
        f"Input lines: {payload}"
    )


def _chunk_low_confidence_blocks(blocks: list[TextBlock], max_chars: int) -> list[list[int]]:
    low_indexes = [idx for idx, block in enumerate(blocks) if block.confidence <= OCR_CORRECTION_BLOCK_CONFIDENCE_THRESHOLD and block.text.strip()]
    if not low_indexes:
        return []
    chunks: list[list[int]] = []
    current: list[int] = []
    current_chars = 0
    for idx in low_indexes:
        block_chars = len(blocks[idx].text)
        if current and current_chars+block_chars > max_chars:
            chunks.append(current)
            current = []
            current_chars = 0
        current.append(idx)
        current_chars += block_chars
    if current:
        chunks.append(current)
    return chunks


def _safe_confidence_delta(before: float, after: float) -> float:
    delta = after - before
    if delta > 1:
        return 1.0
    if delta < -1:
        return -1.0
    return round(delta, 4)


def _record_correction_stats(stats: CorrectionStats) -> None:
    with _correction_state_lock:
        _correction_state.calls_total += 1
        if stats.applied:
            _correction_state.applied_total += 1
        if stats.reason and not stats.applied:
            _correction_state.failures_total += 1
        _correction_state.latency_ms_total += stats.latency_ms
        _correction_state.changed_characters_total += stats.changed_characters
        _correction_state.confidence_delta_total += stats.confidence_delta


def _dict_from_stats(stats: CorrectionStats) -> dict[str, Any]:
    return {
        "applied": stats.applied,
        "model": stats.model,
        "latency_ms": round(stats.latency_ms, 2),
        "changed_characters": stats.changed_characters,
        "confidence_delta": stats.confidence_delta,
        "corrected_blocks": stats.corrected_blocks,
        "considered_blocks": stats.considered_blocks,
        "retries": stats.retries,
        "reason": stats.reason,
    }


def _unified_diff(raw_text: str, corrected_text: str) -> str:
    if raw_text == corrected_text:
        return ""
    lines = list(difflib.unified_diff(
        raw_text.splitlines(),
        corrected_text.splitlines(),
        fromfile="raw_ocr",
        tofile="corrected_ocr",
        lineterm="",
    ))
    return "\n".join(lines)


def _should_apply_correction(*, lang: str, mode: str, enabled_override: bool | None) -> bool:
    if enabled_override is not None:
        return bool(enabled_override)
    if not OCR_CORRECTION_ENABLED:
        return False
    lang = (lang or "").strip().lower()
    if lang not in {"de", "auto"}:
        return False
    # Keep Fast mode snappy; correction is primarily for Studio/layout and OCR-only extraction.
    return mode in {"layout", "ocr_only"}


def _apply_llm_correction(
    *,
    image: Image.Image,
    blocks: list[TextBlock],
    lang: str,
    mode: str,
    enabled_override: bool | None,
    model_override: str | None,
) -> tuple[list[TextBlock], CorrectionStats, str]:
    stats = CorrectionStats(applied=False, model=(model_override or OCR_CORRECTION_MODEL).strip() or OCR_CORRECTION_MODEL)
    raw_text, _ = _summarise(blocks)

    if not blocks:
        stats.reason = "no_blocks"
        _record_correction_stats(stats)
        return blocks, stats, raw_text

    if not _should_apply_correction(lang=lang, mode=mode, enabled_override=enabled_override):
        stats.reason = "correction_not_enabled_for_request"
        _record_correction_stats(stats)
        return blocks, stats, raw_text

    provider = _get_correction_provider(enabled_override)
    if provider.name() == "noop":
        stats.reason = "correction_provider_unavailable"
        _record_correction_stats(stats)
        return blocks, stats, raw_text

    available, reason = _check_model_available_cached(provider, stats.model, OCR_CORRECTION_TIMEOUT)
    if not available:
        stats.reason = reason or "model_unavailable"
        _record_correction_stats(stats)
        return blocks, stats, raw_text

    chunk_indexes = _chunk_low_confidence_blocks(blocks, OCR_CORRECTION_CHUNK_CHARS)
    if not chunk_indexes:
        stats.reason = "no_low_confidence_blocks"
        _record_correction_stats(stats)
        return blocks, stats, raw_text

    corrected = [block.model_copy() for block in blocks]
    low_before = []
    low_after = []
    start = time.perf_counter()
    total_changed_chars = 0
    total_retries = 0

    for idx_group in chunk_indexes:
        sub_blocks = [corrected[idx] for idx in idx_group]
        sub_lines = [corrected[idx].text for idx in idx_group]
        prompt = _build_german_cleanup_prompt(sub_blocks, sub_lines)
        corrected_lines, retries, err = provider.correct_chunk(
            model=stats.model,
            prompt=prompt,
            image=image,
            timeout_sec=OCR_CORRECTION_TIMEOUT,
            max_retries=OCR_CORRECTION_MAX_RETRIES,
        )
        total_retries += retries
        if err is not None or corrected_lines is None:
            stats.reason = err or "correction_failed"
            continue
        if len(corrected_lines) != len(idx_group):
            stats.reason = "line_count_mismatch"
            continue

        for local_idx, global_idx in enumerate(idx_group):
            before = corrected[global_idx]
            before_text = before.text
            after_text = corrected_lines[local_idx]
            low_before.append(before.confidence)
            if after_text != before_text:
                total_changed_chars += abs(len(after_text) - len(before_text)) + sum(
                    1 for a, b in zip(before_text, after_text) if a != b
                )
                boosted = min(1.0, max(before.confidence, 0.90))
                corrected[global_idx] = before.model_copy(update={"text": after_text, "confidence": round(boosted, 4)})
                low_after.append(boosted)
            else:
                low_after.append(before.confidence)

    stats.latency_ms = (time.perf_counter() - start) * 1000.0
    stats.considered_blocks = sum(len(group) for group in chunk_indexes)
    stats.corrected_blocks = sum(1 for idx in range(len(blocks)) if corrected[idx].text != blocks[idx].text)
    stats.changed_characters = total_changed_chars
    stats.retries = total_retries
    if low_before and low_after:
        before_avg = sum(low_before) / len(low_before)
        after_avg = sum(low_after) / len(low_after)
        stats.confidence_delta = _safe_confidence_delta(before_avg, after_avg)
    stats.applied = stats.corrected_blocks > 0
    if stats.applied:
        stats.reason = "ok"
    elif stats.reason == "":
        stats.reason = "no_changes"

    _record_correction_stats(stats)
    return corrected, stats, raw_text


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
    min_confidence: float | None = None,
    debug_output_dir: str | None = None,
    export_json_path: str | None = None,
    mode: str = "overlay",
    correction_enabled: bool | None = None,
    correction_model: str | None = None,
    visual_diff_mode: bool = False,
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
    raw_pages: list[str] = []
    correction_stats: list[CorrectionStats] = []
    full_text_buffer = io.StringIO()
    raw_text_buffer = io.StringIO()
    wrote_text = False
    wrote_raw_text = False
    confidence_sum = 0.0
    confidence_count = 0

    for page_num in range(1, page_count + 1):
        debug_image_path = None
        t_render_start = time.perf_counter()
        try:
            with tempfile.TemporaryDirectory(prefix="loklingo-ocr-render-") as render_dir:
                images = convert_from_path(
                    file_path,
                    dpi=dpi,
                    first_page=page_num,
                    last_page=page_num,
                    output_folder=render_dir,
                    paths_only=True,
                    thread_count=1,
                )
                if not images:
                    raise ValueError(f"no image returned for page {page_num}")
                image_path = images[0]

                with Image.open(image_path) as image:
                    render_ms = (time.perf_counter() - t_render_start) * 1000
                    t_ocr_start = time.perf_counter()
                    try:
                        blocks = _ocr_pil_image(image, lang, source_dpi=dpi, min_confidence=min_confidence)
                    except Exception as exc:
                        logger.exception("OCR engine error on page %d: %s", page_num, exc)
                        blocks = []
                    ocr_ms = (time.perf_counter() - t_ocr_start) * 1000

                    corrected_blocks, page_correction, raw_page_text = _apply_llm_correction(
                        image=image,
                        blocks=blocks,
                        lang=lang,
                        mode=mode,
                        enabled_override=correction_enabled,
                        model_override=correction_model,
                    )
                    blocks = corrected_blocks
                    correction_stats.append(page_correction)

                    if debug_output_dir:
                        base_name = os.path.splitext(os.path.basename(file_path))[0] or "page"
                        debug_image_path = os.path.join(debug_output_dir, f"{base_name}-page-{page_num:04d}.png")
                        try:
                            _write_debug_overlay(image, blocks, debug_image_path)
                        except Exception as exc:
                            logger.warning("Failed to write OCR debug overlay for page %d: %s", page_num, exc)
                            debug_image_path = None
        except Exception as exc:
            logger.exception("PDF→image conversion error on page %d: %s", page_num, exc)
            raise HTTPException(
                status_code=422,
                detail="Cannot convert PDF to images — ensure it is a valid PDF file",
            )

        logger.info(
            "page %d/%d render_ms=%.0f ocr_ms=%.0f blocks=%d",
            page_num, page_count, render_ms, ocr_ms, len(blocks),
        )

        page_text, page_conf = _summarise(blocks)
        raw_pages.append(raw_page_text)
        pages.append(
            PageResult(
                page_number=page_num,
                text=page_text,
                confidence=page_conf,
                blocks=blocks,
                debug_image_path=debug_image_path,
                raw_text=raw_page_text if visual_diff_mode else None,
                correction_diff=_unified_diff(raw_page_text, page_text) if visual_diff_mode else None,
                correction=_dict_from_stats(correction_stats[-1]) if correction_stats else None,
            )
        )

        if raw_page_text:
            if wrote_raw_text:
                raw_text_buffer.write("\n")
            raw_text_buffer.write(raw_page_text)
            wrote_raw_text = True
        if page_text:
            if wrote_text:
                full_text_buffer.write("\n")
            full_text_buffer.write(page_text)
            wrote_text = True
        confidence_sum += sum(block.confidence for block in blocks)
        confidence_count += len(blocks)

    full_text = full_text_buffer.getvalue()
    raw_full_text = raw_text_buffer.getvalue()
    full_text_buffer.close()
    raw_text_buffer.close()
    avg_conf = round(confidence_sum / confidence_count, 4) if confidence_count else 0.0
    response = OCRPdfResponse(text=full_text, confidence=avg_conf, pages=pages)
    if visual_diff_mode:
        response.raw_text = raw_full_text
        response.correction_diff = _unified_diff(raw_full_text, full_text)
    if correction_stats:
        aggregate = CorrectionStats(
            applied=any(stat.applied for stat in correction_stats),
            model=next((stat.model for stat in correction_stats if stat.model), correction_model or OCR_CORRECTION_MODEL),
            latency_ms=sum(stat.latency_ms for stat in correction_stats),
            changed_characters=sum(stat.changed_characters for stat in correction_stats),
            confidence_delta=round(sum(stat.confidence_delta for stat in correction_stats), 4),
            corrected_blocks=sum(stat.corrected_blocks for stat in correction_stats),
            considered_blocks=sum(stat.considered_blocks for stat in correction_stats),
            retries=sum(stat.retries for stat in correction_stats),
            reason="ok" if any(stat.applied for stat in correction_stats) else "no_changes",
        )
        response.correction = _dict_from_stats(aggregate)
    response.exported_json_path = _write_json_export(response, export_json_path)
    return response


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


@app.get("/health")
def health() -> dict:
    with _cache_lock:
        warmed_models = list(_ocr_cache.keys())

    return {
        "status": "ok",
        "service": "loklingo-ocr",
        "version": "2.2.0",
        "gpu_available": _gpu_available(),
        "models_warmed": warmed_models,
        "models_warmed_count": len(warmed_models),
        "ocr_correction_enabled": OCR_CORRECTION_ENABLED,
        "ocr_correction_model": OCR_CORRECTION_MODEL,
        "ollama_base_url": OLLAMA_BASE_URL,
        "ollama_local_only": _is_local_ollama_url(OLLAMA_BASE_URL),
        "ocr_correction_metrics": {
            "calls_total": _correction_state.calls_total,
            "applied_total": _correction_state.applied_total,
            "failures_total": _correction_state.failures_total,
            "latency_ms_total": round(_correction_state.latency_ms_total, 2),
            "changed_characters_total": _correction_state.changed_characters_total,
            "confidence_delta_total": round(_correction_state.confidence_delta_total, 4),
        },
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
        blocks = _ocr_pil_image(
            image,
            req.lang,
            source_dpi=_extract_image_dpi(image),
            min_confidence=req.min_confidence,
        )
        corrected_blocks, correction_stats, raw_text = _apply_llm_correction(
            image=image,
            blocks=blocks,
            lang=req.lang,
            mode=req.mode,
            enabled_override=req.ocr_correction_enabled,
            model_override=req.ocr_correction_model,
        )
        blocks = corrected_blocks
    except Exception as exc:
        logger.exception("OCR engine error on image: %s", exc)
        raise HTTPException(status_code=500, detail="OCR processing failed")
    finally:
        try:
            image.close()
        except Exception:
            pass

    text, confidence = _summarise(blocks)
    debug_image_path = None
    if req.debug_output_dir:
        debug_image_path = os.path.join(req.debug_output_dir, "ocr-image-debug.png")
        try:
            rendered = Image.open(io.BytesIO(image_bytes))
            try:
                _write_debug_overlay(rendered, blocks, debug_image_path)
            finally:
                rendered.close()
        except Exception as exc:
            logger.warning("Failed to write OCR debug overlay: %s", exc)
            debug_image_path = None

    response = OCRImageResponse(text=text, confidence=confidence, blocks=blocks, debug_image_path=debug_image_path)
    if req.visual_diff_mode:
        response.raw_text = raw_text
        response.correction_diff = _unified_diff(raw_text, text)
    response.correction = _dict_from_stats(correction_stats)
    response.exported_json_path = _write_json_export(response, req.export_json_path)
    return response


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
        min_confidence = payload.min_confidence

        if payload.file_path:
            file_path = _resolve_shared_pdf_path(payload.file_path)
            return _ocr_pdf_file(
                file_path,
                lang,
                dpi,
                min_confidence=min_confidence,
                debug_output_dir=payload.debug_output_dir,
                export_json_path=payload.export_json_path,
                mode=payload.mode,
                correction_enabled=payload.ocr_correction_enabled,
                correction_model=payload.ocr_correction_model,
                visual_diff_mode=payload.visual_diff_mode,
            )

        if payload.pdf_b64:
            try:
                pdf_bytes = base64.b64decode(payload.pdf_b64)
            except Exception:
                raise HTTPException(status_code=400, detail="Invalid base64 PDF data")

            file_path = ""
            try:
                file_path = _write_pdf_bytes_to_temp_pdf(pdf_bytes)
                return _ocr_pdf_file(
                    file_path,
                    lang,
                    dpi,
                    min_confidence=min_confidence,
                    debug_output_dir=payload.debug_output_dir,
                    export_json_path=payload.export_json_path,
                    mode=payload.mode,
                    correction_enabled=payload.ocr_correction_enabled,
                    correction_model=payload.ocr_correction_model,
                    visual_diff_mode=payload.visual_diff_mode,
                )
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
    try:
        min_confidence = float(form.get("min_confidence") or OCR_MIN_CONFIDENCE)
    except (TypeError, ValueError):
        raise HTTPException(status_code=400, detail="Invalid min_confidence value")
    debug_output_dir = form.get("debug_output_dir") or None
    export_json_path = form.get("export_json_path") or None
    mode = str(form.get("mode") or "overlay")
    visual_diff_mode = str(form.get("visual_diff_mode") or "").strip().lower() in {"1", "true", "yes", "on"}
    correction_enabled_form = form.get("ocr_correction_enabled")
    correction_enabled = None
    if correction_enabled_form is not None:
        correction_enabled = str(correction_enabled_form).strip().lower() in {"1", "true", "yes", "on"}
    correction_model = str(form.get("ocr_correction_model") or "").strip() or None

    file_path = ""
    try:
        file_path = _write_upload_to_temp_pdf(upload)
        return _ocr_pdf_file(
            file_path,
            lang,
            dpi,
            min_confidence=min_confidence,
            debug_output_dir=debug_output_dir,
            export_json_path=export_json_path,
            mode=mode,
            correction_enabled=correction_enabled,
            correction_model=correction_model,
            visual_diff_mode=visual_diff_mode,
        )
    finally:
        await upload.close()
        if file_path:
            try:
                os.remove(file_path)
            except OSError:
                logger.warning("Failed to remove temp PDF: %s", file_path)

