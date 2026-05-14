#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../" && pwd)"
cd "$ROOT_DIR"

smoke_only=0
if [[ "${1:-}" == "--smoke" ]]; then
	smoke_only=1
	shift
fi

log_dir="${1:-guide/governance/reviews/test-logs-preflight}"

# Match scheduled reliability workflow defaults when local env vars are unset.
export INTERNAL_TOKEN="${INTERNAL_TOKEN:-dev-internal-token}"
export LOKLINGO_BASE_URL="${LOKLINGO_BASE_URL:-http://localhost:28080}"
export RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS="${RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS:-12}"

echo "preflight env:"
echo "  INTERNAL_TOKEN=<set>"
echo "  LOKLINGO_BASE_URL=${LOKLINGO_BASE_URL}"
echo "  RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS=${RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS}"

if (( smoke_only == 1 )); then
	echo "preflight smoke mode complete"
	exit 0
fi

echo "running weekly reliability local stack..."
bash guide/governance/reviews/run-weekly-reliability-local.sh "$log_dir"

echo "weekly reliability preflight complete"
