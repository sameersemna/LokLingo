#!/usr/bin/env bash
set -euo pipefail

BASE_REF="${BASE_REF:-origin/main}"
HEAD_REF="${HEAD_REF:-HEAD}"
STRICT_LOCAL="${STRICT_LOCAL:-false}"
used_local_fallback="false"

# Ensure base ref exists when running in CI with shallow clones.
if ! git rev-parse --verify "$BASE_REF" >/dev/null 2>&1; then
  echo "Base ref $BASE_REF not found locally. Attempting fetch..."
  git fetch origin main --depth=200 >/dev/null 2>&1 || true
fi

changed_files="$(git diff --name-only "$BASE_REF...$HEAD_REF" || true)"

if [[ -z "$changed_files" ]]; then
  # Local fallback for uncommitted worktree changes.
  changed_files="$(git diff --name-only HEAD || true)"
  used_local_fallback="true"
fi

if [[ -z "$changed_files" ]]; then
  echo "No changed files detected for commit range or local worktree."
  exit 0
fi

governance_changed="$(echo "$changed_files" | grep -E '^guide/governance/.+\.(md|sh)$|^guide/reliability-runbook\.md$|^README\.md$' || true)"

if [[ -z "$governance_changed" ]]; then
  echo "No governance-scope changes detected."
  exit 0
fi

if echo "$governance_changed" | grep -q '^guide/governance/governance-change-log\.md$'; then
  echo "Governance scope changed and governance change log was updated."
  exit 0
fi

if [[ "$used_local_fallback" == "true" && "$STRICT_LOCAL" != "true" ]]; then
  echo "Governance scope changes detected in local worktree without a changelog edit."
  echo "Warning only in local fallback mode. Re-run with STRICT_LOCAL=true to enforce failure."
  echo "Changed governance-scope files:"
  echo "$governance_changed"
  exit 0
fi

echo "Governance scope changes detected but guide/governance/governance-change-log.md was not updated."
echo "Changed governance-scope files:"
echo "$governance_changed"
exit 1
