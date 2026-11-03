"""Adapter for the optional Unlimited OCR sidecar.

This module is intentionally separate from ``app.py``. The OCR service keeps its PaddleOCR
pipeline intact and calls the sidecar only when ``engine`` explicitly selects it.
"""

from __future__ import annotations

import base64
import io
import json
import logging
import math
import os
import re
import urllib.error
import urllib.request
from typing import Any

from PIL import Image

logger = logging.getLogger("loklingo.ocr.unlimited")

OCR_UNLIMITED_ENABLED = os.getenv("OCR_UNLIMITED_ENABLED", "false").strip().lower() in {
    "1",
    "true",
    "yes",
    "on",
}
OCR_UNLIMITED_BASE_URL = os.getenv("OCR_UNLIMITED_BASE_URL", "http://loklingo-ocr-vlm:10000").strip()
OCR_UNLIMITED_MODEL = os.getenv("OCR_UNLIMITED_MODEL", "baidu/Unlimited-OCR").strip() or "baidu/Unlimited-OCR"
OCR_UNLIMITED_BACKEND = os.getenv("OCR_UNLIMITED_BACKEND", "transformers").strip() or "transformers"
OCR_UNLIMITED_TIMEOUT = int(os.getenv("OCR_UNLIMITED_TIMEOUT", "600").strip() or "600")
OCR_UNLIMITED_MAX_PAGES_PER_REQUEST = int(
    os.getenv("OCR_UNLIMITED_MAX_PAGES_PER_REQUEST", "5").strip() or "5"
)
OCR_UNLIMITED_PAGE_OVERLAP = int(os.getenv("OCR_UNLIMITED_PAGE_OVERLAP", "1").strip() or "1")
OCR_UNLIMITED_FALLBACK_TO_PADDLE = os.getenv(
    "OCR_UNLIMITED_FALLBACK_TO_PADDLE", "true"
).strip().lower() in {"1", "true", "yes", "on"}
OCR_UNLIMITED_LANGS = {
    code.strip().lower()
    for code in os.getenv("OCR_UNLIMITED_LANGS", "").split(",")
    if code.strip()
}
OCR_UNLIMITED_DPI = int(os.getenv("OCR_UNLIMITED_DPI", "200").strip() or "200")
OCR_UNLIMITED_MAX_OUTPUT_TOKENS_PER_PAGE = int(
    os.getenv("OCR_UNLIMITED_MAX_OUTPUT_TOKENS_PER_PAGE", "4096").strip() or "4096"
)


def _normalize_base_url(raw: str) -> str:
    return raw.rstrip("/")


def _encode_image(image: Image.Image) -> str:
    buffer = io.BytesIO()
    image.convert("RGB").save(buffer, format="PNG")
    return base64.b64encode(buffer.getvalue()).decode("ascii")


def _post_sidecar(payload: dict[str, Any]) -> dict[str, Any]:
    base = _normalize_base_url(OCR_UNLIMITED_BASE_URL)
    url = f"{base}/infer"
    data = json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=OCR_UNLIMITED_TIMEOUT) as response:
            raw = response.read()
    except urllib.error.HTTPError as exc:  # pragma: no cover - exercised with mocks
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"Unlimited OCR sidecar HTTP {exc.code}: {detail}") from exc
    except Exception as exc:
        raise RuntimeError(f"Unlimited OCR sidecar unavailable: {exc}") from exc

    try:
        decoded = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise RuntimeError("Unlimited OCR sidecar returned invalid JSON") from exc
    if not isinstance(decoded, dict):
        raise RuntimeError("Unlimited OCR sidecar returned a non-object response")
    return decoded


def check_sidecar_health(timeout: float = 1.5) -> bool:
    if not OCR_UNLIMITED_ENABLED:
        return False
    try:
        with urllib.request.urlopen(
            f"{_normalize_base_url(OCR_UNLIMITED_BASE_URL)}/health", timeout=timeout
        ) as response:
            return 200 <= response.status < 300
    except Exception:
        return False


