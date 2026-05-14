#!/usr/bin/env python3
"""Regression checks for weekly review test runner behavior."""

from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "guide" / "governance" / "reviews" / "run-weekly-review-tests.sh"


class RunWeeklyReviewTestsScriptTests(unittest.TestCase):
    def test_preserves_all_logs_when_middle_command_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            matrix = tmp / "matrix.txt"
            log_dir = tmp / "logs"
            matrix.write_text(
                "\n".join(
                    [
                        "pass_first|python3 -c \"print('pass-first')\"",
                        "fail_middle|python3 -c \"import sys; print('fail-middle'); sys.exit(1)\"",
                        "pass_last|python3 -c \"print('pass-last')\"",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            env = os.environ.copy()
            env["WEEKLY_REVIEW_TEST_MATRIX_FILE"] = str(matrix)
            result = subprocess.run(
                ["bash", str(SCRIPT), str(log_dir)],
                capture_output=True,
                text=True,
                cwd=ROOT,
                env=env,
                check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("weekly review tooling regression failures: 1", result.stderr)

            first_log = (log_dir / "pass_first.log").read_text(encoding="utf-8")
            fail_log = (log_dir / "fail_middle.log").read_text(encoding="utf-8")
            last_log = (log_dir / "pass_last.log").read_text(encoding="utf-8")
            self.assertIn("pass-first", first_log)
            self.assertIn("fail-middle", fail_log)
            self.assertIn("pass-last", last_log)

    def test_rejects_missing_label_in_matrix_entry(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            matrix = tmp / "matrix.txt"
            log_dir = tmp / "logs"
            matrix.write_text("|python3 -c \"print('no-label')\"\n", encoding="utf-8")

            env = os.environ.copy()
            env["WEEKLY_REVIEW_TEST_MATRIX_FILE"] = str(matrix)
            result = subprocess.run(
                ["bash", str(SCRIPT), str(log_dir)],
                capture_output=True,
                text=True,
                cwd=ROOT,
                env=env,
                check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("missing label", result.stderr)

    def test_rejects_missing_command_in_matrix_entry(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            matrix = tmp / "matrix.txt"
            log_dir = tmp / "logs"
            matrix.write_text("bad_entry|\n", encoding="utf-8")

            env = os.environ.copy()
            env["WEEKLY_REVIEW_TEST_MATRIX_FILE"] = str(matrix)
            result = subprocess.run(
                ["bash", str(SCRIPT), str(log_dir)],
                capture_output=True,
                text=True,
                cwd=ROOT,
                env=env,
                check=False,
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("missing command", result.stderr)


if __name__ == "__main__":
    unittest.main()
