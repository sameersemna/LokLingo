#!/usr/bin/env python3
"""Generate a compact weekly operator one-pager from summary, trend, and index artifacts."""

from __future__ import annotations

from datetime import datetime, timezone
from pathlib import Path
import sys


def load_text(path: Path) -> str:
    if not path.exists() or path.stat().st_size == 0:
        return ""
    return path.read_text(encoding="utf-8")


def extract_section(lines: list[str], heading: str) -> list[str]:
    header = f"## {heading}"
    start = -1
    for i, line in enumerate(lines):
        if line.strip() == header:
            start = i + 1
            break
    if start < 0:
        return []

    out: list[str] = []
    for line in lines[start:]:
        if line.startswith("## "):
            break
        out.append(line)
    return [line for line in out if line.strip()]


def extract_summary_snapshot(lines: list[str]) -> list[str]:
    out: list[str] = []
    for line in lines:
        if line.startswith("## "):
            break
        if line.startswith("- "):
            out.append(line)
    return out


def extract_index_snapshot(lines: list[str]) -> list[str]:
    wanted_prefixes = (
        "- Overall Smoke Status:",
        "- Severity Hint:",
        "- Recommended First Command:",
        "- Correlation ID:",
    )
    out: list[str] = []
    for line in lines:
        if line.startswith("## "):
            break
        if line.startswith(wanted_prefixes):
            out.append(line)
    return out


def extract_fallback_budget_signal(lines: list[str]) -> str | None:
    for line in lines:
        if not line.startswith("| OCR Fallback Budget Exhausted Total |"):
            continue
        cols = [col.strip() for col in line.strip("|").split("|")]
        if len(cols) < 3:
            return None
        value = cols[1]
        guidance = cols[2]
        status = "unknown"
        try:
            numeric = float(value)
            status = "warning" if numeric > 0 else "normal"
        except ValueError:
            status = "unknown"
        return (
            f"- OCR Fallback Budget Exhausted Signal: total={value}, "
            f"status={status}, guidance={guidance}"
        )
    return None


def main() -> int:
    if len(sys.argv) != 5:
        print(
            "usage: generate-weekly-operator-onepager.py <smoke-summary.md> <provider-trend.md> <artifact-index.md> <output.md>",
            file=sys.stderr,
        )
        return 1

    smoke_summary_path = Path(sys.argv[1])
    provider_trend_path = Path(sys.argv[2])
    artifact_index_path = Path(sys.argv[3])
    output_path = Path(sys.argv[4])

    smoke_summary_lines = load_text(smoke_summary_path).splitlines()
    provider_trend_lines = load_text(provider_trend_path).splitlines()
    artifact_index_lines = load_text(artifact_index_path).splitlines()

    smoke_snapshot = extract_summary_snapshot(smoke_summary_lines)
    index_snapshot = extract_index_snapshot(artifact_index_lines)
    fallback_budget_signal = extract_fallback_budget_signal(smoke_summary_lines)
    severity_totals = extract_section(provider_trend_lines, "Severity Totals")
    escalation = extract_section(provider_trend_lines, "Escalation")
    handoff_commands = extract_section(artifact_index_lines, "Operator Handoff Commands")

    now = datetime.now(timezone.utc).replace(microsecond=0).isoformat()

    lines: list[str] = []
    lines.append("# Weekly Reliability Operator One-Pager")
    lines.append("")
    lines.append(f"- Generated At: {now}")
    lines.append("")

    lines.append("## Operator Triage Snapshot")
    lines.append("")
    if index_snapshot:
        lines.extend(index_snapshot)
    else:
        lines.append("- No index snapshot lines were found.")
    if fallback_budget_signal:
        lines.append(fallback_budget_signal)
    else:
        lines.append("- OCR Fallback Budget Exhausted Signal: unavailable in smoke summary.")

    lines.append("")

    lines.append("## Smoke Summary Snapshot")
    lines.append("")
    if smoke_snapshot:
        lines.extend(smoke_snapshot)
    else:
        lines.append("- No smoke summary snapshot lines were found.")

    lines.append("")
    lines.append("## Provider Trend Highlights")
    lines.append("")
    if severity_totals:
        lines.extend(severity_totals)
    else:
        lines.append("- Severity totals were not available.")
    if escalation:
        lines.append("")
        lines.append("Escalation:")
        for entry in escalation:
            lines.append(f"- {entry.lstrip('- ').strip()}")

    lines.append("")
    lines.append("## Operator Handoff Commands")
    lines.append("")
    if handoff_commands:
        for entry in handoff_commands:
            if entry.startswith("- "):
                lines.append(entry)
    else:
        lines.append("- No handoff commands were found.")

    lines.append("")
    lines.append("## Linked Artifacts")
    lines.append("")
    lines.append(f"- Smoke Ops Summary: {smoke_summary_path}")
    lines.append(f"- Provider Policy Trend: {provider_trend_path}")
    lines.append(f"- Reliability Artifact Index: {artifact_index_path}")

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(output_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
