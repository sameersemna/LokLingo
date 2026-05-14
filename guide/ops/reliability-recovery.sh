#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${LOKLINGO_BASE_URL:-http://localhost:28080}"
INTERNAL_TOKEN="${INTERNAL_TOKEN:-dev-internal-token}"

usage() {
  cat <<'EOF'
Usage:
  reliability-recovery.sh list-dead [limit]
  reliability-recovery.sh replay-dead <job-id>
  reliability-recovery.sh replay-dead-all [limit]
  reliability-recovery.sh queue-status
  reliability-recovery.sh provider-history
  reliability-recovery.sh provider-policy-snapshot [out-file]
  reliability-recovery.sh export-incident-snapshot <correlation-id> [out-file]
  reliability-recovery.sh validate-incident-snapshot <incident-json>
  reliability-recovery.sh generate-incident-brief <incident-json> [out-file]
EOF
}

call_api() {
  local method="$1"
  local url="$2"
  shift 2
  curl -fsS -X "$method" "$url" -H "X-Internal-Token: ${INTERNAL_TOKEN}" "$@"
}

build_provider_policy_snapshot() {
  local out_file="$1"
  local provider_health_tmp
  local prometheus_tmp

  provider_health_tmp="$(mktemp)"
  prometheus_tmp="$(mktemp)"

  call_api GET "${BASE_URL}/api/v1/metrics/providers/health" >"$provider_health_tmp"
  call_api GET "${BASE_URL}/api/v1/metrics/prometheus" >"$prometheus_tmp"

  python3 - "$out_file" "$provider_health_tmp" "$prometheus_tmp" <<'PYEOF'
import json
import re
import sys
from datetime import datetime, timezone

out_file, provider_health_path, prom_path = sys.argv[1:]

with open(provider_health_path, 'r', encoding='utf-8') as f:
  provider_health = json.load(f)

signals = {
  'loklingo_degraded_mode_total': None,
  'loklingo_degraded_render_fallback_total': None,
  'loklingo_degraded_ocr_low_confidence_total': None,
  'loklingo_adaptive_concurrency_reduce_total': None,
  'loklingo_adaptive_concurrency_boost_total': None,
  'loklingo_adaptive_concurrency_clamp_total': None,
  'loklingo_queue_depth': None,
  'loklingo_queue_retry_backlog': None,
}
policy_scores = {}

metric_re = re.compile(r'^(?P<metric>[a-zA-Z_:][a-zA-Z0-9_:]*)\\s+(?P<value>[-+]?\\d+(?:\\.\\d+)?)$')
policy_re = re.compile(r'^loklingo_provider_policy_score\\{provider="(?P<provider>[^\"]+)"\\}\\s+(?P<value>[-+]?\\d+(?:\\.\\d+)?)$')

with open(prom_path, 'r', encoding='utf-8') as f:
  for raw in f:
    line = raw.strip()
    if not line or line.startswith('#'):
      continue
    m_policy = policy_re.match(line)
    if m_policy:
      policy_scores[m_policy.group('provider')] = float(m_policy.group('value'))
      continue
    m = metric_re.match(line)
    if not m:
      continue
    metric = m.group('metric')
    if metric in signals:
      signals[metric] = float(m.group('value'))

payload = {
  'generated_at': datetime.now(timezone.utc).isoformat(),
  'provider_health_snapshot': provider_health,
  'degraded_and_queue_signals': signals,
  'provider_policy_scores': policy_scores,
}

with open(out_file, 'w', encoding='utf-8') as f:
  json.dump(payload, f, indent=2)

print(out_file)
PYEOF

  rm -f "$provider_health_tmp" "$prometheus_tmp"
}

cmd="${1:-}"
if [[ -z "$cmd" ]]; then
  usage
  exit 1
fi

case "$cmd" in
  list-dead)
    limit="${2:-50}"
    call_api GET "${BASE_URL}/api/v1/jobs/dead?limit=${limit}"
    ;;

  replay-dead)
    job_id="${2:-}"
    if [[ -z "$job_id" ]]; then
      usage
      exit 1
    fi
    call_api POST "${BASE_URL}/api/v1/jobs/${job_id}/replay"
    ;;

  replay-dead-all)
    limit="${2:-50}"
    tmp_json="$(mktemp)"
    trap 'rm -f "$tmp_json"' EXIT
    call_api GET "${BASE_URL}/api/v1/jobs/dead?limit=${limit}" >"$tmp_json"

    python3 - "$tmp_json" <<'PYEOF'
