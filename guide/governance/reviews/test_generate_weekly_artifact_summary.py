#!/usr/bin/env python3
"""Regression checks for weekly artifact summary generation."""

from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "guide" / "governance" / "reviews" / "generate-weekly-artifact-summary.py"


class GenerateWeeklyArtifactSummaryTests(unittest.TestCase):
    def test_generates_status_table(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"
            out = tmp / "artifact-summary.md"

            smoke.write_text("ok\n", encoding="utf-8")
            snapshot.write_text(
                json.dumps(
                    {
                        "degraded_and_queue_signals": {
                            "loklingo_ocr_provider_fallback_budget_exhausted_total": 2
                        }
                    }
                ),
                encoding="utf-8",
            )
            for path in [summary, index, trend, escalation, onepager]:
                path.write_text("ok\n", encoding="utf-8")

            result = subprocess.run(
                [
                    "python3",
                    str(SCRIPT),
                    str(smoke),
                    str(snapshot),
                    str(summary),
                    str(index),
                    str(trend),
                    str(escalation),
                    str(onepager),
                    str(out),
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)

            rendered = out.read_text(encoding="utf-8")
            self.assertIn("# Weekly Reliability Artifact Summary", rendered)
            self.assertIn("| Reliability Smoke Report |", rendered)
            self.assertIn("| Weekly Operator One-Pager |", rendered)
            self.assertIn("| present |", rendered)
            self.assertIn("## Signal Highlights", rendered)
            self.assertIn("| OCR Fallback Budget Exhausted Total | 2 | warning |", rendered)

            rows = [line for line in rendered.splitlines() if line.startswith("| ")]
            table_break_idx = rows.index("| Signal | Current Total | Highlight Status | Guidance |")
            lines = rows[:table_break_idx]
            self.assertEqual(len(lines), 9)
            self.assertEqual(lines[0], "| Artifact | Path | Status |")
            self.assertEqual(lines[1], "| --- | --- | --- |")
            expected_labels = [
                "Reliability Smoke Report",
                "Provider Policy Snapshot",
                "Weekly Smoke Ops Summary",
                "Reliability Artifact Index",
                "Provider Policy Trend",
                "Incident Escalation Summary",
                "Weekly Operator One-Pager",
            ]
            parsed_labels = [line.split("|")[1].strip() for line in lines[2:]]
            self.assertEqual(parsed_labels, expected_labels)
            parsed_statuses = [line.split("|")[3].strip() for line in lines[2:]]
            self.assertTrue(all(status in {"present", "missing"} for status in parsed_statuses))


if __name__ == "__main__":
    unittest.main()
