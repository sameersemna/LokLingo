#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../" && pwd)"
cd "$ROOT_DIR"

period="${1:-monthly}"
out_file="${2:-guide/governance/reviews/generated-${period}-reliability-review.md}"

case "$period" in
  monthly) window="30d" ;;
  quarterly) window="30d" ;;
  annual) window="30d" ;;
  *)
    echo "Usage: $0 <monthly|quarterly|annual> [output-file]" >&2
    exit 1
    ;;
esac

base_url="${LOKLINGO_BASE_URL:-http://localhost:28080}"
internal_token="${INTERNAL_TOKEN:-dev-internal-token}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

providers_json="$tmp_dir/providers.json"
ocr_json="$tmp_dir/ocr.json"
lifecycle_json="$tmp_dir/lifecycle.json"

curl -fsS "${base_url}/api/v1/metrics/providers" -H "X-Internal-Token: ${internal_token}" >"$providers_json" || echo '{}' >"$providers_json"
curl -fsS "${base_url}/api/v1/metrics/ocr?window=${window}" -H "X-Internal-Token: ${internal_token}" >"$ocr_json" || echo '{}' >"$ocr_json"
curl -fsS "${base_url}/api/v1/metrics/lifecycle/events?limit=200" -H "X-Internal-Token: ${internal_token}" >"$lifecycle_json" || echo '{"events":[]}' >"$lifecycle_json"

python3 - "$period" "$out_file" "$providers_json" "$ocr_json" "$lifecycle_json" <<'PYEOF'
import json
import sys
from collections import Counter
from datetime import datetime, timezone

period = sys.argv[1]
out_file = sys.argv[2]
providers_path = sys.argv[3]
ocr_path = sys.argv[4]
lifecycle_path = sys.argv[5]


def load_json(path, fallback):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except Exception:
        return fallback

providers = load_json(providers_path, {})
ocr = load_json(ocr_path, {})
lifecycle = load_json(lifecycle_path, {"events": []})

health = providers.get("health", []) or []
provider_rows = providers.get("providers", []) or []

worst_provider = "n/a"
worst_success_rate = 1.0
for item in health:
    sr = float(item.get("success_rate", 1.0) or 0.0)
    if sr < worst_success_rate:
        worst_success_rate = sr
        worst_provider = str(item.get("provider", "unknown"))

retries = 0
timeouts = 0
failovers = 0
for p in provider_rows:
    retries += int(p.get("retry_total", 0) or 0)
    timeouts += int(p.get("timeout_total", 0) or 0)
    failovers += int(p.get("failover_total", 0) or 0)

window_name = ((ocr.get("window") or {}).get("name")) or "last_24h"
ocr_events = int(ocr.get("events_total", 0) or 0)
ocr_success = float(ocr.get("success_rate_pct", 0.0) or 0.0)

lifecycle_events = lifecycle.get("events", []) or []
event_counter = Counter()
for ev in lifecycle_events:
    name = ev.get("event")
    if name:
        event_counter[str(name)] += 1

incident_top = event_counter.most_common(6)
incident_lines = [f"- {name}: {count}" for name, count in incident_top] or ["- No lifecycle events were available in this run."]

now = datetime.now(timezone.utc).strftime("%Y-%m-%d")
next_review = {
    "monthly": "next month",
    "quarterly": "next quarter",
    "annual": "next year",
}[period]

content = f"""# {period.title()} Reliability Review (Generated)

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: {now}
Next Review: {next_review}
Operational Scope: OCR, translation, rendering, queueing, export, incident operations

## Trend Summary

- OCR window analyzed: `{window_name}`
- OCR events observed: `{ocr_events}`
- OCR success rate: `{ocr_success:.2f}%`
- Provider retries observed: `{retries}`
- Provider timeout events observed: `{timeouts}`
- Provider failover events observed: `{failovers}`
- Most unstable provider by success rate: `{worst_provider}`

## Incident Summary

{chr(10).join(incident_lines)}

## Retry and Failure Trends

- Compare `retry_total` and `timeout_total` against the prior {period} archive snapshot.
- Confirm dead-letter trends do not increase faster than successful completion volume.
- Validate chunk checkpoint `persist_failure_total` remains below alerting threshold.

## Provider Instability Report

- Primary watch signal: providers with success rate below 95% in rolling window.
- Secondary watch signal: failover_rate spikes with increasing timeout reasons.
- Action: tune provider ordering and temporary retry budgets under overload periods.

## Action Items

- [ ] Confirm incident follow-ups and ownership routing are current.
- [ ] Confirm retention/archive tasks completed for this {period} review window.
- [ ] Confirm governance pointers and changelog entries are up to date.
"""

with open(out_file, "w", encoding="utf-8") as f:
    f.write(content)

print(out_file)
PYEOF
