# Baidu Unlimited OCR — Research Synthesis

**Status:** Research complete; no code changes made.
**Research date:** 2026-11-03.
**Scope:** Baidu Unlimited OCR (v1, June 2026) and Unlimited OCR 2 (October 2026):
architecture, capabilities, benchmarks, limitations, licensing, APIs, serving, hardware,
and fit for LokLingo.
**Companion plan:** [`../plans/unlimited-ocr-integration.md`](../plans/unlimited-ocr-integration.md).

---

## 1. Executive summary

Baidu Unlimited OCR is a family of open-weight, end-to-end document-parsing
vision-language models built on the DeepSeek-OCR lineage. They replace the usual
"detect layout, crop blocks, run a recognizer per block" pipeline with a single
model that turns ordered page images into structured text (Markdown-like output with
text, tables, formulas, headings, figures, and reading order). Both generations are
MIT-licensed and run locally on NVIDIA GPUs; neither is a hosted Baidu API.

The two generations solve different problems:

- **Unlimited OCR 1** (June 2026) uses **Reference Sliding Window Attention
  (R-SWA)** to keep the decoder's KV cache bounded while every generated token still
  attends to all visual tokens. It has a 32K context, compresses a 1024×1024 page to
  about **256 visual tokens**, and is practical for **dozens of pages** — independent
  coverage puts the practical single-pass ceiling near **40 pages**, not 100.
- **Unlimited OCR 2** (October 2026) replaces R-SWA with **Sparse Reference Sliding
  Window Attention (SR-SWA)** plus an **Unlimited-Horizon Decoding Paradigm**. It
  keeps only a fixed-size set of visual references active (Page Gate + Token Sieve),
  raises the context to **131,072 tokens**, uses about **273 visual tokens per page**,
  and is designed for **100–300 page** documents. It is the stronger long-document
  model, but its serving story is newer and more operationally demanding.

**Recommendation for LokLingo:** integrate **Unlimited OCR 1 first**, as an optional
sidecar provider behind the existing OCR service. Use it for `ocr_only` PDF
extraction and layout/table/formula-heavy documents in languages we have evaluated,
with PaddleOCR as the default and fallback. Do **not** make it the default OCR engine
for every language or for low-latency single-image OCR until our own corpus shows it
is at least as good as the current path. Evaluate Unlimited OCR 2 later, after its
Transformers path and long-document behavior have been validated on this host.

**Host-specific finding:** this machine is an NVIDIA GB10 (DGX Spark class),
`aarch64`, CUDA 13.0, compute capability **SM121**. Docker GPU access works
(`nvidia-smi -L` succeeds inside a CUDA container). However, Baidu's documented
SGLang path for Unlimited OCR 1 is blocked on SM121 because the prebuilt
`sgl_kernel` binaries do not include SM121; the validated workaround is the
**Transformers + no-repeat-ngram** path. Unlimited OCR 2's serving recipe uses a
custom SGLang wheel and sparse/DASH Triton kernels, so it carries the same or greater
risk on this host. The integration plan therefore treats the serving backend as a
Phase 1 spike, not an assumption.

---

## 2. Sources and confidence

