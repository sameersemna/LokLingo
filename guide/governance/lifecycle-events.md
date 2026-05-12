# Operational Lifecycle Events Standard

Governance Version: 1.0
Review Owner: Backend Platform Owner
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Structured lifecycle events for job orchestration, retries, failover, and completion

## Standard Event Set

- job_created
- ocr_started
- ocr_completed
- translation_started
- chunk_retry
- provider_failover
- render_started
- render_completed
- export_completed
- job_finished

## Required Metadata Fields

| Field | Description |
| --- | --- |
| event_type | Lifecycle event name from the standard set. |
| event_time_utc | Event timestamp in UTC ISO-8601 format. |
| job_id | Platform job identifier. |
| request_id | API request identifier for traceability. |
| correlation_id | Cross-stage correlation ID for full workflow lineage. |
| pipeline_stage | ocr, translation, render, export, or orchestration. |
| provider | OCR/translation/render provider used at this stage. |
| attempt | Attempt number for retries and failovers. |
| status | started, completed, failed, degraded, skipped. |
| duration_ms | Stage duration where applicable. |
| error_code | Normalized error code (required on failure/degradation). |
| error_class | Timeout, provider_error, validation_error, infra_error, unknown. |
| tenant_or_scope | Tenant/project scope identifier when applicable. |

## Correlation Rules

- correlation_id is mandatory for every event.
- correlation_id must stay stable from job_created through job_finished.
- chunk_retry and provider_failover events must include parent stage event reference when available.

## Retention Policy

- Hot retention: 30 days in primary analytics store for active operations.
- Warm retention: 180 days aggregated rollups for trend analysis.
- Cold retention: annual summary artifacts retained per governance retention policy.
- Deletion/archival jobs should run daily and log completion status.

## Event Quality Rules

- Events must be emitted in canonical stage order unless explicit failure/abort occurs.
- Missing required metadata makes event invalid and should increment event_quality_error metrics.
- Consumers must tolerate duplicate event delivery using idempotency key (`job_id + event_type + attempt + event_time_utc`).
