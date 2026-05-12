# Incident Summary

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 3 export failure packet

- Incident ID: INC-2026-05-12-EXPORT-03
- Severity: Sev 3
- Current Status: resolved
- Start Time (UTC): 2026-05-12 20:16
- End Time (UTC): 2026-05-12 21:03
- Incident Commander: Backend On-Call
- Primary Owner: Export Pipeline Owner
- Impact Summary: Export artifacts intermittently failed for a subset of completed jobs; core translation and rendering remained available.
- Affected Systems: Export packaging stage, artifact download endpoint
- Detection Method: Export failure-rate alert and support ticket correlation
- Current Mitigation State: Export retry path stabilized and failed packaging jobs were replayed successfully
- User Communication State: Internal ops updates hourly; no broad stakeholder notice required because impact remained limited and recoverable
- Links to Dashboards/Logs: Export error panel, packaging logs, replay queue metrics
