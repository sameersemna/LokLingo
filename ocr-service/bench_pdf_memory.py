from __future__ import annotations

import argparse
import resource
import time
from pathlib import Path

from app import _ocr_pdf_file


def max_rss_mb() -> float:
    # Linux reports ru_maxrss in KB.
    return resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024.0


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Run OCR PDF extraction and report elapsed time + peak RSS."
    )
    parser.add_argument("pdf", type=Path, help="Path to input PDF file")
    parser.add_argument("--lang", default="auto", help="Language hint (default: auto)")
    parser.add_argument("--dpi", type=int, default=200, help="Render DPI (72-400)")
    args = parser.parse_args()

    if not args.pdf.exists() or not args.pdf.is_file():
        raise SystemExit(f"PDF does not exist: {args.pdf}")
    if args.dpi < 72 or args.dpi > 400:
        raise SystemExit("dpi must be between 72 and 400")

    rss_before = max_rss_mb()
    t0 = time.perf_counter()

    response = _ocr_pdf_file(str(args.pdf), args.lang, args.dpi)

    elapsed = time.perf_counter() - t0
    rss_after = max_rss_mb()

    print(f"PDF: {args.pdf}")
    print(f"Pages processed: {len(response.pages)}")
    print(f"Text chars: {len(response.text)}")
    print(f"Avg confidence: {response.confidence:.4f}")
    print(f"Elapsed seconds: {elapsed:.2f}")
    print(f"Peak RSS before run (MB): {rss_before:.1f}")
    print(f"Peak RSS after run  (MB): {rss_after:.1f}")
    print(f"Delta peak RSS (MB): {rss_after - rss_before:.1f}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
