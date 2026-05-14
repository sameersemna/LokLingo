#!/usr/bin/env python3
"""Regression checks for reliability automation workflow structure."""

from __future__ import annotations

import unittest
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[3]
WORKFLOW = ROOT / ".github" / "workflows" / "reliability-automation.yml"


class ReliabilityAutomationWorkflowTests(unittest.TestCase):
    def test_review_jobs_include_preflight_smoke_step(self) -> None:
        payload = yaml.safe_load(WORKFLOW.read_text(encoding="utf-8"))
        jobs = payload["jobs"]

        expected_jobs = {
            "reliability-review-monthly",
            "reliability-review-quarterly",
            "reliability-review-annual",
        }

        for job_name in expected_jobs:
            self.assertIn(job_name, jobs)
            steps = jobs[job_name]["steps"]
            step_names = [step.get("name", "") for step in steps if isinstance(step, dict)]
            self.assertIn("Run weekly reliability preflight smoke", step_names)


if __name__ == "__main__":
    unittest.main()