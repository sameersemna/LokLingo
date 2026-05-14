#!/usr/bin/env python3
"""Generate a concise markdown incident brief from an incident snapshot JSON file."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


def fmt_value(value: Any) -> str:
    if value is None:
        return "n/a"
    if isinstance(value, float):
        return f"{value:.2f}"
    return str(value)


def generate_markdown(payload: dict[str, Any]) -> str:
    summary = payload.get("derived_summary") or {}
    stage_counts = summary.get("lifecycle_stage_counts") or {}
    lowest_policy = summary.get("lowest_provider_policy")

    reasons = summary.get("triage_trigger_reasons") or []
    reasons_text = ", ".join(reasons) if reasons else "none"

    lines: list[str] = []
    lines.append("# LokLingo Incident Brief")
    lines.append("")
    lines.append(f"- Generated At: {fmt_value(payload.get('generated_at'))}")
    lines.append(f"- Correlation ID: {fmt_value(payload.get('correlation_id'))}")
    lines.append(f"- Triage Severity Hint: {fmt_value(summary.get('triage_severity_hint'))}")
    lines.append(f"- Trigger Reasons: {reasons_text}")
    lines.append("")

    lines.append("## Signal Snapshot")
    lines.append("")
    lines.append("| Signal | Value |")
    lines.append("| --- | --- |")
    signal_rows = [
        ("Queue Depth", summary.get("queue_depth")),
        ("Retry Backlog", summary.get("retry_backlog")),
        ("Degraded Total", summary.get("degraded_total")),
        ("Render Fallback Total", summary.get("render_fallback_total")),
        ("OCR Low Confidence Total", summary.get("ocr_low_confidence_total")),
        ("Provider Timeouts Total", summary.get("provider_timeouts_total")),
        ("Provider Retries Total", summary.get("provider_retries_total")),
        ("OCR Rejected Total", summary.get("ocr_rejected_total")),
        ("Lifecycle Event Count", summary.get("lifecycle_event_count")),
    ]
    for name, value in signal_rows:
        lines.append(f"| {name} | {fmt_value(value)} |")
    lines.append("")

    lines.append("## Lifecycle Stage Counts")
    lines.append("")
    lines.append("| Stage | Count |")
    lines.append("| --- | --- |")
    if isinstance(stage_counts, dict) and stage_counts:
        for stage, count in sorted(stage_counts.items(), key=lambda item: item[0]):
            lines.append(f"| {stage} | {fmt_value(count)} |")
    else:
        lines.append("| n/a | 0 |")
    lines.append("")

    lines.append("## Provider Policy")
    lines.append("")
    if isinstance(lowest_policy, dict):
        lines.append(f"- Lowest Provider: {fmt_value(lowest_policy.get('provider'))}")
        lines.append(f"- Lowest Score: {fmt_value(lowest_policy.get('score'))}")
    else:
        lines.append("- Lowest Provider: n/a")
        lines.append("- Lowest Score: n/a")
    lines.append("")

    lines.append("## Operator Actions")
    lines.append("")
    lines.append("1. Validate queue and retry pressure using guide/ops/reliability-recovery.sh queue-status.")
    lines.append("2. Capture provider policy snapshot using guide/ops/reliability-recovery.sh provider-policy-snapshot.")
    lines.append("3. Export full incident packet using guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>.")

    return "\n".join(lines) + "\n"


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: generate-incident-brief.py <incident-snapshot.json> <brief-output.md>", file=sys.stderr)
        return 1

    in_path = Path(sys.argv[1])
    out_path = Path(sys.argv[2])

    if not in_path.exists() or in_path.stat().st_size == 0:
        print(f"input snapshot missing or empty: {in_path}", file=sys.stderr)
        return 1

    with in_path.open("r", encoding="utf-8") as handle:
        payload = json.load(handle)
    if not isinstance(payload, dict):
        print("input payload must be a JSON object", file=sys.stderr)
        return 1

    markdown = generate_markdown(payload)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(markdown, encoding="utf-8")
    print(out_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
