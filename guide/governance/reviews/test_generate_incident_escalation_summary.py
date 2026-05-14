#!/usr/bin/env python3
"""Regression checks for incident escalation summary generation."""

from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "guide" / "governance" / "reviews" / "generate-incident-escalation-summary.py"


class GenerateIncidentEscalationSummaryTests(unittest.TestCase):
    def test_includes_critical_provider_and_commands(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            trend = tmp / "trend.md"
            index = tmp / "index.md"
            out = tmp / "escalation.md"

            trend.write_text(
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: 2",
                        "- Warning: 1",
                        "- Stable: 0",
                        "- Improving: 0",
                        "",
                        "## Escalation",
                        "",
                        "- Critical Providers: litellm, backup",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )
            index.write_text(
                "\n".join(
                    [
                        "# Reliability Artifact Index",
                        "",
                        "- Overall Smoke Status: failed",
                        "- Severity Hint: critical",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-z",
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
                ["python3", str(SCRIPT), str(trend), str(index), str(out)],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)

            summary = out.read_text(encoding="utf-8")
            self.assertIn("- Critical Regression Count: 2", summary)
            self.assertIn("## Critical Providers", summary)
            self.assertIn("- litellm", summary)
            self.assertIn("## Immediate Commands", summary)

    def test_falls_back_when_sections_are_missing(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            trend = tmp / "trend.md"
            index = tmp / "index.md"
            out = tmp / "escalation.md"

            trend.write_text("# Provider Policy Trend\n", encoding="utf-8")
            index.write_text("# Reliability Artifact Index\n", encoding="utf-8")

            result = subprocess.run(
                ["python3", str(SCRIPT), str(trend), str(index), str(out)],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)

            summary = out.read_text(encoding="utf-8")
            self.assertIn("- Critical Regression Count: 0", summary)
            self.assertIn("- No index triage lines were available.", summary)
            self.assertIn("## Critical Providers", summary)
            self.assertIn("- none", summary)
            self.assertIn("- No operator handoff commands were found.", summary)


if __name__ == "__main__":
    unittest.main()