import json
import sys

path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as f:
    payload = json.load(f)

jobs = payload.get('jobs', []) or []
for item in jobs:
    jid = item.get('job_id')
    if jid:
        print(jid)
PYEOF
    while IFS= read -r jid; do
      [[ -z "$jid" ]] && continue
      echo "Replaying dead-letter job: $jid"
      call_api POST "${BASE_URL}/api/v1/jobs/${jid}/replay" >/dev/null || true
    done
    ;;

  queue-status)
    call_api GET "${BASE_URL}/api/v1/metrics/prometheus"
    ;;

  provider-history)
    call_api GET "${BASE_URL}/api/v1/metrics/providers"
    ;;

  provider-policy-snapshot)
    out_file="${2:-guide/incidents/provider-policy-snapshot-$(date -u +%Y%m%dT%H%M%SZ).json}"
    mkdir -p "$(dirname "$out_file")"

    build_provider_policy_snapshot "$out_file"
    ;;

  export-incident-snapshot)
    correlation_id="${2:-}"
    out_file="${3:-guide/incidents/incident-snapshot-${correlation_id}-$(date -u +%Y%m%dT%H%M%SZ).json}"
    if [[ -z "$correlation_id" ]]; then
      usage
      exit 1
    fi

    mkdir -p "$(dirname "$out_file")"
    lifecycle_tmp="$(mktemp)"
    providers_tmp="$(mktemp)"
    ocr_tmp="$(mktemp)"
    provider_policy_tmp="$(mktemp)"
    trap 'rm -f "$lifecycle_tmp" "$providers_tmp" "$ocr_tmp" "$provider_policy_tmp"' EXIT

    build_provider_policy_snapshot "$provider_policy_tmp"
    call_api GET "${BASE_URL}/api/v1/metrics/lifecycle/events?correlation_id=${correlation_id}&limit=500" >"$lifecycle_tmp"
    call_api GET "${BASE_URL}/api/v1/metrics/providers" >"$providers_tmp"
    call_api GET "${BASE_URL}/api/v1/metrics/ocr?window=24h" >"$ocr_tmp"

    python3 - "$out_file" "$correlation_id" "$lifecycle_tmp" "$providers_tmp" "$ocr_tmp" "$provider_policy_tmp" <<'PYEOF'
import json
import sys
from datetime import datetime, timezone
from collections import Counter

out_file, correlation_id, lifecycle_path, providers_path, ocr_path, provider_policy_path = sys.argv[1:]


def load(path):
    with open(path, 'r', encoding='utf-8') as f:
        return json.load(f)


def first_path(payload, candidates, default=None):
    for path in candidates:
        current = payload
        ok = True
        for key in path:
            if not isinstance(current, dict) or key not in current:
                ok = False
                break
            current = current[key]
        if ok:
            return current
    return default


def lifecycle_events_list(payload):
    if isinstance(payload, dict):
        events = payload.get('events')
        if isinstance(events, list):
            return events
    if isinstance(payload, list):
        return payload
    return []


def top_policy_drop(policy_scores):
    if not isinstance(policy_scores, dict) or not policy_scores:
        return None
    items = [(k, v) for k, v in policy_scores.items() if isinstance(v, (int, float))]
    if not items:
        return None
    provider, score = min(items, key=lambda x: x[1])
    return {
        'provider': provider,
        'score': round(float(score), 4),
    }


