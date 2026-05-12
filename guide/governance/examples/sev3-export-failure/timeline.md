# Incident Timeline (UTC)

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 3 export failure packet

| Time (UTC) | Actor | Event Type | Action/Observation | Severity | Evidence Link |
| --- | --- | --- | --- | --- | --- |
| 20:16 | export-alerting | detection | Export failure rate exceeded warning threshold for completed jobs | Sev 3 | export error dashboard |
| 20:24 | Backend On-Call | triage | Confirmed failures were limited to export packaging and did not block core localization flow | Sev 3 | support ticket notes |
| 20:31 | Export Pipeline Owner | mitigation | Replayed failed export jobs and reduced packaging batch size temporarily | Sev 3 | replay queue metrics |
| 20:46 | Backend On-Call | stabilize | Export retries succeeded and failure rate fell below warning threshold | Sev 3 | export panel |
| 21:03 | Backend On-Call | close | Closed after stable recovery and successful replay verification | Sev 3 | closure note |
