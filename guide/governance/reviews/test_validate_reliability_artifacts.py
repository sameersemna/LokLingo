#!/usr/bin/env python3
"""Regression checks for weekly reliability artifact validation."""

from __future__ import annotations

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from datetime import datetime, timedelta, timezone


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "guide" / "governance" / "reviews" / "validate-reliability-artifacts.py"


def _write(path: Path, text: str) -> None:
    path.write_text(text, encoding="utf-8")


class ValidateReliabilityArtifactsTests(unittest.TestCase):
    def test_accepts_complete_artifact_set(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"

            _write(smoke, json.dumps({"ok": True}))
            _write(snapshot, json.dumps({"ok": True}))
            _write(summary, "# Summary\n")
            _write(index, "# Index\n")
            _write(
                trend,
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: 1",
                        "- Warning: 0",
                        "- Stable: 0",
                        "- Improving: 0",
                        "",
                        "## Escalation",
                    ]
                )
                + "\n",
            )
            _write(
                onepager,
                "\n".join(
                    [
                        "# Weekly Reliability Operator One-Pager",
                        "",
                        "## Operator Triage Snapshot",
                        "",
                        "- Severity Hint: critical",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Operator Handoff Commands",
                    ]
                )
                + "\n",
            )
            _write(
                escalation,
                "\n".join(
                    [
                        "# Weekly Incident Escalation Summary",
                        "",
                        f"- Generated At: {datetime.now(timezone.utc).replace(microsecond=0).isoformat()}",
                        "- Critical Regression Count: 1",
                        "",
                        "## Triage Snapshot",
                        "",
                        "- Severity Hint: critical",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Critical Providers",
                        "",
                        "- litellm",
                        "",
                        "## Immediate Commands",
                        "",
                        "- bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>",
                    ]
                )
                + "\n",
            )

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
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_critical_trend_without_escalation(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"

            _write(smoke, json.dumps({"ok": True}))
            _write(snapshot, json.dumps({"ok": True}))
            _write(summary, "# Summary\n")
            _write(index, "# Index\n")
            _write(
                trend,
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: 2",
                        "- Warning: 0",
                        "- Stable: 0",
                        "- Improving: 0",
                    ]
                )
                + "\n",
            )
            _write(
                onepager,
                "\n".join(
                    [
                        "# Weekly Reliability Operator One-Pager",
                        "",
                        "## Operator Triage Snapshot",
                        "",
                        "- Severity Hint: critical",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Operator Handoff Commands",
                    ]
                )
                + "\n",
            )
            _write(
                escalation,
                "\n".join(
                    [
                        "# Weekly Incident Escalation Summary",
                        "",
                        f"- Generated At: {datetime.now(timezone.utc).replace(microsecond=0).isoformat()}",
                        "- Critical Regression Count: 2",
                        "",
                        "## Triage Snapshot",
                        "",
                        "- Severity Hint: critical",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Critical Providers",
                        "",
                        "- litellm",
                        "",
                        "## Immediate Commands",
                        "",
                        "- bash guide/ops/reliability-recovery.sh export-incident-snapshot <correlation-id>",
                    ]
                )
                + "\n",
            )

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
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("provider policy trend artifact", result.stderr)

    def test_rejects_critical_escalation_without_providers_or_commands(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"

            _write(smoke, json.dumps({"ok": True}))
            _write(snapshot, json.dumps({"ok": True}))
            _write(summary, "# Summary\n")
            _write(index, "# Index\n")
            _write(
                trend,
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: 1",
                        "- Warning: 0",
                        "- Stable: 0",
                        "- Improving: 0",
                        "",
                        "## Escalation",
                    ]
                )
                + "\n",
            )
            _write(
                escalation,
                "\n".join(
                    [
                        "# Weekly Incident Escalation Summary",
                        "",
                        f"- Generated At: {datetime.now(timezone.utc).replace(microsecond=0).isoformat()}",
                        "- Critical Regression Count: 1",
                        "",
                        "## Triage Snapshot",
                        "",
                        "- Severity Hint: critical",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Critical Providers",
                        "",
                        "- none",
                        "",
                        "## Immediate Commands",
                        "",
                        "- No operator handoff commands were found.",
                    ]
                )
                + "\n",
            )
            _write(
                onepager,
                "\n".join(
                    [
                        "# Weekly Reliability Operator One-Pager",
                        "",
                        "## Operator Triage Snapshot",
                        "",
                        "- Severity Hint: critical",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Operator Handoff Commands",
                    ]
                )
                + "\n",
            )

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
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("incident escalation summary", result.stderr)

    def test_rejects_malformed_critical_count_line(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"

            _write(smoke, json.dumps({"ok": True}))
            _write(snapshot, json.dumps({"ok": True}))
            _write(summary, "# Summary\n")
            _write(index, "# Index\n")
            _write(
                trend,
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: two",
                        "- Warning: 0",
                        "- Stable: 0",
                        "- Improving: 0",
                    ]
                )
                + "\n",
            )
            _write(
                escalation,
                "\n".join(
                    [
                        "# Weekly Incident Escalation Summary",
                        "",
                        f"- Generated At: {datetime.now(timezone.utc).replace(microsecond=0).isoformat()}",
                        "- Critical Regression Count: 0",
                        "",
                        "## Triage Snapshot",
                        "",
                        "- Severity Hint: warning",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Critical Providers",
                        "",
                        "- none",
                        "",
                        "## Immediate Commands",
                        "",
                        "- bash guide/ops/reliability-recovery.sh queue-status",
                    ]
                )
                + "\n",
            )
            _write(
                onepager,
                "\n".join(
                    [
                        "# Weekly Reliability Operator One-Pager",
                        "",
                        "## Operator Triage Snapshot",
                        "",
                        "- Severity Hint: warning",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Operator Handoff Commands",
                    ]
                )
                + "\n",
            )

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
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("provider policy trend artifact", result.stderr)

    def test_rejects_stale_escalation_summary_when_freshness_configured(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"

            _write(smoke, json.dumps({"ok": True}))
            _write(snapshot, json.dumps({"ok": True}))
            _write(summary, "# Summary\n")
            _write(index, "# Index\n")
            _write(
                trend,
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: 0",
                        "- Warning: 1",
                        "- Stable: 0",
                        "- Improving: 0",
                    ]
                )
                + "\n",
            )
            stale = datetime.now(timezone.utc).replace(microsecond=0) - timedelta(hours=30)
            _write(
                escalation,
                "\n".join(
                    [
                        "# Weekly Incident Escalation Summary",
                        "",
                        f"- Generated At: {stale.isoformat()}",
                        "- Critical Regression Count: 0",
                        "",
                        "## Triage Snapshot",
                        "",
                        "- Severity Hint: warning",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Critical Providers",
                        "",
                        "- none",
                        "",
                        "## Immediate Commands",
                        "",
                        "- bash guide/ops/reliability-recovery.sh queue-status",
                    ]
                )
                + "\n",
            )
            _write(
                onepager,
                "\n".join(
                    [
                        "# Weekly Reliability Operator One-Pager",
                        "",
                        "## Operator Triage Snapshot",
                        "",
                        "- Severity Hint: warning",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-x",
                        "",
                        "## Operator Handoff Commands",
                    ]
                )
                + "\n",
            )

            env = os.environ.copy()
            env.update({"RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS": "12"})

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
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                env=env,
                check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("too old", result.stderr)

    def test_rejects_escalation_summary_missing_generated_at(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"

            _write(smoke, json.dumps({"ok": True}))
            _write(snapshot, json.dumps({"ok": True}))
            _write(summary, "# Summary\n")
            _write(index, "# Index\n")
            _write(
                trend,
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
            )
            _write(
                escalation,
                "\n".join(
                    [
                        "# Weekly Incident Escalation Summary",
                        "",
                        "- Critical Regression Count: 0",
                        "",
                        "## Triage Snapshot",
                        "",
                        "- Severity Hint: stable",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh provider-history",
                        "- Correlation ID: corr-x",
                        "",
                        "## Critical Providers",
                        "",
                        "- none",
                        "",
                        "## Immediate Commands",
                        "",
                        "- bash guide/ops/reliability-recovery.sh provider-history",
                    ]
                )
                + "\n",
            )
            _write(
                onepager,
                "\n".join(
                    [
                        "# Weekly Reliability Operator One-Pager",
                        "",
                        "## Operator Triage Snapshot",
                        "",
                        "- Severity Hint: stable",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh provider-history",
                        "- Correlation ID: corr-x",
                        "",
                        "## Operator Handoff Commands",
                    ]
                )
                + "\n",
            )

            env = os.environ.copy()
            env.update({"RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS": "12"})
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
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                env=env,
                check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("generated timestamp", result.stderr)

    def test_rejects_escalation_summary_invalid_generated_at(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"

            _write(smoke, json.dumps({"ok": True}))
            _write(snapshot, json.dumps({"ok": True}))
            _write(summary, "# Summary\n")
            _write(index, "# Index\n")
            _write(
                trend,
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
            )
            _write(
                escalation,
                "\n".join(
                    [
                        "# Weekly Incident Escalation Summary",
                        "",
                        "- Generated At: not-a-timestamp",
                        "- Critical Regression Count: 0",
                        "",
                        "## Triage Snapshot",
                        "",
                        "- Severity Hint: stable",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh provider-history",
                        "- Correlation ID: corr-x",
                        "",
                        "## Critical Providers",
                        "",
                        "- none",
                        "",
                        "## Immediate Commands",
                        "",
                        "- bash guide/ops/reliability-recovery.sh provider-history",
                    ]
                )
                + "\n",
            )
            _write(
                onepager,
                "\n".join(
                    [
                        "# Weekly Reliability Operator One-Pager",
                        "",
                        "## Operator Triage Snapshot",
                        "",
                        "- Severity Hint: stable",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh provider-history",
                        "- Correlation ID: corr-x",
                        "",
                        "## Operator Handoff Commands",
                    ]
                )
                + "\n",
            )

            env = os.environ.copy()
            env.update({"RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS": "12"})
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
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                env=env,
                check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("generated timestamp", result.stderr)

    def test_accepts_fresh_generated_at_with_z_suffix(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"

            _write(smoke, json.dumps({"ok": True}))
            _write(snapshot, json.dumps({"ok": True}))
            _write(summary, "# Summary\n")
            _write(index, "# Index\n")
            _write(
                trend,
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: 0",
                        "- Warning: 1",
                        "- Stable: 0",
                        "- Improving: 0",
                    ]
                )
                + "\n",
            )
            fresh_z = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
            _write(
                escalation,
                "\n".join(
                    [
                        "# Weekly Incident Escalation Summary",
                        "",
                        f"- Generated At: {fresh_z}",
                        "- Critical Regression Count: 0",
                        "",
                        "## Triage Snapshot",
                        "",
                        "- Severity Hint: warning",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-z",
                        "",
                        "## Critical Providers",
                        "",
                        "- none",
                        "",
                        "## Immediate Commands",
                        "",
                        "- bash guide/ops/reliability-recovery.sh queue-status",
                    ]
                )
                + "\n",
            )
            _write(
                onepager,
                "\n".join(
                    [
                        "# Weekly Reliability Operator One-Pager",
                        "",
                        "## Operator Triage Snapshot",
                        "",
                        "- Severity Hint: warning",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-z",
                        "",
                        "## Operator Handoff Commands",
                    ]
                )
                + "\n",
            )

            env = os.environ.copy()
            env.update({"RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS": "12"})
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
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                env=env,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_accepts_fresh_generated_at_with_explicit_offset(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            smoke = tmp / "smoke.json"
            snapshot = tmp / "snapshot.json"
            summary = tmp / "summary.md"
            index = tmp / "index.md"
            trend = tmp / "trend.md"
            escalation = tmp / "escalation.md"
            onepager = tmp / "onepager.md"

            _write(smoke, json.dumps({"ok": True}))
            _write(snapshot, json.dumps({"ok": True}))
            _write(summary, "# Summary\n")
            _write(index, "# Index\n")
            _write(
                trend,
                "\n".join(
                    [
                        "# Provider Policy Trend",
                        "",
                        "## Severity Totals",
                        "",
                        "- Critical: 0",
                        "- Warning: 1",
                        "- Stable: 0",
                        "- Improving: 0",
                    ]
                )
                + "\n",
            )
            fresh_offset = datetime.now(timezone.utc).replace(microsecond=0).isoformat()
            _write(
                escalation,
                "\n".join(
                    [
                        "# Weekly Incident Escalation Summary",
                        "",
                        f"- Generated At: {fresh_offset}",
                        "- Critical Regression Count: 0",
                        "",
                        "## Triage Snapshot",
                        "",
                        "- Severity Hint: warning",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-offset",
                        "",
                        "## Critical Providers",
                        "",
                        "- none",
                        "",
                        "## Immediate Commands",
                        "",
                        "- bash guide/ops/reliability-recovery.sh queue-status",
                    ]
                )
                + "\n",
            )
            _write(
                onepager,
                "\n".join(
                    [
                        "# Weekly Reliability Operator One-Pager",
                        "",
                        "## Operator Triage Snapshot",
                        "",
                        "- Severity Hint: warning",
                        "- Recommended First Command: bash guide/ops/reliability-recovery.sh queue-status",
                        "- Correlation ID: corr-offset",
                        "",
                        "## Operator Handoff Commands",
                    ]
                )
                + "\n",
            )

            env = os.environ.copy()
            env.update({"RELIABILITY_ESCALATION_SUMMARY_MAX_AGE_HOURS": "12"})
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
                ],
                capture_output=True,
                text=True,
                cwd=ROOT,
                env=env,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()
