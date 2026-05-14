#!/usr/bin/env python3
"""Deterministic smoke regression harness for lifecycle lineage verification."""

from __future__ import annotations

import http.server
import json
import os
import socketserver
import subprocess
import tempfile
import threading
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SMOKE_SUITE = ROOT / "guide" / "reliability-smoke-suite.sh"


class _MockHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, fmt: str, *args: object) -> None:  # pragma: no cover
        return

    def do_POST(self) -> None:  # noqa: N802
        if self.path == "/api/v1/jobs":
            payload = {
                "job_id": "mock-job-id",
                "correlation_id": "mock-correlation-id",
                "status": "pending",
            }
            body = json.dumps(payload).encode("utf-8")
            self.send_response(202)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        self.send_error(404)

    def do_GET(self) -> None:  # noqa: N802
        if self.path.startswith("/api/v1/metrics/prometheus"):
            metrics = "\n".join(
                [
                    "loklingo_pipeline_timeouts_total 0",
                    "loklingo_pipeline_retries_total 0",
                    "loklingo_queue_depth 0",
                ]
            ).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(metrics)))
            self.end_headers()
            self.wfile.write(metrics)
            return

        if self.path.startswith("/api/v1/metrics/lifecycle/events"):
            payload = {
                "timestamp": "2026-05-13T00:00:00Z",
                "filters": {},
                "count": 0,
                "events": [],
            }
            body = json.dumps(payload).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        self.send_error(404)


class ReusableTCPServer(socketserver.TCPServer):
    allow_reuse_address = True


class SmokeLineageRegressionTest(unittest.TestCase):
    def test_fails_when_lineage_never_appears(self) -> None:
        with ReusableTCPServer(("127.0.0.1", 0), _MockHandler) as server:
            host, port = server.server_address
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()

            with tempfile.TemporaryDirectory() as tmpdir:
                report_path = Path(tmpdir) / "smoke.json"
                env = os.environ.copy()
                env.update(
                    {
                        "LOKLINGO_BASE_URL": f"http://{host}:{port}",
                        "INTERNAL_TOKEN": "test-token",
                        "SMOKE_SUITE_MODE": "timeout_storm_only",
                        "STORM_JOB_COUNT": "1",
                        "STORM_RECOVERY_MAX_SEC": "1",
                        "TIMEOUT_STORM_MAX_SEC": "10",
                        "LIFECYCLE_VERIFY_MAX_SEC": "2",
                    }
                )

                result = subprocess.run(
                    ["bash", str(SMOKE_SUITE), "--json-out", str(report_path)],
                    cwd=ROOT,
                    env=env,
                    capture_output=True,
                    text=True,
                    check=False,
                )

                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                payload = json.loads(report_path.read_text(encoding="utf-8"))
                self.assertEqual(payload.get("correlation_id"), "mock-correlation-id")
                self.assertEqual(payload.get("lineage_verified"), False)
                self.assertEqual(payload.get("lifecycle_event_count"), 0)

            server.shutdown()
            thread.join(timeout=2)


if __name__ == "__main__":
    unittest.main()
