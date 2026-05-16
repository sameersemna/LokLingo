#!/usr/bin/env bash
set -euo pipefail

HOSTNAME_TARGET="promaxgb10-6116"
FRONTEND_URL="http://${HOSTNAME_TARGET}:3000"
BACKEND_URL="http://${HOSTNAME_TARGET}:28080"
OCR_URL="http://${HOSTNAME_TARGET}:8000"
HEALTH_DASHBOARD_URL="${BACKEND_URL}/api/v1/health/dashboard"
READY_URL="${BACKEND_URL}/ready"

resolve_lan_ip() {
  local ip
  ip=$(ip route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "src") {print $(i+1); exit}}') || true
  if [[ -n "${ip:-}" ]]; then
    printf '%s\n' "$ip"
    return 0
  fi

  ip=$(hostname -I 2>/dev/null | awk '{print $1}') || true
  if [[ -n "${ip:-}" ]]; then
    printf '%s\n' "$ip"
    return 0
  fi

  printf 'unknown\n'
}

fetch_json() {
  local url="$1"
  curl -fsS --max-time 8 "$url"
}

python_probe() {
  local label="$1"
  local json_body="$2"
  JSON_BODY="$json_body" python3 - "$label" <<'PY'
from __future__ import annotations
import json
import os
import sys

label = sys.argv[1]
body = json.loads(os.environ['JSON_BODY'])
print(f"[{label}] status={body.get('status', 'unknown')}")
for name, dep in body.get('dependencies', {}).items():
    status = dep.get('status', 'unknown')
    detail = dep.get('details') or {}
    extra = []
    if isinstance(detail, dict):
        for key in ('queue_depth', 'stuck_jobs', 'retry_backlog', 'dead_letter_count', 'latency_ms'):
            if key in detail:
                extra.append(f"{key}={detail[key]}")
    suffix = f" ({', '.join(extra)})" if extra else ''
    print(f"  - {name}: {status}{suffix}")
PY
}

status=0
lan_ip="$(resolve_lan_ip)"
printf 'Detected LAN IP: %s\n' "$lan_ip"
printf '\n'

for url in "$FRONTEND_URL" "$READY_URL" "$OCR_URL/health"; do
  if curl -fsS --max-time 8 "$url" >/dev/null; then
    printf '[OK] %s\n' "$url"
  else
    printf '[FAIL] %s\n' "$url"
    status=1
  fi
done

printf '\nHealth dashboard:\n'
if dashboard_json="$(fetch_json "$HEALTH_DASHBOARD_URL")"; then
  python_probe "dashboard" "$dashboard_json"
else
  printf '[FAIL] %s\n' "$HEALTH_DASHBOARD_URL"
  status=1
fi

if [[ "$lan_ip" != "unknown" ]]; then
  printf '\nLAN IP checks:\n'
  for url in "http://${lan_ip}:3000" "http://${lan_ip}:28080/health" "http://${lan_ip}:8000/health"; do
    if curl -fsS --max-time 8 "$url" >/dev/null; then
      printf '[OK] %s\n' "$url"
    else
      printf '[FAIL] %s\n' "$url"
      status=1
    fi
  done
fi

if [[ "$status" -eq 0 ]]; then
  printf '\nHealth check PASSED\n'
else
  printf '\nHealth check FAILED\n'
  exit 1
fi
