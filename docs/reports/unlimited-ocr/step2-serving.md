# Step 2 / Step 3 — Unlimited OCR Sidecar Status

## Delivered

- `ocr-vlm/server.py` — FastAPI sidecar with `GET /health`, `GET /v1/models`, and
  `POST /infer`.
- `ocr-vlm/Dockerfile` — CUDA 13.0/Python 3.12 image, non-root user, model cache volume,
  health check, no host port in compose.
- `ocr-vlm/requirements.txt`.
- `ocr-vlm/test_server.py` — contract tests using a fake engine.
- `docker-compose.yml` optional `ocr-vlm` profile with GPU reservation.
- `docker-compose.prod.yml` hardening overrides.
- OCR service adapter and page-window normalization.

## Verified locally

- Sidecar image builds on ARM64/CUDA 13.0 after pinning `torchvision==0.27.1`; the
  original `0.23.1` pin had no cu130 aarch64 wheel.
- Sidecar contract tests: `python3 -m pytest test_server.py -q` → 3 passed.
- Sidecar container starts with `LOAD_MODEL_ON_STARTUP=false`; `/health` and
  `/v1/models` respond.
- Compose dev/profile config renders.
- Compose production config renders with CI-style secrets.
- OCR service tests: 110 passed + 4 subtests.

## CP1 real-model validation

- **CP1 passed:** the real `baidu/Unlimited-OCR` model loaded on the GB10 and returned
  OCR text through `/infer` at pinned revision
  `8249d20b2822a7d8a4c22c8d8c9069b5ac36bda3`.
- Single-page inference: 2.2–3.7 s (warm, simple synthetic page).
- Two-page inference: 0.8–2.7 s after warm-up.
- Ten-page PDF end-to-end through `/api/v1/ocr/pdf` with `engine=unlimited`: HTTP 200,
  10/10 pages, 25.8 s (~2.6 s/page).
- 100-page synthetic PDF: 100/100 pages, no empty pages, ~300 s.
- 200-page synthetic PDF after repetition retry and 5-page windows: 200/200 pages,
  correct first/last pages, retries on pages 26/58/200, ~580 s.
- The model's `<PAGE>` count is unreliable for larger sparse windows (10-page request
  returned 8 markers), so the adapter adaptively splits windows down to one page and
  retries pages whose output collapses into repeated lines.
- Sidecar container memory observed at ~2.5 GiB host RAM; image size ~5.66 GB.
- `matplotlib` was required by the model code and added to the sidecar image.
- vLLM and SGLang paths are not implemented; the sidecar currently uses Transformers.
- Offline loading (`HF_HUB_OFFLINE=1`) works from the seeded model cache.

## Remaining validation

- Real-document 100/200-page quality and reproducibility.
- Real-corpus CER/WER/table evaluation and language allowlist.
- vLLM/SGLang comparison.
- Real-user canary.

## Next command when the operator is ready

```bash
docker build -t loklingo/ocr-vlm:dev ocr-vlm
docker compose --profile ocr-vlm up -d --build loklingo-ocr-vlm
docker compose logs -f loklingo-ocr-vlm
```

The model revision is pinned in `.env.example` and `docker-compose.yml`; change it only with a reviewed smoke test.
