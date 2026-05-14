# Reliability Review Workflows

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Reliability trend analysis, incident review, resilience planning

This workflow defines monthly, quarterly, and annual review automation for LokLingo reliability governance.

## Review Automation Entry Point

- Script: `guide/governance/reviews/generate-reliability-review.sh`
- CI workflow: `.github/workflows/reliability-automation.yml`
- Manual generation:

```bash
bash guide/governance/reviews/generate-reliability-review.sh monthly
bash guide/governance/reviews/generate-reliability-review.sh quarterly
bash guide/governance/reviews/generate-reliability-review.sh annual
```

## Monthly Reliability Review

Goal:
- Validate current reliability posture and highlight active regressions.

Generated outputs:
- Trend summaries
- Incident summaries
- Retry and failure trend highlights
- Provider instability snapshot
- Weekly smoke ops summary artifact (`generated-weekly-smoke-ops-summary.md`) combining smoke status with any generated incident brief
	and provider-policy/degraded-signal snapshot highlights
- Reliability artifact index (`generated-reliability-artifact-index.md`) linking smoke report, provider snapshot, incident packet, brief, and summary
- Provider policy trend artifact (`generated-provider-policy-trend.md`) comparing current provider policy scores against optional prior baseline snapshot
- Weekly operator one-pager (`generated-weekly-operator-onepager.md`) merging smoke summary, provider trend highlights, and artifact-index handoff commands
- Weekly incident escalation summary (`generated-incident-escalation-summary.md`) extracting critical-regression count, critical providers, and immediate command bundle
- Weekly artifact status summary (`generated-weekly-artifact-summary.md`) listing all mandatory weekly artifacts and presence status in one operator-focused table
- Mandatory artifact validation gate (`validate-reliability-artifacts.py`) enforcing weekly smoke report, provider snapshot, summary, artifact index, provider trend, escalation summary, and one-pager presence, including one-pager triage markers, provider-trend severity/escalation structure checks, and escalation-summary critical-provider/command requirements when critical regressions are detected
	- Optional freshness enforcement for escalation summaries is available through `RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS`.

Failure diagnostics:
- Weekly review tooling regression tests write logs under `guide/governance/reviews/test-logs/` in CI and upload them as `reliability-weekly-review-test-logs` when that test step fails.
- Weekly review tooling tests are executed through `guide/governance/reviews/run-weekly-review-tests.sh` to keep logging behavior consistent between local and CI runs.
- CI verifies expected per-test log files exist before uploading failure logs so missing test output is treated as a workflow defect.
- `test_run_weekly_review_tests.py` validates the runner's failure path with an injected test matrix and ensures logs are still produced for commands after a failing entry.
- The weekly artifact summary generator now enforces stable row order plus `present`/`missing` status values so the operator table stays machine-checkable.
- `test_reliability_automation_workflow.py` verifies that the monthly, quarterly, and annual review jobs each include the preflight smoke gate before artifact generation.

Local full-stack validation:
- Run `guide/governance/reviews/run-weekly-reliability-local.sh` to execute review-tooling regressions, incident validator tests, lineage harness checks, and script syntax/compile checks in one command.
- Run `guide/governance/reviews/run-weekly-reliability-preflight.sh` to apply schedule-like env defaults before invoking the same local stack.
- Use `guide/governance/reviews/run-weekly-reliability-preflight.sh --smoke` in CI or preflight checks to verify environment defaults without launching the local stack.

Review cadence smoke gate:
- Monthly, quarterly, and annual reliability review jobs now run the same preflight smoke check before generating their respective artifacts so environment drift is caught consistently across all cadences.

Scheduled baseline persistence:
- Reliability automation restores/saves `previous-provider-policy-snapshot.json` via GitHub Actions cache on scheduled runs,
  enabling trend deltas in subsequent runs without manual baseline management.

Regression harness coverage:
- `guide/ops/test_reliability_smoke_lineage.py` validates that smoke automation fails when a captured correlation id never yields lifecycle events.

## Quarterly Resilience Review

Goal:
- Evaluate structural resilience over the quarter and tune failover/retry strategy.

Generated outputs:
- Quarter-over-quarter retry and timeout movement
- Dead-letter and queue pressure trend checks
- Provider failover pattern checks
- Ownership and runbook adjustment recommendations

## Annual Operational Summary

Goal:
- Provide executive operational summary and planning baseline for next year.

Generated outputs:
- Reliability trend summary
- Incident pattern summary
- Retry/failure trend summary
- Provider instability report
- Governance and ownership action checklist

## Governance Integration

This workflow is coupled to existing governance controls:

- Governance index linkage via `guide/governance/README.md`
- Retention and archive controls via `guide/governance/run-governance-maintenance.sh`
- Ownership references in `guide/reliability-runbook.md`
- Incident workflows via `guide/governance/incident-templates.md`

Generated artifacts must be archived under the governance archive process before final signoff.
