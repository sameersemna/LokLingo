#!/usr/bin/env bash
set -euo pipefail

HOSTNAME_TARGET="promaxgb10-6116"
BACKEND_URL="http://${HOSTNAME_TARGET}:28080"
DASHBOARD_URL="${BACKEND_URL}/api/v1/health/dashboard"
METRICS_URL="${BACKEND_URL}/api/v1/metrics/prometheus"

fetch_json() {
  curl -fsS --max-time 8 "$1"
}

fetch_text() {
  curl -fsS --max-time 8 "$1"
}

printf 'LokLingo LAN monitor\n'
printf '=====================\n\n'

if dashboard_json="$(fetch_json "$DASHBOARD_URL")"; then
  JSON_BODY="$dashboard_json" python3 - <<'PY'
from __future__ import annotations
import json
import os

body = json.loads(os.environ['JSON_BODY'])
print(f"Dashboard status: {body.get('status', 'unknown')}")
print(f"Uptime seconds: {body.get('uptime_seconds', 0)}")
for name, dep in body.get('dependencies', {}).items():
    detail = dep.get('details') or {}
    line = [f"{name}: {dep.get('status', 'unknown')}" ]
    if isinstance(detail, dict):
        for key in ('latency_ms', 'queue_depth', 'stuck_jobs', 'retry_backlog', 'dead_letter_count'):
            if key in detail:
                line.append(f"{key}={detail[key]}")
    print('  ' + ', '.join(line))
PY
else
  printf 'Dashboard unavailable: %s\n' "$DASHBOARD_URL"
fi

printf '\nKey metrics:\n'
if metrics_text="$(fetch_text "$METRICS_URL")"; then
  printf '%s\n' "$metrics_text" | awk '
    /loklingo_pipeline_ocr_latency_avg_ms/ {print "  OCR latency avg ms: "$2}
    /loklingo_queue_depth/ {print "  Queue depth: "$2}
    /loklingo_queue_dead_letter_volume/ {print "  Failed jobs (dead letters): "$2}
    /loklingo_queue_stuck_jobs/ {print "  Stuck jobs: "$2}
  '
else
  printf 'Unable to fetch Prometheus metrics\n'
fi

printf '\nSystem usage:\n'
if command -v docker >/dev/null 2>&1; then
  docker stats --no-stream --format '  {{.Name}} CPU={{.CPUPerc}} MEM={{.MemUsage}}' loklingo-backend loklingo-ocr loklingo-ollama loklingo-frontend 2>/dev/null || true
fi

if command -v nvidia-smi >/dev/null 2>&1; then
  printf '\nGPU usage:\n'
  nvidia-smi --query-gpu=name,utilization.gpu,memory.used,memory.total --format=csv,noheader 2>/dev/null || true
else
  printf '\nGPU usage: nvidia-smi not available\n'
fi
