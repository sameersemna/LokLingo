#!/usr/bin/env bash
set -euo pipefail

HOSTNAME_TARGET="promaxgb10-6116"
FRONTEND_URL="http://${HOSTNAME_TARGET}:3000"
BACKEND_URL="http://${HOSTNAME_TARGET}:28080"
OCR_URL="http://${HOSTNAME_TARGET}:8000"

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

check_http() {
  local label="$1"
  local url="$2"
  if curl -fsS --max-time 5 "$url" >/dev/null; then
    printf '[OK] %s reachable: %s\n' "$label" "$url"
  else
    printf '[FAIL] %s unreachable: %s\n' "$label" "$url"
    return 1
  fi
}

check_health_json() {
  local label="$1"
  local url="$2"
  local expected_fragment="$3"
  local body

  if ! body="$(curl -fsS --max-time 5 "$url")"; then
    printf '[FAIL] %s unreachable: %s\n' "$label" "$url"
    return 1
  fi

  if printf '%s' "$body" | grep -F "$expected_fragment" >/dev/null; then
    printf '[OK] %s healthy: %s\n' "$label" "$url"
  else
    printf '[FAIL] %s unexpected response: %s\n' "$label" "$url"
    printf '       body: %s\n' "$body"
    return 1
  fi
}

lan_ip="$(resolve_lan_ip)"
printf 'Detected LAN IP: %s\n' "$lan_ip"
printf '\n'

printf 'Hostname resolution check for %s:\n' "$HOSTNAME_TARGET"
if resolved_hosts=$(getent hosts "$HOSTNAME_TARGET"); then
  printf '[OK] Hostname resolves:\n%s\n' "$resolved_hosts"
else
  printf '[FAIL] Hostname does not resolve from this machine\n'
fi
printf '\n'

status=0
check_http "Frontend" "${FRONTEND_URL}" || status=1
check_health_json "Backend health" "${BACKEND_URL}/health" '"service":"loklingo-backend"' || status=1
check_health_json "OCR health" "${OCR_URL}/health" '"service":"loklingo-ocr"' || status=1
printf '\n'

if [[ "$lan_ip" != "unknown" ]]; then
  check_http "Frontend (LAN IP)" "http://${lan_ip}:3000" || status=1
  check_health_json "Backend health (LAN IP)" "http://${lan_ip}:28080/health" '"service":"loklingo-backend"' || status=1
  check_health_json "OCR health (LAN IP)" "http://${lan_ip}:8000/health" '"service":"loklingo-ocr"' || status=1
  printf '\n'
fi

printf 'Ready-to-open LAN URLs:\n'
printf '  %s\n' "$FRONTEND_URL"
printf '  %s\n' "$BACKEND_URL"
printf '  %s\n' "$OCR_URL"
printf '\n'

if [[ "$status" -eq 0 ]]; then
  printf 'LAN access check PASSED\n'
else
  printf 'LAN access check FAILED\n'
  exit 1
fi
