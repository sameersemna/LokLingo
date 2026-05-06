#!/usr/bin/env python3
"""Parse LokLingo backend docker logs for chunk latency and retry metrics.

Extracts:
- chunk durations
- slow chunks (>30s)
- retry occurrences

Outputs:
- max chunk latency
- avg chunk latency
- total chunks
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any


SLOW_CHUNK_THRESHOLD_MS = 30_000


@dataclass
class ChunkEvent:
    time: str
    level: str
    msg: str
    job_id: str
    chunk_idx: int | None
    duration_ms: int


@dataclass
class RetryEvent:
    time: str
    level: str
    job_id: str
    attempt: int | None
    backoff_ms: int | None
    err: str | None


@dataclass
class ParseResult:
    chunk_events: list[ChunkEvent] = field(default_factory=list)
    slow_chunks: list[ChunkEvent] = field(default_factory=list)
    retry_events: list[RetryEvent] = field(default_factory=list)



def _parse_rfc3339_to_utc(ts: str) -> datetime:
    # Accept common RFC3339 formats from slog JSON output.
    return datetime.fromisoformat(ts.replace("Z", "+00:00")).astimezone(timezone.utc)



def _read_docker_logs(container: str, since: str) -> str:
    cmd = ["docker", "logs"]
    if since:
        cmd.extend(["--since", since])
    cmd.append(container)

    proc = subprocess.run(cmd, check=False, capture_output=True, text=True)
    combined = (proc.stdout or "") + "\n" + (proc.stderr or "")
    if proc.returncode != 0:
        raise RuntimeError(f"docker logs failed ({proc.returncode}): {' '.join(cmd)}\n{combined.strip()}")
    return combined



def _iter_json_log_records(raw: str) -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    for line in raw.splitlines():
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



def _matches_job_id(record: dict[str, Any], job_id: str) -> bool:
    if not job_id:
        return True
    return record.get("job_id") == job_id



def _record_time(record: dict[str, Any]) -> str:
    ts = record.get("time")
    if isinstance(ts, str):
        return ts
    return ""



def parse_records(records: list[dict[str, Any]], job_id: str) -> ParseResult:
    result = ParseResult()

    for rec in records:
        if not _matches_job_id(rec, job_id):
            continue

        msg = rec.get("msg")
        if not isinstance(msg, str):
            continue

        if msg in {"chunk_translated", "chunk_translation_failed", "slow_chunk_translation"}:
            dur = rec.get("duration_ms")
            if not isinstance(dur, int):
                continue
            event = ChunkEvent(
                time=_record_time(rec),
                level=str(rec.get("level", "")),
                msg=msg,
                job_id=str(rec.get("job_id", "")),
                chunk_idx=rec.get("chunk_idx") if isinstance(rec.get("chunk_idx"), int) else None,
                duration_ms=dur,
            )
            result.chunk_events.append(event)
            if dur > SLOW_CHUNK_THRESHOLD_MS or msg == "slow_chunk_translation":
                result.slow_chunks.append(event)
            continue

        # Retries are emitted as: "translate transient error, backing off".
        if msg == "translate transient error, backing off":
            retry = RetryEvent(
                time=_record_time(rec),
                level=str(rec.get("level", "")),
                job_id=str(rec.get("job_id", "")),
                attempt=rec.get("attempt") if isinstance(rec.get("attempt"), int) else None,
                backoff_ms=rec.get("backoff_ms") if isinstance(rec.get("backoff_ms"), int) else None,
                err=str(rec.get("err")) if rec.get("err") is not None else None,
            )
            result.retry_events.append(retry)

    return result



def _format_chunk_event(e: ChunkEvent) -> str:
    chunk = "?" if e.chunk_idx is None else str(e.chunk_idx)
    return (
        f"time={e.time} level={e.level} msg={e.msg} "
        f"job_id={e.job_id} chunk_idx={chunk} duration_ms={e.duration_ms}"
    )



def _format_retry_event(e: RetryEvent) -> str:
    attempt = "?" if e.attempt is None else str(e.attempt)
    backoff = "?" if e.backoff_ms is None else str(e.backoff_ms)
    err = "" if e.err is None else e.err
    return (
        f"time={e.time} level={e.level} msg=translate transient error, backing off "
        f"job_id={e.job_id} attempt={attempt} backoff_ms={backoff} err={err}"
    )



def _print_output(result: ParseResult) -> None:
    print("== Chunk Durations ==")
    if not result.chunk_events:
        print("(none)")
    else:
        for ev in result.chunk_events:
            print(_format_chunk_event(ev))

    print("\n== Slow Chunks (>30s) ==")
    if not result.slow_chunks:
        print("(none)")
    else:
        for ev in result.slow_chunks:
            print(_format_chunk_event(ev))

    print("\n== Retry Occurrences ==")
    if not result.retry_events:
        print("(none)")
    else:
        for ev in result.retry_events:
            print(_format_retry_event(ev))

    durations = [ev.duration_ms for ev in result.chunk_events]
    max_latency = max(durations) if durations else 0
    avg_latency = (sum(durations) / len(durations)) if durations else 0.0
    total_chunks = len(result.chunk_events)

    print("\n== Summary ==")
    print(f"max chunk latency (ms): {max_latency}")
    print(f"avg chunk latency (ms): {avg_latency:.2f}")
    print(f"total chunks: {total_chunks}")



def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Parse LokLingo backend docker logs for chunk/retry metrics")
    p.add_argument("--container", default="loklingo-backend", help="backend container name")
    p.add_argument(
        "--since",
        default="",
        help="optional docker --since value (e.g. 30m, 2h, RFC3339 timestamp)",
    )
    p.add_argument(
        "--job-id",
        default="",
        help="optional job id filter (only parse records for this job)",
    )
    p.add_argument(
        "--min-time",
        default="",
        help="optional UTC RFC3339 minimum timestamp filter applied after parsing (e.g. 2026-05-06T12:00:00Z)",
    )
    return p.parse_args()



def main() -> int:
    args = parse_args()

    try:
        raw = _read_docker_logs(args.container, args.since)
    except Exception as exc:  # noqa: BLE001
        print(str(exc), file=sys.stderr)
        return 1

    records = _iter_json_log_records(raw)

    if args.min_time:
        try:
            cutoff = _parse_rfc3339_to_utc(args.min_time)
        except Exception as exc:  # noqa: BLE001
            print(f"invalid --min-time value: {exc}", file=sys.stderr)
            return 2

        filtered: list[dict[str, Any]] = []
        for rec in records:
            ts = rec.get("time")
            if not isinstance(ts, str):
                continue
            try:
                if _parse_rfc3339_to_utc(ts) >= cutoff:
                    filtered.append(rec)
            except Exception:
                continue
        records = filtered

    result = parse_records(records, args.job_id)
    _print_output(result)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
