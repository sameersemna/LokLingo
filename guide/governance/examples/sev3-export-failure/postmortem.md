# Postmortem

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 3 export failure packet

## Summary

- Incident ID: INC-2026-05-12-EXPORT-03
- Severity: Sev 3
- Systems Affected: Export packaging stage, artifact download path
- Duration: 47 minutes
- Impact Summary: A subset of export artifacts failed temporarily, while the main localization workflow remained available.

## Timeline

- 2026-05-12 20:16 UTC: export failure-rate threshold breached.
- 2026-05-12 20:24 UTC: impact limited to export path confirmed.
- 2026-05-12 20:31 UTC: replay and packaging-size mitigation applied.
- 2026-05-12 21:03 UTC: incident closed.

## Detection and Response

- Detection Method: export failure-rate alert and correlated support requests.
- Time to Detect: 5 minutes.
- Time to Mitigate: 15 minutes.
- Time to Recover: 47 minutes.

## Root Cause Analysis

- Primary Root Cause: export packaging path became unstable under a burst of large artifact jobs.
- Contributing Factors: replay support existed, but packaging batch behavior was not adaptive to artifact size pressure.
- Why Detection Lagged (if applicable): detection was adequate; user confirmation came slightly later because failures were intermittent.

## Mitigation and Recovery

- What Worked: replay mechanism, limited-scope triage, packaging batch-size reduction.
- What Did Not Work: export path lacked earlier size-pressure signaling before failure rate rose.
- Temporary vs Permanent Fixes: batch-size reduction was temporary; export size-aware throttling and earlier alerts are permanent follow-ups.

## Lessons Learned

- Technical: export packaging should expose size-pressure metrics before failure rate increases.
- Process: Sev 3 handling was appropriate because primary workflows remained usable.
- Communication: internal-only cadence was sufficient given limited, recoverable blast radius.

## Prevention Actions

- Action 1: add size-aware export throttling for large artifact bursts.
- Action 2: add export artifact-size trend chart to the default reliability dashboard.
- Action 3: review export replay automation for partial-batch failures.
