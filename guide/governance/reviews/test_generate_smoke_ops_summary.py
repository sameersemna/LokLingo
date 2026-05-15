#!/usr/bin/env python3
"""Regression checks for smoke ops summary generation."""

from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "guide" / "governance" / "reviews" / "generate-smoke-ops-summary.py"


class GenerateSmokeOpsSummaryTests(unittest.TestCase):
    def test_includes_fallback_budget_threshold_row(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            provider_snapshot = tmp / "provider.json"
            out = tmp / "summary.md"

            smoke.write_text(
                json.dumps(
                    {
                        "generated_at": "2026-05-15T00:00:00Z",
                        "overall_status": "ok",
                        "correlation_id": "corr-smoke",
                        "job_id": "job-smoke",
                        "lifecycle_event_count": 8,
                        "lineage_verified": True,
                        "suites": [
                            {
                                "name": "pdf",
                                "status": "passed",
                                "duration_seconds": 42,
                                "note": "all good",
                            }
                        ],
                    }
                ),
                encoding="utf-8",
            )
            provider_snapshot.write_text(
                json.dumps(
                    {
                        "provider_policy_scores": {"paddle": 0.7, "tesseract": 0.9},
                        "degraded_and_queue_signals": {
                            "loklingo_queue_depth": 2,
                            "loklingo_queue_retry_backlog": 0,
                            "loklingo_degraded_mode_total": 1,
                            "loklingo_degraded_render_fallback_total": 0,
                            "loklingo_ocr_provider_fallback_budget_exhausted_total": 4,
                            "loklingo_adaptive_concurrency_clamp_total": 0,
                        },
                    }
                ),
                encoding="utf-8",
            )

            result = subprocess.run(
                [
                    "python3",
                    str(SCRIPT),
                    str(smoke),
                    "NONE",
                    str(provider_snapshot),
                    str(out),
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)

            generated = out.read_text(encoding="utf-8")
            self.assertIn("OCR Fallback Budget Exhausted Total", generated)
            self.assertIn("warn delta > 0 and < 3 over 10m, critical delta >= 3 over 10m", generated)
            self.assertIn("| OCR Fallback Budget Exhausted Total | 4 |", generated)


if __name__ == "__main__":
    unittest.main()
