"""Manifest-driven evaluation harness for PaddleOCR and Unlimited OCR.

The harness is intentionally dependency-light: it reuses the CER/WER helpers from
``eval_ocr.py`` and the Unlimited OCR parser from ``unlimited_ocr.py``.

Dataset layouts
---------------
Directory mode:
    <dataset-dir>/
        sample-001.png
        sample-001.txt

Manifest mode:
    [
      {
        "id": "de-invoice-001",
        "language": "de",
        "document_type": "invoice",
        "pages": ["de-invoice-001-p1.png"],
        "reference": "de-invoice-001.txt",
        "features": ["table"]
      }
    ]

Usage:
    python eval_unlimited.py eval-data --engine paddle --json-out paddle.json
    python eval_unlimited.py eval-data --engine unlimited --json-out uocr.json
"""

from __future__ import annotations

import argparse
import json
import time
from pathlib import Path
from typing import Any

from PIL import Image

from eval_ocr import IMAGE_SUFFIXES, cer, wer


def _load_manifest(dataset: Path) -> list[dict[str, Any]]:
    if dataset.is_file() and dataset.suffix.lower() == ".json":
        payload = json.loads(dataset.read_text(encoding="utf-8"))
        if not isinstance(payload, list):
            raise SystemExit("Manifest must be a JSON array")
        return payload
    if not dataset.is_dir():
        raise SystemExit(f"Dataset not found: {dataset}")

    samples: list[dict[str, Any]] = []
    for image_path in sorted(dataset.iterdir()):
        if image_path.suffix.lower() not in IMAGE_SUFFIXES:
            continue
        ref = image_path.with_suffix(".txt")
        if ref.exists():
            samples.append(
                {
                    "id": image_path.stem,
                    "language": "auto",
                    "document_type": "unknown",
                    "pages": [str(image_path)],
                    "reference": str(ref),
                    "features": [],
                }
            )
    return samples


def _run_paddle(image: Image.Image, lang: str) -> tuple[str, float, dict[str, Any]]:
    from app import _ocr_pil_image, _summarise

    started = time.perf_counter()
    blocks = _ocr_pil_image(image, lang)
    text, confidence = _summarise(blocks)
    elapsed_ms = (time.perf_counter() - started) * 1000.0
    return text, confidence, {"latency_ms": round(elapsed_ms, 2), "blocks": len(blocks)}


def _run_unlimited(image: Image.Image, lang: str) -> tuple[str, float, dict[str, Any]]:
    from unlimited_ocr import request_unlimited_image

    started = time.perf_counter()
    blocks, text, quality = request_unlimited_image(image)
    elapsed_ms = (time.perf_counter() - started) * 1000.0
    return text, 0.0, {
        "latency_ms": round(elapsed_ms, 2),
        "blocks": len(blocks),
        "warnings": quality.get("warnings", []),
        "quality": quality,
        "language": lang,
    }


def _evaluate(samples: list[dict[str, Any]], engine: str) -> dict[str, Any]:
    results: list[dict[str, Any]] = []
    for sample in samples:
        pages = sample.get("pages") or []
        if not pages:
            continue
        reference_path = Path(sample["reference"])
        reference = reference_path.read_text(encoding="utf-8")
        hypotheses: list[str] = []
        page_metrics: list[dict[str, Any]] = []
        for page in pages:
            with Image.open(page) as image:
                image = image.convert("RGB")
                if engine == "unlimited":
                    text, confidence, meta = _run_unlimited(image, sample.get("language", "auto"))
                else:
                    text, confidence, meta = _run_paddle(image, sample.get("language", "auto"))
            hypotheses.append(text)
            page_metrics.append(meta)
        hypothesis = "\n\n".join(hypotheses)
        result = {
            "id": sample.get("id") or reference_path.stem,
            "language": sample.get("language", "auto"),
            "document_type": sample.get("document_type", "unknown"),
            "features": sample.get("features", []),
            "pages": len(pages),
            "cer": round(cer(reference, hypothesis), 4),
            "wer": round(wer(reference, hypothesis), 4),
            "chars": len(reference),
            "confidence": round(confidence, 4),
            "latency_ms": round(sum(item.get("latency_ms", 0.0) for item in page_metrics), 2),
            "table_teds": None,
            "formula_cdm": None,
            "reading_order_edit": None,
            "page_metrics": page_metrics,
        }
        results.append(result)
        print(
            f"{result['id']}: CER={result['cer']:.4f} WER={result['wer']:.4f} "
            f"pages={result['pages']} latency_ms={result['latency_ms']:.0f}"
        )

    count = len(results) or 1
    summary = {
        "engine": engine,
        "samples": len(results),
        "pages": sum(item["pages"] for item in results),
        "mean_cer": round(sum(item["cer"] for item in results) / count, 4),
        "mean_wer": round(sum(item["wer"] for item in results) / count, 4),
        "mean_latency_ms": round(sum(item["latency_ms"] for item in results) / count, 2),
        "results": results,
    }
    return summary


def _write_markdown(summary: dict[str, Any], path: Path) -> None:
    lines = [
        "# Unlimited OCR Evaluation",
        "",
        f"- Engine: `{summary['engine']}`",
        f"- Samples: {summary['samples']}",
        f"- Pages: {summary['pages']}",
        f"- Mean CER: {summary['mean_cer']:.4f}",
        f"- Mean WER: {summary['mean_wer']:.4f}",
        f"- Mean latency: {summary['mean_latency_ms']:.0f} ms",
        "",
        "| Sample | Language | Type | Pages | CER | WER | Latency ms |",
        "| --- | --- | --- | ---: | ---: | ---: | ---: |",
    ]
    for item in summary["results"]:
        lines.append(
            f"| {item['id']} | {item['language']} | {item['document_type']} | "
            f"{item['pages']} | {item['cer']:.4f} | {item['wer']:.4f} | {item['latency_ms']:.0f} |"
        )
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Evaluate PaddleOCR or Unlimited OCR against references.")
    parser.add_argument("dataset", type=Path, help="Dataset directory or JSON manifest")
    parser.add_argument("--engine", choices=["paddle", "unlimited"], default="paddle")
    parser.add_argument("--json-out", type=Path, default=None)
    parser.add_argument("--report-md", type=Path, default=None)
    args = parser.parse_args()

    samples = _load_manifest(args.dataset)
    if not samples:
        raise SystemExit("No image/reference pairs found")
    summary = _evaluate(samples, args.engine)
    if args.json_out:
        args.json_out.parent.mkdir(parents=True, exist_ok=True)
        args.json_out.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if args.report_md:
        args.report_md.parent.mkdir(parents=True, exist_ok=True)
        _write_markdown(summary, args.report_md)
    print(
        f"\nSamples: {summary['samples']} | mean CER: {summary['mean_cer']:.4f} | "
        f"mean WER: {summary['mean_wer']:.4f}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
