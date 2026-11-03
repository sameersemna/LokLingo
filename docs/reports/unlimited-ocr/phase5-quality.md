# Step 5/7 — Quality Smoke Evaluation

Status: synthetic smoke test complete; real-corpus gate remains open.

## Corpus

Four synthetic single-line pages, one each in English, German, Spanish, and French. Each
page has exact ground truth text. This is a **smoke test**, not a representative document
corpus; it does not cover scans, tables, formulas, multi-column layouts, or non-Latin
scripts.

## Results

| Engine | Samples | Mean CER | Mean WER | Mean latency |
| --- | ---: | ---: | ---: | ---: |
| Unlimited OCR | 4 | 0.0000 | 0.0000 | ~0.63–0.78 s/page |
| PaddleOCR | 4 | 0.0000 | 0.0000 | ~1.3–2.5 s/page |

Reports:

- [`uocr-synthetic-eval.md`](uocr-synthetic-eval.md)
- [`paddle-synthetic-eval.md`](paddle-synthetic-eval.md)

## Real-document and language smoke

- Real GLSL 4.60.7 specification, first 100 pages: 100/100 pages, 217,905 chars,
  no warnings/degenerate flags, ~26.8 min. A 200-page attempt exposed runaway table
  generation and timed out; caps/retry were added but the 200-page real run is pending.
- Arabic sample: UOCR output was garbled/reversed; PaddleOCR lacks an Arabic model in
  this image. Arabic stays out of the allowlist.
- Mixed CJK sample: UOCR captured Chinese, Japanese, and Korean; PaddleOCR omitted the
  Korean line. CJK is promising but not yet allowlisted.

See [`real-document-validation.md`](real-document-validation.md) and
[`language-coverage-smoke.md`](language-coverage-smoke.md).

## Long-document smoke

- 100-page synthetic PDF: 100/100 pages, no empty pages, ~300 s.
- 200-page synthetic PDF (before retry tuning): 200/200 pages but repeated text on the
  final page and a page-number mismatch.
- 200-page synthetic PDF (after repeated-line retry and 5-page windows): 200/200 pages,
  correct first/last page text, retries logged for pages 26, 58, and 200, ~580 s
  (~2.9 s/page).

## Decisions

- **Allowlist remains empty.** Synthetic Latin text is not sufficient evidence for
  automatic routing. Keep `OCR_UNLIMITED_LANGS` empty until real per-language documents
  are evaluated.
- **Confidence remains `0.0` + quality metadata.** No calibrated confidence exists.
- **LLM correction remains off** for Unlimited OCR output.

## Required before enabling auto routing

- Real corpus for each target language, including scans, tables, formulas, and headings.
- CER/WER/table/reading-order comparison against PaddleOCR.
- Manual heading/structure review.
- Failure and repetition review at 100+ pages.
- Documented language allowlist approved by the operator.
