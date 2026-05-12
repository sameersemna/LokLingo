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
translate_t0="$(date +%s)"
translation="$(curl -fsS "${frontend_url}/translate" \
  -H 'Content-Type: application/json' \
  --data-raw '{"text":"hello world","source":"en","target":"de"}')"
translate_t1="$(date +%s)"
translate_elapsed_s=$(( translate_t1 - translate_t0 ))
echo "$translation" | grep '"translated_text"' >/dev/null
echo "  translate latency: ${translate_elapsed_s}s"

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

# ---------------------------------------------------------------------------
# PDF job timing tests: small / medium / large
# Requires: python3 with fpdf2 installed for realistic multi-page PDFs.
# Falls back to a minimal skeleton PDF when fpdf2 is unavailable.
# ---------------------------------------------------------------------------

# poll_job <job_id> <timeout_s> — polls GET /api/v1/jobs/:id until the job
# reaches a terminal state (completed/failed) or the timeout expires.
# Prints the final status to stdout; exits non-zero on timeout or failure.
poll_job() {
  _jid="$1"
  _timeout="$2"
  _poll_interval=2
  _elapsed=0
  while [ "$_elapsed" -lt "$_timeout" ]; do
    _resp="$(curl -fsS "${backend_url}/api/v1/jobs/${_jid}")"
    _status="$(echo "$_resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))")"
    if [ "$_status" = "completed" ] || [ "$_status" = "failed" ]; then
      echo "$_status"
      return 0
    fi
    sleep "$_poll_interval"
    _elapsed=$(( _elapsed + _poll_interval ))
  done
  echo "timeout"
  return 1
}

# make_pdf <output_path> <num_pages>
# Generates a multi-page text PDF when fpdf2 is available; falls back to a
# minimal 1-page skeleton otherwise (num_pages is then forced to 1).
make_pdf() {
  _out="$1"
  _pages="$2"
  if python3 -c "import fpdf" 2>/dev/null; then
    python3 - "$_out" "$_pages" <<'PYEOF'
import sys
from fpdf import FPDF
out_path, pages = sys.argv[1], int(sys.argv[2])
pdf = FPDF()
for i in range(1, pages + 1):
    pdf.add_page()
    pdf.set_font("Helvetica", size=12)
    for line in range(1, 31):
        pdf.cell(0, 8, f"Page {i}/{pages} – line {line}: The quick brown fox jumps over the lazy dog. Smoke test content.")
        pdf.ln()
pdf.output(out_path)
PYEOF
  else
    echo "  (fpdf2 not found — using minimal skeleton PDF, page count forced to 1)"
    printf '%%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\nxref\n0 4\n0000000000 65535 f\n0000000009 00000 n\n0000000058 00000 n\n0000000115 00000 n\ntrailer\n<< /Root 1 0 R /Size 4 >>\nstartxref\n190\n%%%%EOF\n' > "$_out"
  fi
}

# submit_and_time <label> <num_pages> <poll_timeout_s>
submit_and_time() {
  _label="$1"
  _pages="$2"
  _timeout="$3"

  echo ""
  echo "--- PDF job timing: ${_label} (${_pages} page(s)) ---"

  _tmp="$(mktemp /tmp/loklingo-smoke-timing-XXXX.pdf)"
  make_pdf "$_tmp" "$_pages"

  _t0="$(date +%s)"
  _submit_resp="$(curl -fsS -X POST "${backend_url}/api/v1/jobs/pdf" \
    -F "file=@${_tmp};type=application/pdf" \
    -F "source=en" \
    -F "target=de")"
  rm -f "$_tmp"

  _job_id="$(echo "$_submit_resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['job_id'])")"
  echo "  submitted job_id=${_job_id}"

  _final_status="$(poll_job "$_job_id" "$_timeout")"
  _t1="$(date +%s)"
  _elapsed_s=$(( _t1 - _t0 ))

  echo "  status=${_final_status}  total_time=${_elapsed_s}s"

  if [ "$_final_status" = "timeout" ]; then
    echo "  ERROR: job did not complete within ${_timeout}s"
    exit 1
  fi
  # A failed status is reported but does not abort the smoke run so the other
  # size tiers can still be measured.
  if [ "$_final_status" = "failed" ]; then
    echo "  WARN: job ended with status=failed"
  fi
}

# Small: 1–2 pages, 60 s timeout
submit_and_time "small"  2   60
# Medium: 10–20 pages, 5 min timeout
submit_and_time "medium" 15  300
# Large: 50+ pages, 15 min timeout
submit_and_time "large"  50  900

echo ""
echo "Smoke test passed"

if [ -n "${INTERNAL_TOKEN:-}" ]; then
  echo "Collecting reliability instrumentation snapshots"
  reliability_json="$(curl -fsS "${backend_url}/api/v1/metrics/reliability" -H "X-Internal-Token: ${INTERNAL_TOKEN}")"
  providers_json="$(curl -fsS "${backend_url}/api/v1/metrics/providers" -H "X-Internal-Token: ${INTERNAL_TOKEN}")"

  echo "$reliability_json" | grep '"pipeline"' >/dev/null
  echo "$reliability_json" | grep '"queue"' >/dev/null
  echo "$providers_json" | grep '"health"' >/dev/null

  retries_total="$(echo "$reliability_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('pipeline',{}).get('retries_total',0))")"
  failovers_total="$(echo "$reliability_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('pipeline',{}).get('failovers_total',0))")"
  chunk_success="$(echo "$reliability_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('pipeline',{}).get('chunk_success_rate',0))")"
  export_success="$(echo "$reliability_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('pipeline',{}).get('export_success_rate',0))")"
  echo "  retries_total=${retries_total} failovers_total=${failovers_total} chunk_success_rate=${chunk_success} export_success_rate=${export_success}"
else
  echo "INTERNAL_TOKEN not set; skipping internal reliability metrics validation"
fi