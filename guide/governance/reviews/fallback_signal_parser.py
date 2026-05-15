#!/usr/bin/env python3
"""Shared parser for OCR fallback-budget signal rows in smoke summaries."""

from __future__ import annotations

from dataclasses import dataclass


ROW_PREFIX = "| OCR Fallback Budget Exhausted Total |"


@dataclass(frozen=True)
class FallbackBudgetSignal:
    found: bool
    malformed: bool
    value: str | None = None
    guidance: str | None = None
    error: str | None = None


def parse_fallback_budget_signal(lines: list[str]) -> FallbackBudgetSignal:
    for line in lines:
        candidate = line.strip()
        if not candidate.startswith(ROW_PREFIX):
            continue
        cols = [col.strip() for col in candidate.strip("|").split("|")]
        if len(cols) < 3:
            return FallbackBudgetSignal(
                found=True,
                malformed=True,
                error="malformed fallback-budget signal row in smoke summary: expected 3 columns",
            )

        value = cols[1]
        guidance = cols[2]
        if value == "" or guidance == "":
            return FallbackBudgetSignal(
                found=True,
                malformed=True,
                error="malformed fallback-budget signal row in smoke summary: missing value or guidance",
            )

        return FallbackBudgetSignal(
            found=True,
            malformed=False,
            value=value,
            guidance=guidance,
        )

    return FallbackBudgetSignal(found=False, malformed=False)


def build_onepager_signal_line(signal: FallbackBudgetSignal) -> str | None:
    if not signal.found or signal.malformed or signal.value is None or signal.guidance is None:
        return None

    status = "unknown"
    try:
        numeric = float(signal.value)
        status = "warning" if numeric > 0 else "normal"
    except ValueError:
        status = "unknown"

    return (
        f"- OCR Fallback Budget Exhausted Signal: total={signal.value}, "
        f"status={status}, guidance={signal.guidance}"
    )
