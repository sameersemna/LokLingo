#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

JSON_OUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --json-out)
      JSON_OUT="${2:-}"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

BASE_URL="${LOKLINGO_BASE_URL:-http://localhost:28080}"
INTERNAL_TOKEN="${INTERNAL_TOKEN:-dev-internal-token}"
OBS_SMOKE_MAX_SEC="${OBS_SMOKE_MAX_SEC:-90}"
DEV_SMOKE_MAX_SEC="${DEV_SMOKE_MAX_SEC:-600}"
API_CHECK_MAX_SEC="${API_CHECK_MAX_SEC:-30}"
TIMEOUT_STORM_MAX_SEC="${TIMEOUT_STORM_MAX_SEC:-180}"
STORM_JOB_COUNT="${STORM_JOB_COUNT:-20}"
STORM_RECOVERY_MAX_SEC="${STORM_RECOVERY_MAX_SEC:-120}"
STORM_RECOVERY_DEPTH_THRESHOLD="${STORM_RECOVERY_DEPTH_THRESHOLD:-6}"
STORM_TIMEOUT_DELTA_MAX="${STORM_TIMEOUT_DELTA_MAX:-8}"
STORM_RETRY_DELTA_MAX="${STORM_RETRY_DELTA_MAX:-80}"
LIFECYCLE_VERIFY_MAX_SEC="${LIFECYCLE_VERIFY_MAX_SEC:-20}"
SMOKE_SUITE_MODE="${SMOKE_SUITE_MODE:-full}"

SUITE_NAMES=()
SUITE_STATUS=()
SUITE_DURATION=()
SUITE_NOTE=()
LAST_CORRELATION_ID=""
LAST_JOB_ID=""
LAST_LIFECYCLE_EVENT_COUNT="0"
LAST_LINEAGE_VERIFIED="false"

record_suite() {
  SUITE_NAMES+=("$1")
  SUITE_STATUS+=("$2")
  SUITE_DURATION+=("$3")
  SUITE_NOTE+=("$4")
}

run_timed_suite() {
  local name="$1"
  local max_sec="$2"
  shift 2

  local start end duration
  start=$(date +%s)
  if "$@"; then
    end=$(date +%s)
    duration=$((end - start))
    if (( duration > max_sec )); then
      record_suite "$name" "regression" "$duration" "duration ${duration}s exceeded ${max_sec}s budget"
      return 2
    fi
    record_suite "$name" "passed" "$duration" "ok"
    return 0
  fi

  local rc=$?
  end=$(date +%s)
  duration=$((end - start))
  if (( rc == 2 )); then
    record_suite "$name" "regression" "$duration" "threshold breach"
    return 2
  fi
  record_suite "$name" "failed" "$duration" "command failed"
  return 1
}

api_health_check() {
  local providers_url="${BASE_URL}/api/v1/metrics/providers"
  local provider_health_url="${BASE_URL}/api/v1/metrics/providers/health"
  local ocr_url="${BASE_URL}/api/v1/metrics/ocr?window=1h"
  local lifecycle_url="${BASE_URL}/api/v1/metrics/lifecycle/events?limit=1"

  curl -fsS "$providers_url" -H "X-Internal-Token: ${INTERNAL_TOKEN}" >/dev/null
  curl -fsS "$provider_health_url" -H "X-Internal-Token: ${INTERNAL_TOKEN}" >/dev/null
  curl -fsS "$ocr_url" -H "X-Internal-Token: ${INTERNAL_TOKEN}" >/dev/null
  curl -fsS "$lifecycle_url" -H "X-Internal-Token: ${INTERNAL_TOKEN}" >/dev/null
}

prom_metric_value() {
  local metric_name="$1"
  local prom_url="${BASE_URL}/api/v1/metrics/prometheus"
  curl -fsS "$prom_url" -H "X-Internal-Token: ${INTERNAL_TOKEN}" | awk -v m="$metric_name" '$1 == m { print $2; found=1; exit } END { if (!found) print 0 }'
}

extract_enqueue_identifiers() {
  python3 -c 'import json, sys; payload = json.load(sys.stdin); print(payload.get("correlation_id", "")); print(payload.get("job_id", ""))'
}

fetch_lifecycle_event_count() {
  local correlation_id="$1"
  local lifecycle_url="${BASE_URL}/api/v1/metrics/lifecycle/events?correlation_id=${correlation_id}&limit=5"

  curl -fsS "$lifecycle_url" -H "X-Internal-Token: ${INTERNAL_TOKEN}" | python3 -c 'import json, sys; payload = json.load(sys.stdin); print(int(payload.get("count", 0) or 0))'
}

verify_lifecycle_lineage() {
  local correlation_id="$1"
  local waited=0
  local count=0

  if [[ -z "$correlation_id" ]]; then
    LAST_LIFECYCLE_EVENT_COUNT="0"
    LAST_LINEAGE_VERIFIED="false"
    return 1
  fi

  while (( waited < LIFECYCLE_VERIFY_MAX_SEC )); do
    count="$(fetch_lifecycle_event_count "$correlation_id" 2>/dev/null || echo 0)"
    count="${count%%.*}"
    if (( count > 0 )); then
      LAST_LIFECYCLE_EVENT_COUNT="$count"
      LAST_LINEAGE_VERIFIED="true"
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done

  LAST_LIFECYCLE_EVENT_COUNT="$count"
  LAST_LINEAGE_VERIFIED="false"
  return 2
}