def language_allowed(lang: str) -> bool:
    if not OCR_UNLIMITED_LANGS:
        return False
    return lang.strip().lower() in OCR_UNLIMITED_LANGS


def resolve_engine(requested: str, lang: str) -> str:
    requested = (requested or "").strip().lower()
    if requested in {"paddle", "unlimited"}:
        return requested
    if requested == "auto":
        if OCR_UNLIMITED_ENABLED and language_allowed(lang):
            return "unlimited"
        return "paddle"
    if requested in {"", "None", "none"}:
        return "paddle"
    raise ValueError(f"unsupported OCR engine: {requested}")


REF_PATTERN = re.compile(r"<\|ref\|>(.*?)<\|/ref\|>\s*<\|det\|>(.*?)<\|/det\|>", re.DOTALL)
DIRECT_DET_PATTERN = re.compile(r"<\|det\|>\s*([^\s<]+)\s*([^\n<]*)<\|/det\|>", re.DOTALL)
PAGE_SPLIT_PATTERN = re.compile(r"<PAGE>", re.IGNORECASE)
V2_BLOCK_PATTERN = re.compile(
    r"<BLOCK>\s*([A-Za-z_]+)(?:<LEVEL(\d+)>)?\s*\[([^\]]+)\]\s*<FORK>(.*?)(?=<BLOCK>|$)",
    re.DOTALL,
)


def _extract_boxes(raw_box: str) -> list[list[float]]:
    raw_box = raw_box.strip()
    if not raw_box:
        return []

    # The model can emit either a bare box, ``label [x1,y1,x2,y2]``, or a list of
    # boxes. Extract bracketed groups wherever they begin.
    bracket_start = raw_box.find("[")
    if bracket_start >= 0:
        bracket_text = raw_box[bracket_start:]
        chunks = re.findall(r"\[[^\[\]]*\]", bracket_text)
        results: list[list[float]] = []
        for chunk in chunks or [bracket_text]:
            try:
                values = [float(v.strip()) for v in chunk.strip("[]").split(",") if v.strip()]
            except ValueError:
                continue
            if len(values) == 4:
                results.append(values)
        if results:
            return results
        return []

    try:
        values = [float(v.strip()) for v in raw_box.split(",") if v.strip()]
    except ValueError:
        return []
    return [values] if len(values) == 4 else []


def _scale_bbox(box: list[float], width: int, height: int) -> list[float] | None:
    if len(box) != 4:
        return None
    if not all(math.isfinite(value) for value in box):
        return None
    x1, y1, x2, y2 = box
    scaled = [
        float(min(max(x1 / 999.0 * width, 0.0), width)),
        float(min(max(y1 / 999.0 * height, 0.0), height)),
        float(min(max(x2 / 999.0 * width, 0.0), width)),
        float(min(max(y2 / 999.0 * height, 0.0), height)),
    ]
    if scaled[2] <= scaled[0] or scaled[3] <= scaled[1]:
        return None
    return scaled


def _classify_label(label: str) -> tuple[str, int]:
    normalized = label.strip().lower()
    mapping = {
        "title": ("title", 1),
        "headline": ("title", 1),
        "heading": ("heading", 2),
        "section": ("heading", 2),
        "subheading": ("subheading", 3),
        "table": ("table", 0),
        "formula": ("formula", 0),
        "equation": ("formula", 0),
        "math": ("formula", 0),
        "figure": ("figure", 0),
        "image": ("figure", 0),
        "chart": ("figure", 0),
        "header": ("annotation", 0),
        "footer": ("annotation", 0),
        "page_number": ("annotation", 0),
        "footnote": ("annotation", 0),
        "caption": ("caption", 0),
    }
    return mapping.get(normalized, ("body", 0))


def repeated_line_ratio(text: str) -> float:
    lines = [line.strip() for line in (text or "").splitlines() if line.strip()]
    if len(lines) < 3:
        return 0.0
    return float(len(lines) - len(set(lines))) / float(len(lines))