| ID | Source | Type | What it contributes | Confidence notes |
| --- | --- | --- | --- | --- |
| S1 | [arXiv:2606.23050 — Unlimited OCR Works](https://arxiv.org/pdf/2606.23050) | Primary paper (v1) | R-SWA formulation, architecture, training data, OmniDocBench and long-horizon results, limitations | Author-reported; no independent reproduction found |
| S2 | [Hugging Face: baidu/Unlimited-OCR](https://huggingface.co/baidu/Unlimited-OCR) | Primary model card/code (v1) | Transformers/vLLM/SGLang commands, inference API, output post-processing, license metadata | Model card; commands may drift |
| S3 | [GitHub: baidu/Unlimited-OCR](https://github.com/baidu/Unlimited-OCR) | Primary code repo | MIT LICENSE, README, v2 update pointer, issues | Repo shared by v1/v2; check tags/revisions |
| S4 | [Hugging Face: baidu/Unlimited-OCR-2](https://huggingface.co/baidu/Unlimited-OCR-2) | Primary model card/code (v2) | v2 Transformers/SGLang recipes, memory knobs, OpenAI-compatible request shape, 131K context | Model card; no vLLM recipe listed |
| S5 | [Unlimited OCR 2 technical report (PDF)](https://huggingface.co/baidu/Unlimited-OCR-2/blob/main/Unlimited-OCR-2.pdf) | Primary paper (v2) | SR-SWA (Page Gate, Token Sieve), Unlimited-Horizon Decoding, MarathonDocBench, scaling results, limitations | Author-reported; MarathonDocBench is Baidu-proposed |
| S6 | [MarkTechPost — Baidu Open-Sources Unlimited OCR 2](https://www.marktechpost.com/2026/10/26/baidu-unlimited-ocr-2-open-source-long-document-ocr/) | Secondary news | v2 headline numbers, comparison table, serving notes, sub-scores from Baidu's leaderboard chart | Some numbers are chart-derived; vendor-reported |
| S7 | [Baidu Baike — Unlimited OCR](https://baike.baidu.com/en/item/Unlimited%20OCR/2567993) | Secondary encyclopedia | v1 release history, R-SWA/DeepEncoder explanation, OmniDocBench scores | Contains an internal parameter-count inconsistency (see §3) |
| S8 | [The Menon Lab — Unlimited-OCR](https://themenonlab.blog/blog/unlimited-ocr-baidu-free-pdf-parsing) | Secondary blog | Plain-language explanation, quick-start snippets, MIT/commercial-use claim | Simplified; not a primary source |
| S9 | [vLLM Recipes — baidu/Unlimited-OCR](https://recipes.vllm.ai/baidu/Unlimited-OCR) | Primary serving recipe | vLLM image tags, required logits processor, prompt/ngram requirements, ≥8 GB claim | Recipe; practical VRAM is higher for long prefill |
| S10 | [GitHub issue #32 — GB10/ARM64 SGLang blocked](https://github.com/baidu/Unlimited-OCR/issues/32) | Community issue | SM121/aarch64 failure mode; Transformers+ngram workaround; tested torch 2.12.1+cu130 | Directly relevant to this host; community-reported |
| S11 | [GitHub issue #27 — deployment blockers](https://github.com/baidu/Unlimited-OCR/issues/27) | Community issue | Early missing Docker/vLLM/quantized weights; maintainer responses | Historical; vLLM/Docker have since appeared |
| S12 | [Digital Applied — 3B single-pass analysis](https://www.digitalapplied.com/blog/baidu-unlimited-ocr-3b-document-automation-2026) | Independent review | Practical ~40-page ceiling, VRAM tiers, production caveats, benchmark credibility | Independent but secondary; useful sanity check |
| S13 | [AIReiter — What Baidu's 3B model can and can't do](https://aireiter.com/blog/unlimited-ocr-baidu-api-review) | Independent review | Token-budget math, KV-cache math, serving requirements, long-doc degradation, self-host cost | Independent; some numbers modeled, not measured |
| S14 | [HF model API metadata](https://huggingface.co/api/models/baidu/Unlimited-OCR) | Primary metadata | `license: mit`, file list, weights/config names | Verified directly on 2026-11-03 |
| S15 | [HF config.json (v1)](https://huggingface.co/baidu/Unlimited-OCR/raw/main/config.json) / [config.json (v2)](https://huggingface.co/baidu/Unlimited-OCR-2/raw/main/config.json) | Primary config | 12 layers, 64 routed experts/6 active, 128 sliding window, 32K vs 131K positions, 273 tokens/page (v2) | Verified directly on 2026-11-03 |
| S16 | Model code (`modeling_unlimitedocr.py`, v1/v2) | Primary code | `<PAGE>` splitting, `<|ref|>/<|det|>` parsing, bbox 0–999 scaling, `infer`/`infer_multi` behavior | Verified directly on 2026-11-03 |

Source hierarchy used below: **primary** (papers, model cards, configs, code,
licenses) beats **secondary** (news, blogs), which beats **community** (issues,
Reddit). Vendor-reported benchmark numbers are explicitly labeled.

---

## 3. Generations at a glance

| Property | Unlimited OCR (v1) | Unlimited OCR 2 |
| --- | --- | --- |
| Release | 2026-06-22 | 2026-10-26 |
| Base | DeepSeek-OCR checkpoint, continue-trained | Initialized from Unlimited OCR v1, post-trained |
| Parameters | 3B total, ~500M active (3B-MoE-A570M) | 3.34B total weights; paper describes 3B total / ~500M active |
| Decoder | 12 layers, 64 routed experts, 6 active/token, 2 shared experts | Same backbone family; SR-SWA/DASH sparse attention |
| Long-context mechanism | R-SWA: all visual + prompt tokens, last 128 output tokens | SR-SWA: Page Gate + Token Sieve, fixed active reference set |
| Context | 32,768 tokens | 131,072 tokens (`max_position_embeddings`), YaRN rope scaling |
| Visual tokens/page | ~256 (1024×1024, 16× compression) | ~273 (`per_page_image_tokens: 273`) |
| Practical document length | Dozens of pages; ~40-page practical ceiling | 100–300 pages (MarathonDocBench 103–300) |
| Serving | Transformers, vLLM, SGLang (custom wheel) | Transformers (native), SGLang (custom wheel); no vLLM recipe listed |
| License | MIT (weights + code) | MIT (weights + code) |
| Best fit | First integration, single-GPU, ≤40-page docs | Later upgrade for hundred-page docs, after serving validation |

**Parameter-count inconsistency:** Baidu Baike [S7] first says "30 billion total and
5 billion activated" and later says "3 billion total and 500 million activated." The
paper [S1], config [S15], and model card [S2] support **3B total / ~500M active**.
The MarkTechPost v2 article [S6] says **3.34B** and "active parameters not disclosed";
the v2 paper [S5] says **3B total / 500M activated**. Treat "3B-class, ~0.5B active"
as the working description, and verify exact counts from the safetensors metadata for
the revision you deploy.

---

## 4. Architecture

### 4.1 DeepEncoder

Both generations keep the DeepSeek-OCR **DeepEncoder**: a SAM-ViT-B branch cascaded
with a CLIP-L branch, with 16× token compression at the bridge. The first half uses
window attention on raw image tokens; global attention is reserved for the compressed
tokens. This keeps activations low for high-resolution pages while preserving text
features.

- A 1024×1024 PDF page becomes about **256 visual tokens** in v1 [S1, S13].
- v2 uses **273 image tokens per page** (config `sparse_attention_config.per_page_image_tokens`)
  [S15].
- v1 keeps two resolution modes: **Base** (1024×1024, used for multi-page) and
  **Gundam** (dynamic tiling/crop for single-page detail) [S1, S2].
- The encoder is frozen during v1 training; v2 also freezes the encoder and aligner
  [S1, S5].

The practical consequence: multi-page/PDF mode is Base-only. Very small text on dense
pages can be missed because the model never sees the higher-detail Gundam crops in
multi-page mode. Baidu attributes most long-document repetition errors to this, not to
attention drift [S1].

### 4.2 Decoder and MoE

The decoder is a 12-layer Mixture-of-Experts language model:
64 routed experts, 6 selected per token, plus 2 shared experts. v1 activates about
500M parameters per token. The vocabulary is 129,280 tokens; the model uses a
DeepSeek-style architecture with `use_mla: false`, 10 attention heads, and
`v_head_dim: 128`.

The important part for serving is the attention and cache policy, not the MoE itself.
The MoE keeps per-token compute low; the attention change is what makes long output
affordable.

### 4.3 R-SWA (v1)

Reference Sliding Window Attention is the v1 contribution. For a generated token at
step `t`, the accessible set is:

```
N(t) = P ∪ D_n(t)
P = {1, ..., L_m}                                  # visual tokens + prompt
D_n(t) = { j | max(L_m+1, L_m+t-n) ≤ j ≤ L_m+t-1 } # causal output window
```

`L_m` is the fixed prefix length (visual + prompt); `n` defaults to **128**. Every
token sees the full image/prompt, but only the last `n` output tokens. The KV cache
therefore becomes:

```
C_MHA(T)    = L_m + T
C_R-SWA(T)  = L_m + min(n, T) ≤ L_m + n
cache ratio = (L_m + min(n, T)) / (L_m + T) → 0 as T grows
```

The cache behaves like a fixed-capacity queue: once the output window is full, the
oldest output KV is evicted. This is why v1 can decode very long outputs with flat
memory and per-step attention cost. The paper's FlashAttention-3 kernel measurements
show constant per-call latency vs. growing latency for standard MHA [S1].

**What R-SWA does not fix:** the visual prefix still grows with page count. At
256 tokens/page, 100 pages add 25,600 visual tokens before any output. The 32K
context therefore bounds the number of pages, and prefill cost still rises with page
count [S1].

### 4.4 SR-SWA (v2)

SR-SWA replaces "attend to all visual tokens" with a **fixed-size active reference
set**. The paper describes two stages [S5]:

1. **Page Gate** — for a query associated with page `g(q)`, retain only blocks that
   intersect the current page (or otherwise relevant pages). Block boundaries can
   straddle pages, so the gate is block-based. Positions are factorized into a bounded
   page-level coordinate (alternating 0–1 pattern) and a monotonic intra-page
   coordinate, to avoid positional explosion as page count scales.
2. **Token Sieve** — summarize candidate blocks into compact key representations,
   score them against the query, apply α-entmax, and retain the **top-k** blocks.
   The routing probabilities become a mean-centered log-prior bias in the attention
   logits, so routing is trained end-to-end with the language-modeling objective
   rather than an auxiliary distillation loss.

The retained set is `R(q) = L(q) ∪ selected remote blocks`, where `L(q)` is a fixed
local window. MarkTechPost [S6] summarizes the shipped configuration as a **512-token
reference set**: a **128-token local window** plus **top-16 selection over 32-token
blocks**. The v2 config confirms `sparse_block_size: 32`, `sparse_topk_blocks: 16`,
`sparse_local_block: 4`, and `per_page_image_tokens: 273` [S15]. The code names the
mechanism **DASH** block-sparse attention; "SR-SWA" is the paper/marketing name for
the same deployed mechanism [S5, S16].

The effect: per-query visual attention no longer grows with document length, so
per-step decoding latency is near-constant from short to hundred-page documents.
The paper reports **7.0×** per-step speedup vs. full attention and **5.6×** vs. R-SWA
at hundred-page scale [S5].

### 4.5 Unlimited-Horizon Decoding

Even with SR-SWA, prefill and retained KV caches for hundreds of pages can exceed GPU
memory. v2 introduces page-chunked decoding [S5]:

- Partition the document into overlapping page chunks (one-page boundary overlap).
- Encode/decode the first chunk, then for each next chunk reuse the boundary page's
  KV and the previous chunk's text KV tail.
- Release visual KV caches outside the active chunk.

The paper reports peak KV cache dropping from **47.00 GB to 1.77 GB** for a
single-sequence hundred-page run, and throughput rising from **360 to 1,315 TPS**
at batch size 16 [S5]. This is the mechanism behind the "unlimited-horizon" claim.
It is not literally unlimited: context length, chunk size, and output budget still
bound the run.

### 4.6 Token-budget intuition

| Pages in one pass | v1 visual tokens | v1 tokens left for output (32K) | v1 output budget/page |
| --- | ---: | ---: | ---: |
| 10 | ~2,560 | ~30,200 | ~3,020 |
| 20 | ~5,120 | ~27,600 | ~1,380 |
| 40 | ~10,240 | ~22,500 | ~560 |

The table is from an independent review [S13] and is a useful planning heuristic, not
a guarantee: the actual output length depends on page density. Dense two-column pages
can exceed 560 tokens/page, so the practical ceiling can be lower than 40 pages.
For v2 at 273 tokens/page, 100 pages consume ~27,300 visual tokens; 300 pages consume
~81,900 of the 131,072-token context, leaving little room for output unless page
windowing is used. This is why the v2 serving recipe exposes page-window scheduling
and a per-page output cap.

---

## 5. Capabilities

### 5.1 What the model does

- **End-to-end document parsing** from ordered page images to structured text.
- **Multi-page, one-shot parsing** (v1: dozens of pages; v2: hundreds).
- **Layout-aware output**: text, headings, tables, formulas, figures, reading order.
- **Bounding boxes** in the raw output (`<|det|>label [bbox]<|/det|>`), normalized to
  0–999 relative to the page image.
- **Figure extraction**: `image` regions can be cropped and replaced with Markdown
  image links by the model's own post-processing code.
- **Cross-page context**: v1 keeps all pages' visual tokens in the prefix; v2 keeps a
  fixed reference set and reuses boundary-page KV across chunks.
- **Local/self-hosted**: no per-page API bill, no document upload to a third party.
- **MIT license**: commercial use, modification, redistribution, subject to the
  license notice.

### 5.2 What it is not

- **Not a general OCR API for arbitrary photos.** It is trained on PDF/document
  pages; handwriting, severe skew, stamps, and low-quality camera captures are not
  its benchmarked strength.
- **Not a language-specific OCR engine.** The training data is document-parsing data;
  language coverage is not explicitly enumerated. Independent testing suggests strong
  English and likely Chinese, with weaker results on mixed multilingual content such
  as Urdu [S12, S13]. Do not assume parity with PaddleOCR's dedicated Arabic/Indic
  models.
- **Not a confidence-scored OCR engine.** The model emits text and boxes, not
  calibrated per-block confidence. LokLingo's confidence-based fallback logic needs a
  proxy or an explicit "unknown" state.
- **Not infinite-context.** v1 is bounded by 32K prefill; v2 is bounded by 131K plus
  page chunking. Baidu's own limitation sections say so [S1, S5].
- **Not a drop-in replacement for PaddleOCR's preprocessing stack.** It has its own
  image preprocessing and expects page images, not PDFs.

### 5.3 Output format

The raw generation is Markdown-like and includes grounding markup:

```
<PAGE>
<|ref|>title<|/ref|><|det|>title [45, 32, 955, 88]<|/det|>
Document title

<|ref|>text<|/ref|><|det|>text [45, 100, 500, 140]<|/det|>
First paragraph...

<|det|>table [45, 300, 955, 700]<|/det|>
| ... |
```

Details verified in the model code [S16]:

- **Page separator:** multi-page output is split on `<PAGE>`.
- **Grounding:** the code recognizes both
  `<|ref|>label<|/ref|><|det|>[[x1,y1,x2,y2]]<|/det|>` and direct
  `<|det|>label [x1,y1,x2,y2]<|/det|>` forms.
- **Coordinates:** normalized by `/999` and multiplied by the rendered image width and
  height. A label can have one box or a list of boxes.
- **Labels:** the code treats `image` specially (crops it); other labels are text
  blocks. Typical labels include `title`, `text`, `table`, `formula`, `image`, etc.
- **Clean text:** strip `<|ref|>`/`<|det|>` markers for plain text; keep boxes if you
  need overlay coordinates. The HF README includes a post-processing regex for
  OmniDocBench evaluation [S2].

**Caveat:** one independent review says page boundaries are not emitted natively and
suggests asking for page labels [S13]. The model code explicitly splits on `<PAGE>`
and the multi-page prompt is `Multi page parsing.` [S16]. Treat `<PAGE>` availability
as something to verify against the exact model revision; if it is missing, inject
page markers per request window rather than guessing boundaries.

---

## 6. Benchmarks

### 6.1 OmniDocBench (single-page document parsing)

Baidu's v1 paper reports [S1]:

| Benchmark | Overall ↑ | Text Edit ↓ | Formula CDM ↑ | Table TEDS ↑ | Table TEDS-S ↑ | Read-order Edit ↓ |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| OmniDocBench v1.5 — Unlimited OCR | **93.23** | 0.038 | 92.61 | 90.93 | 94.07 | 0.045 |
| OmniDocBench v1.5 — DeepSeek-OCR | 87.01 | 0.073 | 83.37 | 84.97 | 88.80 | 0.086 |
| OmniDocBench v1.6 — Unlimited OCR | **93.92** | 0.042 | 95.79 | 90.16 | 93.32 | 0.129 |
| OmniDocBench v1.6 — DeepSeek-OCR 2 | 90.25 | 0.050 | 91.84 | 83.89 | 87.75 | 0.144 |

v2's paper reports **94.02** on OmniDocBench v1.6 (Table 3) [S5].

Interpretation:

- These are the authors' numbers. OmniDocBench is an external benchmark (OpenDataLab),
  which is stronger than a purely self-built eval, but we found no independent
  reproduction of these exact scores.
- v1 beats its DeepSeek-OCR baseline by ~6.2 points overall and improves formula/table
  metrics substantially.
- v1.6 is a newer benchmark cut; do not compare v1.5 and v1.6 scores as if they were
  the same test.

### 6.2 Multi-page benchmarks

Baidu's v2 paper introduces **MarathonDocBench** (100 documents, 103–300 pages,
205k annotated layout elements) and also evaluates on **MPDocBench** (up to ~30 pages).
Author-reported overall scores [S5, S6]:

| Model | MPDocBench overall ↑ | MarathonDocBench overall ↑ |
| --- | ---: | ---: |
| Unlimited OCR 2 | **84.59** | **77.06** |
| Unlimited OCR (v1) | 61.89 | 0.0 / not reported (40-page input limit) |
| Qwen3.8-Max | 78.30 | 22.93 |
| Qwen-3.8-Flash-Next | 77.36 | 27.83 |
| Gemini-3.8-Flash | 76.25 | 28.61 |
| Kimi-K3 | 60.77 | 33.74 |
| Gemini-3.1-Pro Preview | 71.79 | 26.97 |

MarkTechPost's chart-derived sub-scores for v2 [S6]:

| Metric | MPDocBench | MarathonDocBench |
| --- | ---: | ---: |
| Text | 94.8 | 89.6 |
| Formula | 89.4 | 87.9 |
| Reading order | 92.3 | 85.3 |
| Tables | 87.8 | 74.8 |
| Heading hierarchy (TEDS) | 60.1 | **48.2** |

The weak spot is **heading hierarchy**, especially on hundred-page documents (48.2).
Tables also degrade from 87.8 to 74.8. Long-document text remains strong at 89.6.
The v2 paper itself lists document-level structure (heading hierarchies) as a
limitation of its sparse-attention design [S5].

**Benchmark caveats:**

- MarathonDocBench is proposed by Baidu; it is not an independent third-party
  benchmark. MPDocBench is public.
- MarkTechPost says Baidu claims first place on both benchmarks against nine models,
  but all numbers are vendor-reported and some appear only in a chart image.
- The v1 paper's long-horizon evaluation is an in-house test set, not a public
  benchmark.

### 6.3 v1 long-horizon quality

From the v1 paper's in-house multi-page set [S1]:

| Pages | 2 | 5 | 10 | 15 | 20 | 40+ |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Distinct-20 ↑ | 99.76% | 99.78% | 97.49% | 99.92% | 98.73% | 96.08% |
| Distinct-35 ↑ | 99.87% | 99.98% | 99.83% | 99.99% | 99.89% | **96.90%** |
| Edit distance ↓ | 0.0362 | 0.0452 | 0.0526 | 0.0787 | 0.0572 | **0.1069** |

Quality degrades measurably at 40+ pages: edit distance roughly triples from the
2-page case, and repetition starts to appear (Distinct-35 drops to 96.90%). The
non-monotonic 15-page result (0.0787 vs. 0.0572 at 20 pages) means this is a trend,
not a per-page guarantee. Baidu attributes most failures to small text at Base
resolution, not to losing attention direction [S1].

### 6.4 Throughput and memory

v1 [S1]:

- At 6,144 output tokens: **7,847.71 TPS** vs. DeepSeek-OCR **5,822.87 TPS** (~35%
  faster). At 256 tokens the two are effectively identical.
- OmniDocBench Base mode, 512 concurrency: **5,580 TPS** vs. **4,951 TPS** (+12.7%).
  The advantage grows with output length.

v2 [S5]:

- **7.0×** per-step speedup vs. full attention and **5.6×** vs. R-SWA at hundred-page
  scale.
- Page-chunked decoding: peak KV **47.00 GB → 1.77 GB** (batch size 1); throughput
  **360 → 1,315 TPS** (batch size 16).

These are ideal-condition or author-cluster measurements. Our GB10 numbers will
differ, especially with Transformers (no continuous batching) vs. vLLM/SGLang.

---

## 7. Limitations and failure modes

### 7.1 Model limitations

1. **Finite context / prefill growth.** v1 has 32K context; v2 has 131K and chunked
   decoding, but prefill still grows with pages. "Unlimited" refers to decode-side
   cache behavior, not infinite input. Baidu's future work lists 128K context and a
   prefill pool for automatic page fetching [S1, S5].
2. **Long-document quality degradation.** v1 edit distance rises from 0.0362 at
   2 pages to 0.1069 at 40+ pages; repetition increases. v2 is far better at hundred
   pages but heading hierarchy remains weak (48.2) and tables degrade (74.8).
3. **Small-text loss in multi-page mode.** Multi-page/PDF uses Base 1024×1024, not
   Gundam dynamic crops. Dense pages, footnotes, and small print suffer [S1, S13].
4. **Heading hierarchy and document-level structure.** v2's sparse attention does not
   preserve global document information, which limits document-level structure
   understanding [S5].
5. **Language coverage is undocumented.** The training corpus is document-parsing
   data, not a declared multilingual benchmark. Independent testing suggests English
   is reliable, Chinese likely, and mixed multilingual content (e.g., Urdu) can
   struggle [S12, S13]. Arabic/Indic quality is unknown; do not assume parity with
   PaddleOCR's dedicated models.
6. **No calibrated confidence.** Raw output has no per-block confidence. Any
   confidence-based routing or low-confidence warning needs a proxy or must be
   disabled for this provider.
7. **Hallucination/repetition risk.** VLM OCR can omit, duplicate, or invent content,
   especially on long or degraded documents. The no-repeat-ngram processor mitigates
   loops but does not guarantee fidelity. Always preserve a fallback and, for critical
   documents, a human verification path.
8. **Special-token/recipe sensitivity.** The models require specific prompts,
   `skip_special_tokens=False`, and the no-repeat-ngram logits processor. The vLLM
   recipe warns that without the recipe the model can return empty output [S9].
   The SGLang path requires a custom wheel and flags; the v1 model card has a
   `kernels` version inconsistency (text says 0.9.0, command installs 0.11.7) [S2, S13].
9. **Quantization sensitivity.** OCR VLMs are unusually sensitive to quantization.
   LokLingo's own OCR guidance records DeepSeek-OCR CER rising from 0.78% to 15.6%
   with Q4_K_M. Use BF16 or, at minimum, Q8 for production; treat community INT4/AWQ
   builds as experiments [S11, project `guide/ocr-fine-tuning-workflow.md`].
10. **No official hosted API.** There is no first-party Baidu endpoint; self-hosting
    is the intended path. The model card lists Transformers/vLLM/SGLang; operational
    ownership is ours [S2, S13].

### 7.2 Serving limitations

1. **Custom kernels on new hardware.** v1's SGLang path fails on GB10/SM121 because
   `sgl_kernel` binaries target sm80–sm100, not SM121. The fix requires recompiling
   `sgl_kernel` against CUDA 13.0 for SM121; the validated workaround is Transformers
   + ngram [S10]. v2's custom SGLang wheel and sparse/DASH Triton kernels carry the
   same risk.
2. **vLLM image support must be validated.** The official vLLM image
   `vllm/vllm-openai:unlimited-ocr` publishes both `linux/arm64` and `linux/amd64`
   manifests (verified with `docker manifest inspect`), but an arm64 manifest does not
   guarantee SM121 kernel support. Run a smoke test before committing to vLLM.
3. **Memory sizing.** BF16 weights are ~6.67 GB. A 32K prefill adds ~1.875 GiB of KV
   cache in theory; vision activations and allocator overhead sit on top. Practical
   guidance is **12 GB minimum, 24 GB comfortable** for BF16; the "8 GB" claim applies
   to short/single-page or quantized setups [S12, S13]. Our GB10 has ~121 GiB unified
   memory, so capacity is not the constraint; throughput and kernel compatibility are.
4. **Cold start/model load.** Loading a 3B model and compiling kernels can take
   minutes. Health checks and `start_period` must reflect this; the OCR service must
   not fail startup just because the optional sidecar is still warming.
5. **Prefix caching assumptions break.** vLLM must run with
   `--no-enable-prefix-caching` and `--mm-processor-cache-gb 0` for v1; SGLang uses
   `--disable-radix-cache` and related flags. These reduce throughput but are required
   by the sliding-window cache design [S9, S2].
6. **Custom wheel supply chain.** The SGLang wheel is a Baidu-built development wheel
   (around 12 MB), not an upstream release. Pin its SHA256 and the model revision.
   `trust_remote_code=True` executes Python from the model repo; treat it as code
   supply chain, not just weights [S2, S3].

### 7.3 Product/UX limitations

- **Confidence display.** LokLingo currently shows OCR confidence in job results and
  uses low-confidence warnings. Unlimited OCR has no confidence; we need an explicit
  "quality unknown" state or a calibrated proxy.
- **Language allowlist.** Users may expect every language to work. The provider
  should be opt-in per language until evaluated, with a clear message when the
  fallback engine handled the document.
- **Output format.** The raw output is Markdown with grounding tags; the current
  product expects plain text and blocks. Normalization is required, and it must
  preserve paragraph/table structure without leaking `<|det|>` markers into
  translated text.
- **Page-level translation.** The PDF translation path currently translates per-page
  text. Unlimited OCR can return cross-page context but the rest of the pipeline still
  chunks by page; cross-page tables/headings may still be split. A document-level
  translation chunking strategy is a separate improvement.

---

## 8. Licensing and compliance

- **Weights:** MIT, confirmed by the HF model API (`license: mit`) and the GitHub
  LICENSE file [S3, S14].
- **Code:** MIT (GitHub LICENSE, copyright 2026 Baidu) [S3].
- **Commercial use:** permitted; modify and redistribute permitted; keep the
  copyright/license notice; no warranty.
- **Third-party components:** the repos acknowledge DeepSeek-OCR, DeepSeek-OCR-2, and
  PaddleOCR, and the model repo contains custom Python modules (`modeling_*.py`,
  `deepencoder.py`, sparse/DASH kernels). MIT on Baidu's repository does not
  automatically relicense third-party code. Before redistribution or a commercial
  release, review the notices/licenses of the vendored code and dependencies. Keep a
  `THIRD_PARTY_NOTICES` file and the model card citation.
- **Model card citation:** `@misc{yin2026unlimitedocrworks, ...}` from the HF model
  card [S2]. Cite the paper and model revision in internal docs.
- **Data privacy:** local inference means document images do not leave the host.
  Model download from Hugging Face is the only external network dependency, and it can
  be pre-fetched for air-gapped operation.
- **Security:** `trust_remote_code=True` is arbitrary code execution at load time.
  Vendor the model code at a pinned revision, review it, and run the inference
  container with least privilege, no public port, and restricted egress. Do not load
  a mutable `main` revision in production.

---

## 9. APIs and serving

### 9.1 Transformers (v1)

Tested Python 3.12.3 + CUDA 12.9 per the model card; our GB10 uses CUDA 13.0 and
aarch64, and community testing used torch 2.12.1+cu130 successfully [S10].

```python
import torch
from transformers import AutoModel, AutoTokenizer

model_name = "baidu/Unlimited-OCR"
tokenizer = AutoTokenizer.from_pretrained(model_name, trust_remote_code=True)
model = AutoModel.from_pretrained(
    model_name,
    trust_remote_code=True,
    use_safetensors=True,
    torch_dtype=torch.bfloat16,
).eval().cuda()

# Single image: gundam (dynamic tiling) or base
model.infer(
    tokenizer,
    prompt="<image>document parsing.",
    image_file="page.jpg",
    output_path="out/single",
    base_size=1024,
    image_size=640,
    crop_mode=True,
    max_length=32768,
    no_repeat_ngram_size=35,
    ngram_window=128,
    save_results=True,
)

# Multi-page / PDF: base mode only
model.infer_multi(
    tokenizer,
    prompt="<image>Multi page parsing.",
    image_files=["page1.png", "page2.png", "page3.png"],
    output_path="out/multi",
    image_size=1024,
    max_length=32768,
    no_repeat_ngram_size=35,
    ngram_window=1024,
    save_results=True,
)
```

Notes:

- `infer` supports gundam (`base_size=1024`, `image_size=640`, `crop_mode=True`) and
  base (`image_size=1024`, `crop_mode=False`).
- `infer_multi` does **not** support crop mode; all images are resized to
  `image_size` (1024 for quality) and concatenated at one `<image>` position.
- PDF input is not native: render pages to images first (PyMuPDF at 300 DPI in the
  model card example).
- `save_results=True` writes `result.md` and `result_with_boxes_*.jpg`; the code
  splits pages on `<PAGE>` and parses `<|ref|>/<|det|>` markers.
- The model card pins `torch==2.10.0`, `transformers==4.57.1`, `torchvision==0.25.0`,
  `pymupdf`, `einops`, `addict`, `easydict`, `psutil`. Treat these as a starting
  point, not a universal compatibility matrix.

### 9.2 vLLM (v1)

Official recipe: <https://recipes.vllm.ai/baidu/Unlimited-OCR> [S9].

```bash
docker run --rm --gpus all --network host --ipc host \
  vllm/vllm-openai:unlimited-ocr \
  baidu/Unlimited-OCR \
  --trust-remote-code \
  --logits_processors vllm.model_executor.models.unlimited_ocr:NGramPerReqLogitsProcessor \
  --no-enable-prefix-caching \
  --mm-processor-cache-gb 0
```

Required request properties:

- Prompt must begin with a literal `<image>` token (e.g.
  `<image>Multi page parsing.`). There is no chat template.
- `skip_special_tokens: False`.
- `ngram_size=35`, `window_size=128` for single image, `1024` for multi-page/PDF.
- Use `temperature=0` for deterministic parsing.
- The vLLM recipe says a single GPU with **≥8 GB VRAM** is enough for BF16; practical
  long-prefill use needs more (see §7.2).

Docker tags: `vllm/vllm-openai:unlimited-ocr` (CUDA 13.0) and
`vllm/vllm-openai:unlimited-ocr-cu129` (Hopper/CUDA 12.9). The vLLM image publishes
arm64 and amd64 manifests; validate SM121 before relying on it.

### 9.3 SGLang (v1)

The model card ships a custom wheel
`wheel/sglang-0.0.0.dev11416+g92e8bb79e-py3-none-any.whl` and pins
`kernels==0.11.7` in the command block (the prose mentions 0.9.0; use the command
block and verify) [S2, S13].

```bash
uv venv --python 3.12
source .venv/bin/activate
uv pip install wheel/sglang-0.0.0.dev11416+g92e8bb79e-py3-none-any.whl
uv pip install kernels==0.11.7
uv pip install pymupdf==1.27.2.2

python -m sglang.launch_server \
  --model baidu/Unlimited-OCR \
  --served-model-name Unlimited-OCR \
  --attention-backend fa3 \
  --page-size 1 \
  --mem-fraction-static 0.8 \
  --context-length 32768 \
  --enable-custom-logit-processor \
  --disable-overlap-schedule \
  --skip-server-warmup \
  --host 0.0.0.0 \
  --port 10000
```

The OpenAI-compatible request adds `images_config.image_mode` (`gundam` or `base`)
and a custom logit processor with `ngram_size=35`, `window_size=128` (single) or
`1024` (multi-page). For long documents, stream the response and set a generous
timeout (the model card example uses 1,200 s).

**Do not plan on SGLang for this host without a kernel spike.** Issue #32 documents
the SM121 failure and the Transformers+ngram workaround [S10].

### 9.4 Transformers (v2)

v2's Transformers path is native (no custom wheel needed for the model itself) and
accepts a list of page images [S4]:

```python
model_name = "baidu/Unlimited-OCR-2"
tokenizer = AutoTokenizer.from_pretrained(model_name, trust_remote_code=True)
model = AutoModel.from_pretrained(
    model_name, trust_remote_code=True, use_safetensors=True,
    torch_dtype=torch.bfloat16,
).eval().cuda()

pages = [f"doc/page_{i}.jpg" for i in range(1, 8)]
text, num_tokens = model.infer_multi(
    tokenizer,
    image_files=pages,
    output_path="out/multi",
    save_results=True,
)
```

The prompt is built automatically as `"<PAGE><image>" * n_pages + "Multi page parsing."`.
`infer` defaults to `max_length=131072`, `page_count_guard=True`,
`max_page_tokens=20000`, `dash_topm=16`. For single-page dynamic tiling, use
`crop_mode=True`; for multi-page, `crop_mode=False` (base mode).

v2's model card pins Python 3.10, `torch==2.13.0`, `transformers==4.52.1`,
`huggingface-hub==0.36.0`, `tokenizers==0.21.4`. The code imports sparse/DASH Triton
modules; validate them on aarch64/SM121 before production.

### 9.5 SGLang (v2)

The v2 model card provides a custom wheel
`sglang-0.5.20.dev10+g8d62f5fb9c-py3-none-any.whl` and a long server command with
page-window scheduling, fastokens, CUDA graphs, and memory knobs [S4]. The Hugging
Face file listing at research time instead contained
`sglang-0.5.20.dev13+g75b9b06b02-py3-none-any.whl`, so the card text and the shipped
wheel can drift; pin the exact file you install and verify its checksum. Important
environment variables include:

| Variable | Meaning | Suggested starting point |
| --- | --- | --- |
| `SGLANG_UOCR_EANABLE_DOC_LIMIT` | Enable page-window scheduling (spelling is from the model card) | `1` |
| `SGLANG_UOCR_DOC_GUNDAM_PAGE_WINDOW` | Pages per window, gundam mode | `10` |
| `SGLANG_UOCR_DOC_BASE_PAGE_WINDOW` | Pages per window, base mode | `10` |
| `SGLANG_UOCR_MAX_PAGE_TOKENS` | Per-page output cap | `20000` |
| `SGLANG_OPT_UOCR_SAM_REL_POS_BIAS_MB` | SAM attention-bias chunk cap; default can allocate ~462 MiB and OOM | `64` |
| `SGLANG_UOCR_SHM_WRAP_MODEL_DATA` | Pass large crop tensors through `/dev/shm` | `1` |
| `SGLANG_UOCR_SHM_WRAP_MIN_MB` | Minimum size for SHM wrapping | `1` |
| `SGLANG_VLM_CACHE_SIZE_MB` | VLM cache size | `0` |
| `PYTORCH_CUDA_ALLOC_CONF` | Reduce fragmentation | `expandable_segments:True` |

The v2 OpenAI request uses `max_tokens = tokens_per_page * len(pages)` (the global
document budget must be positive), `images_config.crop_mode`, and custom params
`page_token_id=128827`, `eos_token_id=1`, `max_page_tokens=20000`. The final chunk may
carry usage only; the client must tolerate an empty `choices` array.

### 9.6 Serving recommendation for LokLingo

| Option | Pros | Cons | Verdict |
| --- | --- | --- | --- |
| Transformers wrapper (v1) | Known to work on GB10; simplest model code; no custom kernel | No continuous batching; slower; we own the server loop | **Phase 1 default** |
| vLLM (v1) | OpenAI API, batching, official image | SM121 kernel support unverified; must disable prefix caching | **Spike candidate** |
| SGLang (v1) | Designed for this model; page scheduling | SM121 blocked; custom wheel; kernel risk | **Do not use initially** |
| v2 Transformers | 131K, long docs, native path | Sparse/DASH Triton on SM121 unverified; slower | **Later evaluation** |
| v2 SGLang | Best hundred-page throughput | Custom wheel, complex memory knobs, SM121 risk | **Later, after upstream support** |

---

## 10. Hardware and environment requirements

### 10.1 Model sizing

| Item | v1 | v2 |
| --- | --- | --- |
| BF16 weights | ~6.67 GB (3.336B params) | ~6.7 GB (3.34B params) |
| Visual tokens/page | ~256 | ~273 |
| Max context | 32,768 | 131,072 |
| Practical VRAM | 12 GB min, 24 GB comfortable for BF16 long prefill | More for full context; page-window decoding reduces peak KV |
| Disk cache | ~7 GB weights + tokenizer/assets + wheel; budget 20 GB | Similar |

The vLLM recipe's "≥8 GB" is a short/single-page floor. Independent KV-cache math
for v1: ~60 KiB/token × 32,768 ≈ 1.875 GiB for a full prefill, plus weights and
activations [S13]. Long multi-page prefill is the memory driver, not decode.

### 10.2 This host (verified 2026-11-03)

- GPU: **NVIDIA GB10**, compute capability **SM121**, aarch64.
- Driver 580.178.04, CUDA 13.0.
- Unified memory: ~121 GiB total, ~70 GiB available at research time.
- Docker 29.6.2 with NVIDIA runtime; `docker run --gpus all nvidia/cuda:13.0.0-base-ubuntu24.04 nvidia-smi -L`
  returns `GPU 0: NVIDIA GB10`, so GPU passthrough works.
- Existing LokLingo OCR container is CPU-only (`paddlepaddle`, not
  `paddlepaddle-gpu`) and runs as a Python 3.11 slim image. The GPU is currently
  available for a sidecar.
- **Critical:** SGLang's prebuilt `sgl_kernel` matrix lacks SM121; issue #32 reports
  `dlopen` failure and validates Transformers + ngram as the workaround [S10].
- vLLM's official image has an arm64 manifest, but SM121 kernel support is not
  guaranteed; smoke-test before adoption.
- Model download requires Hugging Face access (public, no token). Pre-fetch for
  offline operation and pin revisions.

### 10.3 Software versions

| Path | Tested/pinned | Notes |
| --- | --- | --- |
| v1 Transformers | Python 3.12.3, torch 2.10.0, transformers 4.57.1, CUDA 12.9 | GB10 community success with torch 2.12.1+cu130, transformers 4.57.1 |
| v1 vLLM | vLLM 0.25.0+, dedicated image | `--trust-remote-code`, logits processor, no prefix cache |
| v1 SGLang | custom wheel `0.0.0.dev11416`, kernels 0.11.7 | SM121 blocked |
| v2 Transformers | Python 3.10, torch 2.13.0, transformers 4.52.1 | Sparse/DASH Triton validation needed |
| v2 SGLang | custom wheel `0.5.20.dev10` (card) / `dev13` (repo listing) | SM121 risk; complex memory knobs; verify wheel SHA |

Version conflicts are real: v1 and v2 pin different torch/transformers versions.
Do not put both in one Python environment. Use separate containers or a single
chosen version per deployment.

---

## 11. Fit for LokLingo

### 11.1 Current OCR architecture (baseline)

- **Frontend** → backend/OCR service for synchronous image OCR; async jobs for PDFs
  and images.
- **Go backend** (`backend/internal/services/ocr_provider_chain.go`) has a pluggable
  `OCRProvider` chain: `paddle`, `tesseract`, `ollama`. Today Paddle and Tesseract
  point at the same OCR service and send no engine selector, so "fallback" is
  effectively a retry, not real engine diversity.
- **OCR service** (`ocr-service/app.py`) is a FastAPI/PaddleOCR CPU service with
  preprocessing, layout/reading order, optional Surya, VLM fallback via Ollama, LLM
  correction, and block/bbox output.
- **PDF jobs** first extract embedded text natively; if empty, they call OCR. Image
  jobs always OCR, then translate blocks and render an overlay.
- **Confidence** is used for low-confidence warnings; OCR service returns per-block
  confidence.
- **Observability** records provider selection, failures, fallbacks, latency,
  confidence, retries, and fallback-budget exhaustion.

### 11.2 Where Unlimited OCR adds value

1. **Document structure.** Tables, formulas, reading order, and headings are weak in
   pure line-based Paddle output. Unlimited OCR returns structured Markdown and
   bounding boxes, which can improve PDF extraction and future layout rendering.
2. **Multi-page context.** One-shot parsing can preserve cross-page tables, numbered
   clauses, and running headers better than page-by-page OCR.
3. **Scanned PDFs.** For image-only PDFs, a 3B document parser may outperform a
   line-level OCR pipeline on complex layouts.
4. **Local/private.** No per-page API cost; documents stay on the LAN host.
5. **Bounding boxes.** The raw output includes boxes, enabling image overlay
   translation and visualization without a separate detector.

### 11.3 Where it does not fit (yet)

1. **Default multilingual OCR.** No declared language coverage; Arabic/Indic/Bengali
   quality is unknown. Keep Paddle/Surya/VLM fallback for those.
2. **Low-latency single-image OCR.** A 3B VLM is heavier than PaddleOCR CPU for a
   quick screenshot; the current sync path has a 30 s frontend timeout.
3. **Confidence-based quality gates.** No calibrated confidence; do not fabricate one.
4. **Exact text fidelity.** VLM OCR can normalize, omit, or hallucinate; critical
   documents need verification.
5. **Replacing the existing provider chain wholesale.** The current chain has no real
   engine selection; add it deliberately rather than layering another fake fallback.

### 11.4 Recommended integration shape

**Separate inference sidecar + existing OCR service as adapter/normalizer.**

```
Frontend / Go backend
        │
        ▼
OCR service (FastAPI, CPU)  ── engine=paddle ──► PaddleOCR (existing)
        │
        └── engine=unlimited ──► Unlimited OCR sidecar (GPU, OpenAI-compatible
                                  or thin Transformers server)
```

Why:

- Keeps PaddleOCR and PyTorch/CUDA dependency stacks isolated.
- Lets the sidecar restart/upgrade independently; model load does not block the OCR
  service.
- Keeps PDF rendering, block normalization, bbox scaling, and response schema in one
  place (the OCR service), so the Go backend and frontend contracts stay stable.
- Enables real fallback: OCR service can fall back to Paddle on UOCR failure; backend
  can fall back at the HTTP level if the OCR service itself fails.
- Avoids a Go reimplementation of `<PAGE>`/`<|det|>` parsing and PDF rendering.

The detailed architecture, data flow, phases, and task backlog are in
[`../plans/unlimited-ocr-integration.md`](../plans/unlimited-ocr-integration.md).

---

## 12. Validation checklist before production

These are the questions the implementation plan must answer with measurements, not
assumptions:

- [ ] Does the chosen v1 serving path (Transformers or vLLM) work on GB10/SM121?
- [ ] What are cold-start time, steady-state VRAM, and TPS for 1, 10, 20, 40, and
      100-page documents on this host?
- [ ] Does the model emit `<PAGE>` separators reliably for multi-page requests, and
      are bboxes in the expected 0–999 coordinate space?
- [ ] How does quality compare with PaddleOCR on our corpus for each target language
      (CER/WER, table TEDS, reading order, heading hierarchy)?
- [ ] Does quality hold at 40+ pages, or does repetition/truncation appear?
- [ ] What page-window size and overlap maximize quality/latency for long PDFs?
- [ ] How should confidence be represented when the model provides none?
- [ ] Does UOCR output survive the translation pipeline without leaking markup or
      breaking tables?
- [ ] Does fallback to Paddle work when the sidecar is killed, times out, returns
      malformed output, or OOMs?
- [ ] Can the sidecar run without a public port, as non-root, with pinned model code
      and a reviewed license notice?
- [ ] Is the GPU shared safely with Ollama/LiteLLM, and what is the eviction policy?
- [ ] Is v2 (Transformers or SGLang) viable on SM121, and does it materially improve
      our 100+ page documents over v1 chunking?

---

## 13. References

1. **S1** — Baidu Inc., *Unlimited OCR Works: Welcome the Era of One-shot
   Long-horizon Parsing*, arXiv:2606.23050, 22 Jun 2026.
   <https://arxiv.org/pdf/2606.23050>
2. **S2** — Hugging Face, `baidu/Unlimited-OCR` model card.
   <https://huggingface.co/baidu/Unlimited-OCR>
3. **S3** — GitHub, `baidu/Unlimited-OCR` repository and MIT LICENSE.
   <https://github.com/baidu/Unlimited-OCR>
4. **S4** — Hugging Face, `baidu/Unlimited-OCR-2` model card.
   <https://huggingface.co/baidu/Unlimited-OCR-2>
5. **S5** — Baidu ERNIE Team, *Unlimited OCR Works 2: Pioneering the OCR-3.0 Era:
   From Long- to Unlimited-Horizon Parsing* (technical report PDF).
   <https://huggingface.co/baidu/Unlimited-OCR-2/blob/main/Unlimited-OCR-2.pdf>
6. **S6** — MarkTechPost, *Baidu Open-Sources Unlimited OCR 2 for Hundred-Page
   Document Parsing*, 26 Oct 2026.
   <https://www.marktechpost.com/2026/10/26/baidu-unlimited-ocr-2-open-source-long-document-ocr/>
7. **S7** — Baidu Baike, *Unlimited OCR*.
   <https://baike.baidu.com/en/item/Unlimited%20OCR/2567993>
8. **S8** — The Menon Lab, *Unlimited-OCR: Baidu's Free Open-Source Model Parses
   Entire PDFs in One Shot*.
   <https://themenonlab.blog/blog/unlimited-ocr-baidu-free-pdf-parsing>
9. **S9** — vLLM Recipes, `baidu/Unlimited-OCR`.
   <https://recipes.vllm.ai/baidu/Unlimited-OCR>
10. **S10** — GitHub issue #32, *[GB10 / ARM64] SGLang blocked on DGX Spark (SM121)
    — please document Transformers+ngram workaround*.
    <https://github.com/baidu/Unlimited-OCR/issues/32>
11. **S11** — GitHub issue #27, *High-level deployment blockers for community
    adoption*.
    <https://github.com/baidu/Unlimited-OCR/issues/27>
12. **S12** — Digital Applied, *Baidu Unlimited-OCR 3B: Single-Pass PDFs,
    MIT-Licensed*, 23 Jul 2026.
    <https://www.digitalapplied.com/blog/baidu-unlimited-ocr-3b-document-automation-2026>
13. **S13** — AIReiter, *Unlimited OCR: What Baidu's 3B Model Can and Can't Do*,
    last updated 2026-07-28.
    <https://aireiter.com/blog/unlimited-ocr-baidu-api-review>
14. **S14** — Hugging Face model API metadata, `baidu/Unlimited-OCR`
    (`license: mit`, file list). Verified 2026-11-03.
15. **S15** — Hugging Face configs:
    <https://huggingface.co/baidu/Unlimited-OCR/raw/main/config.json> and
    <https://huggingface.co/baidu/Unlimited-OCR-2/raw/main/config.json>.
    Verified 2026-11-03.
16. **S16** — Model code `modeling_unlimitedocr.py` (v1 and v2), Hugging Face
    repositories. Verified 2026-11-03.
17. **Project context** — `guide/ocr-fine-tuning-workflow.md`,
    `guide/API.md`, `backend/internal/services/ocr_*`, `ocr-service/app.py`,
    `docker-compose.yml`.
