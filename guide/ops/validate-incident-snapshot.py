#!/usr/bin/env python3
"""Validate incident snapshot JSON structure for CI and ops checks."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


REQUIRED_ROOT_KEYS = [
    "generated_at",
    "correlation_id",
    "lifecycle_events",
    "provider_metrics",
    "ocr_metrics",
    "provider_policy_snapshot",
    "derived_summary",
]

REQUIRED_DERIVED_KEYS = [
    "lifecycle_event_count",
    "lifecycle_stage_counts",
    "queue_depth",
    "retry_backlog",
    "degraded_total",
    "render_fallback_total",
    "ocr_low_confidence_total",
    "provider_timeouts_total",
    "provider_retries_total",
    "ocr_rejected_total",
    "lowest_provider_policy",
    "triage_severity_hint",
    "triage_trigger_reasons",
]

ALLOWED_SEVERITIES = {"normal", "warning", "critical"}

REQUIRED_PROVIDER_SIGNALS = [
    "loklingo_degraded_mode_total",
    "loklingo_degraded_render_fallback_total",
    "loklingo_degraded_ocr_low_confidence_total",
    "loklingo_ocr_provider_fallback_budget_exhausted_total",
    "loklingo_adaptive_concurrency_reduce_total",
    "loklingo_adaptive_concurrency_boost_total",
    "loklingo_adaptive_concurrency_clamp_total",
    "loklingo_queue_depth",
    "loklingo_queue_retry_backlog",
]


def fail(message: str) -> None:
    print(f"invalid incident snapshot: {message}", file=sys.stderr)
    raise SystemExit(1)


def ensure(condition: bool, message: str) -> None:
    if not condition:
        fail(message)


def ensure_number_or_null(value: Any, field_name: str) -> None:
    ensure(value is None or isinstance(value, (int, float)), f"{field_name} must be numeric or null")


def validate_provider_policy_snapshot(snapshot: Any) -> None:
    ensure(isinstance(snapshot, dict), "provider_policy_snapshot must be an object")

    required_snapshot_keys = [
        "generated_at",
        "provider_health_snapshot",
        "degraded_and_queue_signals",
        "provider_policy_scores",
    ]
    missing_snapshot = [key for key in required_snapshot_keys if key not in snapshot]
    ensure(not missing_snapshot, f"missing provider_policy_snapshot keys: {missing_snapshot}")

    signals = snapshot["degraded_and_queue_signals"]
    ensure(isinstance(signals, dict), "provider_policy_snapshot.degraded_and_queue_signals must be an object")
    missing_signals = [key for key in REQUIRED_PROVIDER_SIGNALS if key not in signals]
    ensure(not missing_signals, f"missing degraded_and_queue_signals keys: {missing_signals}")
    for signal in REQUIRED_PROVIDER_SIGNALS:
        ensure(isinstance(signals[signal], (int, float)), f"degraded_and_queue_signals.{signal} must be numeric")

    scores = snapshot["provider_policy_scores"]
    ensure(isinstance(scores, dict), "provider_policy_snapshot.provider_policy_scores must be an object")
    for provider, score in scores.items():
        ensure(isinstance(provider, str) and provider != "", "provider policy score keys must be non-empty strings")
        ensure(isinstance(score, (int, float)), f"provider policy score for {provider} must be numeric")
        ensure(0 <= score <= 1, f"provider policy score for {provider} must be within [0, 1]")


def validate(path: Path) -> None:
    ensure(path.exists(), f"file not found: {path}")
    ensure(path.stat().st_size > 0, f"file is empty: {path}")

    with path.open("r", encoding="utf-8") as handle:
        payload = json.load(handle)

    ensure(isinstance(payload, dict), "root payload must be an object")

    missing_root = [key for key in REQUIRED_ROOT_KEYS if key not in payload]
    ensure(not missing_root, f"missing root keys: {missing_root}")

    validate_provider_policy_snapshot(payload["provider_policy_snapshot"])

    derived = payload["derived_summary"]
    ensure(isinstance(derived, dict), "derived_summary must be an object")

    missing_derived = [key for key in REQUIRED_DERIVED_KEYS if key not in derived]
    ensure(not missing_derived, f"missing derived_summary keys: {missing_derived}")

    ensure(isinstance(derived["lifecycle_event_count"], int), "lifecycle_event_count must be an integer")
    ensure(isinstance(derived["lifecycle_stage_counts"], dict), "lifecycle_stage_counts must be an object")

    ensure_number_or_null(derived["queue_depth"], "queue_depth")
    ensure_number_or_null(derived["retry_backlog"], "retry_backlog")
    ensure_number_or_null(derived["degraded_total"], "degraded_total")
    ensure_number_or_null(derived["render_fallback_total"], "render_fallback_total")
    ensure_number_or_null(derived["ocr_low_confidence_total"], "ocr_low_confidence_total")
    ensure_number_or_null(derived["provider_timeouts_total"], "provider_timeouts_total")
    ensure_number_or_null(derived["provider_retries_total"], "provider_retries_total")
    ensure_number_or_null(derived["ocr_rejected_total"], "ocr_rejected_total")

    severity = derived["triage_severity_hint"]
    ensure(isinstance(severity, str), "triage_severity_hint must be a string")
    ensure(severity in ALLOWED_SEVERITIES, f"triage_severity_hint must be one of {sorted(ALLOWED_SEVERITIES)}")

    reasons = derived["triage_trigger_reasons"]
    ensure(isinstance(reasons, list), "triage_trigger_reasons must be an array")
    ensure(all(isinstance(item, str) for item in reasons), "triage_trigger_reasons must contain strings")

    lowest_policy = derived["lowest_provider_policy"]
    ensure(
        lowest_policy is None or isinstance(lowest_policy, dict),
        "lowest_provider_policy must be null or an object",
    )
    if isinstance(lowest_policy, dict):
        ensure("provider" in lowest_policy, "lowest_provider_policy.provider is required")
        ensure("score" in lowest_policy, "lowest_provider_policy.score is required")
        ensure(isinstance(lowest_policy["provider"], str), "lowest_provider_policy.provider must be a string")
        ensure(isinstance(lowest_policy["score"], (int, float)), "lowest_provider_policy.score must be numeric")

    print(f"incident-snapshot-valid: {path}")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("usage: validate-incident-snapshot.py <incident-snapshot.json>", file=sys.stderr)
        raise SystemExit(1)
    validate(Path(sys.argv[1]))
