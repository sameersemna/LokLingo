# Postmortem

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 2 queue backlog packet

## Summary

- Incident ID: INC-2026-05-12-QUEUE-02
- Severity: Sev 2
- Systems Affected: Job queue, worker concurrency path, completion latency across core workflows
- Duration: 54 minutes
- Impact Summary: Customer jobs completed slowly due to elevated queue depth and oldest-job age, but service remained available.

## Timeline

- 2026-05-12 17:08 UTC: queue depth critical threshold breached.
- 2026-05-12 17:14 UTC: broad latency impact confirmed.
- 2026-05-12 17:22 UTC: low-risk throughput mitigation applied.
- 2026-05-12 18:02 UTC: incident closed.

## Detection and Response

- Detection Method: queue depth/age alert and smoke-test latency drift.
- Time to Detect: 4 minutes.
- Time to Mitigate: 14 minutes.
- Time to Recover: 54 minutes.

## Root Cause Analysis

- Primary Root Cause: transient worker pressure combined with non-critical batch load increased queue contention.
- Contributing Factors: backlog protection thresholds existed, but batch shedding was manual and applied after customer latency had already grown.
- Why Detection Lagged (if applicable): detection was timely; operational response required correlation between queue age and downstream latency.

## Mitigation and Recovery

- What Worked: queue alerting, low-risk load reduction, worker throughput monitoring.
- What Did Not Work: backlog relief was too dependent on manual intervention.
- Temporary vs Permanent Fixes: traffic reduction was temporary; automated backlog shedding and queue-aware concurrency controls are permanent follow-ups.

## Lessons Learned

- Technical: backlog protection should trigger adaptive throughput controls earlier.
- Process: queue incidents need a default runbook branch with explicit low-risk mitigation order.
- Communication: stakeholder update timing was appropriate once user-visible delay crossed objective thresholds.

## Prevention Actions

- Action 1: add automated batch shedding rule for sustained queue-age growth.
- Action 2: add queue-age trend to default reliability landing dashboard.
- Action 3: review concurrency settings against peak-hour workload monthly.
