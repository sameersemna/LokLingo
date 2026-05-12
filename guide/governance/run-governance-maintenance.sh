#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

strict_mode="${STRICT_MODE:-false}"
if [[ "$strict_mode" != "true" ]]; then
	current_branch="$(git -C "$ROOT_DIR/../.." rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"
	if [[ "$current_branch" == release/* ]]; then
		strict_mode="true"
	fi
fi

echo "Strict mode: $strict_mode"

echo "[1/8] Governance metadata check"
bash "$ROOT_DIR/check-governance-metadata.sh"

echo "[2/8] Governance freshness check"
bash "$ROOT_DIR/check-governance-freshness.sh"

echo "[3/8] Governance changelog discipline check"
if [[ "$strict_mode" == "true" ]]; then
	STRICT_LOCAL=true BASE_REF="${BASE_REF:-origin/main}" HEAD_REF="${HEAD_REF:-HEAD}" bash "$ROOT_DIR/check-governance-changelog-discipline.sh"
else
	BASE_REF="${BASE_REF:-origin/main}" HEAD_REF="${HEAD_REF:-HEAD}" bash "$ROOT_DIR/check-governance-changelog-discipline.sh"
fi

echo "[4/8] Governance archive index generation"
bash "$ROOT_DIR/generate-governance-archive-index.sh"

echo "[5/8] Governance current pointers update"
bash "$ROOT_DIR/update-governance-current-pointers.sh"

echo "[6/8] Governance drift snapshot generation"
bash "$ROOT_DIR/generate-governance-drift-snapshot.sh"

echo "[7/8] Governance drift summary generation"
bash "$ROOT_DIR/generate-governance-drift-summary.sh"

echo "[8/8] Governance drift trend update"
bash "$ROOT_DIR/update-governance-drift-trend.sh"

echo "Governance maintenance routine completed."
