# Step 5 / Step 10 — Backend And End-to-End Validation

## Delivered

- `OCRProviderUnlimited` and `engine` propagation through `OCRClient` options.
- `providersForRequest` reorders the provider chain when a job requests
  `ocr_engine=paddle|unlimited`, even when the deployed `OCR_PROVIDER` is different.
- Provider-specific timeout: `OCR_UNLIMITED_PROVIDER_TIMEOUT_SECONDS`.
- Job fields `ocr_engine` and `force_ocr`; handler validation and job responses.
- `force_ocr` bypasses native PDF text extraction and enters the OCR path.
- OCR result metadata (`engine`, `engine_version`, `fallback_from`, `warnings`, `quality`).
- Frontend API types and job-detail engine badge.
- Error handling: UOCR failure falls back to Paddle where configured.

## Verified end-to-end

Command path:

```text
POST /api/v1/jobs/pdf (ocr_engine=unlimited, force_ocr=true)
  -> backend worker
  -> OCR service (engine=unlimited)
  -> loklingo-ocr-vlm (GPU)
  -> baidu/Unlimited-OCR
```

Result:

- Job status: `completed`.
- `ocr_engine`: `unlimited`.
- `force_ocr`: `true`.
- `processing_method`: `ocr`.
- Pages: 10 / 10.
- Provider log: `ocr_provider_selected provider=unlimited position=0`.
- `ocr_confidence`: omitted (`0`, because Unlimited OCR does not emit calibrated
  confidence).
- No fallback occurred.
- End-to-end latency: ~26–27 seconds for the synthetic 10-page PDF.

This was tested with `OCR_PROVIDER=paddle` to prove that a per-job `ocr_engine` request
reorders the provider chain correctly.

## Fallback validation

With the sidecar stopped and `OCR_UNLIMITED_ENABLED=true`:

- `POST /api/v1/ocr/pdf` with `engine=unlimited` returned HTTP 200.
- Response `engine=paddle`, `fallback_from=unlimited`, warning
  `fell back from unlimited OCR to PaddleOCR`.
- One-page extraction completed in ~2.6 s.

## Pending

- Backend-provider fallback (as opposed to OCR-service internal fallback) under a
  simulated OCR-service outage.
- Real canary metrics.
- Corpus quality evaluation and language allowlist.
- Frontend control to select the engine (API and job badge are implemented; the upload UI
  remains default Paddle).
