# Language Coverage Smoke Test — Repository Sample Images

Status: smoke test only; no language allowlist change.

## Samples and results

### Arabic (`guide/render_samples/arabic_overlay.png`)

- Unlimited OCR returned Arabic-shaped text but the visible output was reversed/garbled
  relative to normal reading order.
- The current PaddleOCR build raised `No models are available for lang='arabic'`.
- Decision: **Arabic stays out of the Unlimited OCR allowlist**. Keep the existing
  Surya/VLM fallback path enabled where available, and require a real Arabic corpus before
  any change.

### Mixed CJK (`guide/render_samples/cjk_vertical.png`)

- Unlimited OCR returned Japanese, Chinese, Korean, and vertical Japanese text; 4 blocks,
  ~3.2 s.
- PaddleOCR (`lang=ch`/`japan`) returned Chinese/Japanese but omitted the Korean line in
  this sample; ~4.2 s.
- Decision: promising for zh/ja/ko, but the sample is mixed-script and vertical. Keep it
  out of the allowlist until a per-language real corpus is evaluated.

### Real-world scene (`frontend/public/samples/realworld_overlay.jpg`)

- Unlimited OCR returned a small mixed Chinese/Bengali fragment. The image is a scene,
  not a document, so it is not a valid OCR-quality benchmark.
- Decision: no routing change.

## Conclusion

The smoke test confirms the documented language risk: Unlimited OCR is promising for
CJK, not approved for Arabic, and not validated for Indic/mixed-script documents. The
allowlist remains empty.
