# Governance Release Checklist

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Merge readiness checklist for reliability-impacting governance and operational changes

Use this checklist when reliability-impacting changes are prepared for merge.

## Trigger Conditions

Apply this checklist if a change affects one or more of:

- Incident severity policy or escalation behavior
- SLO/SLA targets or threshold interpretation
- Alert routing, dashboard thresholds, or lifecycle event requirements
- Incident templates, on-call runbooks, or operational cadence
- Governance archive/indexing/retention behavior

## Merge Readiness Checklist

- [ ] Governance metadata fields are present and current on all modified governance docs.
- [ ] Governance change log includes an entry for this operationally meaningful change.
- [ ] Reliability runbook links reflect new or moved governance docs.
- [ ] Top-level reliability quick links in README are still accurate.
- [ ] Incident examples remain aligned with current severity matrix language.
- [ ] Archive layout/index impact considered and updated when required.
- [ ] Local checks executed:
- [ ] `bash guide/governance/check-governance-metadata.sh`
- [ ] `bash guide/governance/check-governance-freshness.sh`
- [ ] `bash guide/governance/check-governance-changelog-discipline.sh`

## Release Signoff Block

- Change summary:
- Reliability impact class: <none|low|moderate|high>
- Governance owner signoff:
- Date (UTC):
- Linked change-log entry:
