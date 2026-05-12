#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DRIFT_ROOT="$ROOT_DIR/archive/drift"
TREND_FILE="$DRIFT_ROOT/trend.csv"

latest_report="$(find "$DRIFT_ROOT" -type f -name '*_drift-report.md' 2>/dev/null | sort | tail -n 1 || true)"
if [[ -z "$latest_report" ]]; then
  echo "No drift report found under $DRIFT_ROOT"
  exit 1
fi

report_date="$(basename "$latest_report" | sed 's/_drift-report\.md$//')"

extract_status() {
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

metadata_status="$(extract_status "Metadata compliance")"
freshness_status="$(extract_status "Review freshness")"
changelog_status="$(extract_status "Changelog discipline")"

if [[ -z "$metadata_status" ]]; then metadata_status="UNKNOWN"; fi
if [[ -z "$freshness_status" ]]; then freshness_status="UNKNOWN"; fi
if [[ -z "$changelog_status" ]]; then changelog_status="UNKNOWN"; fi

mkdir -p "$DRIFT_ROOT"
if [[ ! -f "$TREND_FILE" ]]; then
  echo "date_utc,metadata_status,freshness_status,changelog_status,report_path" > "$TREND_FILE"
fi

tmp_file="${TREND_FILE}.tmp"
{
  head -n 1 "$TREND_FILE"
  tail -n +2 "$TREND_FILE" | grep -v "^${report_date}," || true
  echo "${report_date},${metadata_status},${freshness_status},${changelog_status},${latest_report#"$ROOT_DIR/"}"
} > "$tmp_file"

mv "$tmp_file" "$TREND_FILE"

echo "Updated governance drift trend: $TREND_FILE"
