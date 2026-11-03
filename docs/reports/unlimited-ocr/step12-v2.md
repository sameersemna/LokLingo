# Step 12 — Unlimited OCR 2 Evaluation

Status: deferred / not started.

Unlimited OCR 2 is intentionally out of the production path for this iteration. The v1
sidecar and adapter must first pass CP1 (real model serving on GB10/SM121) and CP3 (quality
gate). v2 uses a custom SGLang wheel and sparse/DASH kernels; the documented SM121
incompatibility makes it a separate spike, not a follow-on edit.

## Prerequisites before starting

- CP1 and CP3 completed.
- A separate `ocr-vlm-v2` environment; do not mix v1/v2 Python dependencies.
- A 100–300 page evaluation corpus.
- Confirmation that the v2 Transformers path works on SM121; SGLang only if `sgl_kernel`
  supports SM121 or is rebuilt from source.

## Decision rule

Adopt v2 only if it materially improves page-count/quality on 100+ page documents without
increasing failure, OOM, or operational complexity beyond the team's support capacity.
Otherwise keep v1 page-windowing.
