# Step 13 — Rollout And Rollback Status

The feature is implemented behind an off-by-default flag. CP1 passed (real model load and a
10-page end-to-end OCR smoke test on GB10). No corpus quality gate or canary has been run yet.

## Current safe default

```env
OCR_PROVIDER=paddle
OCR_UNLIMITED_ENABLED=false
OCR_UNLIMITED_LANGS=
```

## Rehearsed evidence

- 100-page synthetic PDF: 100/100 pages, no empty pages, ~300 s.
- 200-page synthetic PDF after repetition retry: 200/200 pages, correct first/last
  pages, retries on pages 26/58/200, ~580 s.
- Job-level `ocr_engine=unlimited` routing works while `OCR_PROVIDER=paddle`.
- Sidecar-stopped fallback and config-only rollback both completed through PaddleOCR.
- Frontend build/test/lint pass; dev/prod/UOCR compose configs render.
- Canary runner executed against the backend: 2/2 jobs completed with
  `ocr_engine=unlimited` (1 page 2.0 s; 10 pages 24.0 s), no warnings; report at
  [`canary-run.json`](canary-run.json).
- Real 100-page GLSL specification: 100/100 pages, 26.8 min, no warnings/degenerate
  flags; see [`real-document-validation.md`](real-document-validation.md).
- Real 200-page attempt exposed runaway table generation and exceeded the 30-minute client
  timeout; caps/retry were added, but the 200-page real run has not been repeated.

## Rollout sequence

1. Complete `docs/reports/unlimited-ocr/step2-serving.md` and record CP1 metrics.
2. Run the evaluation harness on the target corpus:
   ```bash
   cd ocr-service
   python3 eval_unlimited.py eval-data --engine paddle --json-out ../docs/reports/unlimited-ocr/paddle.json
   python3 eval_unlimited.py eval-data --engine unlimited --json-out ../docs/reports/unlimited-ocr/uocr.json
   ```
3. Choose `OCR_UNLIMITED_LANGS` from the passing languages.
4. Enable `engine=unlimited` for internal/opt-in jobs.
5. Canary a small percentage of `ocr_only` jobs.
6. Default to Unlimited OCR only after the quality gate and canary show no regression.

## Rollback

```bash
OCR_UNLIMITED_ENABLED=false
OCR_PROVIDER=paddle
docker compose --profile ocr-vlm stop loklingo-ocr-vlm
```

Restart the backend and OCR service. No database migration or response-schema removal is
required; the new fields are additive.

## Not yet done

- Real-user traffic canary (the batch runner and rollback path are verified).
- A separate backend OCR engine for OCR-service outages; the current providers are
  adapters to one service, so this is an architectural limitation rather than a pending
  integration step.
- Real-document quality gate and language allowlist; synthetic Latin smoke tests are
  clean but not representative.
- v2 production integration; v2 Transformers probe works but requires a separate parser
  and quality work.
