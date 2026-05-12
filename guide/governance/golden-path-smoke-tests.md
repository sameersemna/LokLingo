# Golden-Path Reliability Smoke Tests

Governance Version: 1.0
Review Owner: QA + Service Owners
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: End-to-end reliability validation for core visual localization flows

## Goals

- Automated and repeatable checks for core flows.
- Provider-aware behavior verification including failover assumptions.
- Low-maintenance execution with stable fixtures and deterministic assertions.

## Required Test Coverage

| Flow | Validation |
| --- | --- |
| Image upload | Upload succeeds, job is created, lifecycle event emitted. |
| OCR | OCR stage starts/completes and returns non-empty structured output. |
| Translation | Translation completes and language target is applied. |
| Rendering | Render stage completes and output artifact is retrievable. |
| Compare flow | Baseline vs localized output compare view metadata is generated correctly. |
| Export flow | Export artifact builds successfully and is downloadable. |

## Automation Design

- Run with fixed sample assets in `guide/render_samples` or equivalent stable fixtures.
- Use deterministic provider config per environment to reduce flaky assertions.
- Capture lifecycle event stream and assert required stage transitions.
- Keep one canonical golden-path suite; avoid branching variants unless production topology differs materially.

## Cadence and Ownership

- On every backend/frontend merge to main: required.
- Post-deploy to staging: required.
- Daily in production-like environment: required.
- Ownership: QA automation owner for suite integrity; service owners for stage-specific failures.

## Failure Escalation Path

1. Test failure opens reliability incident ticket with run artifact links.
2. Stage owner triages within SLA based on severity classification.
3. Repeated failure pattern (>= 3 in 24h) escalates to service owner and reliability review agenda.

## Maintenance Rules

- Keep fixture set small and representative.
- Prefer contract assertions (status/events/artifact availability) over brittle pixel-level checks.
- Review flaky tests weekly and either stabilize or remove with replacement coverage.
