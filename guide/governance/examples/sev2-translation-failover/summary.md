# Incident Summary

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 2 translation provider failover packet

- Incident ID: INC-2026-05-12-TRANS-02
- Severity: Sev 2
- Current Status: resolved
- Start Time (UTC): 2026-05-12 14:12
- End Time (UTC): 2026-05-12 15:05
- Incident Commander: Backend On-Call
- Primary Owner: Backend Translation Owner
- Impact Summary: Primary translation provider timed out repeatedly; failover kept service available with elevated latency and retries.
- Affected Systems: Translation stage, provider failover path, render latency downstream
- Detection Method: Provider timeout alerts and elevated retry frequency
- Current Mitigation State: Provider routing preference adjusted and retry pressure returned to baseline
- User Communication State: Internal ops updates every 30 minutes; stakeholder update issued after customer latency impact was confirmed
- Links to Dashboards/Logs: Provider metrics dashboard, retry trend panel, backend translation logs
