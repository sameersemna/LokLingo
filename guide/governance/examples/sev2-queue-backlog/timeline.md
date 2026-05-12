# Incident Timeline (UTC)

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 2 queue backlog packet

| Time (UTC) | Actor | Event Type | Action/Observation | Severity | Evidence Link |
| --- | --- | --- | --- | --- | --- |
| 17:08 | queue-alerting | detection | Queue depth exceeded critical threshold and oldest-job age crossed 15 minutes | Sev 2 | queue dashboard |
| 17:14 | Backend On-Call | triage | Confirmed completion latency rising across active workflows with no full outage | Sev 2 | job latency panel |
| 17:22 | Platform/SRE Owner | mitigation | Reduced non-critical batch traffic and lowered worker contention on shared resources | Sev 2 | worker config change |
| 17:35 | Backend On-Call | stabilize | Queue age trend reversed and throughput recovered toward baseline | Sev 2 | queue age panel |
| 18:02 | Backend On-Call | close | Closed after two stable windows below warning threshold | Sev 2 | closure note |
