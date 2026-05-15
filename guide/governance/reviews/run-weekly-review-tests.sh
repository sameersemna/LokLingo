#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../" && pwd)"
cd "$ROOT_DIR"

log_dir="${1:-guide/governance/reviews/test-logs}"
mkdir -p "$log_dir"

matrix_file="${WEEKLY_REVIEW_TEST_MATRIX_FILE:-}"

declare -a test_specs=()

if [[ -n "$matrix_file" ]]; then
  if [[ ! -f "$matrix_file" ]]; then
    echo "weekly review matrix file not found: $matrix_file" >&2
    exit 1
  fi
  while IFS='|' read -r label command || [[ -n "$label$command" ]]; do
    if [[ -z "$label$command" ]]; then
      continue
    fi
    if [[ -z "$label" ]]; then
      echo "weekly review matrix entry missing label for command: $command" >&2
      exit 1
    fi
    if [[ -z "$command" ]]; then
      echo "weekly review matrix entry missing command for label: $label" >&2
      exit 1
    fi
    test_specs+=("$label|$command")
  done < "$matrix_file"
else
  test_specs+=("test_fallback_signal_parser|python3 guide/governance/reviews/test_fallback_signal_parser.py")
  test_specs+=("test_generate_weekly_operator_onepager|python3 guide/governance/reviews/test_generate_weekly_operator_onepager.py")
  test_specs+=("test_generate_smoke_ops_summary|python3 guide/governance/reviews/test_generate_smoke_ops_summary.py")
  test_specs+=("test_validate_reliability_artifacts|python3 guide/governance/reviews/test_validate_reliability_artifacts.py")
  test_specs+=("test_generate_incident_escalation_summary|python3 guide/governance/reviews/test_generate_incident_escalation_summary.py")
  test_specs+=("test_generate_weekly_artifact_summary|python3 guide/governance/reviews/test_generate_weekly_artifact_summary.py")
  test_specs+=("test_run_weekly_review_tests|python3 guide/governance/reviews/test_run_weekly_review_tests.py")
  test_specs+=("test_reliability_automation_workflow|python3 guide/governance/reviews/test_reliability_automation_workflow.py")
fi

if (( ${#test_specs[@]} == 0 )); then
  echo "weekly review matrix is empty" >&2
  exit 1
fi

run_test() {
  local label="$1"
  local command="$2"
  local log_file="$log_dir/${label}.log"
  if bash -lc "$command" 2>&1 | tee "$log_file"; then
    return 0
  fi
  return 1
}

failures=0

for spec in "${test_specs[@]}"; do
  IFS='|' read -r label command <<< "$spec"
  if ! run_test "$label" "$command"; then
    failures=$((failures + 1))
  fi
done

if (( failures > 0 )); then
  echo "weekly review tooling regression failures: $failures" >&2
  exit 1
fi
