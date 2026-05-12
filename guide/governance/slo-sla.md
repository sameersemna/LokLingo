# SLO and SLA Definitions

Governance Version: 1.0
Review Owner: Service Owners (Backend + OCR + Frontend)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Production reliability objectives for OCR, translation, rendering, queues, retries

## Service Objectives

| Objective | Target | Measurement Method | Ownership | Review Cadence |
| --- | --- | --- | --- | --- |
| OCR success rate | >= 99.0% daily | Successful OCR jobs / total OCR attempts from lifecycle events and OCR metrics endpoint. | OCR Service Owner | Weekly review, monthly target recalibration |
| Translation completion rate | >= 99.5% daily | Completed translation stages / translation_started events per day, excluding canceled jobs. | Backend Translation Owner | Weekly |
| Render completion rate | >= 99.0% daily | render_completed / render_started events per day. | Backend Rendering Owner | Weekly |
| Median image latency | <= 8s daily p50 | Median job duration from job_created to render_completed for image jobs. | Backend + Frontend Owners | Weekly |
| PDF completion latency | <= 180s daily p50 | Median job duration from job_created to job_finished for PDF jobs. | Backend + OCR Owners | Weekly |
| Retry threshold | <= 3 retries per chunk at p95 | Distribution from chunk_retry lifecycle events and provider metrics. | Backend Translation Owner | Weekly |
| Queue backlog threshold | <= 500 queued jobs for >10 min (warn), <= 1000 for >10 min (critical) | Queue depth + queue age from Redis/worker telemetry. | Platform/SRE Owner | Daily checks, weekly review |

## SLA Defaults

- Sev 1 acknowledgment SLA: <= 5 minutes.
- Sev 2 acknowledgment SLA: <= 15 minutes.
- Sev 3 acknowledgment SLA: <= 30 minutes.
- Sev 4 triage SLA: <= 1 business day.

## Error Budget Handling

- If any objective misses target for two consecutive days, open a reliability action item.
- If error budget burn is severe (projected monthly miss > 2x), freeze non-critical reliability-risky changes until mitigation is in place.

## Reporting Rules

- Weekly reliability review includes objective trends and breaches.
- Monthly governance signoff includes breach root cause summary and remediation status.
- Quarterly snapshot records target changes and rationale.
