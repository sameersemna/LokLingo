# Reliability Runbook

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Reliability telemetry, incident operations, escalation, and governance controls

This document is the source of truth for LokLingo reliability operations.
It centralizes telemetry interpretation, threshold guidance, incident handling,
handoff standards, and closure criteria.

## 📊 Reliability Telemetry

LokLingo can persist OCR and LiteLLM reliability events into Postgres for
time-windowed operations dashboards.

Operational governance framework docs:

* [Governance index](governance/README.md)
* [Reliability Severity Matrix](governance/severity-matrix.md)
* [Incident Management Templates](governance/incident-templates.md)
* [SLO and SLA Definitions](governance/slo-sla.md)
* [Reliability Dashboard Specification](governance/dashboard-spec.md)
* [Observability and Reliability Instrumentation Specification](governance/observability-reliability-spec.md)
* [Golden-Path Reliability Smoke Tests](governance/golden-path-smoke-tests.md)
* [Operational Lifecycle Events Standard](governance/lifecycle-events.md)
* [On-Call First Response Playbook](governance/oncall-first-response-playbook.md)
* [Incident Artifact Template Bundle](governance/incidents-template/README.md)
* [Governance Archive Bootstrap Script](governance/create-governance-archive-structure.sh)
* [Governance Archive Index Generator](governance/generate-governance-archive-index.sh)
* [Governance Current Pointers Updater](governance/update-governance-current-pointers.sh)
* [Governance Drift Snapshot Generator](governance/generate-governance-drift-snapshot.sh)
* [Governance Drift Summary Generator](governance/generate-governance-drift-summary.sh)
* [Governance Drift Trend Updater](governance/update-governance-drift-trend.sh)
* [Governance Maintenance Runner](governance/run-governance-maintenance.sh)
* [Governance Metadata Check Script](governance/check-governance-metadata.sh)
* [Governance Freshness Check Script](governance/check-governance-freshness.sh)
* [Governance Changelog Discipline Script](governance/check-governance-changelog-discipline.sh)
* [Governance Change Log](governance/governance-change-log.md)
* [Governance Archive Layout](governance/governance-archive-layout.md)
* [Governance Operating Calendar](governance/governance-operating-calendar.md)
* [Governance Current Pointers](governance/current-pointers.md)
* [Governance Release Checklist](governance/governance-release-checklist.md)
* [Governance Drift Dashboard Template](governance/governance-drift-dashboard-template.md)
* [Governance Maintainer Quickstart](governance/MAINTAINER-QUICKSTART.md)
* [Reliability Review Workflows](governance/reliability-review-workflows.md)
* [Monthly Governance Signoff Template](governance/reviews/monthly-governance-signoff-template.md)
* [Quarterly Governance Review Template](governance/reviews/quarterly-governance-review-template.md)
* [Annual Governance Summary Template](governance/reviews/annual-governance-summary-template.md)
* [Reliability Recovery Tooling](ops/README.md)
* [Incident Example Index](governance/examples/README.md)
* [Example Sev 1 OCR Outage Packet](governance/examples/sev1-ocr-outage/summary.md)
* [Example Sev 2 Translation Failover Packet](governance/examples/sev2-translation-failover/summary.md)
* [Example Sev 2 Queue Backlog Packet](governance/examples/sev2-queue-backlog/summary.md)
* [Example Sev 3 Export Failure Packet](governance/examples/sev3-export-failure/summary.md)

Runbook quick links:

