#!/usr/bin/env python3
"""Regression checks for weekly operator one-pager generation."""

from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "guide" / "governance" / "reviews" / "generate-weekly-operator-onepager.py"


class GenerateWeeklyOperatorOnepagerTests(unittest.TestCase):
    def test_includes_triage_snapshot_and_handoff(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke_summary = tmp / "summary.md"
            provider_trend = tmp / "trend.md"
            artifact_index = tmp / "index.md"
            out = tmp / "onepager.md"

            smoke_summary.write_text(
                "\n".join(
                    [
                        "# Reliability Smoke Ops Summary",
                        "",
                        "- Generated At: 2026-05-13T00:00:00Z",
                        "- Overall Status: failed",
                        "",
                        "## Degraded Signal Threshold Reference",
                        "",
                        "| Signal | Current Value | Threshold Guidance |",
                        "| --- | ---: | --- |",
                        "| OCR Fallback Budget Exhausted Total | 2 | warn delta > 0 and < 3 over 10m, critical delta >= 3 over 10m |",
                        "",
                        "## Suite Results",
                        "",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )
            provider_trend.write_text(
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: 1",
                        "- Warning: 0",
                        "- Stable: 0",
                        "- Improving: 1",
                        "",
                        "## Escalation",
                        "",
                        "- bash guide/ops/reliability-recovery.sh queue-status",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )
            artifact_index.write_text(
                "\n".join(
                    [
                        "# Reliability Artifact Index",
                        "",
                        "- Overall Smoke Status: failed",
                        "- Severity Hint: critical",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Operator Handoff Commands",
                        "",
                        "- bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            result = subprocess.run(
                ["python3", str(SCRIPT), str(smoke_summary), str(provider_trend), str(artifact_index), str(out)],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)

            generated = out.read_text(encoding="utf-8")
            self.assertIn("## Operator Triage Snapshot", generated)
            self.assertIn("- Severity Hint: critical", generated)
            self.assertIn("- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status", generated)
            self.assertIn("- OCR Fallback Budget Exhausted Signal: total=2, status=warning", generated)
            self.assertIn("## Operator Handoff Commands", generated)

    def test_fails_on_malformed_fallback_signal_row(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke_summary = tmp / "summary.md"
            provider_trend = tmp / "trend.md"
            artifact_index = tmp / "index.md"
            out = tmp / "onepager.md"

            smoke_summary.write_text(
                "\n".join(
                    [
                        "# Reliability Smoke Ops Summary",
                        "",
                        "## Degraded Signal Threshold Reference",
                        "",
                        "| Signal | Current Value | Threshold Guidance |",
                        "| --- | ---: | --- |",
                        "| OCR Fallback Budget Exhausted Total |",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )
            provider_trend.write_text(
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: 0",
                        "- Warning: 0",
                        "- Stable: 1",
                        "- Improving: 0",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )
            artifact_index.write_text(
                "\n".join(
                    [
                        "# Reliability Artifact Index",
                        "",
                        "- Overall Smoke Status: ok",
                        "- Severity Hint: normal",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-y",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            result = subprocess.run(
                ["python3", str(SCRIPT), str(smoke_summary), str(provider_trend), str(artifact_index), str(out)],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("malformed fallback-budget signal row", result.stderr)


if __name__ == "__main__":
    unittest.main()
