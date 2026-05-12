#!/usr/bin/env bash
set -euo pipefail

today="$(date -u +%Y-%m-%d)"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

files=("$ROOT_DIR/guide/reliability-runbook.md")
while IFS= read -r file; do
  files+=("$file")
done < <(find "$ROOT_DIR/guide/governance" -type f -name "*.md" | sort)

failures=0
warnings=0

for file in "${files[@]}"; do
  last_review="$(grep -E '^Last Review:' "$file" | sed 's/^Last Review:[[:space:]]*//' || true)"
  next_review="$(grep -E '^Next Review:' "$file" | sed 's/^Next Review:[[:space:]]*//' || true)"

  if [[ -z "$last_review" || -z "$next_review" ]]; then
    echo "ERROR: Missing Last Review/Next Review in $file"
    failures=1
    continue
  fi

  if ! date -u -d "$last_review" +%Y-%m-%d >/dev/null 2>&1; then
    echo "ERROR: Invalid Last Review date in $file: $last_review"
    failures=1
  fi

  if ! date -u -d "$next_review" +%Y-%m-%d >/dev/null 2>&1; then
    echo "ERROR: Invalid Next Review date in $file: $next_review"
    failures=1
    continue
  fi

  if [[ "$next_review" < "$today" ]]; then
    echo "ERROR: Review overdue in $file (Next Review: $next_review, Today: $today)"
    failures=1
  fi

  # Advisory warning if last review is older than 180 days.
  last_ts="$(date -u -d "$last_review" +%s)"
  today_ts="$(date -u -d "$today" +%s)"
  age_days="$(( (today_ts - last_ts) / 86400 ))"
  if [[ "$age_days" -gt 180 ]]; then
    echo "WARN: Last Review older than 180 days in $file ($last_review)"
    warnings=1
  fi
done

if [[ "$failures" -ne 0 ]]; then
  echo "Governance freshness check failed."
  exit 1
fi

if [[ "$warnings" -ne 0 ]]; then
  echo "Governance freshness check passed with warnings."
  exit 0
fi

echo "Governance freshness check passed for ${#files[@]} files."
