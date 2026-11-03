# Step 13 — Rollout And Rollback Status

The feature is implemented behind an off-by-default flag. CP1 passed (real model load and a
10-page end-to-end OCR smoke test on GB10). No corpus quality gate or canary has been run yet.

## Current safe default

```env
OCR_PROVIDER=paddle
OCR_UNLIMITED_ENABLED=false
OCR_UNLIMITED_LANGS=
```

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

- Real canary metrics.
- Backend-level OCR-service outage fallback (OCR-service internal fallback to Paddle is
  verified with the sidecar stopped).
- Long-document 100/200-page validation.
- v2 evaluation.
- Corpus quality gate and language allowlist.
