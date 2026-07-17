#!/usr/bin/env bash
# LokLingo self-healing recovery script.
# Detects unhealthy dependencies and attempts automated recovery.
#
# Usage:
#   bash scripts/self-heal.sh              # diagnose + heal once
#   bash scripts/self-heal.sh --watch      # loop every 60s
#   bash scripts/self-heal.sh --dry-run    # report only
#
# Env:
#   LOKLINGO_BASE_URL   default http://localhost:28080
#   OCR_URL             default http://localhost:8000
#   FRONTEND_URL        default http://localhost:3000
#   INTERNAL_TOKEN      required for metrics/DLQ recovery actions
#   COMPOSE_FILES       compose -f args (default base+dev)
#   SELF_HEAL_INTERVAL  watch interval seconds (default 60)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Prefer already-exported env; else read from .env; else default.
_load_dotenv_port() {
  local key="$1" default="$2"
  local current
  current="$(printenv "$key" 2>/dev/null || true)"
  if [[ -n "${current:-}" ]]; then
    printf '%s\n' "$current"
    return 0
  fi
  if [[ -f .env ]]; then
    local val
    val="$(grep -E "^${key}=" .env 2>/dev/null | tail -n1 | cut -d= -f2- | tr -d '\r' || true)"
    if [[ -n "${val:-}" ]]; then
      printf '%s\n' "$val"
      return 0
    fi
  fi
  printf '%s\n' "$default"
}

BACKEND_HOST_PORT="$(_load_dotenv_port BACKEND_HOST_PORT 28080)"
OCR_HOST_PORT="$(_load_dotenv_port OCR_HOST_PORT 8000)"
FRONTEND_HOST_PORT="$(_load_dotenv_port FRONTEND_HOST_PORT 3000)"

BASE_URL="${LOKLINGO_BASE_URL:-http://localhost:${BACKEND_HOST_PORT}}"
OCR_URL="${OCR_URL:-http://localhost:${OCR_HOST_PORT}}"
FRONTEND_URL="${FRONTEND_URL:-http://localhost:${FRONTEND_HOST_PORT}}"
INTERNAL_TOKEN="${INTERNAL_TOKEN:-}"
if [[ -z "$INTERNAL_TOKEN" && -f .env ]]; then
  INTERNAL_TOKEN="$(grep -E '^INTERNAL_TOKEN=' .env 2>/dev/null | tail -n1 | cut -d= -f2- | tr -d '\r' || true)"
fi
COMPOSE_FILES="${COMPOSE_FILES:--f docker-compose.yml -f docker-compose.dev.yml}"
INTERVAL="${SELF_HEAL_INTERVAL:-60}"
DRY_RUN=0
WATCH=0

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --watch) WATCH=1 ;;
    -h|--help)
      sed -n '2,20p' "$0"
      exit 0
      ;;
  esac
done

