# Governance Incident Examples

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example incident packets by severity and failure mode

Use these example packets as reference implementations for real incidents. They are intentionally short, structured, and mapped to common LokLingo failure patterns.

## Example Map

| Example | Severity | Failure Mode | Primary Learning Focus |
| --- | --- | --- | --- |
| [Sev 1 OCR outage](sev1-ocr-outage/summary.md) | Sev 1 | Core dependency outage | Fast escalation, readiness diagnosis, full workflow impact |
| [Sev 2 translation failover](sev2-translation-failover/summary.md) | Sev 2 | Provider degradation with working failover | Latency management, failover tuning, stakeholder threshold |
| [Sev 2 queue backlog](sev2-queue-backlog/summary.md) | Sev 2 | Worker pressure and queue age growth | Throughput protection, low-risk mitigation, backlog triage |
| [Sev 3 export failure](sev3-export-failure/summary.md) | Sev 3 | Recoverable export-path instability | Partial-path diagnosis, replay strategy, scoped communication |

## Usage Guidance

- Start from the example closest to the current failure mode.
- Copy structure, not wording; adapt severity, timing, and evidence to the live incident.
- Keep example packets aligned with the current severity matrix and incident templates.
