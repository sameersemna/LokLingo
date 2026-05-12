# Governance Archive Layout

Governance Version: 1.0
Review Owner: Reliability Lead (Platform)
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: Archive structure and naming conventions for monthly, quarterly, annual, and incident governance artifacts

Use this layout to keep governance records discoverable and low-maintenance.

## Recommended Folder Layout

```text
guide/governance/archive/
  incidents/
    2026/
      2026-05-12_INC-2026-05-12-OCR-01_sev1/
  monthly/
    2026/
      2026-05_signoff.md
  quarterly/
    2026/
      2026-Q2_review.md
  annual/
    2026/
      2026_summary.md
```

## Naming Rules

- Incident bundles: `<YYYY-MM-DD>_<INCIDENT_ID>_<severity>/`
- Monthly signoff artifacts: `<YYYY-MM>_signoff.md`
- Quarterly review artifacts: `<YYYY-QN>_review.md`
- Annual summary artifacts: `<YYYY>_summary.md`
- Supporting evidence folders may append `_artifacts` when a document has multiple linked files.

## Archive Rules

- Archive completed incident bundles after closure and watchback completion.
- Archive monthly signoff outputs within the first 5 business days of the next month.
- Archive quarterly reviews within 10 business days of quarter close.
- Archive annual summaries before yearly planning closes.

## Retrieval Guidance

- Prefer year-based partitioning for every artifact type.
- Keep archive file names stable after publication.
- Record archive locations in the governance index or review packet when practical.
