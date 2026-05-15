#!/usr/bin/env python3
"""Fail when mandatory weekly reliability artifacts are missing or empty."""

from __future__ import annotations

import sys
from pathlib import Path
import re
from datetime import datetime, timezone
import os


def is_present(path: Path) -> bool:
    return path.exists() and path.stat().st_size > 0


def has_required_onepager_sections(path: Path) -> bool:
    if not is_present(path):
        return False
    text = path.read_text(encoding="utf-8")
    required_markers = [
        "## Operator Triage Snapshot",
        "- Severity Hint:",
        "- Recommended First Command:",
        "- Correlation ID:",
        "## Operator Handoff Commands",
    ]
    return all(marker in text for marker in required_markers)


def extract_smoke_fallback_budget_total(path: Path) -> tuple[str | None, bool]:
    if not is_present(path):
        return None, False
    text = path.read_text(encoding="utf-8")
    for line in text.splitlines():
        if not line.startswith("| OCR Fallback Budget Exhausted Total |"):
            continue
        cols = [col.strip() for col in line.strip("|").split("|")]
        if len(cols) < 3:
            return None, True
        value = cols[1]
        guidance = cols[2]
        if value == "" or guidance == "":
            return None, True
        return value, False
    return None, False


def onepager_has_fallback_signal(path: Path, expected_total: str) -> bool:
    if not is_present(path):
        return False
    text = path.read_text(encoding="utf-8")
    marker = f"- OCR Fallback Budget Exhausted Signal: total={expected_total},"
    return marker in text


def has_valid_provider_trend_structure(path: Path) -> bool:
    if not is_present(path):
        return False
    text = path.read_text(encoding="utf-8")
    required_markers = [
        "# Provider Policy Trend",
        "## Severity Totals",
        "- Critical:",
        "- Warning:",
        "- Stable:",
        "- Improving:",
    ]
    if not all(marker in text for marker in required_markers):
        return False

    critical_count = 0
    saw_critical_line = False
    parsed_critical_line = False
    for line in text.splitlines():
        if line.startswith("- Critical:"):
            saw_critical_line = True
            match = re.search(r"- Critical:\s*(\d+)", line)
            if match:
                critical_count = int(match.group(1))
                parsed_critical_line = True
            break

    if saw_critical_line and not parsed_critical_line:
        return False

    if critical_count > 0 and "## Escalation" not in text:
        return False

    return True


def has_valid_escalation_summary_structure(path: Path) -> bool:
    if not is_present(path):
        return False
    text = path.read_text(encoding="utf-8")
    required_markers = [
        "# Weekly Incident Escalation Summary",
        "- Critical Regression Count:",
        "## Triage Snapshot",
        "- Severity Hint:",
        "- Recommended First Command:",
        "- Correlation ID:",
        "## Critical Providers",
        "## Immediate Commands",
    ]
    if not all(marker in text for marker in required_markers):
        return False

    critical_count = 0
    for line in text.splitlines():
        if line.startswith("- Critical Regression Count:"):
            match = re.search(r"- Critical Regression Count:\s*(\d+)", line)
            if match:
                critical_count = int(match.group(1))
            break

    if critical_count > 0:
        # For critical regressions, summary must include concrete providers and commands.
        if "## Critical Providers\n\n- none" in text:
            return False
        has_bash_command = any(line.startswith("- bash ") for line in text.splitlines())
        if not has_bash_command:
            return False

    return True


def escalation_summary_is_fresh(path: Path, max_age_hours: int) -> bool:
    if max_age_hours <= 0:
        return True
    text = path.read_text(encoding="utf-8")
    generated_line = None
    for line in text.splitlines():
        if line.startswith("- Generated At:"):
            generated_line = line
            break
    if not generated_line:
        return False

    value = generated_line.split(":", 1)[1].strip()
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"

    try:
        generated_at = datetime.fromisoformat(value)
    except ValueError:
        return False

    if generated_at.tzinfo is None:
        generated_at = generated_at.replace(tzinfo=timezone.utc)

    age_seconds = (datetime.now(timezone.utc) - generated_at).total_seconds()
    return age_seconds <= max_age_hours * 3600


def main() -> int:
    if len(sys.argv) != 8:
        print(
            "usage: validate-reliability-artifacts.py <smoke-report> <provider-snapshot> <smoke-summary> <artifact-index> <provider-trend> <escalation-summary> <operator-onepager>",
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
    max_age_hours = int(os.environ.get("RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS", "0"))

    required = [
        ("reliability smoke report", smoke_report),
        ("provider policy snapshot", provider_snapshot),
        ("weekly smoke summary", smoke_summary),
        ("artifact index", artifact_index),
        ("provider policy trend", provider_trend),
        ("incident escalation summary", escalation_summary),
        ("weekly operator one-pager", operator_onepager),
    ]

    missing = [name for name, path in required if not is_present(path)]
    if missing:
        print(f"missing mandatory artifacts: {missing}", file=sys.stderr)
        return 1

    if not has_required_onepager_sections(operator_onepager):
        print(
            "weekly operator one-pager is missing required triage snapshot or handoff sections",
            file=sys.stderr,
        )
        return 1

    fallback_total, fallback_row_malformed = extract_smoke_fallback_budget_total(smoke_summary)
    if fallback_row_malformed:
        print(
            "weekly smoke summary has malformed OCR fallback-budget signal row",
            file=sys.stderr,
        )
        return 1

    if fallback_total is not None and fallback_total.lower() != "n/a":
        if not onepager_has_fallback_signal(operator_onepager, fallback_total):
            print(
                "weekly operator one-pager is missing required OCR fallback-budget triage signal line",
                file=sys.stderr,
            )
            return 1

    if not has_valid_provider_trend_structure(provider_trend):
        print(
            "provider policy trend artifact is missing required severity structure or escalation section",
            file=sys.stderr,
        )
        return 1

    if not has_valid_escalation_summary_structure(escalation_summary):
        print(
            "incident escalation summary is missing required triage markers, command bundle, or critical-regression details",
            file=sys.stderr,
        )
        return 1

    if not escalation_summary_is_fresh(escalation_summary, max_age_hours):
        print(
            "incident escalation summary generated timestamp is missing, invalid, or too old for configured freshness window",
            file=sys.stderr,
        )
        return 1

    print("mandatory-artifacts-valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
