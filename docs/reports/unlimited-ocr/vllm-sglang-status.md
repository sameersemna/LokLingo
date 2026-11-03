# vLLM / SGLang Comparison Status

## Decision

The production path remains the Transformers sidecar. It is the only serving path validated
end-to-end on the GB10 host.

## vLLM

- The official `vllm/vllm-openai:unlimited-ocr` image publishes an arm64 manifest, but
  SM121 kernel support is not guaranteed.
- It was not pulled or run. Pulling/building the image and validating the vLLM kernel
  matrix would be a separate infrastructure spike.
- The model card recipe remains available if a future benchmark justifies the effort.

## SGLang

- v1 SGLang is blocked by the prebuilt `sgl_kernel` matrix lacking SM121 (GitHub issue #32).
- v2 SGLang is also unvalidated on this host and uses a custom wheel.
- Compiling `sgl_kernel` from source for SM121 is a follow-up infrastructure task, not a
  drop-in configuration change.

## Transformers (validated)

- Real model load and OCR on GB10.
- Real 100-page specification: 100/100 pages, 26.8 min.
- 200-page synthetic stress test with adaptive splitting and repetition retry.
- v2 Transformers probe works with a C compiler and dedicated parser.

## Future benchmark

Compare Transformers vs vLLM/SGLang on the same 100-page corpus for:
- p50/p95 per page,
- throughput at concurrency 1/2/4,
- peak memory,
- cold start,
- page-count/quality parity.

Adopt a different backend only if it is SM121-compatible and measurably better.
