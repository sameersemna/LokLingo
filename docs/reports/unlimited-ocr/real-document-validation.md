# Real-Document Validation — GLSL 4.60.7 Specification

## Document

- Source: local `GLSLangSpec.4.60.pdf`
- Pages: 238; first 100 pages used for the completed run
- Content: real specification text, tables, code, and section headings
- DPI: 150
- Engine: Unlimited OCR v1, Transformers backend, revision `8249d20b...`

## 100-page result

- Status: HTTP 200
- Pages: 100/100
- Characters: 217,905
- Empty pages: 0
- Warnings: 0
- Degenerate flags: 0
- Runtime: **1,608.6 s (~26.8 min, ~16 s/page)**
- Page 1 begins with the correct document title/version; page 100 contains plausible GLSL output-variable content.

## 200-page attempt

An earlier 200-page run against the same real document exceeded the 30-minute client timeout.
The sidecar was still generating on a complex table around page 186, producing a runaway
sequence of repeated table rows. That output was discarded.

Mitigations added afterward:

- Per-page output budget reduced from 20,000 to 4,096 tokens.
- Sidecar caps v1 total context using the requested output budget.
- Table-row/character/repetition degeneration detection and single-page retry.
- Backend Unlimited provider timeout raised from 900 s to 3,600 s.

The 200-page real-document run has not been re-run after these changes. A 200-page real run
is expected to take roughly one hour and should be executed asynchronously or with an
extended job budget.

## Rough text-layer comparison

Comparing the first 10 UOCR pages with `pdftotext -layout` gave CER 0.3821 / WER 0.6807.
This is **not** a valid quality score: the reference includes dot-leader table-of-contents
rows, page furniture, and layout spacing that the OCR text does not reproduce. It is
recorded only to show why text-layer comparison is insufficient; real quality review needs
human-checked ground truth.

## Conclusion

Unlimited OCR v1 can process a real 100-page technical specification with complete page
coverage and no degeneration after the new caps. Real 200-page/table-heavy documents remain
a long-running, asynchronous workload and require further quality validation.
