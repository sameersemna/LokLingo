#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$ROOT_DIR/../.." && pwd)"

REPORT_DATE="$(date -u +%Y-%m-%d)"
REPORT_TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
REPORT_YEAR="$(date -u +%Y)"
OUT_DIR="$ROOT_DIR/archive/drift/$REPORT_YEAR"
OUT_FILE="$OUT_DIR/${REPORT_DATE}_drift-report.md"

mkdir -p "$OUT_DIR"

run_check() {
  local label="$1"
  shift
  local output
  if output="$($@ 2>&1)"; then
    echo "PASS"$'\n'"$output"
  else
    echo "FAIL"$'\n'"$output"
  fi
}

metadata_result="$(run_check metadata bash "$ROOT_DIR/check-governance-metadata.sh")"
freshness_result="$(run_check freshness bash "$ROOT_DIR/check-governance-freshness.sh")"
changelog_result="$(run_check changelog env BASE_REF=origin/main HEAD_REF=HEAD bash "$ROOT_DIR/check-governance-changelog-discipline.sh")"

metadata_status="$(echo "$metadata_result" | head -n1)"
freshness_status="$(echo "$freshness_result" | head -n1)"
changelog_status="$(echo "$changelog_result" | head -n1)"

metadata_body="$(echo "$metadata_result" | tail -n +2)"
freshness_body="$(echo "$freshness_result" | tail -n +2)"
changelog_body="$(echo "$changelog_result" | tail -n +2)"

# Keep report dates valid for freshness checks.
next_review="$(date -u -d "+90 days" +%Y-%m-%d)"

cat > "$OUT_FILE" <<EOF
# Governance Drift Snapshot

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: $REPORT_DATE
Next Review: $next_review
Operational Scope: Automated governance drift snapshot generated from governance checks

## Snapshot Metadata

- Report Date (UTC): $REPORT_DATE
- Generated At (UTC): $REPORT_TS
- Source: guide/governance/generate-governance-drift-snapshot.sh

## Check Status

| Check | Status |
| --- | --- |
| Metadata compliance | $metadata_status |
| Review freshness | $freshness_status |
| Changelog discipline | $changelog_status |

## Metadata Check Output

\`\`\`text
$metadata_body
\`\`\`

## Freshness Check Output

\`\`\`text
$freshness_body
\`\`\`

## Changelog Discipline Output

\`\`\`text
$changelog_body
\`\`\`
EOF

echo "Generated governance drift snapshot: $OUT_FILE"

if [[ "$metadata_status" != "PASS" || "$freshness_status" != "PASS" ]]; then
  echo "Governance drift snapshot generated with failing required checks."
  exit 1
fi

exit 0
