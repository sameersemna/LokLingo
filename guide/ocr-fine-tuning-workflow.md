# OCR Fine-Tuning & Evaluation Workflow

Research-grounded workflow for improving LokLingo's OCR quality on
Arabic-script and Indic languages. This is an offline (training-time) process —
the runtime service stays as-is and consumes the resulting models via Ollama.

## Background (what the research says)

- **KITAB-Bench** (ACL 2025, [arXiv:2502.14949](https://arxiv.org/abs/2502.14949)):
  VLMs beat pipeline OCR on Arabic by ~60% CER. PaddleOCR ≈ 0.79 CER,
  Qwen2.5-VL ≈ 0.46, GPT-4o ≈ 0.06 on the ArabicOCR subset.
- **QARI-OCR** ([arXiv:2506.02295](https://arxiv.org/abs/2506.02295)):
  iterative LoRA fine-tuning of Qwen2-VL-2B on *synthetic* Arabic data reached
  WER 0.160 / CER 0.061 on diacritic-rich text. Models: `NAMAA-Space/Qari-OCR-*`.
- **Baseer** ([arXiv:2509.18174](https://arxiv.org/abs/2509.18174)):
  decoder-only fine-tune of a pretrained MLLM on synthetic + real Arabic
  documents; SOTA WER 0.25 for Arabic document→Markdown.
- **Post-correction** ([arXiv:2502.01205](https://arxiv.org/html/2502.01205v1)):
  zero-shot small-LLM correction can *worsen* CER; fine-tuning on synthetic
  OCR-error pairs is the reliable fix
  ([Springer](https://link.springer.com/article/10.1007/s10032-025-00522-0)).
- **Urdu caveat**: Nastaliq does not transfer from Naskh-trained models —
  Urdu needs its own data.
- **Latin scripts (EN/DE)**: PaddleOCR's `en`/`german` models are strong on
  modern print; reserve VLM fallback for handwriting, degraded scans, and
  German Fraktur/historical print (eval data: GT4HistOCR on Zenodo).
  German correction is covered by `Keyvan/german-ocr`; for English, the
  Pleias post-OCR correction dataset (1B words, HF `Pclanglais/post-ocr-correction`)
  is the training base for a future EN corrector.
- **GGUF quantization warning**: for OCR VLMs use Q5_K_M or higher (ideally
  BF16/Q8_0) — measured Q4_K_M quantization raised DeepSeek-OCR CER from
  0.78% to 15.6%. OCR is unusually quantization-sensitive.
- **Ollama OCR VLMs**: dedicated OCR models on Ollama (`glm-ocr`,
  `maternion/Qianfan-OCR`) can be set via `OCR_VLM_MODEL` without code changes.

## Step 1 — Measure first

Build a small eval set per target language (20–50 pages is enough to start):

```
eval-data/
  ar-001.png
  ar-001.txt   # expert-verified transcription, UTF-8
  ...
```

Run the harness (inside the OCR container or a venv with service deps):

```bash
python ocr-service/eval_ocr.py eval-data --lang ar --json-out reports/ar-baseline.json
```

Record mean CER/WER per configuration you change (engine, threshold,
correction on/off). KITAB-Bench and Misraj-DocOCR samples can seed the set.

## Step 2 — Cheap wins before training

1. Enable the VLM fallback (`OCR_VLM_FALLBACK_ENABLED=true`) and re-measure.
2. Enable Surya (`OCR_SURYA_ENABLED=true`, requires `surya-ocr` + torch in the
   OCR image) and re-measure.
3. Try an existing Arabic LoRA via Ollama (Qwen2.5-VL-based fine-tunes such as
   `DIMI-Arabic-OCR-V2`; avoid Qwen2-VL GGUFs in Ollama — see ollama#14388)
   by setting `OCR_VLM_MODEL`.

Only fine-tune if none of these meet the quality bar.

## Step 3 — Synthetic-data fine-tuning (correction model)

Recipe following the Springer/QARI approach, targeting an Ollama-servable
correction model (like `Keyvan/german-ocr` but for Arabic):

1. **Collect clean text** in the target language (public-domain books,
   government forms, newspapers).
2. **Synthesize OCR errors**: character confusions common for the script
   (e.g. ب/ت/ث dot confusions for Arabic), random deletions/insertions at
   1–5% character rate, line-level noise.
3. **Render synthetic pages** (fonts, noise, skew) if training a VLM; for a
   text-only corrector, (noisy, clean) pairs suffice.
4. **Fine-tune** a small instruct model (Qwen2.5-3B class) with LoRA
   (unsloth or axolotl; r=16, 1–3 epochs).
5. **Convert to GGUF** and serve via Ollama (`ollama create`), then set
   `OCR_CORRECTION_MODEL` / `OCR_CORRECTION_LANGS` accordingly.
6. **Validate** with the eval harness; the runtime similarity guard
   (`OCR_CORRECTION_MIN_SIMILARITY`) stays on as a safety net.

## Step 4 — Governance

- Log every quality-affecting change in
  [governance-change-log.md](governance/governance-change-log.md).
- Store eval reports under `guide/reports/` and reference them from the
  change log entry.
- Re-run the harness after any OCR-related dependency upgrade.
