# Reliability Recovery Tooling

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Failed job recovery, dead-letter replay, incident snapshot exports

LokLingo recovery tooling entry point:

- `guide/ops/reliability-recovery.sh`

## Commands

List dead-letter jobs:

```bash
bash guide/ops/reliability-recovery.sh list-dead 50
```

Replay one dead-letter job:

```bash
bash guide/ops/reliability-recovery.sh replay-dead <job-id>
```

Replay dead-letter backlog (best-effort):

```bash
bash guide/ops/reliability-recovery.sh replay-dead-all 50
```

Inspect provider history snapshot:

```bash
bash guide/ops/reliability-recovery.sh provider-history
```

Export provider-policy and degraded-signal snapshot:

```bash
bash guide/ops/reliability-recovery.sh provider-policy-snapshot
```

## OCR Fallback-Budget Triage Quick Block

Use this when alerting reports fallback-budget exhaustion in OCR provider chains.

- Prometheus metric: `loklingo_ocr_provider_fallback_budget_exhausted_total`
- Warning threshold (10m delta): `> 0 and < 3`
- Critical threshold (10m delta): `>= 3`

Quick checks:

```bash
curl -sS "http://localhost:28080/api/v1/metrics/prometheus" -H "X-Internal-Token: $INTERNAL_TOKEN" | rg "loklingo_ocr_provider_fallback_budget_exhausted_total|loklingo_provider_timeout_total|loklingo_queue_depth|loklingo_queue_retry_backlog"
bash guide/ops/reliability-recovery.sh provider-history
bash guide/ops/reliability-recovery.sh provider-policy-snapshot
```

Immediate actions:

- Confirm `OCR_PROVIDER_TIMEOUT_SECONDS` and `OCR_MAX_FALLBACKS` still match current provider latency behavior.
- Correlate fallback-budget increases with provider timeout and queue-pressure deltas before changing concurrency.
- If critical threshold persists across two windows, open Sev 2+ workflow and page OCR owner.

Inspect queue recovery status:

```bash
bash guide/ops/reliability-recovery.sh queue-status
```

Export incident snapshot packet:

```bash
bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>
```

Validate incident snapshot schema:

```bash
python3 guide/ops/validate-incident-snapshot.py <incident-snapshot.json>
```

Or via recovery CLI:

```bash
bash guide/ops/reliability-recovery.sh validate-incident-snapshot <incident-snapshot.json>
```

Generate markdown incident brief from snapshot:

```bash
python3 guide/ops/generate-incident-brief.py <incident-snapshot.json> <brief-output.md>
```

Or via recovery CLI:

```bash
bash guide/ops/reliability-recovery.sh generate-incident-brief <incident-snapshot.json> <brief-output.md>
```

The incident snapshot now embeds the provider policy/degraded snapshot payload automatically under `provider_policy_snapshot`.
The incident snapshot also includes a compact triage section under `derived_summary`
with lifecycle stage counts, queue/degraded highlights, provider timeout/retry totals,
and the lowest provider policy score.
The same block now includes `triage_severity_hint` and `triage_trigger_reasons`
to speed up first-response prioritization.

Scheduled automation can also export a real incident packet and markdown brief when
`RELIABILITY_INCIDENT_CORRELATION_ID` is available in the workflow environment, or
when a future smoke report includes `correlation_id` or `last_correlation_id`.

The weekly reliability smoke report now emits `correlation_id`, `last_correlation_id`,
`job_id`, and `last_job_id` when the timeout-storm probe enqueues async jobs successfully.
It also records `lifecycle_event_count` and `lineage_verified` after polling
`/api/v1/metrics/lifecycle/events` for the captured correlation id before writing the report.

Deterministic lineage regression harness:

```bash
python3 guide/ops/test_reliability_smoke_lineage.py
```

The harness runs the smoke suite against a local mock API and asserts the suite fails
when correlation ids are present but lifecycle event count remains zero.

Weekly review helpers:

```bash
python3 guide/governance/reviews/generate-provider-policy-trend.py <current-provider-snapshot.json> <previous-provider-snapshot.json|NONE> <output.md>
python3 guide/governance/reviews/generate-weekly-operator-onepager.py <smoke-summary.md> <provider-trend.md> <artifact-index.md> <output.md>
python3 guide/governance/reviews/generate-incident-escalation-summary.py <provider-trend.md> <artifact-index.md> <output.md>
python3 guide/governance/reviews/generate-weekly-artifact-summary.py <smoke-report.json> <provider-snapshot.json> <smoke-summary.md> <artifact-index.md> <provider-trend.md> <escalation-summary.md> <operator-onepager.md> <output.md>
python3 guide/governance/reviews/validate-reliability-artifacts.py <smoke-report.json> <provider-snapshot.json> <smoke-summary.md> <artifact-index.md> <provider-trend.md> <escalation-summary.md> <operator-onepager.md>
bash guide/governance/reviews/run-weekly-review-tests.sh [log-dir]
bash guide/governance/reviews/run-weekly-reliability-local.sh [log-dir]
bash guide/governance/reviews/run-weekly-reliability-preflight.sh [log-dir]
python3 guide/governance/reviews/test_generate_weekly_operator_onepager.py
python3 guide/governance/reviews/test_validate_reliability_artifacts.py
python3 guide/governance/reviews/test_generate_incident_escalation_summary.py
python3 guide/governance/reviews/test_generate_weekly_artifact_summary.py
python3 guide/governance/reviews/test_run_weekly_review_tests.py
python3 guide/governance/reviews/test_reliability_automation_workflow.py
```

Fallback-row parser contract:

- Fallback-budget signal parsing rules are centralized in `guide/governance/reviews/fallback_signal_parser.py`.
- Both generator and validator paths consume this parser:
	- `generate-weekly-operator-onepager.py`
	- `validate-reliability-artifacts.py`
- Update parser rules and `test_fallback_signal_parser.py` together to avoid drift.

The mandatory artifact validator now requires the weekly operator one-pager to include
the triage snapshot markers and operator handoff commands section, preventing structurally incomplete handoff artifacts from passing CI.
It also requires the provider trend artifact to include severity totals and enforces an escalation section whenever critical regressions are present.
The incident escalation summary artifact is also validated for required triage markers and,
when critical regression count is non-zero, must include concrete critical providers and executable command lines.
Freshness checks can be enabled by setting `RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS`
before running the validator.

Provider trend output now includes severity labels (`critical`, `warning`, `stable`, `improving`) per provider delta,
and the artifact index includes an operator handoff command block keyed by smoke severity.
When any provider delta is `critical`, the trend artifact also emits an escalation block with immediate triage commands.
The weekly operator one-pager now also lifts the severity hint, recommended first command,
and correlation id into a single triage snapshot section so first response does not require opening the artifact index separately.
When weekly review regression tests fail in CI, the workflow uploads `reliability-weekly-review-test-logs`
to speed diagnosis without rerunning locally.
Weekly runs also generate `generated-weekly-artifact-summary.md`, and CI publishes it in step summary output for fast operator scan.

## Required Environment

- `INTERNAL_TOKEN`
- `LOKLINGO_BASE_URL` (defaults to `http://localhost:28080`)