log() { printf '[self-heal %s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
ok() { log "OK  $*"; }
warn() { log "WARN $*"; }
fail() { log "FAIL $*"; }

compose() {
  # shellcheck disable=SC2086
  docker compose $COMPOSE_FILES "$@"
}

http_code() {
  local url="$1"
  curl -sS -o /dev/null -w '%{http_code}' --max-time 8 "$url" 2>/dev/null || printf '000'
}

json_get() {
  local url="$1"
  if [[ -n "$INTERNAL_TOKEN" ]]; then
    curl -fsS --max-time 10 -H "X-Internal-Token: ${INTERNAL_TOKEN}" "$url" 2>/dev/null || true
  else
    curl -fsS --max-time 10 "$url" 2>/dev/null || true
  fi
}

restart_service() {
  local svc="$1"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    warn "dry-run: would restart $svc"
    return 0
  fi
  log "restarting $svc"
  compose restart "$svc" || compose up -d "$svc" || true
}

heal_once() {
  local actions=0
  local backend_code ocr_code frontend_code ready_code

  backend_code="$(http_code "${BASE_URL}/health")"
  ocr_code="$(http_code "${OCR_URL}/health")"
  frontend_code="$(http_code "${FRONTEND_URL}/")"
  ready_code="$(http_code "${BASE_URL}/ready")"

  log "probes backend=${backend_code} ready=${ready_code} ocr=${ocr_code} frontend=${frontend_code}"

  if [[ "$backend_code" != "200" ]]; then
    fail "backend health not 200"
    restart_service loklingo-backend
    actions=$((actions + 1))
  else
    ok "backend health"
  fi

  if [[ "$ocr_code" != "200" ]]; then
    fail "OCR health not 200"
    restart_service loklingo-ocr
    # OCR often depends on Ollama for correction path
    restart_service loklingo-ollama
    actions=$((actions + 1))
  else
    ok "OCR health"
  fi

  # Accept 200 and common redirect codes; 404 often means wrong host port mapping.
  if [[ "$frontend_code" == "200" || "$frontend_code" == "301" || "$frontend_code" == "302" || "$frontend_code" == "304" ]]; then
    ok "frontend"
  elif [[ "$frontend_code" == "404" ]]; then
    warn "frontend returned 404 at ${FRONTEND_URL} — check FRONTEND_HOST_PORT / FRONTEND_URL (skipping restart)"
  else
    fail "frontend not healthy (code=${frontend_code})"
    restart_service loklingo-frontend
    actions=$((actions + 1))
  fi

  # Readiness degradation: inspect dependencies when possible
  if [[ "$ready_code" != "200" ]]; then
    warn "readiness degraded (code=${ready_code})"
    ready_body="$(json_get "${BASE_URL}/ready" || true)"
    if printf '%s' "$ready_body" | grep -qi 'redis'; then
      warn "redis dependency unhealthy — operator must fix REDIS_URL / Redis host (no local restart if external)"
    fi
    if printf '%s' "$ready_body" | grep -qi 'ocr'; then
      restart_service loklingo-ocr
      actions=$((actions + 1))
    fi
    if printf '%s' "$ready_body" | grep -qi 'litellm'; then
      warn "LiteLLM unreachable — check LITELLM_BASE_URL / external gateway"
    fi
  else
    ok "readiness"
  fi

  # Stuck jobs / DLQ recovery when internal token available
  if [[ -n "$INTERNAL_TOKEN" ]]; then
    prom="$(json_get "${BASE_URL}/api/v1/metrics/prometheus" || true)"
    stuck="$(printf '%s\n' "$prom" | awk '/^loklingo_queue_stuck_jobs / {print $2; exit}')"
    dead="$(printf '%s\n' "$prom" | awk '/^loklingo_queue_dead_letter_volume / {print $2; exit}')"
    depth="$(printf '%s\n' "$prom" | awk '/^loklingo_queue_depth / {print $2; exit}')"

    log "queue depth=${depth:-?} stuck=${stuck:-?} dead=${dead:-?}"

    # Stuck jobs: backend recover-stale runs on dequeue; bounce worker process via backend restart if stuck persists.
    if [[ -n "${stuck:-}" ]] && awk "BEGIN {exit !(${stuck}+0 >= 1)}"; then
      warn "stuck jobs detected (${stuck})"
      if [[ "$DRY_RUN" -eq 0 ]]; then
        # Soft recovery: hit dashboard to force operational visibility; restart backend to re-run RecoverStale loop.
        restart_service loklingo-backend
        actions=$((actions + 1))
      fi
    fi

    # Dead-letter auto-replay is intentionally opt-in via env (can amplify poison messages).
    if [[ "${SELF_HEAL_REPLAY_DEAD:-0}" == "1" && -n "${dead:-}" ]] && awk "BEGIN {exit !(${dead}+0 >= 1)}"; then
      warn "replaying dead-letter jobs (SELF_HEAL_REPLAY_DEAD=1)"
      if [[ "$DRY_RUN" -eq 0 ]]; then
        bash guide/ops/reliability-recovery.sh replay-dead-all 20 || true
        actions=$((actions + 1))
      fi
    fi
  else
    warn "INTERNAL_TOKEN unset — skipping metrics/DLQ recovery actions"
  fi

  # Container restart loop detection (POSIX-friendly; no process substitution).
  if command -v docker >/dev/null 2>&1; then
    _container_list="$(docker ps -a --filter 'name=loklingo-' --format '{{.Names}} {{.Status}}' 2>/dev/null || true)"
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      name="${line%% *}"
      status="${line#* }"
      if printf '%s' "$status" | grep -qiE 'Restarting|unhealthy'; then
        fail "container $name is $status"
        if [[ "$DRY_RUN" -eq 0 ]]; then
          docker restart "$name" >/dev/null 2>&1 || true
          actions=$((actions + 1))
        fi
      fi
    done <<EOF
${_container_list}
EOF
    unset _container_list
  fi

  if [[ "$actions" -eq 0 ]]; then
    ok "no healing actions required"
    return 0
  fi
  warn "performed ${actions} healing action(s)"
  return 0
}

if [[ "$WATCH" -eq 1 ]]; then
  log "watch mode every ${INTERVAL}s (dry_run=${DRY_RUN})"
  while true; do
    heal_once || true
    sleep "$INTERVAL"
  done
else
  heal_once
fi