def is_degenerate_page(text: str) -> bool:
    """Detect runaway tables, repeated lines, or excessive output."""
    value = text or ""
    if len(value) > 40000:
        return True
    if value.count("<tr") > 200:
        return True
    if repeated_line_ratio(value) > 0.4:
        return True
    return _repitition_ratio(value) > 0.05


def _repitition_ratio(text: str, n: int = 35) -> float:
    tokens = text.split()
    if len(tokens) < n:
        return 0.0
    ngrams = [tuple(tokens[i : i + n]) for i in range(len(tokens) - n + 1)]
    total = len(ngrams)
    distinct = len(set(ngrams))
    return float(total - distinct) / float(total) if total else 0.0


def parse_unlimited_v2_markdown(
    raw: str,
    image_width: int,
    image_height: int,
    engine_version: str = OCR_UNLIMITED_MODEL,
) -> tuple[list[dict[str, Any]], str, list[str], dict[str, Any]]:
    blocks: list[dict[str, Any]] = []
    warnings: list[str] = []
    for match in V2_BLOCK_PATTERN.finditer(raw or ""):
        label = match.group(1).strip() or "text"
        level = int(match.group(2) or "0")
        box_text = match.group(3).strip()
        content = match.group(4).strip()
        region_class, hierarchy = _classify_label(label)
        if level:
            hierarchy = level
        boxes = [_scale_bbox(box, image_width, image_height) for box in _extract_boxes(box_text)]
        boxes = [box for box in boxes if box is not None] or [[0.0, 0.0, float(image_width), float(image_height)]]
        group_id = len(blocks)
        for box in boxes:
            blocks.append(
                {
                    "text": content,
                    "bbox": box,
                    "confidence": 0.0,
                    "reading_order": len(blocks) + 1,
                    "region_class": region_class,
                    "hierarchy_level": hierarchy,
                    "column_id": 0,
                    "text_group": group_id,
                }
            )
    plain_blocks: list[str] = []
    seen_groups: set[int] = set()
    for block in blocks:
        group = block.get("text_group")
        if isinstance(group, int):
            if group in seen_groups:
                continue
            seen_groups.add(group)
        if block.get("text", "").strip():
            plain_blocks.append(block["text"].strip())
    plain_text = "\n".join(plain_blocks).strip()
    markup_leak = "<BLOCK>" in plain_text or "<FORK>" in plain_text
    if markup_leak:
        warnings.append("OCR output contained unparsed v2 block markers")
    quality = {
        "page_marker_ok": True,
        "page_count_match": True,
        "truncated": False,
        "empty_page_count": 0,
        "bbox_invalid_count": 0,
        "markup_leak": markup_leak,
        "repetition_ratio": _repitition_ratio(plain_text),
        "repeated_line_ratio": repeated_line_ratio(plain_text),
        "table_rows": plain_text.count("<tr"),
        "char_count": len(plain_text),
        "degenerate": is_degenerate_page(plain_text),
        "engine": "unlimited-v2",
        "engine_version": engine_version,
    }
    return blocks, plain_text, warnings, quality


