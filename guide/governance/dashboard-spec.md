# Reliability Dashboard Specification

Governance Version: 1.0
Review Owner: Observability Owner
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Internal reliability observability for queueing, provider behavior, OCR and rendering throughput

## Dashboard Sections

1. Executive Health
2. Queue and Throughput
3. Provider and Retry Health
4. Error and Timeout Trends
5. Failover and Degradation Signals
6. Incident Timeline and Correlation Views

## Required Metrics and Visualizations

| Metric | Recommended Visualization | Alert Thresholds |
| --- | --- | --- |
| Queue depth | Time-series line + threshold bands | Warn: > 500 for 10 min. Critical: > 1000 for 10 min. |
| Queue age (oldest job) | Time-series line | Warn: > 5 min. Critical: > 15 min. |
| Provider latency (OCR/translation) | p50/p95/p99 lines | Warn: p95 > 2x baseline for 15 min. Critical: p95 > 3x baseline for 15 min. |
| Retry frequency | Stacked bars by reason/provider | Warn: > 5% of chunks retried in 10 min. Critical: > 15%. |
| Failover frequency | Event count timeline | Warn: > 3 failovers per 10 min. Critical: > 10. |
| OCR throughput | Jobs/min line | Alert on sustained drop > 30% from baseline for 15 min. |
| Render throughput | Jobs/min line | Alert on sustained drop > 30% from baseline for 15 min. |
| Error-rate trend | Time-series line by stage | Warn: > 2% for 10 min. Critical: > 5% for 10 min. |
| Timeout trend | Time-series line by provider/stage | Warn: > 1% for 10 min. Critical: > 3% for 10 min. |

## Panel Requirements

- Every panel must show time window, aggregation method, and last update timestamp.
- Every reliability-critical panel must include direct links to runbook triage steps.
- Severity colors must align with the severity matrix definitions.

## Required Operational Views

- Incident view: correlation-aware timeline (OCR -> translation -> render -> export) keyed by `correlation_id` and `job_id`.
- Provider health view: success/failure rates, retry/failover rates, timeout rate, and latency distribution buckets.
- Queue health view: queue depth, processing concurrency, stuck jobs, retry backlog, dead-letter volume.
- Export reliability view: export success rate and terminal failure trend.

## Alert Routing

- Warn alerts route to service owner channels.
- Critical alerts route to on-call paging plus incident channel.
- Repeated warn alerts (3+ in 1 hour) should auto-promote to escalation review.

## Data Sources

- Backend metrics endpoints (`/api/v1/metrics/providers`, `/api/v1/metrics/ocr`).
- Backend reliability endpoint (`/api/v1/metrics/reliability`).
- Lifecycle event query endpoint (`/api/v1/metrics/lifecycle/events`).
- Prometheus scrape endpoint (`/api/v1/metrics/prometheus`).
- Reliability event tables and dashboard SQL.
- Worker queue telemetry and provider integration counters.

## Reference Artifact

- Ready-to-import Grafana dashboard JSON: `guide/governance/dashboards/loklingo-reliability-dashboard.json`
- Incident timeline companion dashboard JSON: `guide/governance/dashboards/loklingo-incident-timeline-dashboard.json`
