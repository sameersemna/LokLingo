#!/usr/bin/env sh
set -eu

frontend_url="${FRONTEND_URL:-http://localhost:13000}"
backend_url="${BACKEND_URL:-http://localhost:28080}"
ocr_url="${OCR_URL:-http://localhost:18000}"

echo "Checking backend health: ${backend_url}/health"
backend_health="$(curl -fsS "${backend_url}/health")"
echo "$backend_health" | grep '"status":"ok"' >/dev/null

echo "Checking OCR health: ${ocr_url}/health"
ocr_health="$(curl -fsS "${ocr_url}/health")"
echo "$ocr_health" | grep '"status":"ok"' >/dev/null

echo "Checking frontend translate path: ${frontend_url}/translate"
translation="$(curl -fsS "${frontend_url}/translate" \
  -H 'Content-Type: application/json' \
  --data-raw '{"text":"hello world","source":"en","target":"de"}')"
echo "$translation" | grep '"translated_text"' >/dev/null

echo "Checking PDF job endpoint: POST ${backend_url}/api/v1/jobs/pdf"
pdf_tmp="$(mktemp /tmp/loklingo-smoke-XXXX.pdf)"
# Write a minimal PDF header — the handler validates the field, not the content.
printf '%%PDF-1.4\n%%%%EOF\n' > "${pdf_tmp}"
pdf_job="$(curl -fsS -X POST "${backend_url}/api/v1/jobs/pdf" \
  -F "file=@${pdf_tmp};type=application/pdf" \
  -F "source=en" \
  -F "target=de")"
rm -f "${pdf_tmp}"
echo "$pdf_job" | grep '"job_id"' >/dev/null

echo "Smoke test passed"