simulate_timeout_storm() {
  local submit_url="${BASE_URL}/api/v1/jobs"
  local started_timeouts started_retries
  local final_timeouts final_retries timeout_delta retry_delta
  local queue_depth=0
  local submit_response parsed correlation_id job_id

  started_timeouts="$(prom_metric_value "loklingo_pipeline_timeouts_total")"
  started_retries="$(prom_metric_value "loklingo_pipeline_retries_total")"

  for i in $(seq 1 "$STORM_JOB_COUNT"); do
    submit_response="$(curl -fsS -X POST "$submit_url" \
      -H "Content-Type: application/json" \
      -d "{\"type\":\"translate_text\",\"text\":\"reliability timeout storm probe ${i}\",\"source\":\"en\",\"target\":\"es\"}" \
    )" || return 1

    parsed="$(printf '%s' "$submit_response" | extract_enqueue_identifiers)" || return 1

    correlation_id="$(printf '%s\n' "$parsed" | sed -n '1p')"
    job_id="$(printf '%s\n' "$parsed" | sed -n '2p')"
    if [[ -n "$correlation_id" ]]; then
      LAST_CORRELATION_ID="$correlation_id"
    fi
    if [[ -n "$job_id" ]]; then
      LAST_JOB_ID="$job_id"
    fi
  done

  local waited=0
  while (( waited < STORM_RECOVERY_MAX_SEC )); do
    queue_depth="$(prom_metric_value "loklingo_queue_depth")"
    queue_depth="${queue_depth%%.*}"
    if (( queue_depth <= STORM_RECOVERY_DEPTH_THRESHOLD )); then
      break
    fi
    sleep 2
    waited=$((waited + 2))
  done

  if (( queue_depth > STORM_RECOVERY_DEPTH_THRESHOLD )); then
    echo "queue depth recovery threshold exceeded: depth=${queue_depth} threshold=${STORM_RECOVERY_DEPTH_THRESHOLD}" >&2
    return 2
  fi

  final_timeouts="$(prom_metric_value "loklingo_pipeline_timeouts_total")"
  final_retries="$(prom_metric_value "loklingo_pipeline_retries_total")"
  final_timeouts="${final_timeouts%%.*}"
  final_retries="${final_retries%%.*}"
  started_timeouts="${started_timeouts%%.*}"
  started_retries="${started_retries%%.*}"

  timeout_delta=$((final_timeouts - started_timeouts))
  retry_delta=$((final_retries - started_retries))
  if (( timeout_delta > STORM_TIMEOUT_DELTA_MAX )); then
    echo "timeout delta threshold exceeded: delta=${timeout_delta} max=${STORM_TIMEOUT_DELTA_MAX}" >&2
    return 2
  fi
  if (( retry_delta > STORM_RETRY_DELTA_MAX )); then
    echo "retry delta threshold exceeded: delta=${retry_delta} max=${STORM_RETRY_DELTA_MAX}" >&2
    return 2
  fi

  if ! verify_lifecycle_lineage "$LAST_CORRELATION_ID"; then
    echo "lifecycle lineage not visible for correlation id: ${LAST_CORRELATION_ID}" >&2
    return 2
  fi

  return 0
}

overall_status="passed"
if [[ "$SMOKE_SUITE_MODE" == "timeout_storm_only" ]]; then
  if ! run_timed_suite "timeout_storm_guard" "$TIMEOUT_STORM_MAX_SEC" simulate_timeout_storm; then
    overall_status="failed"
  fi
else
  if ! run_timed_suite "observability" "$OBS_SMOKE_MAX_SEC" sh guide/smoke-observability.sh; then
    overall_status="failed"
  fi
  if ! run_timed_suite "golden_path" "$DEV_SMOKE_MAX_SEC" sh guide/smoke-dev.sh; then
    overall_status="failed"
  fi
  if ! run_timed_suite "api_compare_flow" "$API_CHECK_MAX_SEC" api_health_check; then
    overall_status="failed"
  fi
  if ! run_timed_suite "timeout_storm_guard" "$TIMEOUT_STORM_MAX_SEC" simulate_timeout_storm; then
    overall_status="failed"
  fi
fi

for status in "${SUITE_STATUS[@]}"; do
  if [[ "$status" == "failed" ]]; then
    overall_status="failed"
  elif [[ "$status" == "regression" && "$overall_status" != "failed" ]]; then
    overall_status="regression"
  fi
done

report_file="${JSON_OUT:-guide/reliability-smoke-report.json}"
mkdir -p "$(dirname "$report_file")"

{
  echo "{"
  echo "  \"generated_at\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"," 
  echo "  \"correlation_id\": \"${LAST_CORRELATION_ID}\"," 
  echo "  \"last_correlation_id\": \"${LAST_CORRELATION_ID}\"," 
  echo "  \"job_id\": \"${LAST_JOB_ID}\"," 
  echo "  \"last_job_id\": \"${LAST_JOB_ID}\"," 
  echo "  \"lifecycle_event_count\": ${LAST_LIFECYCLE_EVENT_COUNT}," 
  echo "  \"lineage_verified\": ${LAST_LINEAGE_VERIFIED}," 
  echo "  \"overall_status\": \"${overall_status}\"," 
  echo "  \"suites\": ["
  for i in "${!SUITE_NAMES[@]}"; do
    comma=","
    if (( i == ${#SUITE_NAMES[@]} - 1 )); then
      comma=""
    fi
    echo "    {\"name\": \"${SUITE_NAMES[$i]}\", \"status\": \"${SUITE_STATUS[$i]}\", \"duration_seconds\": ${SUITE_DURATION[$i]}, \"note\": \"${SUITE_NOTE[$i]}\"}${comma}"
  done
  echo "  ]"
  echo "}"
} >"$report_file"

cat "$report_file"

if [[ "$overall_status" == "failed" || "$overall_status" == "regression" ]]; then
  exit 1
fi
