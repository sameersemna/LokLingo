#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "Usage: $0 <incident_id> <severity> [YYYY-MM-DD]"
  echo "Example: $0 INC-2026-0512-01 sev2 2026-05-12"
  exit 1
fi

INCIDENT_ID="$1"
SEVERITY_RAW="$2"
DATE_STAMP="${3:-$(date -u +%Y-%m-%d)}"

# Normalize severity to a safe folder token.
SEVERITY="$(echo "$SEVERITY_RAW" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9_-')"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_DIR="$SCRIPT_DIR/incidents-template"
TARGET_ROOT="${LOKLINGO_INCIDENTS_DIR:-$SCRIPT_DIR/../incidents}"
TARGET_DIR="$TARGET_ROOT/${DATE_STAMP}_${INCIDENT_ID}_${SEVERITY}"

mkdir -p "$TARGET_DIR"

for f in summary.md timeline.md handoff.md closure.md postmortem.md action-tracker.csv; do
  cp "$TEMPLATE_DIR/$f" "$TARGET_DIR/$f"
done

# Pre-fill minimal metadata placeholders for quick start.
sed -i "s/- Incident ID:/- Incident ID: $INCIDENT_ID/" "$TARGET_DIR/summary.md"
sed -i "s/- Severity:/- Severity: $SEVERITY_RAW/" "$TARGET_DIR/summary.md"
sed -i "s/- Incident ID:/- Incident ID: $INCIDENT_ID/" "$TARGET_DIR/closure.md"
sed -i "s/- Final Severity:/- Final Severity: $SEVERITY_RAW/" "$TARGET_DIR/closure.md"
sed -i "s/- Incident ID:/- Incident ID: $INCIDENT_ID/" "$TARGET_DIR/handoff.md"
sed -i "s/- Incident ID:/- Incident ID: $INCIDENT_ID/" "$TARGET_DIR/postmortem.md"

echo "Created incident bundle: $TARGET_DIR"
echo "Next step: populate summary.md and timeline.md, then share the folder link in the incident channel."
