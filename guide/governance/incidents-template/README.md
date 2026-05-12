# Incident Artifact Template Bundle

Governance Version: 1.0
Review Owner: Incident Commander Rotation Owner
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Standard incident records for handoff, closure, and postmortem

Create an incident folder using this structure:

```text
incidents/<YYYY-MM-DD>_<INCIDENT_ID>_<severity>/
```

Copy these files into the incident folder:

- summary.md
- timeline.md
- handoff.md
- closure.md
- postmortem.md
- action-tracker.csv

## Usage Notes

- Keep all times in UTC.
- Keep one accountable owner per action item.
- Link evidence artifacts for every closed action.

## Quick Start Command

From repository root:

```bash
bash guide/governance/create-incident-bundle.sh <incident_id> <severity> [YYYY-MM-DD]
```

Example:

```bash
bash guide/governance/create-incident-bundle.sh INC-2026-0512-01 Sev2 2026-05-12
```
