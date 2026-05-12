# Incident Management Templates

Governance Version: 1.0
Review Owner: Reliability Program Owner
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Incident intake, outage communication, post-incident improvement loop

Use these templates as copy-ready blocks for operational incidents.

## 1) Incident Report Template

- Summary:
- Severity:
- Incident ID:
- Status:
- Start Time (UTC):
- End Time (UTC):
- Impact:
- Affected Systems:
- Detection Method:
- Timeline:
- Mitigation:
- Lessons Learned:
- Prevention Actions:
- Owner:

## 2) Outage Summary Template

- Summary:
- Impact:
- User Effect:
- Affected Systems:
- Detection Method:
- Start Time (UTC):
- Recovery Time (UTC):
- Timeline:
- Mitigation:
- Current Status:
- Next Update:

## 3) Postmortem Template

- Summary:
- Incident Scope:
- Impact:
- Timeline:
- Affected Systems:
- Detection Method:
- What Worked:
- What Failed:
- Mitigation:
- Root Cause:
- Lessons Learned:
- Prevention Actions:
- Action Owners and Due Dates:

## 4) Root Cause Analysis (RCA) Template

- Summary:
- Problem Statement:
- Impact:
- Timeline:
- Affected Systems:
- Detection Method:
- Triggering Event:
- Contributing Factors:
- Root Cause (technical + process):
- Mitigation:
- Lessons Learned:
- Prevention Actions:

## 5) Mitigation Plan Template

- Summary:
- Objective:
- Impact to Address:
- Affected Systems:
- Detection Signals:
- Mitigation Steps:
- Rollback Plan:
- Validation Method:
- Timeline and Checkpoints:
- Owner:
- Escalation Path:

## 6) Follow-Up Action Tracking Template

| Action ID | Summary | Impact Addressed | Owner | Due Date (UTC) | Status | Dependency | Evidence Link | Last Update (UTC) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| A-001 | <action> | <risk or gap> | <name/role> | <YYYY-MM-DD> | open | <if any> | <url/path> | <timestamp> |

Status values: open, in_progress, blocked, done.

## Minimum Data Quality Rules

- Every artifact must include summary, impact, timeline, affected systems, detection method, mitigation, lessons learned, and prevention actions.
- Every follow-up action must have exactly one accountable owner and a due date.
- Close the incident only after tracker items are either done or explicitly accepted as deferred with owner approval.