def parse_unlimited_markdown(
    raw: str,
    image_width: int,
    image_height: int,
    engine_version: str = OCR_UNLIMITED_MODEL,
) -> tuple[list[dict[str, Any]], str, list[str], dict[str, Any]]:
    """Return blocks, plain text, warnings, and quality metadata for one image."""
    if "<BLOCK>" in (raw or "") and "<FORK>" in (raw or ""):
        return parse_unlimited_v2_markdown(raw, image_width, image_height, engine_version)

    blocks: list[dict[str, Any]] = []
    warnings: list[str] = []
    raw_text = raw or ""
    position = 0

    # ref/det form.
    for match in REF_PATTERN.finditer(raw or ""):
        label = match.group(1).strip() or "text"
        box_text = match.group(2)
        content = raw[match.end() : raw.find("<|ref|>", match.end())] if False else ""
        # Content follows the marker on subsequent lines; capture until next marker or end.
        start = match.end()
        next_marker = min(
            [i for i in (raw.find("<|ref|>", start), raw.find("<|det|>", start)) if i != -1] + [len(raw)]
        )
        content = raw[start:next_marker].strip()
        quality_label, hierarchy = _classify_label(label)
        region_class = label.lower() if label.lower() in {
            "title",
            "heading",
            "text",
            "table",
            "formula",
            "image",
            "caption",
            "body",
        } else quality_label
        boxes = [_scale_bbox(box, image_width, image_height) for box in _extract_boxes(box_text)]
        boxes = [box for box in boxes if box is not None]
        if not boxes:
            boxes = [[0.0, 0.0, float(image_width), float(image_height)]]
        text = content.strip()
        group_id = len(blocks)
        for box in boxes:
            blocks.append(
                {
                    "text": text,
                    "bbox": box,
                    "confidence": 0.0,
                    "reading_order": len(blocks) + 1,
                    "region_class": region_class,
                    "hierarchy_level": hierarchy,
                    "column_id": 0,
                    "text_group": group_id,
                }
            )

    # Direct det form, if nothing was recovered above.
    if not blocks:
        for match in DIRECT_DET_PATTERN.finditer(raw or ""):
            label = match.group(1).strip() or "text"
            box_text = match.group(2).strip()
            start = match.end()
            next_marker = min(
                [i for i in (raw.find("<|det|>", start),) if i != -1] + [len(raw)]
            )
            content = raw[start:next_marker].strip()
            quality_label, hierarchy = _classify_label(label)
            region_class = quality_label
            boxes = [_scale_bbox(box, image_width, image_height) for box in _extract_boxes(box_text)]
            boxes = [box for box in boxes if box is not None]
            if not boxes:
                boxes = [[0.0, 0.0, float(image_width), float(image_height)]]
            group_id = len(blocks)
            for box in boxes:
                blocks.append(
                    {
                        "text": content.strip(),
                        "bbox": box,
                        "confidence": 0.0,
                        "reading_order": len(blocks) + 1,
                        "region_class": region_class,
                        "hierarchy_level": hierarchy,
                        "column_id": 0,
                        "text_group": group_id,
                    }
                )

    if not blocks:
        # Plain text output without grounding: expose one page-level block.
        stripped = (raw or "").strip()
        if stripped:
            blocks.append(
                {
                    "text": stripped,
                    "bbox": [0.0, 0.0, float(image_width), float(image_height)],
                    "confidence": 0.0,
                    "reading_order": 1,
                    "region_class": "body",
                    "hierarchy_level": 0,
                    "column_id": 0,
                }
            )

    plain_blocks: list[str] = []
    seen_groups: set[int] = set()
    for block in blocks:
        group = block.get("text_group")
        if isinstance(group, int):
            if group in seen_groups:
                continue
            seen_groups.add(group)
        text = block.get("text", "").strip()
        if text:
            plain_blocks.append(text)
    plain_text = "\n".join(plain_blocks).strip()
    markup_leak = "<|ref|>" in plain_text or "<|det|>" in plain_text
    if markup_leak:
        warnings.append("OCR output contained unparsed grounding markers")
    repetition = _repitition_ratio(plain_text)
    quality = {
        "page_marker_ok": True,
        "page_count_match": True,
        "truncated": False,
        "empty_page_count": 0,
        "bbox_invalid_count": 0,
        "markup_leak": markup_leak,
        "repetition_ratio": repetition,
        "repeated_line_ratio": repeated_line_ratio(plain_text),
        "table_rows": plain_text.count("<tr"),
        "char_count": len(plain_text),
        "degenerate": is_degenerate_page(plain_text),
        "engine": "unlimited",
        "engine_version": engine_version,
    }
    if repetition > 0.05:
        warnings.append("possible repeated output detected")
    return blocks, plain_text, warnings, quality


