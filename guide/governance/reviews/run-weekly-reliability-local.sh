#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../" && pwd)"
cd "$ROOT_DIR"

log_dir="${1:-guide/governance/reviews/test-logs-local}"

python3 -m py_compile \
  guide/governance/reviews/generate-provider-policy-trend.py \
  guide/governance/reviews/generate-incident-artifact-index.py \
  guide/governance/reviews/generate-weekly-operator-onepager.py \
  guide/governance/reviews/generate-incident-escalation-summary.py \
  guide/governance/reviews/generate-weekly-artifact-summary.py \
  guide/governance/reviews/validate-reliability-artifacts.py \
  guide/governance/reviews/test_generate_weekly_operator_onepager.py \
  guide/governance/reviews/test_validate_reliability_artifacts.py \
  guide/governance/reviews/test_generate_incident_escalation_summary.py \
  guide/governance/reviews/test_generate_weekly_artifact_summary.py \
  guide/governance/reviews/test_run_weekly_review_tests.py \
  guide/governance/reviews/test_reliability_automation_workflow.py \
  guide/ops/test_validate_incident_snapshot.py \
  guide/ops/test_reliability_smoke_lineage.py

bash -n guide/reliability-smoke-suite.sh
bash -n guide/ops/reliability-recovery.sh
bash -n guide/governance/reviews/run-weekly-review-tests.sh
bash -n guide/governance/reviews/run-weekly-reliability-preflight.sh

bash guide/governance/reviews/run-weekly-review-tests.sh "$log_dir"
python3 guide/ops/test_validate_incident_snapshot.py
python3 guide/ops/test_reliability_smoke_lineage.py

echo "weekly reliability local validation complete"
