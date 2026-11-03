# Unlimited OCR — Real-User Canary Plan

Status: ready to execute; not executed because no real-user traffic is available in this
environment.

## Scope

Canary `ocr_engine=unlimited` for `ocr_only` PDF jobs in a staging or low-risk production
cohort. Keep `OCR_PROVIDER=paddle` and `OCR_UNLIMITED_LANGS` empty for automatic routing.

## Preconditions

- Sidecar healthy, model revision pinned, `ocr-vlm` profile running.
- `OCR_UNLIMITED_ENABLED=true` on the OCR service.
- At least one approved language in `OCR_UNLIMITED_LANGS` after a real-corpus quality gate.
- Paddle fallback enabled.
- Dashboards for job failures, OCR fallback, truncation/repetition, and p95 latency.

## Execution

Use the canary runner for an explicit batch:

```bash
WRITE_API_TOKEN=... python3 scripts/unlimited_ocr_canary.py \
  --backend-url http://127.0.0.1:28080 \
  --files canary/*.pdf \
  --engine unlimited \
  --json-out docs/reports/unlimited-ocr/canary-run.json
```

For real traffic, route a small percentage of `ocr_only` jobs with
`ocr_engine=unlimited` (feature flag or job-creation policy) and compare against the
Paddle cohort.

## Abort thresholds

Rollback immediately if any of these is observed:

- Job failure or dead-letter rate increases by more than **0.5 percentage points**.
- OCR fallback rate exceeds **5%** for 15 minutes.
- Truncation/repetition flags exceed **1%** of pages.
- p95 OCR latency exceeds **2×** the Paddle cohort.
- Any user-visible data-loss or silently empty page.
- GPU OOM/restart loop.

## Rollback

```bash
OCR_UNLIMITED_ENABLED=false
OCR_PROVIDER=paddle
docker compose --profile ocr-vlm stop loklingo-ocr-vlm
```

Restart the OCR service. Confirm a test job completes through PaddleOCR.

## Exit criteria

- At least 100 documents or two hours of canary traffic, whichever is longer.
- No abort threshold breach.
- Per-language quality spot-check shows no regression.
- Operator approves any default change separately.