* [Checkpoint counters and thresholds](#5-checkpoint-reliability-counters-and-thresholds)
* [Alert-to-response mapping](#alert-to-response-mapping-degradedadaptive-path)
* [Alert-to-command mapping](#alert-to-command-mapping-reliability-recovery-script)
* [Severity response matrix](#severity-response-matrix-command-bundles)
* [Dashboard panel-to-alert-owner map](#dashboard-panel-to-alert-owner-map-degraded-operations)
* [On-call runbook order](#on-call-runbook-order-checkpoint-reliability)
* [Incident handoff template](#incident-handoff-template-unresolved-reliability-issues)
* [Incident closure definition of done](#incident-closure-definition-of-done-reliability)
* [Follow-up tracking template](#follow-up-tracking-template)
* [Monthly governance signoff checklist](#monthly-governance-signoff-checklist)
* [Governance ownership RACI](#governance-ownership-raci)
* [Reliability review meeting agenda](#reliability-review-meeting-agenda-template)
* [Severity-to-channel communication policy](#severity-to-channel-communication-policy)

### Observability Stack Quickstart (Prometheus + Grafana)

Start LokLingo with auto-provisioned observability stack:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.observability.yml up -d --build
```

Access services:

- Prometheus: `http://localhost:${PROMETHEUS_HOST_PORT:-19090}`
- Grafana: `http://localhost:${GRAFANA_HOST_PORT:-13030}`
- Metrics proxy: `http://localhost:${METRICS_PROXY_HOST_PORT:-18081}`

Grafana defaults:

- Username: `${GRAFANA_ADMIN_USER:-admin}`
- Password: `${GRAFANA_ADMIN_PASSWORD:-admin}`

Auto-provisioned assets:

- Datasource: `LokLingo Prometheus`
- Dashboard: `LokLingo Reliability Overview`
- Dashboard: `LokLingo Incident Timeline Companion`
- Dashboard: `LokLingo Degraded Operations Guard`

Smoke validation:

- `sh guide/smoke-observability.sh`
- `bash guide/reliability-smoke-suite.sh --json-out guide/reliability-smoke-report.json`

Recovery snapshot command (provider policy + degraded signals):

- `bash guide/ops/reliability-recovery.sh provider-policy-snapshot`

Prometheus scrape source:

- `GET /api/v1/metrics/prometheus`
- `GET /api/v1/metrics/providers/health`

Incident lineage query workflow:

```bash
curl -s "http://localhost:28080/api/v1/metrics/lifecycle/events?correlation_id=<id>&limit=200" \
    -H "X-Internal-Token: $INTERNAL_TOKEN" | jq .
```

Important auth note:

- Internal token middleware currently accepts `X-Internal-Token` only.
- The observability compose overlay includes `loklingo-metrics-proxy`, which injects `X-Internal-Token` into scrape requests.
- Keep `INTERNAL_TOKEN` set consistently for backend and observability stack so secured scraping continues to work.

### 1) Apply analytics migrations

If `POSTGRES_DSN` is configured for backend analytics logging, apply SQL
migrations in order:

```bash
for f in backend/migrations/*.sql; do
    psql "$POSTGRES_DSN" -f "$f"
done
```

The new reliability telemetry table is created by:

* `backend/migrations/003_create_reliability_events.sql`
* `backend/migrations/004_add_reliability_retention_policy.sql`

Retention cleanup helper:

```sql
SELECT prune_reliability_events(INTERVAL '30 days');
```

Recommended: run that SQL daily from an external scheduler (system cron,
Kubernetes CronJob, CI maintenance job, etc.).

### 2) Query API-level metrics

The internal metrics endpoint now returns both:

* `reliability`: in-process counters since backend start
* `reliability_windowed.events`: Postgres-aggregated reliability events for the requested window

Example:

```bash
curl "http://localhost:28080/api/v1/metrics/ocr?window=24h" \
    -H "X-Internal-Token: $INTERNAL_TOKEN"
```

Accepted windows: `1h`, `6h`, `24h`, `7d`, `30d`.

### 3) Query dashboard SQL directly

Use:

* `backend/analytics/ocr_dashboard.sql`

This file now includes reliability-focused queries such as top failure reasons,
retry/circuit event volumes, and integration/event trends over time.

### 4) Frontend reliability severity thresholds

The readiness popover mini-widget and sparkline support configurable severity
thresholds through Vite environment variables:

* `VITE_RELIABILITY_PRESSURE_WARN`: warn threshold for pressure score (default `8`)
* `VITE_RELIABILITY_PRESSURE_CRITICAL`: critical threshold for pressure score (default `20`)

Pressure score is computed from the latest 1h metrics as:

```text
litellm.retry_attempts_total
+ litellm.circuit_opened_total
+ ocr.retry_attempts_total
+ ocr.response_rejected_total
```

Set these in the frontend environment used at build time to tune alert
sensitivity per environment (dev/staging/prod).

### 5) Checkpoint reliability counters and thresholds

The internal provider metrics endpoint includes checkpoint lifecycle counters:

```bash
curl "http://localhost:28080/api/v1/metrics/providers" \
    -H "X-Internal-Token: $INTERNAL_TOKEN"
```

Checkpoint payload fields:

* `checkpoints.hit_total`: resumed chunk translations served from persisted checkpoints
* `checkpoints.miss_total`: chunks that required fresh translation
* `checkpoints.persist_failure_total`: checkpoint write failures
* `checkpoints.clear_total`: successful terminal checkpoint cleanup runs
* `checkpoints.clear_failure_total`: failed terminal checkpoint cleanup runs

Frontend reliability panel interpretation (Checkpoint health):

* `hit_rate = hit_total / (hit_total + miss_total)`
* `persist_failure_rate = persist_failure_total / (hit_total + miss_total)`

Configurable Vite thresholds:

* `VITE_CHECKPOINT_HIT_WARN` (default `70`)
* `VITE_CHECKPOINT_HIT_CRITICAL` (default `40`)
* `VITE_CHECKPOINT_PERSIST_FAILURE_WARN` (default `5`)
* `VITE_CHECKPOINT_PERSIST_FAILURE_CRITICAL` (default `15`)

Status logic:

* Hit rate is `warn` below `VITE_CHECKPOINT_HIT_WARN` and `critical` below `VITE_CHECKPOINT_HIT_CRITICAL`.
* Persist failure rate is `warn` at or above `VITE_CHECKPOINT_PERSIST_FAILURE_WARN` and `critical` at or above `VITE_CHECKPOINT_PERSIST_FAILURE_CRITICAL`.
* Operator hint text in the UI explains which threshold is currently triggering severity.

Checkpoint troubleshooting quick-reference:

| Observed state | Likely causes | First-response actions |
| --- | --- | --- |
| Low hit rate (`warn`/`critical`) with low persist failures | New workload patterns, highly unique documents, changed source text normalization, short-lived jobs not reused | Verify request mix changed as expected; compare with previous window (`24h` vs `7d`); confirm chunk key inputs (source/target/text) are stable between retries/replays |
| High persist failure rate | Redis connectivity/latency issues, keyspace pressure, transient write errors | Check Redis health and latency; inspect backend logs for `chunk_checkpoint_persist_failed`; confirm Redis memory/eviction policy and network path |
| High cleanup failure count | Redis scan/delete failures during terminal cleanup, transient Redis outages | Inspect logs for `chunk_checkpoint_cleanup_failed`; validate Redis availability during worker completion bursts; re-check retry/dead-letter spikes that increase cleanup volume |
| Cleanup failures rising while job success remains stable | Cleanup path degraded but translation path still functional | Prioritize Redis maintenance and alerting; keep watching `clear_failure_total` delta rate; verify no growth trend in stale checkpoint keys |
| Persist failures + fallback/degradation events increase together | Shared infrastructure stress (Redis + provider/network), overloaded worker conditions | Correlate with provider timeout counters and retry events; reduce concurrency temporarily (`TRANSLATE_CONCURRENCY`) if needed; inspect infra saturation |

Alerting suggestions (starting points):

* Rate-of-change alert: `persist_failure_total` delta > `5` within `5m` (warn), > `15` within `5m` (critical).
* Rate-of-change alert: `clear_failure_total` delta > `3` within `10m` (warn), > `10` within `10m` (critical).
* Ratio alert: `persist_failure_rate` >= `VITE_CHECKPOINT_PERSIST_FAILURE_WARN` for two consecutive evaluation windows.
* Coverage alert: `hit_rate` below `VITE_CHECKPOINT_HIT_CRITICAL` for two consecutive evaluation windows.
* Correlation alert: checkpoint failure deltas rising together with provider timeout deltas (same window) to prioritize infrastructure triage.
* Fallback-budget exhaustion (OCR chain):
    * warning when `increase(loklingo_ocr_provider_fallback_budget_exhausted_total[10m]) > 0 and increase(loklingo_ocr_provider_fallback_budget_exhausted_total[10m]) < 3`
    * critical when `increase(loklingo_ocr_provider_fallback_budget_exhausted_total[10m]) >= 3`
    * use `for: 5m` to reduce false positives from one-off bursts

Prometheus rule snippet (drop-in starting point):

```yaml
groups:
    - name: loklingo-ocr-fallback-budget
        rules:
            - alert: LokLingoOCRFallbackBudgetExhausted
                expr: increase(loklingo_ocr_provider_fallback_budget_exhausted_total[10m]) > 0 and increase(loklingo_ocr_provider_fallback_budget_exhausted_total[10m]) < 3
                for: 5m
                labels:
                    severity: warning
                    service: loklingo-backend
                annotations:
                    summary: OCR provider fallback budget exhausted
                    description: At least one OCR request exhausted fallback budget in the last 10m.

            - alert: LokLingoOCRFallbackBudgetExhausted
                expr: increase(loklingo_ocr_provider_fallback_budget_exhausted_total[10m]) >= 3
                for: 5m
                labels:
                    severity: critical
                    service: loklingo-backend
                annotations:
                    summary: OCR provider fallback budget repeatedly exhausted
                    description: Repeated fallback-budget exhaustion indicates OCR provider-chain instability.
```

Timeout-storm guardrails (smoke + alert alignment):

* `timeouts_delta_10m > 8` and `retries_delta_10m > 80` should be treated as critical overload behavior.
* Reliability smoke suite knobs:
    * `STORM_TIMEOUT_DELTA_MAX` (default `8`)
    * `STORM_RETRY_DELTA_MAX` (default `80`)
    * `STORM_RECOVERY_DEPTH_THRESHOLD` (default `6`)

Treat these values as environment-specific baselines; tune thresholds using your normal traffic profile and window size.

Alert-to-response mapping (degraded/adaptive path):

* `LokLingoTimeoutStormDetected`
    * First checks: compare `loklingo_pipeline_timeouts_total` and `loklingo_pipeline_retries_total` deltas in the degraded-operations dashboard.
    * Immediate action: reduce translation pressure and verify queue recovery against `loklingo_queue_depth`.
    * Escalation: page on-call and open incident channel immediately.
* `LokLingoAdaptiveClampFrequent`
    * First checks: confirm clamp share and queue depth trend in the degraded-operations dashboard.
    * Immediate action: inspect worker saturation and recent provider timeout spikes before changing concurrency.
    * Escalation: warn service owner; promote if queue depth does not recover within one evaluation window.
* `LokLingoDegradedModeSpike`
    * First checks: break down render fallback vs low OCR confidence in the degraded-operations dashboard.
    * Immediate action: distinguish provider/network stress from OCR quality drift.
    * Escalation: involve OCR owner if low-confidence dominates; involve translation owner if timeout/retry signals rise together.
* `LokLingoRenderFallbackSpike`
    * First checks: correlate fallback increase with recent render latency and output-format changes.
    * Immediate action: verify image/layout rendering path health and confirm passthrough fallback is preserving output delivery.
    * Escalation: treat as warning unless export success or degraded total also trends upward.
* `LokLingoRetryBacklogHigh` or `LokLingoRetryBacklogCritical`
    * First checks: inspect retry backlog, dead-letter volume, and provider timeout reason metrics in the same window.
    * Immediate action: determine whether backlog is provider-driven or queue-capacity driven before replaying or increasing concurrency.
    * Escalation: critical backlog should follow standard incident escalation if accompanied by clamp or timeout-storm alerts.
* `LokLingoOCRFallbackBudgetExhausted`
    * First checks: measure `increase(loklingo_ocr_provider_fallback_budget_exhausted_total[10m])` and compare with provider timeout/failure deltas.
    * Immediate action: verify `OCR_PROVIDER_TIMEOUT_SECONDS` and `OCR_MAX_FALLBACKS` are still aligned with current provider latency profile.
    * Escalation: page OCR/service owner at critical threshold; treat repeated exhaustion as degraded OCR availability.

Alert-to-command mapping (reliability-recovery script):

* `LokLingoQueueDepthHigh` or `LokLingoQueueDepthCritical`
    * `bash guide/ops/reliability-recovery.sh queue-status`
    * `bash guide/ops/reliability-recovery.sh provider-policy-snapshot`
    * `bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>`
* `LokLingoStuckJobsDetected`
    * `bash guide/ops/reliability-recovery.sh queue-status`
    * `bash guide/ops/reliability-recovery.sh list-dead 100`
    * `bash guide/ops/reliability-recovery.sh provider-history`
* `LokLingoDeadLetterGrowth`
    * `bash guide/ops/reliability-recovery.sh list-dead 100`
    * `bash guide/ops/reliability-recovery.sh replay-dead-all 50`
    * `bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>`
* `LokLingoChunkSuccessDrop`
    * `bash guide/ops/reliability-recovery.sh provider-history`
    * `bash guide/ops/reliability-recovery.sh provider-policy-snapshot`
    * `bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>`
* `LokLingoExportSuccessDrop`
    * `bash guide/ops/reliability-recovery.sh queue-status`
    * `bash guide/ops/reliability-recovery.sh provider-history`
    * `bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>`
* `LokLingoProviderTimeoutSpike`
    * `bash guide/ops/reliability-recovery.sh provider-history`
    * `bash guide/ops/reliability-recovery.sh provider-policy-snapshot`
    * `bash guide/ops/reliability-recovery.sh queue-status`
* `LokLingoTimeoutStormDetected`
    * `bash guide/ops/reliability-recovery.sh queue-status`
    * `bash guide/ops/reliability-recovery.sh provider-policy-snapshot`
    * `bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>`
* `LokLingoAdaptiveClampFrequent`
    * `bash guide/ops/reliability-recovery.sh queue-status`
    * `bash guide/ops/reliability-recovery.sh provider-history`
    * `bash guide/ops/reliability-recovery.sh provider-policy-snapshot`
* `LokLingoDegradedModeSpike`
    * `bash guide/ops/reliability-recovery.sh provider-policy-snapshot`
    * `bash guide/ops/reliability-recovery.sh provider-history`
    * `bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>`
* `LokLingoRenderFallbackSpike`
    * `bash guide/ops/reliability-recovery.sh provider-policy-snapshot`
    * `bash guide/ops/reliability-recovery.sh queue-status`
* `LokLingoRetryBacklogHigh` or `LokLingoRetryBacklogCritical`
    * `bash guide/ops/reliability-recovery.sh queue-status`
    * `bash guide/ops/reliability-recovery.sh list-dead 100`
    * `bash guide/ops/reliability-recovery.sh replay-dead-all 50`
* `LokLingoOCRFallbackBudgetExhausted`
    * `bash guide/ops/reliability-recovery.sh provider-history`
    * `bash guide/ops/reliability-recovery.sh provider-policy-snapshot`
    * `bash guide/ops/reliability-recovery.sh queue-status`

After capturing an incident JSON packet, generate a compact handoff brief:

* `python3 guide/ops/generate-incident-brief.py <incident-snapshot.json> <brief-output.md>`

Severity response matrix (command bundles):

| Severity | Typical alerts | Ordered command bundle |
| --- | --- | --- |
| `warning` | `LokLingoQueueDepthHigh`, `LokLingoChunkSuccessDrop`, `LokLingoExportSuccessDrop`, `LokLingoProviderTimeoutSpike`, `LokLingoAdaptiveClampFrequent`, `LokLingoDegradedModeSpike`, `LokLingoRenderFallbackSpike`, `LokLingoOCRFallbackBudgetExhausted` | 1. `bash guide/ops/reliability-recovery.sh queue-status` 2. `bash guide/ops/reliability-recovery.sh provider-policy-snapshot` 3. `bash guide/ops/reliability-recovery.sh provider-history` |
| `critical` | `LokLingoQueueDepthCritical`, `LokLingoDeadLetterGrowth`, `LokLingoRetryBacklogCritical`, `LokLingoTimeoutStormDetected`, `LokLingoOCRFallbackBudgetExhausted` | 1. `bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>` 2. `bash guide/ops/reliability-recovery.sh validate-incident-snapshot <incident-snapshot.json>` 3. `bash guide/ops/reliability-recovery.sh generate-incident-brief <incident-snapshot.json> <brief-output.md>` |
| `critical` with replay required | `LokLingoDeadLetterGrowth`, `LokLingoRetryBacklogCritical` after provider-driven triage is ruled out | 1. `bash guide/ops/reliability-recovery.sh list-dead 100` 2. `bash guide/ops/reliability-recovery.sh replay-dead-all 50` 3. Re-run `bash guide/ops/reliability-recovery.sh queue-status` |

### Dashboard panel-to-alert-owner map (degraded operations)

| Dashboard panel | Primary alert(s) | First owner | Backup owner |
| --- | --- | --- | --- |
| `Degraded Events Total` | `LokLingoDegradedModeSpike` | Reliability Lead (Platform) | Backend On-Call Owner |
| `Degradation Signals (10m)` | `LokLingoRenderFallbackSpike`, `LokLingoDegradedModeSpike` | OCR Service Owner + Rendering Owner | Reliability Lead (Platform) |
| `Adaptive Concurrency Decisions (10m)` | `LokLingoAdaptiveClampFrequent` | Platform/SRE Owner | Backend On-Call Owner |
| `Timeout Storm Guard (10m)` | `LokLingoTimeoutStormDetected` | Translation Service Owner | Platform/SRE Owner |
| `OCR Fallback Budget Exhaustion (10m)` | `LokLingoOCRFallbackBudgetExhausted` | OCR Service Owner | Reliability Lead (Platform) |
| `Queue Pressure` | `LokLingoQueueDepthHigh`, `LokLingoQueueDepthCritical`, `LokLingoRetryBacklogHigh`, `LokLingoRetryBacklogCritical` | Platform/SRE Owner | Backend On-Call Owner |
| `Provider Policy Score` | `LokLingoProviderTimeoutSpike`, `LokLingoTimeoutStormDetected` | Translation Service Owner | Reliability Lead (Platform) |
| `Clamp Share (15m)` | `LokLingoAdaptiveClampFrequent` | Platform/SRE Owner | Reliability Lead (Platform) |
| `Degraded Mode Alert Guard (10m)` | `LokLingoDegradedModeSpike` | Reliability Lead (Platform) | Backend On-Call Owner |

On-call runbook order (checkpoint reliability):

1. Confirm scope and recency
    * Check `checkpoints` counters/rates in the same window where alerts fired.
    * Verify frontend operator hint severity and timestamp match backend metric query time.
2. Validate Redis health first
    * Check Redis availability/latency and recent errors.
    * Search backend logs for `chunk_checkpoint_persist_failed` and `chunk_checkpoint_cleanup_failed`.
3. Correlate translation-path pressure
    * Compare checkpoint failure deltas with provider timeout/retry deltas.
    * If both rise together, treat as shared infra stress rather than isolated checkpoint logic.
4. Apply low-risk mitigation
    * Reduce worker pressure temporarily (`TRANSLATE_CONCURRENCY`) if saturation is suspected.
    * Keep write/read paths active unless cleanup failure is cascading into broader queue instability.
5. Verify recovery and close-out
    * Confirm failure deltas flatten and hit rate returns toward baseline.
    * Record incident window, suspected root cause, and any threshold tuning updates.

Known-good baseline example (reference only):

For a stable environment with moderate retry/replay activity, a healthy 24h window
often looks like:

* `hit_rate`: `75%` to `95%`
* `persist_failure_rate`: `< 1%`
* `clear_failure_total` delta: `0` to `2` per `24h`
* `clear_total` roughly tracks terminal PDF job volume

Use this as a comparison point, not a hard SLA. Recalibrate per environment,
traffic mix, retry frequency, and document similarity profile.

Threshold change management (when to retune):

Retune checkpoint thresholds after any material operational shift, including:

* Workload shape changes (new document domains, larger PDFs, different language mix)
* Translation provider/model changes (latency, timeout, or retry profile changes)
* Retry/chunk policy updates (`TRANSLATE_CONCURRENCY`, chunk word ranges, timeout tuning)
* Redis topology/capacity/network changes affecting write/cleanup behavior

Recommended tuning workflow:

1. Capture baseline metrics for at least one stable business cycle (for example, 7 days).
2. Propose threshold deltas in staging first and compare alert frequency vs. incident relevance.
3. Roll to production with explicit before/after tracking on false-positive and missed-alert rates.
4. Record the rationale and effective date in ops notes/runbooks.

Reliability release checklist (before merge/deploy):

1. Backend correctness
    * Run: `cd backend && go test ./...`
    * Confirm checkpoint lifecycle tests pass (resume, terminal cleanup, cleanup-failure metric path).
2. Frontend correctness
    * Run: `cd frontend && npm run test`
    * Run: `cd frontend && npm run build`
    * Verify reliability panel renders checkpoint rates, severity badges, and operator hint timestamp.
3. Telemetry contract verification
    * Query: `GET /api/v1/metrics/providers` with `X-Internal-Token`.
    * Confirm `checkpoints` object includes numeric fields:
      `hit_total`, `miss_total`, `persist_failure_total`, `clear_total`, `clear_failure_total`.
4. Threshold/config review
    * Confirm intended values for:
      `VITE_CHECKPOINT_HIT_WARN`, `VITE_CHECKPOINT_HIT_CRITICAL`,
      `VITE_CHECKPOINT_PERSIST_FAILURE_WARN`, `VITE_CHECKPOINT_PERSIST_FAILURE_CRITICAL`.
    * Validate alerting thresholds align with current environment baseline.
5. Runbook/docs parity
    * Update README/runbook sections when semantics, counters, thresholds, or operator hints change.
    * Record release note summary of reliability-impacting changes.

Post-deploy verification (first 30 minutes):

1. Confirm telemetry freshness
    * Verify reliability panel "Updated" timestamp advances normally.
    * Query `/api/v1/metrics/providers` twice (for example, 5 minutes apart) and confirm counters are changing logically.
2. Check checkpoint delta health
    * Ensure `persist_failure_total` and `clear_failure_total` deltas are near expected baseline.
    * Confirm `hit_rate` is not trending below your configured critical threshold window-over-window.
3. Watch alert noise vs signal
    * Review fired alerts for false positives during rollout.
    * Ensure warn/critical transitions align with actual metric movement, not telemetry staleness.
4. Correlate with provider reliability
    * If checkpoint failures rise, compare provider timeout/retry deltas in the same interval.
    * Escalate as shared infrastructure stress when both move together.
5. Rollback/escalation criteria
    * Trigger rollback/escalation if checkpoint failure deltas remain elevated across consecutive windows or user-visible latency/error symptoms increase.
    * Capture incident notes with deploy SHA/time to preserve attribution.

Weekly reliability review checklist:

1. Baseline drift review
    * Compare this week vs previous week for `hit_rate`, `persist_failure_rate`, and failure deltas.
    * Flag sustained shifts rather than one-off spikes.
2. Alert quality audit
    * Count false positives vs actionable alerts for checkpoint-related rules.
    * Adjust thresholds only when alert quality degrades over multiple review periods.
3. Correlation review
    * Check whether checkpoint failures repeatedly co-move with provider timeout/retry pressure.
    * If yes, prioritize shared infra fixes before logic-level tuning.
4. Capacity and policy review
    * Re-evaluate `TRANSLATE_CONCURRENCY`, chunk sizing, and Redis capacity against current traffic.
    * Confirm cleanup success trend (`clear_total` vs `clear_failure_total`) remains healthy.
5. Documentation parity check
    * Update runbook thresholds, known-good baselines, and release checklist notes when operational behavior changes.
    * Record decisions and effective dates for future incident attribution.

### Weekly reliability KPI scorecard

Use this scorecard during weekly reviews to classify operational health quickly.

| KPI | Green | Yellow | Red |
| --- | --- | --- | --- |
| Checkpoint hit rate (weekly median) | `>= 75%` | `60%-74.9%` | `< 60%` |
| Persist failure rate (weekly p95) | `< 1%` | `1%-4.9%` | `>= 5%` |
| Cleanup failure delta (weekly total) | `0-2` | `3-6` | `>= 7` |
| Provider timeout trend (week over week) | flat or down | up to `+10%` | `> +10%` |
| Alert quality (actionable / total) | `>= 70%` actionable | `40%-69.9%` actionable | `< 40%` actionable |

Scorecard use notes:

1. Evaluate all KPI rows using identical window length and sampling cadence as the prior comparison week.
2. Treat any `Red` KPI as a mandatory review item with owner and due date.
3. If two or more KPIs are `Yellow` for consecutive weeks, schedule threshold/capacity review.
4. Rebaseline ranges when sustained workload shape changes occur and document rationale in change-log entries.

### Reliability review meeting agenda template

Use this agenda for weekly reliability reviews; every fourth weekly session can be expanded into the monthly retro format.

1. Open and context (5 minutes)
    * Confirm review window and attendees.
    * Confirm incident volume and any unresolved major incidents.
2. KPI scorecard walk-through (10 minutes)
    * Review each row from the weekly KPI scorecard and assign status (`Green`/`Yellow`/`Red`).
    * Highlight any week-over-week movement and likely drivers.
3. Incident closure quality review (10 minutes)
    * Review incidents closed in window using the closure quality scorecard.
    * Capture recurring documentation or handoff quality gaps.
4. Follow-up SLA and backlog review (10 minutes)
    * Check overdue follow-up items against SLA defaults.
    * Reconfirm owners, due dates, and blockers for open actions.
5. Retro input capture (10 minutes)
    * Record top contributing factors, alert quality trends, and threshold/capacity proposals.
    * Prepare carry-forward notes for monthly retro template fields.
6. Decisions and action log (5 minutes)
    * Finalize action items with owner + due date.
    * Record changes requiring runbook/checklist updates.

Agenda outputs (required):

1. Updated weekly KPI scorecard status snapshot.
2. Action list with owners and due dates.
3. Retro input notes ready for monthly reliability retro template.

Monthly reliability retro template:

Use this lightweight template to keep monthly reliability reviews consistent:

* Review window: `<YYYY-MM-DD .. YYYY-MM-DD>`
* Traffic summary: total translation jobs, PDF share, retry-heavy workload share
* Checkpoint metrics summary:
    * median `hit_rate`
    * p95 `persist_failure_rate`
    * total `clear_failure_total` delta
* Provider reliability summary:
    * timeout reason distribution (`network_timeout`, `gateway_timeout`, etc.)
    * failover frequency trend
* Alert quality:
    * checkpoint alerts fired
    * actionable incidents
    * false positives
* Top 3 contributing factors (infra, workload, config, code)
* Decisions this retro:
    * threshold changes
    * capacity/policy changes
    * backlog items created
* Owners and due dates for follow-up actions

Suggested retro prompts:

1. Which alerts provided early signal vs noise?
2. Did checkpoint failures correlate with provider/network instability or standalone Redis issues?
3. Did recent deploys shift baseline behavior (good or bad)?
4. What one change would most reduce next month’s incident risk?

Metric definitions glossary (review consistency):

* `delta` (counter delta): value at end of window minus value at start of window.
* `hit_rate`: `hit_total / (hit_total + miss_total)` for the same evaluation window.
* `persist_failure_rate`: `persist_failure_total / (hit_total + miss_total)` for the same window.
* `median hit_rate`: 50th percentile of sampled hit-rate observations across the review window.
* `p95 persist_failure_rate`: 95th percentile of sampled persist-failure-rate observations across the review window.
* `alert noise`: alerts that fired but did not require intervention.
* `actionable incident`: alert/event that required operator action (mitigation, rollback, or escalation).

When comparing windows, use identical window lengths and sampling cadence to avoid skewed percentile comparisons.

Data collection cadence recommendation:

* Snapshot reliability metrics every `5m` for operational dashboards and percentile review inputs.
* Keep raw snapshots for at least `35d` so monthly retros can include overlap and seasonality checks.
* For low-traffic environments, use `10m` cadence to reduce noise while preserving trend visibility.
* If cadence changes, annotate the change date in ops notes and avoid cross-cadence percentile comparisons.

Weekly review query checklist (example Monday routine):

1. Pull checkpoint/provider snapshot (start of review):

```bash
curl "http://localhost:28080/api/v1/metrics/providers" \
    -H "X-Internal-Token: $INTERNAL_TOKEN"
```

2. Pull OCR/reliability windows for comparison:

```bash
curl "http://localhost:28080/api/v1/metrics/ocr?window=24h" \
    -H "X-Internal-Token: $INTERNAL_TOKEN"

curl "http://localhost:28080/api/v1/metrics/ocr?window=7d" \
    -H "X-Internal-Token: $INTERNAL_TOKEN"
```

3. Compare against prior week records:

* `hit_rate`, `persist_failure_rate`, and `clear_failure_total` deltas
* provider timeout reason distribution changes
* alert counts: actionable vs false-positive

4. Record outcomes in weekly review notes:

* baseline drift observed (yes/no)
* threshold retune needed (yes/no)
* top action item + owner + due date

### Incident handoff template (unresolved reliability issues)

Use this handoff block between shifts to avoid context loss:

* Incident title / ID:
* Current severity (`Sev 1`/`Sev 2`/`Sev 3`/`Sev 4`) and user impact summary:
* Detection time and latest update time:
* Key metric deltas observed:
    * `hit_rate`
    * `persist_failure_total` delta
    * `clear_failure_total` delta
    * provider timeout/retry deltas
* Checks completed this shift:
    * Redis health
    * provider health/timeouts
    * worker concurrency/chunk settings review
* Mitigations already applied:
* Remaining hypotheses to validate:
* Next 3 actions for incoming shift (ordered):
* Escalation status (who was paged, when):
* Relevant links (dashboards, logs, deploy SHA/PR):

### Incident evidence checklist

Use this checklist to keep incident records consistent and reviewable.

1. Metrics evidence
    * Include at least two comparable windows for key counters/rates (`hit_rate`, persist/cleanup failure deltas, provider timeout/retry deltas).
    * Attach timestamps and query window lengths for every screenshot/export.
2. Log evidence
    * Capture representative backend/OCR/provider errors with time range and correlation identifiers when available.
    * Include both first observed error and latest observed error samples.
3. Deploy and config evidence
    * Record deploy SHA/version, rollout start time, and any rollback/redeploy actions.
    * Note relevant config values changed during the incident (for example thresholds/concurrency/timeouts) with before/after values.
4. User impact evidence
    * Provide scope estimate (affected routes/workflows, estimated volume, regions/tenants if applicable).
    * Include at least one sanitized user-impact sample (error response, latency spike, or failed job example).
5. Decision trail evidence
    * Record mitigation decisions in time order: what was attempted, by whom, and observed effect.
    * Mark open hypotheses and what evidence would confirm or reject each one.
6. Handoff and closure readiness
    * Before shift handoff, verify this checklist is attached to the handoff template.
    * Before closure, verify this checklist supports every item in the closure definition of done.

### Incident artifact bundle template

Use this structure to package incident evidence for handoff, closure review, and postmortem work.

Suggested bundle directory name:

* `incidents/<YYYY-MM-DD>_<INCIDENT_ID>_<severity>`

Minimum bundle contents:

1. `summary.md`
    * Incident scope, impact window, current status, and owner list.
2. `timeline.md`
    * Chronological actions/decisions with UTC timestamps.
3. `metrics/`
    * Exported metric snapshots/charts with explicit window length in filename.
4. `logs/`
    * Sanitized log extracts for first observed, peak, and latest observed symptoms.
5. `deploy/`
    * Deploy SHA/version notes, rollback/redeploy records, and config diffs (before/after).
6. `impact/`
    * Sanitized user-impact samples and scope estimates.
7. `handoff.md`
    * Latest shift handoff block when incident is unresolved.
8. `closure.md`
    * Closure checklist mapping and follow-up actions (owner + due date).

Artifact naming guidance:

1. Include UTC timestamp prefix for ordered files, for example `2026-05-11T14-30Z_metrics_24h.json`.
2. Include window/cadence in metric artifact names to avoid mismatched comparisons.
3. Keep all sensitive identifiers redacted before sharing bundles across broader channels.

### Incident closure definition of done (reliability)

Close a reliability incident only when all items below are true:

1. User impact is resolved
    * No ongoing customer-facing error/latency symptoms attributable to the incident.
2. Metrics are stable across consecutive windows
    * `persist_failure_total` and `clear_failure_total` deltas returned near baseline.
    * `hit_rate` recovered above environment-specific critical threshold and not degrading.
3. Root cause is documented (or narrowed with explicit remaining unknowns)
    * Include evidence: metric snapshots, logs, and deploy/config correlation.
4. Mitigation status is explicit
    * Temporary mitigations are either rolled back or recorded with owner and expiry date.
5. Follow-up actions are tracked
    * At least one prevention task exists when the issue exposed systemic risk.
    * Owners and due dates are assigned.
6. Runbook/docs updated if needed
    * Thresholds, troubleshooting steps, or checklists are updated when learnings changed operational practice.

### Closure signoff checklist

Run this checklist at the moment the incident transitions to `resolved`.

1. Confirm definition-of-done coverage
    * Validate each item in the closure definition has explicit evidence links.
2. Confirm state and timestamps
    * Record the final status change time, confirmation window, and the timestamp of the closure update.
3. Confirm owners and due dates
    * Ensure every follow-up action has one accountable owner and due date.
4. Confirm communication completeness
    * Verify all required channels received the closure update and next-review context.
5. Confirm watchback start
    * Start the post-closure watchback window and log the owner for recurrence monitoring.

Signoff note starter:

```text
[Reliability Incident Update] <INCIDENT_ID>
Status: resolved
Closure signoff: complete
Definition-of-done check: <complete|exception>
Watchback owner: <name/role>
Follow-up tracker: <artifact link>
```

Signoff exception note (use only when closure signoff is not complete):

```text
[Reliability Incident Update] <INCIDENT_ID>
Status: monitoring
Closure signoff: exception
Open signoff gap: <single missing item>
Gap owner: <name/role>
Target signoff time (UTC): <time>
```

### Closure quality scorecard

Use this scorecard to standardize incident closure reviews and identify weak handoff/record quality.

| Dimension | Pass criteria | Score |
| --- | --- | --- |
| Evidence completeness | Metrics, logs, deploy/config, and user-impact evidence are all attached and referenced | `0` or `1` |
| Timeline completeness | Timeline includes detection, mitigation, escalation, and stabilization milestones with UTC timestamps | `0` or `1` |
| Decision traceability | Major decisions include owner, rationale, and observed outcome | `0` or `1` |
| Handoff quality | Incoming shift had clear next actions, open hypotheses, and required links | `0` or `1` |
| Follow-up assignment quality | Every follow-up item has an owner and due date aligned with SLA defaults (or approved override) | `0` or `1` |
| Runbook/change parity | Relevant runbook/checklist/threshold docs updated or explicitly marked "no change required" | `0` or `1` |

Score interpretation:

1. `6/6`: strong closure quality; no documentation follow-up required.
2. `4-5/6`: acceptable closure; create at least one documentation/process improvement task.
3. `<=3/6`: weak closure quality; require service owner review before final closure acceptance.

### Post-incident follow-up SLA defaults

Use these default SLAs for follow-up actions unless an incident commander or service owner explicitly overrides them.

| Follow-up action type | Default owner | Target due date from incident close | Escalate if overdue by |
| --- | --- | --- | --- |
| Immediate fix verification (production behavior remains stable) | On-call engineer + service owner | 1 business day | 1 business day |
| Hardening task (code/path resilience improvement) | Service owner (backend or frontend) | 5 business days | 2 business days |
| Alert/threshold retune review | SRE/platform owner + service owner | 5 business days | 2 business days |
| Runbook/process update | On-call engineer + service owner reviewer | 3 business days | 2 business days |
| Postmortem completion (if required) | Incident commander | 10 business days | 3 business days |
| Capacity or infra follow-up (Redis/network/provider) | SRE/platform owner | 10 business days | 3 business days |

SLA operating notes:

1. Record each follow-up item with owner and due date in `closure.md` within the incident artifact bundle.
2. If ownership spans teams, designate one accountable owner and list collaborators separately.
3. When SLA overrides are applied, include rationale and approver in the incident record.

### Follow-up tracking template

Use this template in `closure.md` to track post-incident actions consistently.

| Item ID | Action summary | Action type | Accountable owner | Collaborators | Due date (UTC) | SLA default | Status | Last update (UTC) | Overdue escalation owner |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `F1` | `<one-line action>` | `<verification|hardening|retune|runbook|postmortem|capacity>` | `<name/role>` | `<name/role or n/a>` | `<YYYY-MM-DD>` | `<for example 5 business days>` | `<open|in_progress|blocked|done>` | `<YYYY-MM-DD HH:MM>` | `<name/role>` |

Tracking notes:

1. Use one row per follow-up action and keep `Item ID` stable for status updates.
2. If status is `blocked`, include blocker reason and unblock owner directly below the table.
3. Escalate overdue items based on the SLA default table unless an approved override is documented.
4. Mark `done` only when evidence links are attached (PR, deploy note, or runbook update).

### Follow-up review cadence

Use this cadence to keep follow-up actions moving after incident closure.

1. Daily (while any item is not `done`)
    * Review all `blocked` or overdue items and post owner updates in the incident follow-up thread.
2. Twice-weekly service-owner check
    * Confirm due dates are still realistic and reassign ownership if capacity changed.
3. Weekly reliability review checkpoint
    * Bring unresolved follow-up items to the reliability review meeting with current status and blocker notes.
4. Overdue escalation trigger
    * If an item exceeds the SLA overdue window, escalate to the default escalation owner and incident commander.

Cadence note starter:

```text
Follow-up review window (UTC): <start-end>
Open items: <count>
Blocked items: <count>
Overdue items: <count>
Escalations raised: <count + owner>
```

### Follow-up completion evidence checklist

Use this checklist before changing any follow-up item status to `done`.

1. Confirm deliverable evidence
    * Attach the primary proof link (PR, deploy note, runbook update, or postmortem section).
2. Confirm production impact check
    * State whether production behavior is unchanged, improved, or intentionally modified.
3. Confirm validation window
    * Record the time window used to validate outcomes and the key metrics/logs reviewed.
4. Confirm ownership closeout
    * Ensure accountable owner signs off and collaborator acknowledgments are captured when required.
5. Confirm tracker parity
    * Update `closure.md` item row and any related weekly review artifact with matching status.

Completion evidence note starter:

```text
Follow-up item: <F#>
Completion status: done
Primary evidence link: <url/path>
Validation window (UTC): <start-end>
Outcome summary: <one line>
Owner signoff: <name/role>
```

### Follow-up status taxonomy

Use these status definitions for every row in the follow-up tracking table.

| Status | When to use | Required update fields |
| --- | --- | --- |
| `open` | Item exists but execution has not started | owner, due date, action summary |
| `in_progress` | Owner is actively executing the action | last update timestamp, current step, next checkpoint |
| `blocked` | Progress cannot continue without external input or dependency resolution | blocker reason, unblock owner, target unblock time |
| `done` | Completion evidence checklist is satisfied and owner signoff is recorded | primary evidence link, validation window, outcome summary |

Transition rules:

1. `open -> in_progress`: only after owner confirms execution start and next checkpoint time.
2. `in_progress -> blocked`: include explicit blocker reason and designated unblock owner.
3. `blocked -> in_progress`: record what changed to remove the blocker.
4. `in_progress -> done`: allowed only when completion evidence note is attached.
5. `done -> in_progress`: allowed if regression is detected; include regression trigger and reopen timestamp.

### Blocked-item escalation playbook

Use this playbook when follow-up items remain in `blocked` status.

| Blocked duration vs due date | Escalation level | Required action |
| --- | --- | --- |
| Before due date and <24h blocked | `L0` | Keep owner-level handling; post blocker detail and next checkpoint time |
| >=24h blocked or due date at risk | `L1` | Escalate to service owner; assign unblock owner and recovery plan |
| Due date missed by <=1 business day | `L2` | Escalate to service owner + incident commander; require revised due date |
| Due date missed by >1 business day | `L3` | Escalate to service owner + incident commander + SRE/platform owner; require mitigation or scope reduction decision |

Escalation handling rules:

1. Every escalation update must include item ID, blocker reason, unblock owner, and next checkpoint time.
2. For `L2` and `L3`, include whether customer-risk or release-risk is increasing.
3. Clear escalation only after the item transitions out of `blocked` and one confirmation update is posted.

Blocked escalation message starter:

```text
Follow-up item: <F#>
Status: blocked
Escalation level: <L0|L1|L2|L3>
Blocker reason: <one line>
Unblock owner: <name/role>
Risk note: <customer|release|none>
Next checkpoint (UTC): <time>
```

### Blocked escalation exception matrix

Use this matrix only when normal blocked escalation timing must be temporarily overridden.

| Exception type | When allowed | Mandatory approver | Mandatory expiry field |
| --- | --- | --- | --- |
| Approved pause | Work is intentionally paused for a documented business or operational reason | Service owner | Pause expiry (UTC) |
| Dependency freeze | Required external dependency team confirms a freeze period | Service owner + dependency owner | Dependency freeze end (UTC) |
| Release blackout window | Org or platform release blackout blocks execution | Service owner + SRE/platform owner | Blackout end (UTC) |

Exception control rules:

1. Every exception entry must include approver name, approval time, and expiry time.
2. Exceptions do not cancel follow-up ownership; owner remains accountable for reactivation at expiry.
3. If expiry is reached without unblock progress, resume normal blocked escalation at `L1` or current higher level.
4. Extend exceptions only with a new approval entry; never edit old entries in place.

Blocked exception note starter:

```text
Follow-up item: <F#>
Status: blocked
Exception type: <approved_pause|dependency_freeze|release_blackout>
Approver: <name/role>
Approval time (UTC): <time>
Exception expiry (UTC): <time>
Reactivation owner: <name/role>
```

### Exception expiry sweep checklist

Use this checklist to prevent stale blocked exceptions from remaining active past expiry.

1. Daily sweep
    * Review all active blocked exceptions and compare current time against each exception expiry.
2. Shift handoff sweep
    * Confirm whether any exception will expire before the next handoff window and flag it explicitly.
3. Expired exception action
    * If an exception is expired, reactivate the follow-up item and resume blocked escalation at `L1` or the higher active level.
4. Owner confirmation
    * Require reactivation owner acknowledgment for every expired exception within the same update cycle.
5. Exception extension control
    * Allow extension only with new approver entry, new expiry timestamp, and reason for extension.

Expiry sweep note starter:

```text
Sweep time (UTC): <time>
Active exceptions reviewed: <count>
Expired exceptions found: <count>
Reactivated items: <F# list>
Extensions approved: <F# list or none>
Next sweep by (UTC): <time>
```

### Weekly exception audit checklist

Use this checklist before each reliability review meeting to keep exception usage healthy.

1. Measure exception volume
    * Count active exceptions by type (`approved_pause`, `dependency_freeze`, `release_blackout`).
2. Measure extension rate
    * Count how many exceptions were extended at least once during the audit window.
3. Measure reactivation misses
    * Count items where expiry passed without same-cycle reactivation confirmation.
4. Review aging profile
    * Flag exceptions older than one week and confirm whether continuation is still justified.
5. Record corrective actions
    * Assign owners for reducing repeated extensions or reactivation misses.

Weekly exception audit note starter:

```text
Audit window (UTC): <start-end>
Active exceptions by type: <counts>
Extension rate: <extended/total>
Reactivation misses: <count>
Aging exceptions (>7d): <F# list or none>
Corrective actions: <owner + due date>
```

### Monthly exception trend review

Use this review to roll weekly exception audits into monthly governance and target setting.

Monthly targets:

1. Extension rate target
    * Keep monthly extension rate at or below 20 percent of total exceptions.
2. Reactivation miss target
    * Keep reactivation misses at 0 for expired exceptions.
3. Aging exception target
    * Keep exceptions older than 14 days at 0 unless explicitly approved by service owner and SRE/platform owner.

Monthly trigger thresholds:

1. Extension rate exceeds 30 percent
    * Open a corrective action plan with owner and due date.
2. Any reactivation miss is detected
    * Review sweep execution and handoff process within one business day.
3. More than 2 aging exceptions (>14 days)
    * Escalate in reliability review and require scope reduction or unblock plan.

Monthly trend review note starter:

```text
Review month: <YYYY-MM>
Total exceptions: <count>
Extension rate: <percent>
Reactivation misses: <count>
Aging exceptions (>14d): <count>
Threshold breaches: <list or none>
Corrective action owners: <name + due date>
```

### Exception reduction action plan template

Use this template when monthly trigger thresholds are breached and corrective execution is required.

Plan fields:

1. Plan owner
    * Name one accountable owner for execution and reporting.
2. Hypothesis
    * State why the intervention should reduce exception rate or reactivation misses.
3. Intervention
    * List concrete changes to process, ownership flow, or dependency handling.
4. Expected metric impact
    * Define expected changes in extension rate, reactivation misses, or aging exceptions.
5. Verification window
    * Set start and end timestamps for validating intervention results.
6. Rollback trigger
    * Define conditions that invalidate the intervention and require alternate action.

Exception reduction plan note starter:

```text
Plan ID: <YYYY-MM-EXC-<n>>
Plan owner: <name/role>
Hypothesis: <one line>
Intervention: <one line>
Expected metric impact: <one line>
Verification window (UTC): <start-end>
Rollback trigger: <condition>
Status: <open|in_progress|done>
```

### Action plan outcome review block

Use this block when an exception reduction plan completes its verification window.

```text
Plan ID: <YYYY-MM-EXC-<n>>
Outcome rating: <effective|partially_effective|ineffective>
Observed metric change: <one line>
Target met: <yes|no>
Carry-forward action: <none|new plan id>
Reviewer: <name/role>
Review time (UTC): <time>
```

Outcome review rules:

1. `effective`: all planned target thresholds were met in the verification window.
2. `partially_effective`: at least one target improved, but one or more targets remained out of bounds.
3. `ineffective`: targets did not improve or regressed; open a new action plan before closing review.
4. If `Target met` is `no`, include a carry-forward action or replacement plan ID.

### Monthly governance signoff checklist

Use this checklist before closing monthly reliability governance for exception control.

1. Confirm monthly trend review completion
    * Ensure monthly trend review metrics and threshold breach assessment are documented.
2. Confirm action plan coverage
    * Ensure every threshold breach has an active or completed exception reduction plan.
3. Confirm outcome review closure
    * Ensure completed plans include an outcome review rating and carry-forward decision.
4. Confirm carry-forward ownership
    * Ensure all carry-forward actions list one accountable owner and due date.
5. Confirm signoff approval
    * Obtain month-close signoff from service owner and SRE/platform owner.

Monthly governance signoff note starter:

```text
Governance month: <YYYY-MM>
Trend review complete: <yes|no>
Plans covered: <count/total breaches>
Outcome reviews complete: <count/total completed plans>
Carry-forward actions: <count>
Signoff approvers: <name/role, name/role>
Signoff time (UTC): <time>
```

### Quarter-end exception governance snapshot

Use this snapshot at quarter close to summarize the exception program and identify systemic drivers.

Snapshot fields:

1. Quarter label
    * Identify the quarter being reviewed.
2. Monthly signoff coverage
    * Count how many monthly governance signoffs were completed in the quarter.
3. Persistent driver summary
    * List repeated causes behind exception growth, extensions, or reactivation misses.
4. Action plan rollup
    * Summarize how many action plans were opened, completed, carried forward, or closed ineffective.
5. Leadership escalations
    * Note any drivers requiring leadership review or capacity/ownership changes.
6. Next-quarter focus
    * Record the top corrective priorities for the next quarter.

Quarter-end snapshot note starter:

```text
Quarter: <YYYY-Q#>
Monthly signoffs completed: <count/3>
Open exception drivers: <list>
Action plans opened: <count>
Action plans completed: <count>
Action plans carried forward: <count>
Leadership escalations: <count + owner>
Next-quarter focus: <one line>
```

### Annual exception governance summary

Use this summary at year close to roll up quarterly snapshots and identify persistent system-level drivers.

Summary fields:

1. Year label
    * Identify the calendar year being reviewed.
2. Quarter coverage
    * Count how many quarter-end snapshots were completed in the year.
3. Persistent driver rollup
    * Summarize repeated drivers across the year and note whether they improved, worsened, or stayed flat.
4. Annual action plan rollup
    * Count total action plans opened, completed, carried forward, or marked ineffective across the year.
5. Systemic escalation summary
    * List any issues requiring leadership, platform, or ownership changes.
6. Next-year objectives
    * Record the top exception-reduction objectives for the following year.

Annual summary note starter:

```text
Year: <YYYY>
Quarter snapshots completed: <count/4>
Persistent drivers: <list>
Action plans opened: <count>
Action plans completed: <count>
Action plans carried forward: <count>
Systemic escalations: <count + owner>
Next-year objectives: <one line>
```

### Annual leadership review checklist

Use this checklist to turn the annual exception governance summary into leadership decisions and funded priorities.

1. Confirm summary review
    * Review the annual summary, quarter coverage, and persistent driver rollup with leadership.
2. Confirm decisions required
    * Identify any ownership, process, platform, or capacity changes that require approval.
3. Confirm funded priorities
    * Record which next-year objectives are approved, deferred, or rejected.
4. Confirm owners and due dates
    * Assign one accountable owner and due date to every approved leadership action.
5. Confirm communication path
    * Decide who must receive the annual review output and by when.

Annual leadership review note starter:

```text
Review year: <YYYY>
Summary reviewed: <yes|no>
Decisions required: <list>
Approved priorities: <list>
Deferred priorities: <list>
Rejected priorities: <list>
Action owners: <name/role + due date>
Communication recipients: <list>
```

### Governance archive and retrieval checklist

Use this checklist to store and retrieve monthly, quarterly, and annual governance artifacts consistently.

1. Retention bucket
    * Store monthly, quarterly, and annual governance artifacts in a single dated archive root.
2. Artifact naming
    * Use a date-first prefix plus artifact type, for example `2026-05_monthly-signoff.md`.
3. Index entry
    * Maintain a simple index file that points to each monthly, quarterly, and annual artifact.
4. Retrieval rule
    * Search the index first, then open the referenced artifact bundle or note.
5. Cross-link rule
    * Cross-link monthly, quarterly, and annual summaries back to the index entry used for review.

Archive note starter:

```text
Archive root: <path>
Index file: <path>
Artifacts indexed: <monthly|quarterly|annual>
Last index update (UTC): <time>
Retention note: <policy or exception>
Retrieval owner: <name/role>
```

### Governance artifact retention policy

Use this policy to decide how long to keep monthly, quarterly, and annual governance artifacts.

| Artifact type | Minimum retention | Default storage note | Review trigger |
| --- | --- | --- | --- |
| Monthly governance artifacts | 12 months | Keep with the monthly archive index and linked evidence bundle | Annual review |
| Quarterly governance artifacts | 24 months | Keep with the quarterly archive index and quarter-end snapshot | Annual review |
| Annual governance artifacts | 36 months | Keep with the annual archive index and leadership review output | Leadership or audit review |

Retention policy notes:

1. If local policy requires longer retention, keep the longer window and note the exception in the archive index.
2. If an artifact is under active review or audit, do not purge it until the review closes.
3. When retention is extended, record the approver, reason, and new expiry in the archive index.

Retention note starter:

```text
Artifact type: <monthly|quarterly|annual>
Retention until (UTC): <time>
Storage location: <path>
Exception reason: <none|text>
Approver: <name/role>
```

### Governance index template

Use this template as the top-level pointer file for governance artifacts.

Index fields:

1. Archive root
    * Canonical root directory for governance artifacts.
2. Current month pointer
    * Link/path to the active monthly governance artifact.
3. Current quarter pointer
    * Link/path to the active quarter-end snapshot.
4. Current year pointer
    * Link/path to the annual governance summary.
5. Last updated timestamp
    * UTC timestamp for the most recent index update.

Governance index note starter:

```text
Archive root: <path>
Current month artifact: <path>
Current quarter artifact: <path>
Current year artifact: <path>
Last updated (UTC): <time>
Index owner: <name/role>
```

### Governance index maintenance cadence

Use this cadence to keep the governance index accurate and operationally useful.

1. Daily check
    * Confirm current month pointer still references the active monthly artifact.
2. Weekly check
    * Confirm current quarter pointer reflects latest quarter snapshot or in-progress quarter artifact.
3. Monthly check
    * Confirm current year pointer and archive index entries include the latest month-close signoff outputs.
4. Ownership check
    * Confirm index owner is current and escalation backup owner is noted.

Cadence note starter:

```text
Check window (UTC): <start-end>
Daily checks complete: <yes|no>
Weekly checks complete: <yes|no>
Monthly checks complete: <yes|no>
Index owner: <name/role>
Backup owner: <name/role>
```

### Governance ownership RACI

Use this mini-table to keep governance archive and index responsibilities explicit.

| Governance activity | R (Responsible) | A (Accountable) | C (Consulted) | I (Informed) |
| --- | --- | --- | --- | --- |
| Archive index update | On-call engineer | Service owner | SRE/platform owner | Frontend owner |
| Retention policy exception approval | Service owner | SRE/platform owner | Incident commander | On-call engineer |
| Monthly pointer correctness check | On-call engineer | Service owner | SRE/platform owner | Incident commander |
| Quarterly/annual pointer correctness check | Service owner | SRE/platform owner | Incident commander | On-call engineer |
| Governance index maintenance escalation | On-call engineer | Service owner | SRE/platform owner | Frontend owner |

RACI note starter:

```text
Review window (UTC): <start-end>
Activity reviewed: <name>
Responsible: <name/role>
Accountable: <name/role>
Escalation path: <name/role>
```

### Governance change log entry template

Use this template to record archive, index, retention, and governance-process changes separately from incident reliability changes.

```text
Change date (UTC): <YYYY-MM-DD HH:MM>
Change scope: <archive|index|retention|cadence|ownership>
Change summary: <one line>
Reason for change: <one line>
Expected operational impact: <one line>
Approver: <name/role>
Owner: <name/role>
Verification window (UTC): <start-end>
Rollback trigger: <condition>
```

Governance change-log notes:

1. Record every governance-process change in this format before or immediately after rollout.
2. If the change affects retention or ownership, link the updated policy or RACI section.
3. If verification fails in window, execute rollback and log outcome in a follow-up entry.

### Reliability change-log template (incident or release)

Use this template entry format to keep reliability-impact history comparable:

* Date:
* Change type: `incident` | `release` | `config` | `infra`
* Scope summary:
* Related deploy/PR/issue:
* Pre-change baseline:
    * `hit_rate`
    * `persist_failure_rate`
    * `clear_failure_total` delta
* Post-change observation window:
    * start/end time
    * same metrics as baseline
* Threshold changes (if any):
    * old values
    * new values
    * rationale
* Alert impact:
    * alert volume before/after
    * false-positive trend before/after
* Lessons learned:
* Follow-up actions (owner + due date):

### Reliability ownership model

Use these role responsibilities to avoid ambiguity during reliability operations:

* On-call engineer
    * Owns initial triage, metric validation, mitigation execution, and shift handoff quality.
    * Opens/updates incident records and ensures incident closure criteria are met before proposing close.
* Service owner (backend)
    * Owns checkpoint lifecycle behavior, provider retry/failover behavior, and operational correctness of emitted counters.
    * Reviews code-level reliability changes and verifies required backend tests.
* Frontend owner (operator UX)
    * Owns reliability panel semantics (badge severity, hint messaging, timestamp visibility).
    * Verifies frontend test/build checks for reliability UX changes.
* SRE/Platform owner
    * Owns alert rule quality, threshold strategy, and Redis/platform capacity guardrails.
    * Approves threshold retunes for production after baseline and false-positive analysis.
* Incident commander (for major incidents)
    * Owns escalation path, cross-team coordination, rollback/go-forward decision, and final incident timeline accuracy.

### Approval expectations

1. Production threshold retune: service owner + SRE/Platform owner approval.
2. Incident closure: on-call proposes close, service owner (or incident commander) confirms closure criteria satisfied.
3. Reliability runbook/process changes: at least one service owner reviewer and one on-call reviewer.

### Reliability incident communication template

Use these templates to standardize incident updates across channels.

Internal update template (Slack/Teams):

```text
[Reliability Incident Update] <INCIDENT_ID>
Time (UTC): <YYYY-MM-DD HH:MM>
Severity: <warn|critical>
Current impact: <who/what is affected>
Observed metrics:
- hit_rate: <value>
- persist_failure_total delta: <value over window>
- clear_failure_total delta: <value over window>
- provider timeout/retry delta: <value>
Actions taken since last update:
1) <action>
2) <action>
Next actions (owner + ETA):
1) <owner> - <task> - <ETA>
2) <owner> - <task> - <ETA>
Risk/decision needed: <yes/no + detail>
Next update by: <time>
```

Stakeholder/external status template:

```text
We are currently investigating elevated translation reliability errors affecting a subset of requests.
Current status: <investigating|mitigating|monitoring|resolved>
Start time (UTC): <time>
Impact summary: <brief user-facing impact>
Mitigation progress: <brief summary>
Next update: <time>
```

### Communication cadence guidance

* `Sev 1`: update internal channel every 15 minutes until stable.
* `Sev 2`: update internal channel every 30 minutes while active mitigation is in progress.
* `Sev 3`: update internal channel every 60 minutes while mitigation is in progress.
* `Sev 4`: include in daily operations update unless promoted.
* Post-resolution: send closure note with root-cause summary and follow-up action owners.

### Severity-to-channel communication policy

| Severity/state | Internal ops channel | Stakeholder/status channel | Trigger to post |
| --- | --- | --- | --- |
| `Sev 4` (triage/planned) | Required | Optional | Post internal summary in normal ops cadence; stakeholder note only if risk is increasing |
| `Sev 3` (investigating/mitigating) | Required | Optional | Post internal updates on cadence; post stakeholder update if user-visible impact is confirmed or likely to persist |
| `Sev 2` (investigating/mitigating) | Required | Required when external impact is confirmed | Post internal updates every 30 minutes and stakeholder updates on scheduled interval |
| `Sev 1` (active incident) | Required | Required | Post both channels at incident open and on every scheduled update interval |
| `monitoring` after mitigation | Required | Required if external impact occurred | Post internal monitoring updates until stability criteria are met; post stakeholder monitoring notice when external impact previously existed |
| `resolved` | Required | Required if stakeholder channel was used | Post closure summary with root-cause status and follow-up owners |

Channel policy notes:

1. Do not delay internal ops updates while drafting stakeholder wording; send internal first, then stakeholder update.
2. If severity is upgraded, immediately expand channel coverage to the stricter row.
3. If severity is downgraded, keep prior channel coverage until one stable confirmation update is sent.
4. Include links to dashboards, logs, and deploy references in every internal update.

Pre-send update checklist:

1. Severity label matches current matrix row (`Sev 1`/`Sev 2`/`Sev 3`/`Sev 4`/`monitoring`/`resolved`).
2. Time window and latest metric deltas are explicitly stated.
3. Mitigation actions since last update are listed in ordered sequence.
4. Next update time and owner are explicit.
5. External-facing wording avoids internal-only identifiers and sensitive details.

### Communication escalation exception policy

Use this policy for edge cases where default channel rules are insufficient.

| Exception scenario | Required channel behavior | Extra requirement |
| --- | --- | --- |
| Partial outage (subset of routes/tenants) | Keep internal cadence at current severity; send stakeholder update once impact scope is confirmed | Include explicit impacted scope boundary and unaffected scope statement |
| Suspected false-positive recovery | Continue internal updates until two stable confirmations; delay stakeholder "resolved" until confirmation window completes | Mark status as `monitoring` and include what is being validated |
| Incident reopen within 24h of closure | Resume all channels used previously at the stricter prior severity | Reference previous incident ID and summarize what changed since closure |
| Conflicting telemetry sources | Default to stricter channel coverage until source-of-truth is confirmed | Include discrepancy summary and owner for telemetry validation |
| Planned mitigation with temporary degradation | Send internal pre-notice and stakeholder pre-notice before applying mitigation when user impact is expected | Include expected impact window and rollback criteria |

Exception handling notes:

1. When an exception is active, include the exception label in every update header.
2. Record exception start/end times in the incident timeline.
3. Clear the exception only after one confirmation update states normal policy has resumed.

Exception update header template:

```text
[Reliability Incident Update] <INCIDENT_ID>
Exception: <partial_outage|false_positive_recovery|reopen_24h|telemetry_conflict|planned_degradation>
Exception state: <active|cleared>
Exception start (UTC): <time>
Exception owner: <name>
```

### Escalation decision log template

Use this template whenever a major escalation decision is made (for example rollback, go-forward, capacity reduction, or provider failover policy changes).

```text
Decision ID: <INCIDENT_ID>-D<n>
Time (UTC): <YYYY-MM-DD HH:MM>
Decision owner: <name>
Decision type: <rollback|go_forward|capacity_adjustment|policy_change|other>
Decision summary: <one-line decision>

Options considered:
1) <option> - <pros/cons>
2) <option> - <pros/cons>

Risk tradeoff:
- Risk accepted: <text>
- Risk mitigated: <text>

Approver: <name/role>
Expected effect window: <for example 15-30 minutes>
Verification plan: <metric/log checks and timestamps>
Revisit trigger: <condition that forces decision re-evaluation>
```

Decision log usage notes:

1. Append each decision entry to `timeline.md` and reference it in status updates.
2. Include at least one explicit rejected alternative for high-impact decisions.
3. Mark the final outcome as `effective`, `partially_effective`, or `ineffective` during post-incident review.

### Escalation decision quality checklist

Use this checklist before approving major escalation decisions.

| Quality dimension | Pass criteria | Score |
| --- | --- | --- |
| Evidence sufficiency | At least two comparable metric windows and supporting logs are cited | `0` or `1` |
| Option coverage | At least two viable options (including current path) are documented | `0` or `1` |
| Risk clarity | Accepted and mitigated risks are explicit and concrete | `0` or `1` |
| Approval clarity | Decision owner and approver are named with timestamp | `0` or `1` |
| Verification readiness | Verification plan and revisit trigger are defined before execution | `0` or `1` |

Checklist interpretation:

1. `5/5`: decision quality is strong; proceed.
2. `4/5`: proceed with explicit note on the missing item.
3. `<=3/5`: do not approve without service owner or incident commander review.

Example entry (rollback decision):

```text
Decision ID: INC-2026-05-11-07-D2
Time (UTC): 2026-05-11 14:35
Decision owner: Incident Commander
Decision type: rollback
Decision summary: Roll back backend deployment from sha abc123 to previous stable release.

Options considered:
1) Keep current deployment and reduce concurrency - slower risk reduction, uncertain impact.
2) Roll back deployment - fastest path to recover user-facing reliability.

Risk tradeoff:
- Risk accepted: temporary feature regression from reverted release.
- Risk mitigated: continued elevated timeout and failure rates.

Approver: Service Owner (Backend)
Expected effect window: 15-30 minutes
Verification plan: compare `persist_failure_total` and provider timeout deltas at T+15 and T+30.
Revisit trigger: no measurable improvement by T+30.
Outcome: effective
```

### Decision outcome review block

Use this block during post-incident review for each major decision entry.

```text
Decision ID: <INCIDENT_ID>-D<n>
Outcome rating: <effective|partially_effective|ineffective>
Observed outcome summary: <one-line result>
One-line lesson: <single sentence>
Follow-up owner: <name/role>
Follow-up due date (UTC): <YYYY-MM-DD>
```

Outcome review notes:

1. Complete this block after the verification window closes.
2. If outcome is `partially_effective` or `ineffective`, create a follow-up action tied to SLA defaults.
3. Reference the related decision entry and metric/log evidence links.

Example outcome review entry:

```text
Decision ID: INC-2026-05-11-07-D2
Outcome rating: effective
Observed outcome summary: timeout and persist-failure deltas returned to baseline by T+30.
One-line lesson: rollback was faster and lower-risk than incremental tuning under active customer impact.
Follow-up owner: Service Owner (Backend)
Follow-up due date (UTC): 2026-05-14
```

### Stability confirmation checklist

Use this checklist before moving an incident from `monitoring` to `resolved`.

1. Confirm two stable windows
    * Require at least two consecutive windows at or below baseline for the key provider, timeout, and checkpoint indicators.
2. Check closure criteria
    * Verify that the original trigger condition is no longer present and no compensating mitigation is still masking the issue.
3. Validate artifacts
    * Ensure dashboard links, logs, and the decision record support the closure call.
4. Confirm channel coverage
    * Keep all channels active until the final confirmation update is sent.
5. Record ownership
    * Name the closure owner and the follow-up owner before marking the incident `resolved`.

Closure note starter:

```text
[Reliability Incident Update] <INCIDENT_ID>
Status: resolved
Confirmation window: <start-end UTC>
Stable windows observed: <count>
Closure reason: <one-line summary>
Follow-up owner: <name/role>
```

### Post-closure watchback checklist

Use this checklist after an incident is marked `resolved` but before the 24-hour reopen window expires.

1. Keep reduced watch active
    * Continue a lighter monitoring cadence for the key metrics and confirm the closure note is still accurate.
2. Track reopen triggers
    * Record any regression signs, customer reports, or threshold breaches that would justify a reopen.
3. Preserve evidence continuity
    * Keep dashboards, logs, and decision references linked to the closure artifact bundle.
4. Escalate if the incident reopens
    * Switch immediately to the reopen readiness checklist and restore stricter channel coverage.

Watchback note starter:

```text
[Reliability Incident Update] <INCIDENT_ID>
Status: resolved
Watchback window: <start-end UTC>
Current state: monitoring for recurrence
Reopen trigger check: <clear|watch|reopen>
```

### Reopen readiness checklist

Use this checklist when an incident marked `resolved` reopens within 24 hours.

1. Resume channel coverage
    * Re-activate all channels used before closure and restore the stricter prior severity until scope is revalidated.
2. Re-anchor context
    * Reference the previous incident ID, closure time, and the decision entry most likely related to recurrence.
3. Refresh evidence
    * Capture new metric deltas, fresh logs, and any changed deploy/config context since closure.
4. Re-evaluate prior decision quality
    * Check whether the earlier decision outcome was truly `effective` or only temporarily stabilizing.
5. Reset operational artifacts
    * Update `timeline.md`, `handoff.md`, and `closure.md` to show reopen time and new owner.
6. Reconfirm next review point
    * Set a new verification window and next update time before sending the reopen notice.

Reopen notice starter:

```text
[Reliability Incident Update] <INCIDENT_ID>
Status: reopened
Previous closure time (UTC): <time>
Reason for reopen: <one-line trigger>
Current severity: <warn|critical>
Next update by: <time>
```

### Incident severity matrix (response and escalation)

| Severity | Impact | User effect | Operational urgency | Response expectations | Escalation expectations |
| --- | --- | --- | --- | --- | --- |
| `Sev 1` | Critical service outage in a core workflow or severe data-path failure. | Broad inability to complete OCR, translation, rendering, or export workflows. | Immediate and highest priority. | Acknowledge <= 5 min. Assign incident commander immediately. Update every 15 min. | Page service owner, SRE/platform, and leadership on-call immediately. |
| `Sev 2` | Major degradation in a critical workflow with clear user impact. | High error rate or severe latency, but partial path may remain. | Urgent same-hour response. | Acknowledge <= 15 min. Mitigation owner assigned. Update every 30 min. | Escalate to service owner and SRE/platform. Escalate leadership if unresolved after 60 min. |
| `Sev 3` | Moderate degradation with workaround available or constrained blast radius. | Some users affected with reduced reliability/performance. | Prompt active-shift response. | Acknowledge <= 30 min. Mitigation in current shift. Update every 60 min while active. | Escalate to service owner if trend worsens or exceeds 1 business day. |
| `Sev 4` | Minor issue, localized defect, or non-critical operational warning. | Limited or no immediate customer impact. | Planned response and tracking. | Triage <= 1 business day. Add to reliability backlog. | Escalate only on recurrence, trend growth, or promotion to Sev 3+. |

Escalation notes:

1. Use the latest two comparable windows (same length and cadence) before demoting severity.
2. Attach dashboard links, logs, and deploy SHA in every escalation update.
3. Promote severity immediately if user impact scope expands.
4. Downgrade severity only after metrics stabilize across consecutive windows and user impact is no longer increasing.

Common classification examples:

* OCR outage (no fallback): `Sev 1`
* Translation provider outage with working failover: `Sev 2`
* Rendering degradation with stable completions: `Sev 3`
* Queue backlog rising beyond threshold with SLO risk: `Sev 2`
* Export failures that are intermittent and recoverable: `Sev 3`

### First 60 minutes incident timeline

Use this timeline with the severity matrix above. If severity changes, switch to the stricter timing immediately.

| Time window | `Sev 3`-`Sev 4` actions | `Sev 1`-`Sev 2` actions |
| --- | --- | --- |
| 0-5 min | Acknowledge alert, start metric pull, open incident note | Acknowledge immediately, page service owner + SRE/platform, open incident channel |
| 5-15 min | Validate scope (impact + deltas), start mitigation, send first internal update | Confirm user impact scope, assign incident commander, start mitigation and send first internal update |
| 15-30 min | Re-check deltas in comparable window, adjust mitigation, update channel | Re-check deltas and impact trend, decide rollback/go-forward path, update channel on cadence |
| 30-60 min | Escalate if two windows worsen or impact grows, prepare handoff if unresolved | Escalate continuously if unstable, trigger handoff preparation early if not stabilizing |

Timeline execution notes:

1. Use the internal update template for every scheduled update.
2. Record decision points (mitigation applied, escalation, rollback/no-rollback) with timestamp and owner.
3. If unresolved by 60 minutes, publish an explicit handoff packet using the incident handoff template.

### New on-call quick start (reliability)

For first response, follow this order:

1. Pull current metrics
    * Query `GET /api/v1/metrics/providers` and `GET /api/v1/metrics/ocr?window=24h`.
2. Apply runbook triage order
    * Use the "On-call runbook order (checkpoint reliability)" section above.
3. Send structured updates
    * Use the internal communication template every update interval by severity.
4. Handoff cleanly if unresolved
    * Use the "Incident handoff template" section to transfer context between shifts.
5. Close with criteria, not intuition
    * Use the "Incident closure definition of done" checklist before declaring resolved.

Tip: keep links to dashboard, logs, deploy SHA, and incident thread in every update to reduce context-switch cost.

### Common pitfalls (on-call reliability)

1. Comparing mismatched windows
    * Do not compare `24h` OCR window metrics directly to near-real-time provider counter deltas without normalizing time range.
2. Acting on stale timestamps
    * If the frontend "Updated" timestamp is old, verify backend metric freshness before escalating severity.
3. Treating totals as rates
    * Counter totals alone are not incident severity; use deltas over a fixed interval and compute rates from the same window.
4. Isolating checkpoint errors too early
    * Rising checkpoint failures often co-occur with provider timeout/retry pressure; check shared infra signals before code-level rollback.
5. Closing incidents on first recovery signal
    * Require consecutive stable windows and closure checklist completion, not a single improved sample.

