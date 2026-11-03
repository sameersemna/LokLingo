"""Unit tests for the optional Unlimited OCR adapter."""

import base64
import io
import json
import urllib.request
import unittest
import unittest.mock as mock
from PIL import Image

import unlimited_ocr


class TestResolveEngine(unittest.TestCase):
    def test_paddle(self):
        self.assertEqual(unlimited_ocr.resolve_engine("paddle", "de"), "paddle")

    def test_unlimited(self):
        self.assertEqual(unlimited_ocr.resolve_engine("unlimited", "de"), "unlimited")

    def test_auto_disabled_falls_back(self):
        with mock.patch.object(unlimited_ocr, "OCR_UNLIMITED_ENABLED", False):
            with mock.patch.object(unlimited_ocr, "OCR_UNLIMITED_LANGS", {"de"}):
                self.assertEqual(unlimited_ocr.resolve_engine("auto", "de"), "paddle")

    def test_auto_enabled_allowlisted(self):
        with mock.patch.object(unlimited_ocr, "OCR_UNLIMITED_ENABLED", True):
            with mock.patch.object(unlimited_ocr, "OCR_UNLIMITED_LANGS", {"de"}):
                self.assertEqual(unlimited_ocr.resolve_engine("auto", "de"), "unlimited")

    def test_invalid_raises(self):
        with self.assertRaises(ValueError):
            unlimited_ocr.resolve_engine("bad", "de")


class TestParseUnlimitedMarkdown(unittest.TestCase):
    def test_direct_det_form(self):
        raw = "<|det|>text [10,20,30,40]<|/det|>Hello"
        blocks, text, warnings, quality = unlimited_ocr.parse_unlimited_markdown(raw, 1000, 2000)
        self.assertEqual(text, "Hello")
        self.assertEqual(len(blocks), 1)
        self.assertEqual(blocks[0]["region_class"], "body")
        for actual, expected in zip(
            blocks[0]["bbox"],
            [10.0 * 1000 / 999, 20.0 * 2000 / 999, 30.0 * 1000 / 999, 40.0 * 2000 / 999],
        ):
            self.assertAlmostEqual(actual, expected, places=6)
        self.assertFalse(quality["markup_leak"])

    def test_plain_text_fallback(self):
        blocks, text, warnings, quality = unlimited_ocr.parse_unlimited_markdown("plain text", 100, 100)
        self.assertEqual(text, "plain text")
        self.assertEqual(len(blocks), 1)

    def test_multiple_boxes_create_multiple_blocks(self):
        raw = "<|det|>text [[0,0,999,100],[0,100,999,200]]<|/det|>Hello"
        blocks, text, warnings, quality = unlimited_ocr.parse_unlimited_markdown(raw, 1000, 2000)
        self.assertEqual(text, "Hello")
        self.assertEqual(len(blocks), 2)
        self.assertEqual(blocks[0]["bbox"][3], 100.0 / 999.0 * 2000.0)
        self.assertEqual(blocks[1]["bbox"][1], 100.0 / 999.0 * 2000.0)

    def test_markdown_blocks(self):
        raw = """<|ref|>title<|/ref|><|det|>title [0,0,999,100]<|/det|>
Title text

<|ref|>table<|/ref|><|det|>table [0,100,999,200]<|/det|>
|a|b|
"""
        blocks, text, warnings, quality = unlimited_ocr.parse_unlimited_markdown(raw, 1000, 2000)
        self.assertGreaterEqual(len(blocks), 2)
        self.assertTrue(any(block["region_class"] == "title" for block in blocks))


class TestRepeatedLines(unittest.TestCase):
    def test_detects_repetition(self):
        text = "Document page 200\nDocument page 200\nDocument page 200\nDocument page 200"
        self.assertGreater(unlimited_ocr.repeated_line_ratio(text), 0.4)

    def test_no_repetition(self):
        self.assertEqual(unlimited_ocr.repeated_line_ratio("one\ntwo\nthree"), 0.0)


class TestPageWindows(unittest.TestCase):
    def test_no_overlap(self):
        self.assertEqual(unlimited_ocr.plan_page_windows(12, 5, 0), [(0, 5), (5, 10), (10, 12)])

    def test_overlap(self):
        self.assertEqual(unlimited_ocr.plan_page_windows(12, 5, 1), [(0, 5), (4, 9), (8, 12)])

    def test_single_window(self):
        self.assertEqual(unlimited_ocr.plan_page_windows(3, 10, 1), [(0, 3)])

    def test_empty(self):
        self.assertEqual(unlimited_ocr.plan_page_windows(0, 10, 1), [])


class TestPostSidecar(unittest.TestCase):
    def test_parses_json_response(self):
        response = io.BytesIO(json.dumps({"text": "ok"}).encode("utf-8"))

        class FakeResponse:
            def read(self):
                return response.getvalue()

            def __enter__(self):
                return self

            def __exit__(self, *args):
                return None

        with mock.patch.object(urllib.request, "urlopen", return_value=FakeResponse()):
            result = unlimited_ocr._post_sidecar({"hello": "world"})
        self.assertEqual(result["text"], "ok")


if __name__ == "__main__":
    unittest.main()

