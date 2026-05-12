# Governance Maintainer Quickstart

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Minimum workflow for creating and updating governance documents

Use this guide for all governance updates.

## 1) Decide Change Type

- New governance document
- Update to thresholds, ownership, cadence, templates, examples, or archive policy
- Operational examples only

## 2) Apply Metadata Standard

Every governance markdown document must include:

- Governance Version:
- Review Owner:
- Last Review:
- Next Review:
- Operational Scope:

## 3) Update Indexes

When adding a new document, link it in at least one index:

- guide/governance/README.md
- guide/reliability-runbook.md
- README.md (reliability section)

## 4) Update Governance Change Log

If the change modifies operational expectations, update:

- guide/governance/governance-change-log.md

## 5) Run Local Checks

- bash guide/governance/check-governance-metadata.sh
- bash guide/governance/check-governance-changelog-discipline.sh (warning mode for local worktree fallback)
- STRICT_LOCAL=true bash guide/governance/check-governance-changelog-discipline.sh (enforced local failure mode)

## 5b) One-Command Maintenance

- bash guide/governance/run-governance-maintenance.sh
- STRICT_MODE=true bash guide/governance/run-governance-maintenance.sh

This runs metadata, freshness, changelog discipline, archive index generation, current pointer updates, drift snapshot generation, drift summary generation, and drift trend updates.
The strict variant enforces changelog-discipline failure locally and is auto-enabled on release branches.

## 6) Archive Artifacts as Needed

Initialize archive layout for a year:

- bash guide/governance/create-governance-archive-structure.sh 2026

Archive location:

- guide/governance/archive/