def build_derived_summary(lifecycle_payload, provider_metrics, ocr_metrics, provider_policy_snapshot):
    events = lifecycle_events_list(lifecycle_payload)
    stage_counts = Counter()
    for event in events:
        if not isinstance(event, dict):
            continue
        stage = event.get('stage') or event.get('event') or 'unknown'
        stage_counts[str(stage)] += 1

    signals = {}
    policy_scores = {}
    if isinstance(provider_policy_snapshot, dict):
        signals = provider_policy_snapshot.get('degraded_and_queue_signals') or {}
        policy_scores = provider_policy_snapshot.get('provider_policy_scores') or {}

    provider_timeouts_total = first_path(
        provider_metrics,
        [
            ('reliability', 'litellm', 'provider_timeouts_total'),
            ('reliability_windowed', 'reliability', 'litellm', 'provider_timeouts_total'),
            ('litellm', 'provider_timeouts_total'),
        ],
    )
    provider_retries_total = first_path(
        provider_metrics,
        [
            ('reliability', 'litellm', 'retry_attempts_total'),
            ('reliability_windowed', 'reliability', 'litellm', 'retry_attempts_total'),
            ('litellm', 'retry_attempts_total'),
        ],
    )
    ocr_low_confidence_total = first_path(
        ocr_metrics,
        [
            ('reliability', 'ocr', 'response_rejected_total'),
            ('reliability_windowed', 'reliability', 'ocr', 'response_rejected_total'),
            ('ocr', 'response_rejected_total'),
        ],
    )

    queue_depth = signals.get('loklingo_queue_depth')
    retry_backlog = signals.get('loklingo_queue_retry_backlog')
    degraded_total = signals.get('loklingo_degraded_mode_total')
    render_fallback_total = signals.get('loklingo_degraded_render_fallback_total')

    severity = 'normal'
    reasons = []
    if isinstance(queue_depth, (int, float)) and queue_depth >= 1000:
        severity = 'critical'
        reasons.append('queue_depth>=1000')
    elif isinstance(queue_depth, (int, float)) and queue_depth >= 500:
        if severity != 'critical':
            severity = 'warning'
        reasons.append('queue_depth>=500')

    if isinstance(retry_backlog, (int, float)) and retry_backlog >= 1000:
        severity = 'critical'
        reasons.append('retry_backlog>=1000')
    elif isinstance(retry_backlog, (int, float)) and retry_backlog >= 500:
        if severity != 'critical':
            severity = 'warning'
        reasons.append('retry_backlog>=500')

    if isinstance(provider_timeouts_total, (int, float)) and isinstance(provider_retries_total, (int, float)):
        if provider_timeouts_total > 8 and provider_retries_total > 80:
            severity = 'critical'
            reasons.append('timeout_storm_guard_triggered')

    if isinstance(degraded_total, (int, float)) and degraded_total > 0 and severity == 'normal':
        severity = 'warning'
        reasons.append('degraded_mode_events_present')

    if isinstance(render_fallback_total, (int, float)) and render_fallback_total > 0 and severity == 'normal':
        severity = 'warning'
        reasons.append('render_fallback_events_present')

    return {
        'lifecycle_event_count': len(events),
        'lifecycle_stage_counts': dict(stage_counts),
        'queue_depth': queue_depth,
        'retry_backlog': retry_backlog,
        'degraded_total': degraded_total,
        'render_fallback_total': render_fallback_total,
        'ocr_low_confidence_total': signals.get('loklingo_degraded_ocr_low_confidence_total'),
        'provider_timeouts_total': provider_timeouts_total,
        'provider_retries_total': provider_retries_total,
        'ocr_rejected_total': ocr_low_confidence_total,
        'lowest_provider_policy': top_policy_drop(policy_scores),
        'triage_severity_hint': severity,
        'triage_trigger_reasons': reasons,
    }

provider_policy_snapshot = load(provider_policy_path)
payload = {
    'generated_at': datetime.now(timezone.utc).isoformat(),
    'correlation_id': correlation_id,
    'lifecycle_events': load(lifecycle_path),
    'provider_metrics': load(providers_path),
    'ocr_metrics': load(ocr_path),
    'provider_policy_snapshot': provider_policy_snapshot,
}
payload['derived_summary'] = build_derived_summary(
    payload['lifecycle_events'],
    payload['provider_metrics'],
    payload['ocr_metrics'],
    provider_policy_snapshot,
  )

with open(out_file, 'w', encoding='utf-8') as f:
    json.dump(payload, f, indent=2)

print(out_file)
PYEOF
    ;;

  validate-incident-snapshot)
    incident_json="${2:-}"
    if [[ -z "$incident_json" ]]; then
      usage
      exit 1
    fi
    python3 guide/ops/validate-incident-snapshot.py "$incident_json"
    ;;

  generate-incident-brief)
    incident_json="${2:-}"
    out_file="${3:-guide/incidents/incident-brief-$(date -u +%Y%m%dT%H%M%SZ).md}"
    if [[ -z "$incident_json" ]]; then
      usage
      exit 1
    fi
    mkdir -p "$(dirname "$out_file")"
    python3 guide/ops/generate-incident-brief.py "$incident_json" "$out_file"
    ;;

  *)
    usage
    exit 1
    ;;
esac
