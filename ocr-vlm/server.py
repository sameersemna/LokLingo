"""Minimal GPU inference sidecar for Baidu Unlimited OCR (v1).

This service is intentionally small: it loads the model once, serializes inference, and
returns raw model text. PDF rendering, page windows, bbox parsing, and Paddle fallback live
in the existing OCR service, not here.

Run inside the GPU-enabled container (see Dockerfile). The server is not published on a host
port in production.
"""

from __future__ import annotations

import base64
import binascii
import io
import logging
import os
import tempfile
import threading
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from PIL import Image

logger = logging.getLogger("loklingo.ocr_vlm")
logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s %(message)s")

MODEL_ID = os.getenv("MODEL_ID", "baidu/Unlimited-OCR").strip() or "baidu/Unlimited-OCR"
MODEL_REVISION = os.getenv("MODEL_REVISION", "8249d20b2822a7d8a4c22c8d8c9069b5ac36bda3").strip() or None
if not MODEL_REVISION:
    logger.warning("MODEL_REVISION is empty; pin a reviewed revision before production")
MODEL_DTYPE = os.getenv("MODEL_DTYPE", "bfloat16").strip() or "bfloat16"
MAX_PAGES = int(os.getenv("MAX_PAGES", "32").strip() or "32")
MAX_IMAGE_BYTES = int(os.getenv("MAX_IMAGE_BYTES", str(20 * 1024 * 1024)).strip() or str(20 * 1024 * 1024))
DEFAULT_MAX_TOKENS = int(os.getenv("DEFAULT_MAX_TOKENS", "20000").strip() or "20000")
LOAD_MODEL_ON_STARTUP = os.getenv("LOAD_MODEL_ON_STARTUP", "true").strip().lower() in {
    "1",
    "true",
    "yes",
    "on",
}


class PageInput(BaseModel):
    page_number: int = Field(ge=1)
    image_b64: str = Field(min_length=1)


class InferRequest(BaseModel):
    pages: list[PageInput] = Field(min_length=1, max_length=MAX_PAGES)
    prompt: str = "<image>Multi page parsing."
    crop_mode: bool = False
    base_size: int = 1024
    image_size: int = 640
    max_tokens: int = Field(default=DEFAULT_MAX_TOKENS, ge=1)
    temperature: float = Field(default=0.0, ge=0.0, le=1.0)
    ngram_size: int = Field(default=35, ge=0)
    ngram_window: int = Field(default=1024, ge=0)


class InferResponse(BaseModel):
    text: str
    token_count: int
    finish_reason: str
    model: str
    revision: str | None = None


class _Engine:
    def __init__(self) -> None:
        self.model: Any = None
        self.tokenizer: Any = None
        self.lock = threading.Lock()
        self.load_error: str | None = None

    def load(self) -> None:
        try:
            import torch
            from transformers import AutoModel, AutoTokenizer
        except Exception as exc:  # pragma: no cover - environment dependent
            self.load_error = f"transformers/torch import failed: {exc}"
            raise

        dtype = torch.bfloat16 if MODEL_DTYPE == "bfloat16" else torch.float32
        logger.info("loading %s revision=%s dtype=%s", MODEL_ID, MODEL_REVISION, MODEL_DTYPE)
        kwargs: dict[str, Any] = {
            "trust_remote_code": True,
            "use_safetensors": True,
            "torch_dtype": dtype,
        }
        if MODEL_REVISION:
            kwargs["revision"] = MODEL_REVISION
        self.tokenizer = AutoTokenizer.from_pretrained(
            MODEL_ID, trust_remote_code=True, revision=MODEL_REVISION
        )
        self.model = (
            AutoModel.from_pretrained(MODEL_ID, **kwargs).eval().cuda()
        )
        logger.info("model loaded")

    def ensure_loaded(self) -> None:
        if self.model is None:
            with self.lock:
                if self.model is None:
                    self.load()

    def infer(self, request: InferRequest) -> tuple[str, int]:
        self.ensure_loaded()
        if len(request.pages) > MAX_PAGES:
            raise ValueError(f"too many pages: {len(request.pages)} > {MAX_PAGES}")

        images: list[Image.Image] = []
        for page in request.pages:
            try:
                raw = base64.b64decode(page.image_b64, validate=True)
            except (binascii.Error, ValueError) as exc:
                raise ValueError(f"page {page.page_number}: invalid base64") from exc
            if len(raw) > MAX_IMAGE_BYTES:
                raise ValueError(f"page {page.page_number}: image exceeds size limit")
            try:
                image = Image.open(io.BytesIO(raw)).convert("RGB")
                image.load()
                images.append(image)
            except Exception as exc:
                raise ValueError(f"page {page.page_number}: invalid image") from exc

        with self.lock:
            with tempfile.TemporaryDirectory(prefix="loklingo-uocr-") as tmp:
                paths: list[str] = []
                for index, image in enumerate(images, start=1):
                    path = os.path.join(tmp, f"page_{index:04d}.png")
                    image.save(path, format="PNG")
                    paths.append(path)

                if len(paths) == 1:
                    text = self.model.infer(
                        self.tokenizer,
                        prompt=request.prompt,
                        image_file=paths[0],
                        output_path=tmp,
                        base_size=request.base_size,
                        image_size=request.image_size,
                        crop_mode=request.crop_mode,
                        max_length=32768,
                        no_repeat_ngram_size=request.ngram_size,
                        ngram_window=request.ngram_window,
                        temperature=request.temperature,
                        eval_mode=True,
                    )
                    text = str(text or "")
                    token_count = len(
                        self.tokenizer.encode(text, add_special_tokens=False)
                        if text
                        else []
                    )
                else:
                    text, token_count = self.model.infer_multi(
                        self.tokenizer,
                        prompt=request.prompt,
                        image_files=paths,
                        output_path=tmp,
                        image_size=1024,
                        max_length=32768,
                        no_repeat_ngram_size=request.ngram_size,
                        ngram_window=request.ngram_window,
                        temperature=request.temperature,
                    )
                    text = str(text or "")
                    token_count = int(token_count)
        return text, token_count


engine = _Engine()


@asynccontextmanager
async def lifespan(_: FastAPI):
    if LOAD_MODEL_ON_STARTUP:
        try:
            engine.ensure_loaded()
        except Exception:
            logger.exception("model load failed; /health will report not ready")
    yield


app = FastAPI(title="LokLingo Unlimited OCR sidecar", version="0.1.0", lifespan=lifespan)


@app.get("/health")
def health() -> dict[str, Any]:
    return {
        "status": "ok" if engine.model is not None else "degraded",
        "model": MODEL_ID,
        "revision": MODEL_REVISION,
        "loaded": engine.model is not None,
        "error": engine.load_error,
        "backend": "transformers",
    }


@app.get("/v1/models")
def models() -> dict[str, Any]:
    return {"data": [{"id": MODEL_ID, "object": "model", "owned_by": "loklingo"}]}


@app.post("/infer", response_model=InferResponse)
def infer(request: InferRequest) -> InferResponse:
    try:
        text, token_count = engine.infer(request)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except Exception as exc:
        logger.exception("inference failed")
        raise HTTPException(status_code=503, detail=f"inference failed: {exc}") from exc
    return InferResponse(
        text=text,
        token_count=token_count,
        finish_reason="stop",
        model=MODEL_ID,
        revision=MODEL_REVISION,
    )
