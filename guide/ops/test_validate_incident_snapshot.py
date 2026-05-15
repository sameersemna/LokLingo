#!/usr/bin/env python3
"""Regression checks for incident snapshot validation."""

from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
VALIDATOR = ROOT / "guide" / "ops" / "validate-incident-snapshot.py"


def base_payload() -> dict:
    return {
        "generated_at": "2026-05-13T00:00:00Z",
        "correlation_id": "validator-test",
        "lifecycle_events": {"events": [{"stage": "ingest"}]},
        "provider_metrics": {},
        "ocr_metrics": {},
        "provider_policy_snapshot": {
            "generated_at": "2026-05-13T00:00:00Z",
            "provider_health_snapshot": {},
            "degraded_and_queue_signals": {
                "loklingo_degraded_mode_total": 0,
                "loklingo_degraded_render_fallback_total": 0,
                "loklingo_degraded_ocr_low_confidence_total": 0,
                "loklingo_ocr_provider_fallback_budget_exhausted_total": 0,
                "loklingo_adaptive_concurrency_reduce_total": 0,
                "loklingo_adaptive_concurrency_boost_total": 0,
                "loklingo_adaptive_concurrency_clamp_total": 0,
                "loklingo_queue_depth": 0,
                "loklingo_queue_retry_backlog": 0,
            },
            "provider_policy_scores": {"litellm": 0.8},
        },
        "derived_summary": {
            "lifecycle_event_count": 1,
            "lifecycle_stage_counts": {"ingest": 1},
            "queue_depth": 0,
            "retry_backlog": 0,
            "degraded_total": 0,
            "render_fallback_total": 0,
            "ocr_low_confidence_total": 0,
            "provider_timeouts_total": 0,
            "provider_retries_total": 0,
            "ocr_rejected_total": 0,
            "lowest_provider_policy": {"provider": "litellm", "score": 0.8},
            "triage_severity_hint": "normal",
            "triage_trigger_reasons": [],
        },
    }


class ValidateIncidentSnapshotTests(unittest.TestCase):
    def run_validator(self, payload: dict) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as tmpdir:
            path = Path(tmpdir) / "incident.json"
            path.write_text(json.dumps(payload), encoding="utf-8")
            return subprocess.run(
                ["python3", str(VALIDATOR), str(path)],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )

    def test_accepts_valid_payload(self) -> None:
        result = self.run_validator(base_payload())
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_out_of_range_provider_score(self) -> None:
        payload = base_payload()
        payload["provider_policy_snapshot"]["provider_policy_scores"]["litellm"] = 1.5

        result = self.run_validator(payload)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("within [0, 1]", result.stderr)

    def test_rejects_missing_derived_summary_key(self) -> None:
        payload = base_payload()
        del payload["derived_summary"]["triage_trigger_reasons"]

        result = self.run_validator(payload)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing derived_summary keys", result.stderr)

    def test_rejects_missing_degraded_signal_key(self) -> None:
        payload = base_payload()
        del payload["provider_policy_snapshot"]["degraded_and_queue_signals"]["loklingo_queue_depth"]

        result = self.run_validator(payload)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing degraded_and_queue_signals keys", result.stderr)

    def test_rejects_non_numeric_degraded_signal_value(self) -> None:
        payload = base_payload()
        payload["provider_policy_snapshot"]["degraded_and_queue_signals"]["loklingo_queue_retry_backlog"] = "high"

        result = self.run_validator(payload)

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("must be numeric", result.stderr)


if __name__ == "__main__":
    unittest.main()