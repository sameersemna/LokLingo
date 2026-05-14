#!/usr/bin/env python3
"""Generate a compact operator-facing summary from smoke and incident artifacts."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


def load_json(path: Path) -> dict[str, Any]:
    if not path.exists() or path.stat().st_size == 0:
        return {}
    with path.open("r", encoding="utf-8") as handle:
        payload = json.load(handle)
    return payload if isinstance(payload, dict) else {}


ALERT_THRESHOLD_GUIDANCE = [
    ("Queue Depth", "loklingo_queue_depth", "warn >= 500, critical >= 1000"),
    ("Retry Backlog", "loklingo_queue_retry_backlog", "warn >= 50, critical >= 200"),
    ("Degraded Mode Total", "loklingo_degraded_mode_total", "watch delta > 20 over 10m"),
    ("Render Fallback Total", "loklingo_degraded_render_fallback_total", "watch delta > 5 over 10m"),
    ("Adaptive Clamp Total", "loklingo_adaptive_concurrency_clamp_total", "watch delta > 10 over 10m"),
]


def load_text(path: Path) -> str:
    if not path.exists() or path.stat().st_size == 0:
        return ""
    return path.read_text(encoding="utf-8").strip()


def main() -> int:
    if len(sys.argv) != 5:
        print(
            "usage: generate-smoke-ops-summary.py <smoke-report.json> <incident-brief.md|NONE> <provider-policy-snapshot.json|NONE> <output.md>",
            file=sys.stderr,
        )
        return 1

    smoke_path = Path(sys.argv[1])
    incident_arg = sys.argv[2]
    provider_snapshot_arg = sys.argv[3]
    output_path = Path(sys.argv[4])

    smoke = load_json(smoke_path)
    incident_path = None if incident_arg == "NONE" else Path(incident_arg)
    provider_snapshot_path = None if provider_snapshot_arg == "NONE" else Path(provider_snapshot_arg)
    incident_brief = "" if incident_path is None else load_text(incident_path)
    provider_snapshot = {} if provider_snapshot_path is None else load_json(provider_snapshot_path)

    suites = smoke.get("suites") or []
    if not isinstance(suites, list):
        suites = []

    lines: list[str] = []
    lines.append("# Reliability Smoke Ops Summary")
    lines.append("")
    lines.append(f"- Generated At: {smoke.get('generated_at', 'n/a')}")
    lines.append(f"- Overall Status: {smoke.get('overall_status', 'unknown')}")
    lines.append(f"- Correlation ID: {smoke.get('correlation_id') or smoke.get('last_correlation_id') or 'n/a'}")
    lines.append(f"- Job ID: {smoke.get('job_id') or smoke.get('last_job_id') or 'n/a'}")
    lines.append(f"- Lifecycle Event Count: {smoke.get('lifecycle_event_count', 'n/a')}")
    lines.append(f"- Lineage Verified: {smoke.get('lineage_verified', 'unknown')}")
    lines.append("")
    lines.append("## Suite Results")
    lines.append("")
    lines.append("| Suite | Status | Duration (s) | Note |")
    lines.append("| --- | --- | ---: | --- |")
    for suite in suites:
        if not isinstance(suite, dict):
            continue
        lines.append(
            f"| {suite.get('name', 'unknown')} | {suite.get('status', 'unknown')} | {suite.get('duration_seconds', 'n/a')} | {suite.get('note', '')} |"
        )
    if not suites:
        lines.append("| none | unknown | n/a | no suites recorded |")

    lines.append("")
    lines.append("## Provider Policy Snapshot")
    lines.append("")
    scores = provider_snapshot.get("provider_policy_scores") if isinstance(provider_snapshot, dict) else {}
    if isinstance(scores, dict) and scores:
        ranked = sorted(
            ((provider, score) for provider, score in scores.items() if isinstance(score, (int, float))),
            key=lambda item: item[1],
        )
        if ranked:
            lowest_provider, lowest_score = ranked[0]
            highest_provider, highest_score = ranked[-1]
            lines.append(f"- Lowest Provider Score: {lowest_provider} ({lowest_score:.2f})")
            lines.append(f"- Highest Provider Score: {highest_provider} ({highest_score:.2f})")
        else:
            lines.append("- Provider policy scores were present but not numeric.")
    else:
        lines.append("No provider policy snapshot was available for this run.")

    lines.append("")
    lines.append("## Degraded Signal Threshold Reference")
    lines.append("")
    lines.append("| Signal | Current Value | Threshold Guidance |")
    lines.append("| --- | ---: | --- |")
    signals = provider_snapshot.get("degraded_and_queue_signals") if isinstance(provider_snapshot, dict) else {}
    if not isinstance(signals, dict):
        signals = {}
    for label, key, guidance in ALERT_THRESHOLD_GUIDANCE:
        value = signals.get(key, "n/a")
        lines.append(f"| {label} | {value} | {guidance} |")

    lines.append("")
    lines.append("## Incident Brief")
    lines.append("")
    if incident_brief:
        lines.append(incident_brief)
    else:
        lines.append("No incident brief was generated for this run.")

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text("\n".join(lines).strip() + "\n", encoding="utf-8")
    print(output_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())