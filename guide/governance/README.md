# LokLingo Operational Governance Index

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: OCR, translation, rendering, queueing, export, incident operations

This folder holds the production governance baseline for reliability operations.

## Principles (Lean Governance)

- Bias to operational clarity over process overhead.
- Reuse one template per operational artifact type.
- Prefer measurable thresholds and explicit owners.
- Keep updates in normal engineering cadence (weekly/monthly/quarterly) instead of adding ceremony.

## Document Map

- [Reliability Severity Matrix](severity-matrix.md)
- [Incident Management Templates](incident-templates.md)
- [SLO and SLA Definitions](slo-sla.md)
- [Reliability Dashboard Specification](dashboard-spec.md)
- Grafana Dashboard JSON: `dashboards/loklingo-reliability-dashboard.json`
- Incident Timeline Dashboard JSON: `dashboards/loklingo-incident-timeline-dashboard.json`
- Prometheus Scrape Config: `prometheus/prometheus.yml`
- Metrics Proxy Config: `metrics-proxy/default.conf.template`
- Grafana Provisioning (datasource/dashboard providers): `grafana/provisioning/`
- [Observability and Reliability Instrumentation Specification](observability-reliability-spec.md)
- [Golden-Path Smoke Tests](golden-path-smoke-tests.md)
- [Operational Lifecycle Events](lifecycle-events.md)
- [On-Call First Response Playbook](oncall-first-response-playbook.md)
- [Incident Artifact Template Bundle](incidents-template/README.md)
- [Incident Bundle Bootstrap Script](create-incident-bundle.sh)
- [Governance Archive Bootstrap Script](create-governance-archive-structure.sh)
- [Governance Archive Index Generator](generate-governance-archive-index.sh)
- [Governance Current Pointers Updater](update-governance-current-pointers.sh)
- [Governance Drift Snapshot Generator](generate-governance-drift-snapshot.sh)
- [Governance Drift Summary Generator](generate-governance-drift-summary.sh)
- [Governance Drift Trend Updater](update-governance-drift-trend.sh)
- [Governance Maintenance Runner](run-governance-maintenance.sh)
- [Governance Metadata Check Script](check-governance-metadata.sh)
- [Governance Freshness Check Script](check-governance-freshness.sh)
- [Governance Changelog Discipline Script](check-governance-changelog-discipline.sh)
- [Governance Change Log](governance-change-log.md)
- [Governance Archive Layout](governance-archive-layout.md)
- [Governance Operating Calendar](governance-operating-calendar.md)
- [Governance Current Pointers](current-pointers.md)
- [Governance Release Checklist](governance-release-checklist.md)
- [Governance Drift Dashboard Template](governance-drift-dashboard-template.md)
- Prometheus Alert Rules: `alerts/reliability-alerts.prometheus.yml`
- Observability Compose Overlay: `docker-compose.observability.yml`
- Observability Smoke Script: `../smoke-observability.sh`
- [Governance Maintainer Quickstart](MAINTAINER-QUICKSTART.md)
- [Monthly Governance Signoff Template](reviews/monthly-governance-signoff-template.md)
- [Quarterly Governance Review Template](reviews/quarterly-governance-review-template.md)
- [Annual Governance Summary Template](reviews/annual-governance-summary-template.md)
- [Incident Example Index](examples/README.md)
- [Example Sev 1 OCR Outage Packet](examples/sev1-ocr-outage/summary.md)
- [Example Sev 2 Translation Failover Packet](examples/sev2-translation-failover/summary.md)
- [Example Sev 2 Queue Backlog Packet](examples/sev2-queue-backlog/summary.md)
- [Example Sev 3 Export Failure Packet](examples/sev3-export-failure/summary.md)

## Governance Metadata Standard

Every governance document must include:

```md
Governance Version:
Review Owner:
Last Review:
Next Review:
Operational Scope:
```

## Review Cadence

- Weekly: operational drift checks during reliability review.
- Monthly: threshold and action tracker health review.
- Quarterly: objective/ownership recalibration and artifact hygiene sweep.
- Annually: leadership-level trend and strategy review.

## Validation Status

- Governance metadata is enforced by [check-governance-metadata.sh](check-governance-metadata.sh) and the `governance-check` CI job.
- Governance review freshness is enforced by [check-governance-freshness.sh](check-governance-freshness.sh) and the `governance-freshness-check` CI job.
- Governance changelog discipline is enforced by [check-governance-changelog-discipline.sh](check-governance-changelog-discipline.sh) and the `governance-changelog-check` CI job.
- Governance check outputs are uploaded in CI as artifacts for auditability.
- Weekly scheduled governance maintenance publishes artifacts including archive index, current pointers, and drift snapshots.
- Drift trend history is maintained in `guide/governance/archive/drift/trend.csv`.
- Run locally with `bash guide/governance/check-governance-metadata.sh` before proposing governance doc changes.
- Run locally with `bash guide/governance/check-governance-freshness.sh` to catch overdue review windows.
- Local changelog discipline fallback is warning-only by default; use `STRICT_LOCAL=true` to enforce failure locally.


## Maintainer Rules

- Update [governance-change-log.md](governance-change-log.md) whenever a governance change alters ownership, thresholds, templates, cadence, archive layout, or required examples.
- Run `bash guide/governance/check-governance-metadata.sh` locally before merging governance documentation changes.
- Run `bash guide/governance/check-governance-freshness.sh` locally before merging governance documentation changes.
- If a new governance document is added, link it from this index or the relevant sub-index in the same change.
- Run `bash guide/governance/update-governance-current-pointers.sh` after archiving month/quarter/year artifacts.
- Run `bash guide/governance/run-governance-maintenance.sh` for one-command governance maintenance.
- Use `STRICT_MODE=true bash guide/governance/run-governance-maintenance.sh` to enforce strict changelog discipline locally.
