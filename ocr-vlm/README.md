# LokLingo Unlimited OCR Sidecar

Minimal GPU inference service for `baidu/Unlimited-OCR` (v1). It exposes:

- `GET /health` — model/load status.
- `GET /v1/models` — compatibility probe.
- `POST /infer` — ordered page images + prompt, returns raw model text.

PDF rendering, page windows, `<PAGE>`/`<|det|>` parsing, bbox scaling, quality flags, and
PaddleOCR fallback remain in `../ocr-service`. The sidecar is **not** published on a host
port in production.

## Build and run

```bash
docker build -t loklingo/ocr-vlm:dev .
docker run --rm --gpus all -p 10000:10000 \
  -v loklingo-uocr-models:/models \
  -e MODEL_ID=baidu/Unlimited-OCR \
  -e MODEL_REVISION=<pinned-commit> \
  loklingo/ocr-vlm:dev
```

The first start downloads several gigabytes of weights. Use a model-cache volume and
pre-fetch for air-gapped deployments.

## Smoke test

```bash
curl -fsS http://127.0.0.1:10000/health
curl -fsS http://127.0.0.1:10000/v1/models
```

Then send a page with the same JSON shape the OCR adapter uses:

```json
{
  "pages": [{"page_number": 1, "image_b64": "<base64-png>"}],
  "prompt": "document parsing.",
  "crop_mode": true,
  "max_tokens": 20000,
  "ngram_size": 35,
  "ngram_window": 128
}
```

## Notes

- The model requires `trust_remote_code=True`; pin `MODEL_REVISION` and review the model
  code before production.
- Inference is serialized with a process-wide lock. Scale with multiple sidecar replicas or
  replace this wrapper with the vLLM path after validating SM121 support.
- `LOAD_MODEL_ON_STARTUP=false` starts the HTTP server without loading weights; useful for
  container/config tests, but `/infer` will still attempt a load.
