# Incident Timeline (UTC)

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 2 translation provider failover packet

| Time (UTC) | Actor | Event Type | Action/Observation | Severity | Evidence Link |
| --- | --- | --- | --- | --- | --- |
| 14:12 | provider-alerting | detection | Primary translation provider timeout trend exceeded critical threshold | Sev 2 | provider metrics |
| 14:18 | Backend On-Call | triage | Confirmed failover active and completion rates stable but latency elevated | Sev 2 | retry dashboard |
| 14:27 | Translation Owner | mitigation | Reduced traffic preference to degraded provider and monitored retry burn | Sev 2 | provider routing change |
| 14:40 | Backend On-Call | stabilize | Retry frequency decreased and latency moved toward baseline | Sev 2 | latency panel |
| 15:05 | Backend On-Call | close | Closed after two stable windows with no additional failovers | Sev 2 | closure update |
