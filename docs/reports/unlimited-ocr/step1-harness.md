# Step 1 — Evaluation Harness Status

## Delivered

- `ocr-service/eval_unlimited.py` — manifest/directory-driven harness.
- `ocr-service/test_eval_unlimited.py` — manifest, metric, and report tests.
- Refactored `eval_ocr.py` so importing the metric helpers does not require PaddleOCR.
- Metrics: CER, WER, per-page latency, page count, confidence, optional/null table/formula/
  reading-order fields.

## Commands

```bash
cd ocr-service
python3 -m pytest test_eval_unlimited.py -q
python3 eval_unlimited.py /path/to/corpus --engine paddle \
  --json-out ../docs/reports/unlimited-ocr/paddle.json \
  --report-md ../docs/reports/unlimited-ocr/paddle.md
python3 eval_unlimited.py /path/to/corpus --engine unlimited \
  --json-out ../docs/reports/unlimited-ocr/uocr.json \
  --report-md ../docs/reports/unlimited-ocr/uocr.md
```

## Pending

- A real document corpus with per-language ground truth has not been supplied.
- Paddle baseline and Unlimited OCR quality numbers are therefore not yet recorded.
- Table TEDS/formula CDM remain optional/null until the corresponding evaluation libraries
  are added.
