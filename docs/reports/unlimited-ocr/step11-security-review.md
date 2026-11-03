# Step 11 — Model-Code And Image Security Review

Status: reviewed at the pinned revision; compensating controls documented. Not a full
third-party audit.

## Model code reviewed

- Model: `baidu/Unlimited-OCR`
- Revision: `8249d20b2822a7d8a4c22c8d8c9069b5ac36bda3`
- Files:
  - `modeling_unlimitedocr.py` — sha256 `3894696d68197ad00e89f0963880645dfce49b1933a9f27e78fbc05983547a78`
  - `deepencoder.py` — sha256 `0ae2fb6d1e5ae8cf100fc32f854830acd08c821a0a1f23a94a76588c222ddcf2`
  - `conversation.py` — sha256 `ec7b6ce89bcda643de1f43269ffa66a7b2e65dc3ed30e427958f776546b4ba03`
  - `configuration_deepseek_v2.py` — sha256 `b8470dd616ba8745fce6e27b093aef73a098863cc891b2477dcf9326a36000f7`
  - `modeling_deepseekv2.py` — sha256 `74e36e6bd0ba7bc565ef76464a99baa8e6bccb710ae9c1007b54ac30b855fa4c`

## Findings

1. **`eval()` in `modeling_unlimitedocr.py`** (lines 66, 1094–1123). It parses
   model-produced coordinates/line data in the result-saving/drawing helpers. Our
   sidecar calls `infer(..., eval_mode=True)` and does not set `save_results=True`, so
   these helpers are not reached. The OCR service has its own regex parser and never
   passes model output to `eval`.
2. **`torch.load(checkpoint)` in `deepencoder.py`** is conditional on a supplied
   checkpoint argument. The current model-load path does not pass an external
   checkpoint. If enabling a custom SAM checkpoint later, load only a trusted local
   file with `weights_only=True` where supported.
3. **Unused `requests` import** in `modeling_unlimitedocr.py`; no network call found in
   the pinned modeling code.
4. **No `subprocess`, `os.system`, `exec`, or network calls** found in the reviewed
   files. `trust_remote_code=True` remains an arbitrary-code-execution surface; the
   revision pin, container isolation, non-root user, no public port, and restricted
   egress are the compensating controls.

## Container scan

`trivy image --severity HIGH,CRITICAL --ignore-unfixed` found:

- `transformers==4.57.1`: CVE-2026-4372, CVE-2026-5241, CVE-2026-9856 (HIGH).
  Fixed only in 5.x, which may break the pinned model code. Mitigations: no
  `save_pretrained` from untrusted templates, pinned model revision, isolated
  container, no untrusted model loading, and a v2-specific upgrade evaluation.
- `setuptools`, `wheel`, and `jaraco.context`: patched by the Dockerfile build-tool
  upgrade. A re-scan after the upgrade reduced the HIGH findings to **3**, all in
  `transformers==4.57.1`.

## Required follow-ups

- [ ] Re-run Trivy after the build-tool upgrade.
- [ ] Evaluate Transformers 5.x compatibility in the v2 environment before adopting.
- [ ] Keep the sidecar off the public network and restrict egress.
- [ ] Never enable `save_results=True` or `draw_bounding_boxes` on untrusted output
      without replacing the `eval()` helpers.
- [ ] Keep the model revision pinned and re-review on upgrade.
