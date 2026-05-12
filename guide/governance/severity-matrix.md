# Reliability Severity Matrix

Governance Version: 1.0
Review Owner: Incident Commander Rotation Owner
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Production incidents across OCR, translation, rendering, queues, export

## Severity Definitions

| Severity | Impact | User Effect | Operational Urgency | Response Expectations | Escalation Expectations |
| --- | --- | --- | --- | --- | --- |
| Sev 1 | Critical service outage or severe data-path failure across core workflows. | Broad production impact; customers cannot complete primary workflows. | Immediate, highest-priority response. | Acknowledge in <= 5 min. Incident commander assigned immediately. Mitigation starts at once. Status updates every 15 min. | Page service owner, platform/SRE, and leadership on-call immediately. Cross-team war-room enabled. |
| Sev 2 | Major degradation in one or more critical workflows with meaningful customer impact. | High error rate, severe latency, or partial inability to complete workflows. | Urgent same-hour response. | Acknowledge in <= 15 min. Mitigation owner assigned. Status updates every 30 min. | Escalate to service owner and platform/SRE. Escalate to leadership if unresolved after 60 min. |
| Sev 3 | Moderate degradation with workaround available or constrained impact. | Some customers affected; operations continue with friction. | Prompt response during active support hours. | Acknowledge in <= 30 min. Mitigation plan in current shift. Status updates every 60 min while active. | Escalate to service owner if trend worsens or persists beyond 1 business day. |
| Sev 4 | Minor issue, localized defect, or non-critical reliability warning. | Limited/no immediate customer impact. | Planned response via backlog and scheduled maintenance. | Triage in <= 1 business day. Track in reliability backlog. | Escalate only on trend growth, repeated recurrence, or threshold breach promotion to Sev 3+. |

## Operational Examples

| Scenario | Typical Severity | Notes |
| --- | --- | --- |
| OCR outage (service unavailable) | Sev 1 | If no fallback path and image/PDF workflows blocked. |
| Translation provider outage | Sev 1 or Sev 2 | Sev 1 when failover also fails; Sev 2 when failover works but with degraded latency/error rates. |
| Rendering degradation | Sev 2 or Sev 3 | Sev 2 when completion drops materially; Sev 3 when only latency increases with stable completion. |
| Queue backlog growth | Sev 2 or Sev 3 | Use backlog threshold and age trend; promote to Sev 2 when SLO breach risk is high. |
| Export failures | Sev 2 or Sev 3 | Sev 2 for sustained high failure rates; Sev 3 for intermittent/recoverable failures. |

## Promotion and Demotion Rules

- Promote severity immediately when user impact scope expands or SLO error budget burn is critical.
- Demote only after two consecutive stable measurement windows and no expanding user impact.
- Log every severity change in the incident timeline with timestamp, owner, and rationale.
