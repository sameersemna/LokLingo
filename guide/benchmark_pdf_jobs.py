#!/usr/bin/env python3
"""Run real-world PDF translation benchmarks against LokLingo.

This script does NOT mock anything:
- Uploads real PDF files via /api/v1/jobs/pdf
- Polls /api/v1/jobs/{id} until completion/failure
- Reads backend logs to extract:
  - job duration (job_finished.duration_ms)
  - chunk count (job_finished.chunk_count)
  - max chunk latency (max duration_ms from chunk_translated/chunk_translation_failed)
"""

from __future__ import annotations

import argparse
import json
import mimetypes
import os
import subprocess
import sys
import time
import uuid
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any
from urllib import error, parse, request


@dataclass
class BenchmarkResult:
    label: str
    file_path: str
    job_id: str
    status: str
    total_wall_time_s: float
    job_duration_ms: int | None
    chunk_count: int | None
    max_chunk_latency_ms: int | None
    processed_pages: int | None
    total_pages: int | None
    error: str | None


def _now_utc() -> datetime:
    return datetime.now(timezone.utc)


def _format_rfc3339(dt: datetime) -> str:
    return dt.isoformat().replace("+00:00", "Z")


def _http_json(req: request.Request, timeout_s: float) -> dict[str, Any]:
    try:
        with request.urlopen(req, timeout=timeout_s) as resp:
            raw = resp.read().decode("utf-8")
            if not raw:
                return {}
            return json.loads(raw)
    except error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {exc.code} {exc.reason}: {body}") from exc
    except error.URLError as exc:
        raise RuntimeError(f"request failed: {exc}") from exc


def _build_multipart_pdf_payload(
    file_path: str,
    target: str,
    source: str,
    lang: str,
) -> tuple[bytes, str]:
    boundary = f"----loklingo-bench-{uuid.uuid4().hex}"
    crlf = b"\r\n"

    filename = os.path.basename(file_path)
    content_type = mimetypes.guess_type(filename)[0] or "application/pdf"

    with open(file_path, "rb") as f:
        pdf_bytes = f.read()

    parts: list[bytes] = []

    def add_text_field(name: str, value: str) -> None:
        parts.append(f"--{boundary}".encode("utf-8"))
        parts.append(f'Content-Disposition: form-data; name="{name}"'.encode("utf-8"))
        parts.append(b"")
        parts.append(value.encode("utf-8"))

    add_text_field("target", target)
    add_text_field("source", source)
    add_text_field("lang", lang)

    parts.append(f"--{boundary}".encode("utf-8"))
    parts.append(
        (
            f'Content-Disposition: form-data; name="file"; filename="{filename}"'
        ).encode("utf-8")
    )
    parts.append(f"Content-Type: {content_type}".encode("utf-8"))
    parts.append(b"")
    parts.append(pdf_bytes)

    parts.append(f"--{boundary}--".encode("utf-8"))
    parts.append(b"")

    body = crlf.join(parts)
    return body, boundary


def upload_pdf_job(
    base_url: str,
    file_path: str,
    target: str,
    source: str,
    lang: str,
    timeout_s: float,
) -> str:
    url = parse.urljoin(base_url.rstrip("/") + "/", "api/v1/jobs/pdf")
    body, boundary = _build_multipart_pdf_payload(file_path, target, source, lang)

    req = request.Request(url=url, method="POST", data=body)
    req.add_header("Content-Type", f"multipart/form-data; boundary={boundary}")
    req.add_header("Accept", "application/json")

    payload = _http_json(req, timeout_s=timeout_s)
    job_id = payload.get("job_id")
    if not job_id:
        raise RuntimeError(f"upload did not return job_id: {payload}")
    return str(job_id)


def get_job_status(base_url: str, job_id: str, timeout_s: float) -> dict[str, Any]:
    url = parse.urljoin(base_url.rstrip("/") + "/", f"api/v1/jobs/{job_id}")
    req = request.Request(url=url, method="GET")
    req.add_header("Accept", "application/json")
    return _http_json(req, timeout_s=timeout_s)


