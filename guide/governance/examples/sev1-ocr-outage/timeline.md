# Incident Timeline (UTC)

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 1 OCR outage packet

| Time (UTC) | Actor | Event Type | Action/Observation | Severity | Evidence Link |
| --- | --- | --- | --- | --- | --- |
| 09:04 | platform-alerting | detection | OCR readiness check failed for two consecutive probes | Sev 1 | readiness alert |
| 09:06 | Platform On-Call | triage | Confirmed OCR service unavailable and image/PDF jobs failing | Sev 1 | readiness dashboard |
| 09:10 | OCR Service Owner | mitigation | Restarted OCR container and verified process came up cleanly | Sev 1 | OCR container logs |
| 09:18 | Platform On-Call | escalation | Posted first stakeholder update and opened incident bridge | Sev 1 | incident channel |
| 09:29 | OCR Service Owner | stabilize | OCR success rate returned to baseline in 1h short-window trend | Sev 1 | OCR metrics panel |
| 09:51 | Platform On-Call | close | Closed incident after two stable validation windows | Sev 1 | closure note |
