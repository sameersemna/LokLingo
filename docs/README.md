# LokLingo Documentation

This directory holds research notes, integration plans, and evaluation reports that
are broader than a single service README. Operational runbooks and API references
remain in [`../guide/`](../guide/).

## Index

| Document | Purpose | Status |
| --- | --- | --- |
| [`research/unlimited-ocr.md`](research/unlimited-ocr.md) | Technical research synthesis of Baidu Unlimited OCR / Unlimited OCR 2: architecture, capabilities, benchmarks, limitations, licensing, APIs, and hardware requirements. | Research complete (2026-11-03) |
| [`plans/unlimited-ocr-integration.md`](plans/unlimited-ocr-integration.md) | Step-by-step plan to add Unlimited OCR to LokLingo as an optional document-intelligence provider, including prerequisites, architecture, data flow, testing, risks, rollout, and follow-up agentic coding tasks. | Implemented; real-corpus quality and canary pending |
| [`reports/unlimited-ocr/`](reports/unlimited-ocr/) | Implementation status, environment baseline, serving/security notes, and rollout checklist. | In progress |

## Implementation status

The OCR adapter, backend provider routing, job fields, compose profile, sidecar, evaluation
harness, frontend engine selector, and tests are implemented. The real Unlimited OCR model
passed 10-, 100-, and 200-page synthetic smoke tests, including adaptive window splitting and
repetition retry. Synthetic Latin-language quality is clean, but real-document quality,
language allowlisting, real-user canary, and v2 production integration remain pending. See
`reports/unlimited-ocr/phase5-quality.md`, `long-doc-validation.md`, and
`step12-v2.md`.

## Decision in one paragraph

Unlimited OCR is worth integrating as an **optional, sidecar document-parsing
provider**, not as a wholesale replacement for PaddleOCR. The model is MIT-licensed,
runs locally, and produces structured Markdown with bounding boxes, reading order,
tables, formulas, and headings. The first target should be `ocr_only` PDF extraction
and layout-heavy documents in languages we have evaluated. The existing PaddleOCR
path stays as the default and the fallback. Unlimited OCR 1 is the initial target
because its Transformers path is known to work on the GB10 host; Unlimited OCR 2 is
a later evaluation because its serving path is more complex and its SGLang kernels
are not currently compatible with this host's SM121 GPU.

## Conventions

- Research documents cite primary sources first and label vendor-reported numbers.
- Plans use stable task IDs (`UOCR-01`, `UOCR-02`, ...) so agentic coding sessions can
  reference them without re-reading the whole plan.
- Do not change production defaults in the same change that introduces the provider.
  The feature flag stays off until the Phase 5 quality gate passes.
- Record evaluation results under `docs/reports/` and link them from the plan.
