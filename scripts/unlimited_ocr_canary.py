#!/usr/bin/env python3
"""Run a controlled Unlimited OCR canary against the LokLingo job API.

This does not route production traffic. It submits a small, explicit batch of PDFs with
``ocr_engine=unlimited`` and records completion, latency, page counts, confidence, and
warnings. Use it from a staging/canary host before changing defaults.

Example:
    WRITE_API_TOKEN=... python3 scripts/unlimited_ocr_canary.py \
      --backend-url http://127.0.0.1:28080 \
      --files canary/*.pdf \
      --json-out docs/reports/unlimited-ocr/canary-run.json
"""

from __future__ import annotations

import argparse
import json
import mimetypes
import os
import sys
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path
from typing import Any


def _multipart(fields: dict[str, str], file_path: Path) -> tuple[bytes, str]:
    boundary = f"----loklingo-canary-{uuid.uuid4().hex}"
    parts: list[bytes] = []
    for name, value in fields.items():
        parts.append(
            (
                f"--{boundary}\r\n"
                f'Content-Disposition: form-data; name="{name}"\r\n\r\n'
                f"{value}\r\n"
            ).encode("utf-8")
        )
    content_type = mimetypes.guess_type(file_path.name)[0] or "application/pdf"
    data = file_path.read_bytes()
    parts.append(
        (
            f"--{boundary}\r\n"
            f'Content-Disposition: form-data; name="file"; filename="{file_path.name}"\r\n'
            f"Content-Type: {content_type}\r\n\r\n"
        ).encode("utf-8")
        + data
        + b"\r\n"
    )
    parts.append(f"--{boundary}--\r\n".encode("utf-8"))
    return b"".join(parts), f"multipart/form-data; boundary={boundary}"


def _request_json(url: str, method: str, body: bytes | None, headers: dict[str, str]) -> dict[str, Any]:
    request = urllib.request.Request(url, data=body, method=method, headers=headers)
    with urllib.request.urlopen(request, timeout=60) as response:
        return json.loads(response.read().decode("utf-8"))


def _headers(token: str) -> dict[str, str]:
    headers = {"Accept": "application/json"}
    if token.strip():
        headers["X-API-Token"] = token.strip()
    return headers


def submit(backend: str, token: str, path: Path, engine: str, source: str, target: str) -> str:
    body, content_type = _multipart(
        {
            "source": source,
            "target": target,
            "mode": "ocr_only",
            "ocr_engine": engine,
            "force_ocr": "true",
        },
        path,
    )
    headers = _headers(token)
    headers["Content-Type"] = content_type
    payload = _request_json(f"{backend}/api/v1/jobs/pdf", "POST", body, headers)
    job_id = payload.get("job_id")
    if not job_id:
        raise RuntimeError(f"no job_id in response: {payload}")
    return str(job_id)


def poll(backend: str, token: str, job_id: str, timeout: int, interval: float) -> dict[str, Any]:
    deadline = time.time() + timeout
    headers = _headers(token)
    while time.time() < deadline:
        payload = _request_json(f"{backend}/api/v1/jobs/{job_id}", "GET", None, headers)
        if payload.get("status") in {"completed", "failed"}:
            return payload
        time.sleep(interval)
    raise TimeoutError(f"job {job_id} did not finish within {timeout}s")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--backend-url", default=os.getenv("LOKLINGO_BACKEND_URL", "http://127.0.0.1:28080"))
    parser.add_argument("--token", default=os.getenv("WRITE_API_TOKEN", ""))
    parser.add_argument("--files", nargs="+", required=True, type=Path)
    parser.add_argument("--engine", default="unlimited")
    parser.add_argument("--source", default="en")
    parser.add_argument("--target", default="de")
    parser.add_argument("--timeout", type=int, default=1200)
    parser.add_argument("--interval", type=float, default=2.0)
    parser.add_argument("--json-out", type=Path, default=None)
    args = parser.parse_args()

    if not args.files:
        parser.error("at least one file is required")

    results: list[dict[str, Any]] = []
    failed = 0
    for path in args.files:
        if not path.is_file():
            print(f"missing: {path}", file=sys.stderr)
            failed += 1
            continue
        started = time.perf_counter()
        print(f"submit {path.name} ...", flush=True)
        try:
            job_id = submit(args.backend_url, args.token, path, args.engine, args.source, args.target)
            job = poll(args.backend_url, args.token, job_id, args.timeout, args.interval)
        except Exception as exc:
            failed += 1
            print(f"  {path.name}: ERROR {exc}", flush=True)
            results.append({"file": path.name, "status": "error", "error": str(exc)})
            continue
        elapsed = round(time.perf_counter() - started, 2)
        record = {
            "file": path.name,
            "job_id": job_id,
            "status": job.get("status"),
            "ocr_engine": job.get("ocr_engine"),
            "processing_method": job.get("processing_method"),
            "pages": job.get("total_pages"),
            "processed_pages": job.get("processed_pages"),
            "ocr_confidence": job.get("ocr_confidence"),
            "warnings": job.get("warnings") or [],
            "latency_seconds": elapsed,
            "error": job.get("error"),
        }
        results.append(record)
        if job.get("status") != "completed":
            failed += 1
        print(f"  {path.name}: {record['status']} pages={record['pages']} {elapsed}s", flush=True)

    summary = {
        "engine": args.engine,
        "files": len(args.files),
        "completed": sum(1 for item in results if item.get("status") == "completed"),
        "failed": failed,
        "results": results,
    }
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    if args.json_out:
        args.json_out.parent.mkdir(parents=True, exist_ok=True)
        args.json_out.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
