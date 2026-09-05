"""OCR accuracy evaluation harness (CER/WER against reference transcriptions).

Measures character error rate (CER) and word error rate (WER) of the OCR
pipeline — PaddleOCR, Surya, VLM fallback, and LLM correction included — against
ground-truth text files. Use it to track Arabic/Urdu/Hindi/Bengali quality over
time (cf. KITAB-Bench, arXiv:2502.14949) and to validate engine/config changes.

Dataset layout:
    <dataset-dir>/
        <sample-id>.png|jpg|...   # page image
        <sample-id>.txt           # UTF-8 reference transcription

Usage (inside the OCR container or a venv with service deps):
    python eval_ocr.py /path/to/dataset --lang ar
    python eval_ocr.py /path/to/dataset --lang ar --json-out report.json

CER/WER are computed with Levenshtein distance on normalized text
(NFC, whitespace-collapsed). WER tokenizes on whitespace; for Arabic-script
languages this approximates word boundaries well enough for trending.
"""

from __future__ import annotations

import argparse
import json
import unicodedata
from pathlib import Path

from PIL import Image

from app import _ocr_pil_image, _summarise

IMAGE_SUFFIXES = {".png", ".jpg", ".jpeg", ".tif", ".tiff", ".bmp", ".webp"}


def _normalize(text: str) -> str:
    return " ".join(unicodedata.normalize("NFC", text).split())


def _levenshtein(a: list[str], b: list[str]) -> int:
    if len(a) < len(b):
        a, b = b, a
    previous = list(range(len(b) + 1))
    for i, ca in enumerate(a, start=1):
        current = [i]
        for j, cb in enumerate(b, start=1):
            current.append(min(
                previous[j] + 1,          # deletion
                current[j - 1] + 1,       # insertion
                previous[j - 1] + (ca != cb),  # substitution
            ))
        previous = current
    return previous[-1]


def cer(reference: str, hypothesis: str) -> float:
    ref = list(_normalize(reference))
    hyp = list(_normalize(hypothesis))
    if not ref:
        return 0.0 if not hyp else 1.0
    return _levenshtein(ref, hyp) / len(ref)


def wer(reference: str, hypothesis: str) -> float:
    ref = _normalize(reference).split()
    hyp = _normalize(hypothesis).split()
    if not ref:
        return 0.0 if not hyp else 1.0
    return _levenshtein(ref, hyp) / len(ref)


def main() -> int:
    parser = argparse.ArgumentParser(description="Evaluate OCR accuracy (CER/WER) against reference transcriptions.")
    parser.add_argument("dataset", type=Path, help="Directory with <id>.<img> + <id>.txt pairs")
    parser.add_argument("--lang", default="auto", help="Language hint passed to the OCR pipeline (default: auto)")
    parser.add_argument("--json-out", type=Path, default=None, help="Optional path for a JSON report")
    args = parser.parse_args()

    if not args.dataset.is_dir():
        raise SystemExit(f"Dataset directory does not exist: {args.dataset}")

    samples = []
    for image_path in sorted(args.dataset.iterdir()):
        if image_path.suffix.lower() not in IMAGE_SUFFIXES:
            continue
        ref_path = image_path.with_suffix(".txt")
        if ref_path.exists():
            samples.append((image_path, ref_path))

    if not samples:
        raise SystemExit(f"No image/reference pairs found in {args.dataset}")

    results = []
    total_cer = 0.0
    total_wer = 0.0
    for image_path, ref_path in samples:
        reference = ref_path.read_text(encoding="utf-8")
        with Image.open(image_path) as image:
            blocks = _ocr_pil_image(image.convert("RGB"), args.lang)
        hypothesis, confidence = _summarise(blocks)
        sample_cer = cer(reference, hypothesis)
        sample_wer = wer(reference, hypothesis)
        total_cer += sample_cer
        total_wer += sample_wer
        results.append({
            "sample": image_path.name,
            "lang": args.lang,
            "cer": round(sample_cer, 4),
            "wer": round(sample_wer, 4),
            "confidence": confidence,
            "chars": len(_normalize(reference)),
        })
        print(f"{image_path.name}: CER={sample_cer:.4f} WER={sample_wer:.4f} conf={confidence:.4f}")

    n = len(results)
    summary = {
        "lang": args.lang,
        "samples": n,
        "mean_cer": round(total_cer / n, 4),
        "mean_wer": round(total_wer / n, 4),
        "results": results,
    }
    print(f"\nSamples: {n} | mean CER: {summary['mean_cer']:.4f} | mean WER: {summary['mean_wer']:.4f}")

    if args.json_out:
        args.json_out.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(f"JSON report written to {args.json_out}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
