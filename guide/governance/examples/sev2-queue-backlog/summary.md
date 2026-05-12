# Incident Summary

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 2 queue backlog packet

- Incident ID: INC-2026-05-12-QUEUE-02
- Severity: Sev 2
- Current Status: resolved
- Start Time (UTC): 2026-05-12 17:08
- End Time (UTC): 2026-05-12 18:02
- Incident Commander: Backend On-Call
- Primary Owner: Platform/SRE Owner
- Impact Summary: Queue depth and oldest-job age rose sharply, causing delayed job completion across OCR, translation, and rendering workflows.
- Affected Systems: Redis-backed job queue, worker throughput, downstream completion latency
- Detection Method: Queue depth threshold alert and golden-path completion latency drift
- Current Mitigation State: Worker concurrency tuned, non-critical batch load reduced, and queue age returned below warning threshold
- User Communication State: Internal ops updates every 30 minutes; stakeholder update sent after customer-visible completion delay exceeded target
- Links to Dashboards/Logs: Queue depth panel, oldest-job age panel, worker logs, provider pressure metrics
