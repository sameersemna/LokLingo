# Governance Drift Snapshot

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-10
Operational Scope: Automated governance drift snapshot generated from governance checks

## Snapshot Metadata

- Report Date (UTC): 2026-05-12
- Generated At (UTC): 2026-05-12T15:58:17Z
- Source: guide/governance/generate-governance-drift-snapshot.sh

## Check Status

| Check | Status |
| --- | --- |
| Metadata compliance | PASS |
| Review freshness | PASS |
| Changelog discipline | PASS |

## Metadata Check Output

```text
Governance metadata check passed for 42 files.
```

## Freshness Check Output

```text
Governance freshness check passed for 42 files.
```

## Changelog Discipline Output

```text
Governance scope changes detected in local worktree without a changelog edit.
Warning only in local fallback mode. Re-run with STRICT_LOCAL=true to enforce failure.
Changed governance-scope files:
README.md
guide/reliability-runbook.md
```
