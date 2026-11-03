# Step 12 — Unlimited OCR 2 Evaluation

Status: probe complete; deferred for production.

Unlimited OCR 2 is intentionally out of the production path for this iteration. A
Transformers probe succeeded on the GB10 after adding a C compiler for its Triton/DASH
kernels (the initial run failed with `Failed to find C compiler`).

Probe details:

- Model: `baidu/Unlimited-OCR-2` at `0480966938291445591f69f08a13b041c073a96a`.
- Single-page inference: 3.8 s, 60 output tokens.
- Output format is **not compatible** with the v1 parser: it emits
  `<BLOCK>text[bbox]<FORK>...` rather than `<|det|>...<|/det|>`.
- The synthetic page produced repeated output, so a separate parser, repetition retry,
  and quality gate are required.
- SGLang was not attempted because of the documented SM121 `sgl_kernel` limitation.

## Prerequisites before starting

- CP1 and CP3 completed.
- A separate `ocr-vlm-v2` environment; do not mix v1/v2 Python dependencies.
- A 100–300 page evaluation corpus.
- Confirmation that the v2 Transformers path works on SM121; SGLang only if `sgl_kernel`
  supports SM121 or is rebuilt from source.

## Decision

Keep v2 deferred. The Transformers path runs, but it needs a dedicated adapter/parser,
quality retry, and long-document benchmark before it can be considered. Do not reuse the
v1 parser or production route.
