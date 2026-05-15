#!/usr/bin/env python3
"""Regression checks for shared fallback-budget signal parser."""

from __future__ import annotations

import unittest

from fallback_signal_parser import build_onepager_signal_line, parse_fallback_budget_signal


class FallbackSignalParserTests(unittest.TestCase):
    def test_parses_valid_row_and_builds_warning_line(self) -> None:
        lines = [
            "| Signal | Current Value | Threshold Guidance |",
            "| --- | ---: | --- |",
            "| OCR Fallback Budget Exhausted Total | 2 | warn delta > 0 and < 3 over 10m, critical delta >= 3 over 10m |",
        ]

        parsed = parse_fallback_budget_signal(lines)
        self.assertTrue(parsed.found)
        self.assertFalse(parsed.malformed)
        self.assertEqual(parsed.value, "2")

        signal_line = build_onepager_signal_line(parsed)
        self.assertIsNotNone(signal_line)
        self.assertIn("status=warning", signal_line)

    def test_parses_na_value_as_found_not_malformed(self) -> None:
        lines = [
            "| OCR Fallback Budget Exhausted Total | n/a | warn delta > 0 and < 3 over 10m, critical delta >= 3 over 10m |",
        ]

        parsed = parse_fallback_budget_signal(lines)
        self.assertTrue(parsed.found)
        self.assertFalse(parsed.malformed)
        self.assertEqual(parsed.value, "n/a")

        signal_line = build_onepager_signal_line(parsed)
        self.assertIsNotNone(signal_line)
        self.assertIn("status=unknown", signal_line)

    def test_detects_malformed_row(self) -> None:
        lines = [
            "| OCR Fallback Budget Exhausted Total |",
        ]

        parsed = parse_fallback_budget_signal(lines)
        self.assertTrue(parsed.found)
        self.assertTrue(parsed.malformed)
        self.assertIsNotNone(parsed.error)
        self.assertIsNone(build_onepager_signal_line(parsed))


if __name__ == "__main__":
    unittest.main()
