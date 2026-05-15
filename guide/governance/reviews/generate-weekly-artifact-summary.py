#!/usr/bin/env python3
"""Generate a compact markdown summary of weekly reliability artifacts."""

from __future__ import annotations

import json
from pathlib import Path
import sys


REQUIRED_ROWS = [
    "Reliability Smoke Report",
    "Provider Policy Snapshot",
    "Weekly Smoke Ops Summary",
    "Reliability Artifact Index",
    "Provider Policy Trend",
    "Incident Escalation Summary",
    "Weekly Operator One-Pager",
]

REQUIRED_STATUS_VALUES = {"present", "missing"}


def status(path: Path) -> str:
    return "present" if path.exists() and path.stat().st_size > 0 else "missing"


def load_json(path: Path) -> dict:
    if not path.exists() or path.stat().st_size == 0:
        return {}
    with path.open("r", encoding="utf-8") as handle:
        payload = json.load(handle)
    return payload if isinstance(payload, dict) else {}


def get_fallback_budget_value(provider_snapshot: dict) -> int | None:
    signals = provider_snapshot.get("degraded_and_queue_signals")
    if not isinstance(signals, dict):
        return None
    value = signals.get("loklingo_ocr_provider_fallback_budget_exhausted_total")
    if isinstance(value, (int, float)):
        return int(value)
    return None


def fallback_budget_status(total: int | None) -> str:
    if total is None:
        return "unknown"
    if total > 0:
        return "warning"
    return "normal"


def validate_row_schema(rows: list[tuple[str, Path]]) -> None:
    labels = [label for label, _ in rows]
    if labels != REQUIRED_ROWS:
        raise ValueError(
            "artifact summary row labels do not match required order: "
            f"expected={REQUIRED_ROWS} actual={labels}"
        )

    if len(set(labels)) != len(labels):
        raise ValueError("artifact summary row labels contain duplicates")


def validate_rendered_schema(lines: list[str]) -> None:
    expected_header = [
        "# Weekly Reliability Artifact Summary",
        "",
        "| Artifact | Path | Status |",
        "| --- | --- | --- |",
    ]
    if lines[:4] != expected_header:
        raise ValueError("artifact summary markdown header is malformed")

    rendered_labels: list[str] = []
    for line in lines[4:]:
        if line == "":
            break
        if not line.startswith("| "):
            raise ValueError("artifact summary table row is malformed")
        cols = [col.strip() for col in line.strip("|").split("|")]
        if len(cols) != 3:
            raise ValueError("artifact summary row must have exactly three columns")
        if cols[2] not in REQUIRED_STATUS_VALUES:
            raise ValueError(
                "artifact summary status must be present or missing: "
                f"{cols[2]!r}"
            )
        rendered_labels.append(cols[0])

    if rendered_labels != REQUIRED_ROWS:
        raise ValueError(
            "artifact summary rendered row order mismatch: "
            f"expected={REQUIRED_ROWS} actual={rendered_labels}"
        )


def main() -> int:
    if len(sys.argv) != 9:
        print(
            "usage: generate-weekly-artifact-summary.py <smoke-report> <provider-snapshot> <smoke-summary> <artifact-index> <provider-trend> <escalation-summary> <operator-onepager> <output-md>",
            file=sys.stderr,
        )
        return 1

    smoke_report = Path(sys.argv[1])
    provider_snapshot = Path(sys.argv[2])
    smoke_summary = Path(sys.argv[3])
    artifact_index = Path(sys.argv[4])
    provider_trend = Path(sys.argv[5])
    escalation_summary = Path(sys.argv[6])
    operator_onepager = Path(sys.argv[7])
    out_path = Path(sys.argv[8])

    rows = [
        ("Reliability Smoke Report", smoke_report),
        ("Provider Policy Snapshot", provider_snapshot),
        ("Weekly Smoke Ops Summary", smoke_summary),
        ("Reliability Artifact Index", artifact_index),
        ("Provider Policy Trend", provider_trend),
        ("Incident Escalation Summary", escalation_summary),
        ("Weekly Operator One-Pager", operator_onepager),
    ]
    validate_row_schema(rows)
    provider_snapshot_payload = load_json(provider_snapshot)
    fallback_budget_total = get_fallback_budget_value(provider_snapshot_payload)
    fallback_status = fallback_budget_status(fallback_budget_total)

    lines: list[str] = []
    lines.append("# Weekly Reliability Artifact Summary")
    lines.append("")
    lines.append("| Artifact | Path | Status |")
    lines.append("| --- | --- | --- |")
    for label, path in rows:
        lines.append(f"| {label} | {path} | {status(path)} |")

    lines.append("")
    lines.append("## Signal Highlights")
    lines.append("")
    lines.append("| Signal | Current Total | Highlight Status | Guidance |")
    lines.append("| --- | ---: | --- | --- |")
    rendered_total = "n/a" if fallback_budget_total is None else str(fallback_budget_total)
    lines.append(
        "| OCR Fallback Budget Exhausted Total | "
        f"{rendered_total} | {fallback_status} | "
        "Warn when 10m delta is > 0 and < 3; critical when 10m delta is >= 3. |"
    )

    validate_rendered_schema(lines)

    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(out_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
