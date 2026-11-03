# Step 12 — Unlimited OCR 2 Evaluation

Status: probe complete; deferred for production.

Unlimited OCR 2 is intentionally out of the production path for this iteration. A
Transformers probe succeeded on the GB10 after adding a C compiler for its Triton/DASH
kernels (the initial run failed with `Failed to find C compiler`).

Probe details:

- Model: `baidu/Unlimited-OCR-2` at `0480966938291445591f69f08a13b041c073a96a`.
- Single-page inference: 3.8 s, 60 output tokens.
- The wrong prompt (`<image>document parsing.`) produced repeated
  `<BLOCK>text[bbox]<FORK>` output. The correct v2 prompt,
  `<PAGE><image>Multi page parsing.`, produces parseable
  `<PAGE><BLOCK>title<LEVEL1>[bbox]<FORK>text` output.
- The adapter now auto-detects `<BLOCK>/<FORK>` output and parses it; live single-page and
  two-page v2 requests succeeded.
- A v2-specific prompt is selected for images when `OCR_UNLIMITED_MODEL` ends in `-2`.
- SGLang was not attempted because of the documented SM121 `sgl_kernel` limitation.

## Prerequisites before starting

- CP1 and CP3 completed.
- A separate `ocr-vlm-v2` environment; do not mix v1/v2 Python dependencies.
- A 100–300 page evaluation corpus.
- Confirmation that the v2 Transformers path works on SM121; SGLang only if `sgl_kernel`
  supports SM121 or is rebuilt from source.

## Decision

Keep v2 **opt-in and non-default** for now. The Transformers path and adapter parsing work,
but v2 long-document quality, table handling, and production rollout are not validated.
Use a separate sidecar instance pointed at `baidu/Unlimited-OCR-2`; v1 and v2 cannot share
the same sidecar model process.
