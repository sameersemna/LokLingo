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

# ---------------------------------------------------------------------------
# OCR /ocr/pdf smoke: synthetic multi-page PDF → validate response shape
# Requires: python3 with fpdf2 *or* reportlab installed, OR a pre-built PDF.
# Falls back to a 1-page minimal PDF when neither generator is available.
# ---------------------------------------------------------------------------
echo "Checking OCR /ocr/pdf endpoint: POST ${ocr_url}/ocr/pdf"

SMOKE_PAGES=3

if python3 -c "import fpdf" 2>/dev/null; then
  ocr_pdf_tmp="$(mktemp /tmp/loklingo-ocr-smoke-XXXX.pdf)"
  python3 - "$ocr_pdf_tmp" "$SMOKE_PAGES" <<'PYEOF'
import sys
from fpdf import FPDF
out_path, pages = sys.argv[1], int(sys.argv[2])
pdf = FPDF()
for i in range(1, pages + 1):
    pdf.add_page()
    pdf.set_font("Helvetica", size=16)
    pdf.cell(0, 10, f"Smoke test page {i} of {pages}")
pdf.output(out_path)
PYEOF
elif python3 -c "import reportlab" 2>/dev/null; then
  ocr_pdf_tmp="$(mktemp /tmp/loklingo-ocr-smoke-XXXX.pdf)"
  python3 - "$ocr_pdf_tmp" "$SMOKE_PAGES" <<'PYEOF'
import sys
from reportlab.pdfgen import canvas
out_path, pages = sys.argv[1], int(sys.argv[2])
c = canvas.Canvas(out_path)
for i in range(1, pages + 1):
    c.drawString(72, 720, f"Smoke test page {i} of {pages}")
    c.showPage()
c.save()
PYEOF
else
  # Neither generator available — fall back to a 1-page skeleton PDF
  echo "(fpdf2/reportlab not found — using 1-page skeleton PDF)"
  SMOKE_PAGES=1
  ocr_pdf_tmp="$(mktemp /tmp/loklingo-ocr-smoke-XXXX.pdf)"
  printf '%%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\nxref\n0 4\n0000000000 65535 f\n0000000009 00000 n\n0000000058 00000 n\n0000000115 00000 n\ntrailer\n<< /Root 1 0 R /Size 4 >>\nstartxref\n190\n%%%%EOF\n' > "$ocr_pdf_tmp"
fi

ocr_pdf_resp="$(curl -fsS -X POST "${ocr_url}/ocr/pdf" \
  -F "file=@${ocr_pdf_tmp};type=application/pdf" \
  -F "lang=en" \
  -F "dpi=150")"
rm -f "$ocr_pdf_tmp"

# Validate response has expected keys
echo "$ocr_pdf_resp" | grep '"text"'  >/dev/null
echo "$ocr_pdf_resp" | grep '"pages"' >/dev/null

# Validate the pages array contains the expected number of entries
pages_count="$(echo "$ocr_pdf_resp" | python3 -c "import sys, json; d=json.load(sys.stdin); print(len(d['pages']))")"
if [ "$pages_count" != "$SMOKE_PAGES" ]; then
  echo "ERROR: expected $SMOKE_PAGES pages in OCR response, got $pages_count"
  exit 1
fi
echo "  OCR /ocr/pdf: $pages_count page(s) processed OK"

# ---------------------------------------------------------------------------
# Optional RSS gate: only runs when bench_pdf_memory.py + psutil are present
# inside the OCR container, and docker CLI is available.
# ---------------------------------------------------------------------------
RSS_LIMIT_MB=600
if command -v docker >/dev/null 2>&1; then
  container_id="$(docker compose ps -q loklingo-ocr 2>/dev/null || true)"
  if [ -n "$container_id" ]; then
    echo "Checking OCR container RSS ceiling (<${RSS_LIMIT_MB} MB per-page …)"
    bench_out="$(docker exec "$container_id" python3 bench_pdf_memory.py /tmp/smoke_rss.pdf --dpi 150 --lang en 2>&1 || true)"
    if echo "$bench_out" | grep -q "Delta peak RSS"; then
      delta_mb="$(echo "$bench_out" | grep "Delta peak RSS" | grep -oE '[0-9]+(\.[0-9]+)?' | head -1)"
      echo "  Delta peak RSS = ${delta_mb} MB"
      # Use awk for float comparison (POSIX sh has no float arithmetic)
      ok="$(awk -v d="$delta_mb" -v lim="$RSS_LIMIT_MB" 'BEGIN { print (d < lim) ? "yes" : "no" }')"
      if [ "$ok" != "yes" ]; then
        echo "ERROR: OCR RSS delta ${delta_mb} MB exceeds limit ${RSS_LIMIT_MB} MB"
        exit 1
      fi
    else
      echo "  bench_pdf_memory.py skipped or no PDF available — RSS check skipped"
    fi
  else
    echo "  loklingo-ocr container not running — RSS check skipped"
  fi
fi

echo "Smoke test passed"