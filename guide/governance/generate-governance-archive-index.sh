#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARCHIVE_DIR="$ROOT_DIR/archive"
INDEX_FILE="$ARCHIVE_DIR/index.md"

mkdir -p "$ARCHIVE_DIR"

latest_incident="$(find "$ARCHIVE_DIR/incidents" -mindepth 2 -maxdepth 2 -type d 2>/dev/null | sort | tail -n 1 || true)"
latest_monthly="$(find "$ARCHIVE_DIR/monthly" -type f -name '*_signoff.md' 2>/dev/null | sort | tail -n 1 || true)"
latest_quarterly="$(find "$ARCHIVE_DIR/quarterly" -type f -name '*_review.md' 2>/dev/null | sort | tail -n 1 || true)"
latest_annual="$(find "$ARCHIVE_DIR/annual" -type f -name '*_summary.md' 2>/dev/null | sort | tail -n 1 || true)"

rel_path() {
  local p="$1"
  if [[ -z "$p" ]]; then
    echo "(none yet)"
  else
    echo "${p#"$ROOT_DIR/"}"
  fi
}

cat > "$INDEX_FILE" <<EOF
# Governance Archive Index

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Discoverability index for archived governance artifacts

## Latest Pointers

- Latest incident bundle: $(rel_path "$latest_incident")
- Latest monthly signoff: $(rel_path "$latest_monthly")
- Latest quarterly review: $(rel_path "$latest_quarterly")
- Latest annual summary: $(rel_path "$latest_annual")

## Archive Roots

- incidents/: guide/governance/archive/incidents/
- monthly/: guide/governance/archive/monthly/
- quarterly/: guide/governance/archive/quarterly/
- annual/: guide/governance/archive/annual/

## Maintenance

- Regenerate this index after archiving new monthly, quarterly, annual, or incident artifacts.
- Command: bash guide/governance/generate-governance-archive-index.sh
EOF

echo "Generated governance archive index: $INDEX_FILE"
