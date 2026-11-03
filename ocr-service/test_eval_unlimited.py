"""Tests for the evaluation harness."""

import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from PIL import Image

import eval_unlimited


class TestManifestLoading(unittest.TestCase):
    def test_directory_mode_pairs_images_and_text(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            Image.new("RGB", (10, 10), "white").save(root / "sample.png")
            (root / "sample.txt").write_text("hello", encoding="utf-8")
            samples = eval_unlimited._load_manifest(root)
        self.assertEqual(len(samples), 1)
        self.assertEqual(samples[0]["id"], "sample")
        self.assertEqual(samples[0]["reference"].endswith("sample.txt"), True)

    def test_manifest_mode(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "manifest.json"
            path.write_text(
                json.dumps(
                    [
                        {
                            "id": "x",
                            "language": "de",
                            "pages": ["a.png"],
                            "reference": "a.txt",
                        }
                    ]
                ),
                encoding="utf-8",
            )
            samples = eval_unlimited._load_manifest(path)
        self.assertEqual(samples[0]["id"], "x")
        self.assertEqual(samples[0]["language"], "de")


class TestEvaluation(unittest.TestCase):
    def test_evaluate_uses_engine_runner(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            Image.new("RGB", (10, 10), "white").save(root / "s.png")
            ref = root / "s.txt"
            ref.write_text("hello world", encoding="utf-8")
            samples = [
                {
                    "id": "s",
                    "language": "en",
                    "document_type": "text",
                    "pages": [str(root / "s.png")],
                    "reference": str(ref),
                    "features": [],
                }
            ]
            with mock.patch.object(
                eval_unlimited,
                "_run_paddle",
                return_value=("hello world", 0.9, {"latency_ms": 10, "blocks": 1}),
            ):
                summary = eval_unlimited._evaluate(samples, "paddle")
        self.assertEqual(summary["samples"], 1)
        self.assertEqual(summary["mean_cer"], 0.0)
        self.assertEqual(summary["mean_wer"], 0.0)

    def test_write_markdown(self):
        summary = {
            "engine": "paddle",
            "samples": 1,
            "pages": 1,
            "mean_cer": 0.0,
            "mean_wer": 0.0,
            "mean_latency_ms": 12.0,
            "results": [
                {
                    "id": "s",
                    "language": "en",
                    "document_type": "text",
                    "pages": 1,
                    "cer": 0.0,
                    "wer": 0.0,
                    "latency_ms": 12.0,
                }
            ],
        }
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "report.md"
            eval_unlimited._write_markdown(summary, out)
            text = out.read_text(encoding="utf-8")
        self.assertIn("Unlimited OCR Evaluation", text)
        self.assertIn("| s | en | text |", text)


if __name__ == "__main__":
    unittest.main()
