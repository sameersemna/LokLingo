# Postmortem

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 2 translation provider failover packet

## Summary

- Incident ID: INC-2026-05-12-TRANS-02
- Severity: Sev 2
- Systems Affected: Translation provider routing, downstream render latency
- Duration: 53 minutes
- Impact Summary: Customer workflows completed, but latency and retry volume increased due to provider degradation.

## Timeline

- 2026-05-12 14:12 UTC: provider timeout threshold breached.
- 2026-05-12 14:18 UTC: failover behavior confirmed.
- 2026-05-12 14:27 UTC: provider routing preference adjusted.
- 2026-05-12 15:05 UTC: incident closed.

## Detection and Response

- Detection Method: provider timeout alerts and retry trend panel.
- Time to Detect: 3 minutes.
- Time to Mitigate: 15 minutes.
- Time to Recover: 53 minutes.

## Root Cause Analysis

- Primary Root Cause: transient upstream provider degradation caused timeout spikes.
- Contributing Factors: failover path worked, but preferred-provider weighting prolonged elevated retry pressure.
- Why Detection Lagged (if applicable): no major lag; root cause confirmation depended on provider-side latency trend.

## Mitigation and Recovery

- What Worked: failover logic, retry observability, provider metrics.
- What Did Not Work: provider preference policy was too slow to react to sustained timeout trends.
- Temporary vs Permanent Fixes: routing preference shift was temporary; adaptive provider weighting is the permanent follow-up.

## Lessons Learned

- Technical: provider weighting should respond faster to sustained timeout patterns.
- Process: Sev 2 stakeholder communication threshold was appropriate once user latency impact was verified.
- Communication: internal cadence was sufficient and low-noise.

## Prevention Actions

- Action 1: add adaptive routing rule for sustained timeout spikes.
- Action 2: add failover frequency panel to the default incident dashboard.
- Action 3: review provider timeout threshold sensitivity monthly.
