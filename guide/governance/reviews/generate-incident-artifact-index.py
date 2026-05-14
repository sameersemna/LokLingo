#!/usr/bin/env python3
"""Generate a markdown index for weekly reliability artifacts."""

from __future__ import annotations

import json
import sys
from pathlib import Path


def rel(root: Path, path: Path) -> str:
    try:
        return str(path.relative_to(root))
    except ValueError:
        return str(path)


def status(path: Path) -> str:
    return "present" if path.exists() and path.stat().st_size > 0 else "missing"


def load_json(path: Path) -> dict:
    if not path.exists() or path.stat().st_size == 0:
        return {}
    try:
        with path.open("r", encoding="utf-8") as handle:
            payload = json.load(handle)
        return payload if isinstance(payload, dict) else {}
    except Exception:
        return {}


def recommended_first_command(smoke: dict) -> str:
    lineage_verified = smoke.get("lineage_verified")
    overall = str(smoke.get("overall_status", "unknown"))
    if lineage_verified is False:
        return "bash guide/ops/reliability-recovery.sh queue-status"
    if overall in {"failed", "regression"}:
        return "bash guide/ops/reliability-recovery.sh provider-policy-snapshot"
    return "bash guide/ops/reliability-recovery.sh provider-history"


def severity_hint(smoke: dict) -> str:
    lineage_verified = smoke.get("lineage_verified")
    overall = str(smoke.get("overall_status", "unknown"))
    if lineage_verified is False or overall == "failed":
        return "critical"
    if overall == "regression":
        return "warning"
    if overall == "passed":
        return "normal"
    return "unknown"


def handoff_commands(severity: str) -> list[str]:
    if severity == "critical":
        return [
            "bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>",
            "bash guide/ops/reliability-recovery.sh validate-incident-snapshot <incident-snapshot.json>",
            "bash guide/ops/reliability-recovery.sh generate-incident-brief <incident-snapshot.json> <brief-output.md>",
        ]
    if severity == "warning":
        return [
            "bash guide/ops/reliability-recovery.sh queue-status",
            "bash guide/ops/reliability-recovery.sh provider-policy-snapshot",
            "bash guide/ops/reliability-recovery.sh provider-history",
        ]
    return [
        "bash guide/ops/reliability-recovery.sh provider-history",
    ]


def main() -> int:
    if len(sys.argv) != 7:
        print(
            "usage: generate-incident-artifact-index.py <smoke-report> <provider-snapshot> <incident-snapshot> <incident-brief> <smoke-summary> <output>",
            file=sys.stderr,
        )
        return 1

    root = Path.cwd()
    smoke_report = Path(sys.argv[1])
    provider_snapshot = Path(sys.argv[2])
    incident_snapshot = Path(sys.argv[3])
    incident_brief = Path(sys.argv[4])
    smoke_summary = Path(sys.argv[5])
    output = Path(sys.argv[6])
    smoke_payload = load_json(smoke_report)

    rows = [
        ("Reliability Smoke Report", smoke_report),
        ("Provider Policy Snapshot", provider_snapshot),
        ("Incident Snapshot", incident_snapshot),
        ("Incident Brief", incident_brief),
        ("Smoke Ops Summary", smoke_summary),
    ]

    lines: list[str] = []
    lines.append("# Reliability Artifact Index")
    lines.append("")
    severity = severity_hint(smoke_payload)
    lines.append(f"- Overall Smoke Status: {smoke_payload.get('overall_status', 'unknown')}")
    lines.append(f"- Severity Hint: {severity}")
    lines.append(f"- Recommended First Command: {recommended_first_command(smoke_payload)}")
    lines.append(f"- Correlation ID: {smoke_payload.get('correlation_id') or smoke_payload.get('last_correlation_id') or 'n/a'}")
    lines.append("")
    lines.append("## Operator Handoff Commands")
    lines.append("")
    for cmd in handoff_commands(severity):
        lines.append(f"- {cmd}")
    lines.append("")
    lines.append("| Artifact | Path | Status |")
    lines.append("| --- | --- | --- |")
    for label, path in rows:
        lines.append(f"| {label} | {rel(root, path)} | {status(path)} |")

    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
