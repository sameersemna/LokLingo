#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

required_headers=(
  "Governance Version:"
  "Review Owner:"
  "Last Review:"
  "Next Review:"
  "Operational Scope:"
)

files=(
  "$ROOT_DIR/guide/reliability-runbook.md"
)

while IFS= read -r file; do
  files+=("$file")
done < <(find "$ROOT_DIR/guide/governance" -type f -name "*.md" | sort)

failures=0

for file in "${files[@]}"; do
  for header in "${required_headers[@]}"; do
    if ! grep -q "^${header}" "$file"; then
      echo "Missing header '$header' in $file"
      failures=1
    fi
  done
done

if [[ "$failures" -ne 0 ]]; then
  echo "Governance metadata check failed."
  exit 1
fi

echo "Governance metadata check passed for ${#files[@]} files."
