#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARCHIVE_DIR="$ROOT_DIR/archive"
POINTERS_FILE="$ROOT_DIR/current-pointers.md"

latest_for_pattern() {
  local base_dir="$1"
  local pattern="$2"
  find "$base_dir" -type f -name "$pattern" 2>/dev/null | sort | tail -n 1 || true
}

latest_incident_dir="$(find "$ARCHIVE_DIR/incidents" -mindepth 2 -maxdepth 2 -type d 2>/dev/null | sort | tail -n 1 || true)"
latest_monthly="$(latest_for_pattern "$ARCHIVE_DIR/monthly" "*_signoff.md")"
latest_quarterly="$(latest_for_pattern "$ARCHIVE_DIR/quarterly" "*_review.md")"
latest_annual="$(latest_for_pattern "$ARCHIVE_DIR/annual" "*_summary.md")"

rel_path() {
  local p="$1"
  if [[ -z "$p" ]]; then
    echo "(none yet)"
  else
    echo "${p#"$ROOT_DIR/"}"
  fi
}

cat > "$POINTERS_FILE" <<EOF
# Governance Current Pointers

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Shortcut pointers to current month/quarter/year governance artifacts

## Current Shortcuts

- Current incident reference: $(rel_path "$latest_incident_dir")
- Current month signoff: $(rel_path "$latest_monthly")
- Current quarter review: $(rel_path "$latest_quarterly")
- Current year summary: $(rel_path "$latest_annual")

## How to Refresh

- Run: bash guide/governance/update-governance-current-pointers.sh
- Run after new monthly, quarterly, annual, or incident artifacts are archived.
EOF

echo "Updated governance current pointers: $POINTERS_FILE"
