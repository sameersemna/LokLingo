#!/usr/bin/env python3
"""Generate a compact escalation summary from trend and artifact index outputs."""

from __future__ import annotations

from datetime import datetime, timezone
from pathlib import Path
import re
import sys


def load_text(path: Path) -> str:
    if not path.exists() or path.stat().st_size == 0:
        return ""
    return path.read_text(encoding="utf-8")


def extract_critical_providers(trend_text: str) -> list[str]:
    lines = trend_text.splitlines()
    for line in lines:
        if line.startswith("- Critical Providers:"):
            value = line.split(":", 1)[1].strip()
            if not value:
                return []
            return [item.strip() for item in value.split(",") if item.strip()]
    return []


def extract_critical_count(trend_text: str) -> int:
    for line in trend_text.splitlines():
        if line.startswith("- Critical:"):
            match = re.search(r"- Critical:\s*(\d+)", line)
            if match:
                return int(match.group(1))
    return 0


def extract_index_lines(index_text: str) -> list[str]:
    wanted_prefixes = (
        "- Overall Smoke Status:",
        "- Severity Hint:",
        "- Recommended First Command:",
        "- Correlation ID:",
    )
    out: list[str] = []
    for line in index_text.splitlines():
        if line.startswith("## "):
            break
        if line.startswith(wanted_prefixes):
            out.append(line)
    return out


def extract_handoff_commands(index_text: str) -> list[str]:
    lines = index_text.splitlines()
    out: list[str] = []
    in_section = False
    for line in lines:
        if line.strip() == "## Operator Handoff Commands":
            in_section = True
            continue
        if in_section and line.startswith("## "):
            break
        if in_section and line.startswith("- "):
            out.append(line)
    return out


def main() -> int:
    if len(sys.argv) != 4:
        print(
            "usage: generate-incident-escalation-summary.py <provider-trend.md> <artifact-index.md> <output.md>",
            file=sys.stderr,
        )
        return 1

    trend_path = Path(sys.argv[1])
    index_path = Path(sys.argv[2])
    out_path = Path(sys.argv[3])

    trend_text = load_text(trend_path)
    index_text = load_text(index_path)

    critical_count = extract_critical_count(trend_text)
    critical_providers = extract_critical_providers(trend_text)
    index_lines = extract_index_lines(index_text)
    commands = extract_handoff_commands(index_text)

    now = datetime.now(timezone.utc).replace(microsecond=0).isoformat()

    lines: list[str] = []
    lines.append("# Weekly Incident Escalation Summary")
    lines.append("")
    lines.append(f"- Generated At: {now}")
    lines.append(f"- Critical Regression Count: {critical_count}")
    lines.append("")

    lines.append("## Triage Snapshot")
    lines.append("")
    if index_lines:
        lines.extend(index_lines)
    else:
        lines.append("- No index triage lines were available.")

    lines.append("")
    lines.append("## Critical Providers")
    lines.append("")
    if critical_providers:
        for provider in critical_providers:
            lines.append(f"- {provider}")
    else:
        lines.append("- none")

    lines.append("")
    lines.append("## Immediate Commands")
    lines.append("")
    if commands:
        lines.extend(commands)
    else:
        lines.append("- No operator handoff commands were found.")

    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(out_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