def poll_job_until_terminal(
    base_url: str,
    job_id: str,
    poll_interval_s: float,
    timeout_s: float,
    request_timeout_s: float,
) -> dict[str, Any]:
    started = time.monotonic()
    while True:
        job = get_job_status(base_url, job_id, timeout_s=request_timeout_s)
        status = str(job.get("status", ""))
        if status in {"completed", "failed"}:
            return job

        if time.monotonic() - started > timeout_s:
            raise TimeoutError(f"poll timeout reached for job {job_id}")

        time.sleep(poll_interval_s)


def _parse_backend_log_lines(lines: str) -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    for line in lines.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(obj, dict):
            records.append(obj)
    return records


def _fetch_logs_from_docker(container: str, since: datetime) -> list[dict[str, Any]]:
    cmd = ["docker", "logs", "--since", _format_rfc3339(since), container]
    proc = subprocess.run(cmd, check=False, capture_output=True, text=True)
    combined = (proc.stdout or "") + "\n" + (proc.stderr or "")
    if proc.returncode != 0:
        raise RuntimeError(f"failed to read docker logs: {' '.join(cmd)}\n{combined.strip()}")
    return _parse_backend_log_lines(combined)


def _fetch_logs_from_file(log_file: str, since: datetime) -> list[dict[str, Any]]:
    if not os.path.isfile(log_file):
        raise RuntimeError(f"log file not found: {log_file}")

    since_ts = since.timestamp()
    records: list[dict[str, Any]] = []
    with open(log_file, "r", encoding="utf-8", errors="replace") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            if not isinstance(obj, dict):
                continue
            ts_raw = obj.get("time")
            if isinstance(ts_raw, str):
                try:
                    # slog uses RFC3339 timestamps.
                    ts = datetime.fromisoformat(ts_raw.replace("Z", "+00:00")).timestamp()
                except ValueError:
                    ts = None
                if ts is not None and ts < since_ts:
                    continue
            records.append(obj)
    return records


def extract_metrics_from_logs(
    records: list[dict[str, Any]],
    job_id: str,
) -> tuple[int | None, int | None, int | None]:
    job_duration_ms: int | None = None
    chunk_count: int | None = None
    max_chunk_latency_ms: int | None = None

    for rec in records:
        rec_job_id = rec.get("job_id")
        if rec_job_id != job_id:
            continue

        msg = rec.get("msg")
        dur = rec.get("duration_ms")
        if msg in {"chunk_translated", "chunk_translation_failed"} and isinstance(dur, int):
            if max_chunk_latency_ms is None or dur > max_chunk_latency_ms:
                max_chunk_latency_ms = dur

        if msg == "job_finished":
            if isinstance(rec.get("duration_ms"), int):
                job_duration_ms = int(rec["duration_ms"])
            if isinstance(rec.get("chunk_count"), int):
                chunk_count = int(rec["chunk_count"])

    return job_duration_ms, chunk_count, max_chunk_latency_ms


def run_single_benchmark(
    label: str,
    file_path: str,
    base_url: str,
    target: str,
    source: str,
    lang: str,
    poll_interval_s: float,
    job_timeout_s: float,
    request_timeout_s: float,
    log_mode: str,
    backend_container: str,
    backend_log_file: str,
) -> BenchmarkResult:
    started_at = _now_utc()
    wall_start = time.monotonic()

    job_id = upload_pdf_job(
        base_url=base_url,
        file_path=file_path,
        target=target,
        source=source,
        lang=lang,
        timeout_s=request_timeout_s,
    )

    job = poll_job_until_terminal(
        base_url=base_url,
        job_id=job_id,
        poll_interval_s=poll_interval_s,
        timeout_s=job_timeout_s,
        request_timeout_s=request_timeout_s,
    )

    wall_s = time.monotonic() - wall_start
    status = str(job.get("status", "unknown"))
    processed_pages = job.get("processed_pages")
    total_pages = job.get("total_pages")
    error_msg = job.get("error")

    # Small negative offset to avoid missing logs around upload boundary.
    since = started_at - timedelta(seconds=2)
    records: list[dict[str, Any]] = []

    if log_mode == "docker":
        records = _fetch_logs_from_docker(backend_container, since)
    elif log_mode == "file":
        records = _fetch_logs_from_file(backend_log_file, since)

    job_duration_ms, chunk_count, max_chunk_latency_ms = extract_metrics_from_logs(records, job_id)

    return BenchmarkResult(
        label=label,
        file_path=file_path,
        job_id=job_id,
        status=status,
        total_wall_time_s=wall_s,
        job_duration_ms=job_duration_ms,
        chunk_count=chunk_count,
        max_chunk_latency_ms=max_chunk_latency_ms,
        processed_pages=int(processed_pages) if isinstance(processed_pages, int) else None,
        total_pages=int(total_pages) if isinstance(total_pages, int) else None,
        error=str(error_msg) if error_msg is not None else None,
    )


