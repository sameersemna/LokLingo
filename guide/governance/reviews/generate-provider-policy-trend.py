#!/usr/bin/env python3
"""Generate provider policy score trend summary from current and optional prior snapshots."""

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


def get_scores(payload: dict[str, Any]) -> dict[str, float]:
    raw = payload.get("provider_policy_scores")
    if not isinstance(raw, dict):
        return {}
    out: dict[str, float] = {}
    for provider, score in raw.items():
        if isinstance(provider, str) and provider and isinstance(score, (int, float)):
            out[provider] = float(score)
    return out


def severity_for_delta(delta: float) -> str:
    if delta <= -0.10:
        return "critical"
    if delta <= -0.05:
        return "warning"
    if delta >= 0.05:
        return "improving"
    return "stable"


def main() -> int:
    if len(sys.argv) != 4:
        print(
            "usage: generate-provider-policy-trend.py <current-provider-snapshot.json> <previous-provider-snapshot.json|NONE> <output.md>",
            file=sys.stderr,
        )
        return 1

    current_path = Path(sys.argv[1])
    previous_arg = sys.argv[2]
    out_path = Path(sys.argv[3])

    current_scores = get_scores(load_json(current_path))
    previous_scores = {} if previous_arg == "NONE" else get_scores(load_json(Path(previous_arg)))

    lines: list[str] = []
    lines.append("# Provider Policy Trend")
    lines.append("")
    lines.append("| Provider | Current | Previous | Delta | Severity |")
    lines.append("| --- | ---: | ---: | ---: | --- |")

    severity_counts = {
        "critical": 0,
        "warning": 0,
        "stable": 0,
        "improving": 0,
        "n/a": 0,
    }
    critical_providers: list[str] = []

    providers = sorted(set(current_scores) | set(previous_scores))
    if not providers:
        lines.append("| n/a | n/a | n/a | n/a | n/a |")
        severity_counts["n/a"] += 1
    else:
        for provider in providers:
            cur = current_scores.get(provider)
            prev = previous_scores.get(provider)
            severity = "n/a"
            if cur is None:
                delta = "n/a"
                cur_s = "n/a"
            else:
                cur_s = f"{cur:.2f}"
                if prev is None:
                    delta = "n/a"
                else:
                    delta_value = cur - prev
                    delta = f"{delta_value:+.2f}"
                    severity = severity_for_delta(delta_value)
                    if severity == "critical":
                        critical_providers.append(provider)
            prev_s = "n/a" if prev is None else f"{prev:.2f}"
            lines.append(f"| {provider} | {cur_s} | {prev_s} | {delta} | {severity} |")
            severity_counts[severity] = severity_counts.get(severity, 0) + 1

    lines.append("")
    lines.append("## Severity Totals")
    lines.append("")
    lines.append(f"- Critical: {severity_counts.get('critical', 0)}")
    lines.append(f"- Warning: {severity_counts.get('warning', 0)}")
    lines.append(f"- Stable: {severity_counts.get('stable', 0)}")
    lines.append(f"- Improving: {severity_counts.get('improving', 0)}")

    if critical_providers:
        lines.append("")
        lines.append("## Escalation")
        lines.append("")
        lines.append("Critical provider policy regressions detected; execute immediate triage:")
        lines.append(
            "- bash guide/ops/reliability-recovery.sh provider-policy-snapshot guide/provider-policy-snapshot.json"
        )
        lines.append("- bash guide/ops/reliability-recovery.sh queue-status")
        lines.append(
            "- bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id> guide/incident-snapshot.json"
        )
        lines.append(
            f"- Critical Providers: {', '.join(sorted(set(critical_providers)))}"
        )

    if previous_arg == "NONE":
        lines.append("")
        lines.append("Previous snapshot was not provided; deltas are unavailable for this run.")

    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(out_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
