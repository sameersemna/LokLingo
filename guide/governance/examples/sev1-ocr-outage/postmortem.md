# Postmortem

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 1 OCR outage packet

## Summary

- Incident ID: INC-2026-05-12-OCR-01
- Severity: Sev 1
- Systems Affected: OCR service, image localization flow, PDF OCR stage
- Duration: 47 minutes
- Impact Summary: Core OCR-dependent workflows were unavailable across production.

## Timeline

- 2026-05-12 09:04 UTC: readiness alert fired.
- 2026-05-12 09:10 UTC: OCR container restarted.
- 2026-05-12 09:29 UTC: OCR metrics stabilized.
- 2026-05-12 09:51 UTC: incident closed.

## Detection and Response

- Detection Method: readiness alert and smoke-test failure.
- Time to Detect: < 2 minutes from customer-visible failure.
- Time to Mitigate: 25 minutes.
- Time to Recover: 47 minutes.

## Root Cause Analysis

- Primary Root Cause: OCR service process exited after runtime dependency initialization failure.
- Contributing Factors: insufficient warm-start validation and no early warning on dependency initialization latency.
- Why Detection Lagged (if applicable): detection was timely; diagnosis lagged due to limited startup-failure signal detail.

## Mitigation and Recovery

- What Worked: readiness alerting, restart procedure, incident communication cadence.
- What Did Not Work: startup error detail was not immediately visible in first dashboard view.
- Temporary vs Permanent Fixes: service restart was temporary; startup instrumentation and preflight checks are permanent follow-up actions.

## Lessons Learned

- Technical: OCR startup health needs richer failure classification.
- Process: first response worked, but OCR-specific log links should be in the incident template by default.
- Communication: stakeholder updates were timely and appropriately scoped.

## Prevention Actions

- Action 1: add OCR startup failure metrics and alert detail.
- Action 2: add OCR preflight health check to deploy verification.
- Action 3: add OCR service warm-start trace to dashboard links.
