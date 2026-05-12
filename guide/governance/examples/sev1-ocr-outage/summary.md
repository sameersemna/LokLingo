# Incident Summary

Governance Version: 1.0
Review Owner: Incident Commander
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Example Sev 1 OCR outage packet

- Incident ID: INC-2026-05-12-OCR-01
- Severity: Sev 1
- Current Status: resolved
- Start Time (UTC): 2026-05-12 09:04
- End Time (UTC): 2026-05-12 09:51
- Incident Commander: Platform On-Call
- Primary Owner: OCR Service Owner
- Impact Summary: OCR requests for image and PDF jobs failed across production due to OCR service unavailability.
- Affected Systems: OCR service, PDF ingestion pipeline, image localization flow
- Detection Method: Readiness alert and failing golden-path smoke test
- Current Mitigation State: OCR service restarted and dependency health confirmed across two stable windows
- User Communication State: Stakeholder channel updated at open, mitigation, monitoring, and closure
- Links to Dashboards/Logs: OCR readiness dashboard, backend incident logs, OCR container logs