def plan_page_windows(page_count: int, window_size: int, overlap: int) -> list[tuple[int, int]]:
    """Return zero-based [start, end) page windows with optional overlap."""
    if page_count <= 0:
        return []
    window_size = max(1, min(window_size, 100))
    overlap = max(0, min(overlap, window_size - 1))
    windows: list[tuple[int, int]] = []
    start = 0
    while start < page_count:
        end = min(start + window_size, page_count)
        windows.append((start, end))
        if end >= page_count:
            break
        start = max(start + 1, end - overlap)
    return windows


def _split_pages(raw: str) -> list[str]:
    if PAGE_SPLIT_PATTERN.search(raw or ""):
        parts = [part.strip() for part in PAGE_SPLIT_PATTERN.split(raw or "")]
        if parts and not parts[0]:
            parts = parts[1:]
        return [part for part in parts if part.strip() or len(parts) > 1]
    return [raw.strip()] if raw and raw.strip() else []


def request_unlimited_pdf(
    page_images: list[Image.Image],
    page_numbers: list[int],
    *,
    max_tokens: int | None = None,
    strict: bool = True,
) -> list[tuple[int, str]]:
    """Call the sidecar for a window of pages and return (page_number, raw_text)."""
    if len(page_images) != len(page_numbers):
        raise ValueError("page image/page number mismatch")
    if not OCR_UNLIMITED_ENABLED:
        raise RuntimeError("Unlimited OCR is disabled")
    payload = {
        "pages": [
            {"page_number": page_numbers[index], "image_b64": _encode_image(image)}
            for index, image in enumerate(page_images)
        ],
        "prompt": "<image>Multi page parsing.",
        "crop_mode": False,
        "max_tokens": max_tokens or (OCR_UNLIMITED_MAX_OUTPUT_TOKENS_PER_PAGE * len(page_images)),
        "temperature": 0.0,
        "skip_special_tokens": False,
        "ngram_size": 35,
        "ngram_window": 1024 if len(page_images) > 1 else 128,
    }
    logger.info(
        "unlimited_pdf_request pages=%d prompt=%r max_tokens=%d",
        len(page_images),
        payload.get("prompt"),
        payload.get("max_tokens"),
    )
    response = _post_sidecar(payload)
    text = str(response.get("text") or response.get("content") or "")
    finish_reason = str(response.get("finish_reason") or "")
    if finish_reason == "length":
        text = text  # Length is already partial output; quality manager will flag it.
    pages = _split_pages(text)
    if len(pages) != len(page_images):
        if strict:
            raise RuntimeError(
                f"Unlimited OCR returned {len(pages)} pages for {len(page_images)} inputs"
            )
        logger.warning(
            "unlimited_page_count_mismatch inputs=%d outputs=%d",
            len(page_images),
            len(pages),
        )
    return list(zip(page_numbers, pages))


def request_unlimited_image(image: Image.Image) -> tuple[list[dict[str, Any]], str, dict[str, Any]]:
    prompt = (
        "<PAGE><image>Multi page parsing."
        if OCR_UNLIMITED_MODEL.rstrip("/").endswith("-2")
        else "<image>document parsing."
    )
    payload = {
        "pages": [{"page_number": 1, "image_b64": _encode_image(image)}],
        "prompt": prompt,
        "crop_mode": True,
        "base_size": 1024,
        "image_size": 640,
        "max_tokens": OCR_UNLIMITED_MAX_OUTPUT_TOKENS_PER_PAGE,
        "temperature": 0.0,
        "ngram_size": 35,
        "ngram_window": 128,
    }
    logger.info("unlimited_image_request size=%dx%d", image.width, image.height)
    response = _post_sidecar(payload)
    raw = str(response.get("text") or response.get("content") or "")
    blocks, plain_text, warnings, quality = parse_unlimited_markdown(
        raw,
        image.width,
        image.height,
        engine_version=OCR_UNLIMITED_MODEL,
    )
    quality["truncated"] = str(response.get("finish_reason") or "") == "length"
    quality["warnings"] = warnings
    return blocks, plain_text, quality