def _fmt_ms(value: int | None) -> str:
    if value is None:
        return "n/a"
    return str(value)


def print_report(results: list[BenchmarkResult]) -> None:
    header = [
        "size",
        "status",
        "job_id",
        "wall_s",
        "job_duration_ms",
        "chunk_count",
        "max_chunk_latency_ms",
        "pages",
    ]
    print("\n" + " | ".join(header))
    print("-" * 140)
    for r in results:
        pages = "n/a"
        if r.processed_pages is not None or r.total_pages is not None:
            pages = f"{r.processed_pages}/{r.total_pages}"
        print(
            " | ".join(
                [
                    r.label,
                    r.status,
                    r.job_id,
                    f"{r.total_wall_time_s:.2f}",
                    _fmt_ms(r.job_duration_ms),
                    "n/a" if r.chunk_count is None else str(r.chunk_count),
                    _fmt_ms(r.max_chunk_latency_ms),
                    pages,
                ]
            )
        )
        if r.error:
            print(f"  error: {r.error}")


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Benchmark LokLingo PDF async translation jobs with real HTTP calls")
    p.add_argument("--base-url", default="http://localhost:28080", help="LokLingo backend base URL")
    p.add_argument("--source", default="auto", help="source language passed to /jobs/pdf")
    p.add_argument("--target", required=True, help="target language passed to /jobs/pdf (e.g. de)")
    p.add_argument("--lang", default="auto", help="OCR language hint for PDF jobs")

    p.add_argument("--small", required=True, help="path to small PDF (~2 pages)")
    p.add_argument("--medium", required=True, help="path to medium PDF (~15 pages)")
    p.add_argument("--large", required=True, help="path to large PDF (~50 pages)")

    p.add_argument("--poll-interval", type=float, default=2.0, help="seconds between job status polls")
    p.add_argument("--job-timeout", type=float, default=3600.0, help="max seconds to wait for one job")
    p.add_argument("--request-timeout", type=float, default=60.0, help="timeout seconds for one HTTP request")

    p.add_argument(
        "--log-mode",
        choices=["docker", "file", "none"],
        default="docker",
        help="where to read backend JSON logs for chunk/job metrics",
    )
    p.add_argument("--backend-container", default="loklingo-backend", help="backend container name for --log-mode docker")
    p.add_argument("--backend-log-file", default="", help="backend JSON log path for --log-mode file")
    return p.parse_args()


def main() -> int:
    args = parse_args()

    test_files = [
        ("small", args.small),
        ("medium", args.medium),
        ("large", args.large),
    ]
    for label, path in test_files:
        if not os.path.isfile(path):
            print(f"missing {label} PDF: {path}", file=sys.stderr)
            return 2

    if args.log_mode == "file" and not args.backend_log_file:
        print("--backend-log-file is required when --log-mode file", file=sys.stderr)
        return 2

    results: list[BenchmarkResult] = []
    for label, pdf_path in test_files:
        print(f"\n[{label}] uploading and benchmarking: {pdf_path}")
        try:
            result = run_single_benchmark(
                label=label,
                file_path=pdf_path,
                base_url=args.base_url,
                target=args.target,
                source=args.source,
                lang=args.lang,
                poll_interval_s=args.poll_interval,
                job_timeout_s=args.job_timeout,
                request_timeout_s=args.request_timeout,
                log_mode=args.log_mode,
                backend_container=args.backend_container,
                backend_log_file=args.backend_log_file,
            )
        except Exception as exc:  # noqa: BLE001
            print(f"[{label}] failed: {exc}", file=sys.stderr)
            return 1

        print(f"[{label}] completed with status={result.status}, job_id={result.job_id}")
        results.append(result)

    print_report(results)

    # Exit non-zero if any benchmarked job failed.
    if any(r.status != "completed" for r in results):
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
