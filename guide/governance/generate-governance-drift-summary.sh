#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DRIFT_ROOT="$ROOT_DIR/archive/drift"

latest_report="$(find "$DRIFT_ROOT" -type f -name '*_drift-report.md' 2>/dev/null | sort | tail -n 1 || true)"
if [[ -z "$latest_report" ]]; then
  echo "No drift report found under $DRIFT_ROOT"
  exit 1
fi

report_date="$(basename "$latest_report" | sed 's/_drift-report\.md$//')"
report_dir="$(dirname "$latest_report")"
summary_file="$report_dir/${report_date}_drift-summary.md"
next_review="$(date -u -d "+90 days" +%Y-%m-%d)"

awk_status() {
  local label="$1"
  awk -F'|' -v label="$label" '
    {
      field=$2
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", field)
      if (field == label) {
        status=$3
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", status)
        print status
        exit
      }
    }
  ' "$latest_report"
}

metadata_status="$(awk_status "Metadata compliance")"
freshness_status="$(awk_status "Review freshness")"
changelog_status="$(awk_status "Changelog discipline")"

if [[ -z "$metadata_status" ]]; then metadata_status="UNKNOWN"; fi
if [[ -z "$freshness_status" ]]; then freshness_status="UNKNOWN"; fi
if [[ -z "$changelog_status" ]]; then changelog_status="UNKNOWN"; fi

overall="PASS"
if [[ "$metadata_status" != "PASS" || "$freshness_status" != "PASS" ]]; then
  overall="ATTENTION"
fi

cat > "$summary_file" <<EOF
# Governance Drift Executive Summary

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: $report_date
Next Review: $next_review
Operational Scope: Executive summary derived from latest governance drift snapshot

## Source

- Latest drift report: ${latest_report#"$ROOT_DIR/"}
- Generated at (UTC): $(date -u +%Y-%m-%dT%H:%M:%SZ)

## Status Summary

| Dimension | Status |
| --- | --- |
| Metadata compliance | $metadata_status |
| Review freshness | $freshness_status |
| Changelog discipline | $changelog_status |
| Overall | $overall |

## Notes

- Overall is ATTENTION when required checks (metadata or freshness) are not PASS.
- Changelog discipline warnings should be reviewed before release merges.
EOF

echo "Generated governance drift summary: $summary_file"
