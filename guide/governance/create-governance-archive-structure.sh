#!/usr/bin/env bash
set -euo pipefail

YEAR="${1:-$(date -u +%Y)}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARCHIVE_ROOT="$ROOT_DIR/archive"

mkdir -p "$ARCHIVE_ROOT/incidents/$YEAR"
mkdir -p "$ARCHIVE_ROOT/monthly/$YEAR"
mkdir -p "$ARCHIVE_ROOT/quarterly/$YEAR"
mkdir -p "$ARCHIVE_ROOT/annual/$YEAR"

cat > "$ARCHIVE_ROOT/README.md" <<'EOF'
# Governance Archive

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Historical storage for governance artifacts

This folder stores completed governance artifacts by year.

## Layout

- incidents/<year>/
- monthly/<year>/
- quarterly/<year>/
- annual/<year>/

## Naming Rules

- Incident bundles: <YYYY-MM-DD>_<INCIDENT_ID>_<severity>/
- Monthly signoff: <YYYY-MM>_signoff.md
- Quarterly review: <YYYY-QN>_review.md
- Annual summary: <YYYY>_summary.md
EOF

echo "Initialized governance archive structure for year $YEAR at $ARCHIVE_ROOT"
