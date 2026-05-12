# Observability and Reliability Instrumentation Specification

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: End-to-end instrumentation across OCR, translation, render, export, queue, and provider reliability

## Lifecycle Event Contract

Required lifecycle events:

- job_created
- upload_received
- ocr_started
- ocr_completed
- translation_started
- chunk_started
- chunk_completed
- chunk_retry
- provider_failover
- render_started
- render_completed
- export_started
- export_completed
- job_failed
- job_completed

Required event fields:

- job_id
- correlation_id
- provider
- duration_ms
- retry_count
- queue_depth
- timestamp

## Reliability Metrics Model

Metrics are captured in-process with atomic counters and timing aggregates.

- OCR latency (`ocr_latency_ms`)
- Translation latency (`translation_latency_ms`)
- Render latency (`render_latency_ms`)
- Queue wait time (`queue_wait_ms`)
- Retries (`retries_total`)
- Provider failovers (`failovers_total`)
- Timeout frequency (`timeouts_total`)
- Chunk success rate (`chunk_success_rate`)
- Export success rate (`export_success_rate`)

API:

- `GET /api/v1/metrics/reliability`
- `GET /api/v1/metrics/prometheus`

Prometheus-compatible metrics include pipeline counters/ratios, queue gauges, provider counters, timeout reason counters, and checkpoint counters.

## Provider Health Views

Provider views support LiteLLM, OpenAI-compatible providers, Ollama, and future providers via provider name labels.

Per-provider metrics:

- success_total
- failure_total
- retry_total
- failover_total
- timeout_total
- avg_latency_ms
- latency_buckets (`<=250ms`, `<=500ms`, `<=1000ms`, `<=2000ms`, `>2000ms`)

Health rollups:

- success_rate
- retry_rate
- timeout_rate
- failover_rate

API:

- `GET /api/v1/metrics/providers`

## Queue Visibility

Queue metrics:

- queue_depth
- processing_concurrency
- stuck_jobs
- retry_backlog
- dead_letter_volume

Alert recommendations:

- queue_depth: warn 500, critical 1000
- stuck_jobs: warn 1, critical 5
- retry_backlog: warn 50, critical 200
- dead_letter_volume: warn 1, critical 10

## Failure Tracing and Lineage

Lineage is preserved with `correlation_id` from request admission through worker execution:

- OCR -> translation -> render -> export chain
- Provider retries/failovers are emitted with correlation context
- Terminal outcomes (`job_failed`/`job_completed`) are emitted with total duration

Lifecycle event query API:

- `GET /api/v1/metrics/lifecycle/events`
- Query params: `correlation_id`, `job_id`, `event`, `limit`

## Dashboard Specification

Dashboard sections:

- Pipeline health (latency, retry, chunk/export success)
- Provider health (rates + latency distribution)
- Queue health (depth/concurrency/stuck/retry/dead-letter)
- Incident timeline view keyed by `correlation_id`

Incident-ready views should include:

- Last 1h and 24h trend overlays
- Failing provider and timeout reason breakdown
- Queue pressure and stuck job indicators

Alert rules are maintained in:

- `guide/governance/alerts/reliability-alerts.prometheus.yml`

Grafana reference dashboard is maintained in:

- `guide/governance/dashboards/loklingo-reliability-dashboard.json`
- `guide/governance/dashboards/loklingo-incident-timeline-dashboard.json`

Prometheus scrape config is maintained in:

- `guide/governance/prometheus/prometheus.yml`

Token-injecting metrics proxy config is maintained in:

- `guide/governance/metrics-proxy/default.conf.template`

Grafana provisioning config is maintained in:

- `guide/governance/grafana/provisioning/datasources/prometheus.yml`
- `guide/governance/grafana/provisioning/dashboards/dashboards.yml`

## Operational Ownership

| Scope | Primary Owner | Backup Owner | Responsibilities |
| --- | --- | --- | --- |
| Lifecycle event contract | Backend Platform Owner | Reliability Lead | Maintain event schema, ensure required fields and lineage propagation. |
| Provider observability | Translation Service Owner | Platform/SRE Owner | Monitor provider health, tune failover/retry policy, maintain provider dashboards. |
| Queue reliability metrics | Platform/SRE Owner | Backend On-Call Owner | Track queue pressure, stuck jobs, dead-letter backlog, and threshold tuning. |
| OCR and render latency telemetry | OCR Service Owner + Rendering Owner | Backend Platform Owner | Keep stage latency metrics accurate and review drift during reliability review. |
| Incident dashboard readiness | Reliability Lead | Incident Commander Rotation Owner | Ensure incident views and alert rules remain actionable and current. |
