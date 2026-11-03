# Step 11 — Security And Supply-Chain Checklist

Status: code-level controls implemented; image/model verification pending.

## Implemented

- Sidecar has no host port in compose; only `loklingo-net`.
- Sidecar container runs as non-root (`uid 10001`).
- Production compose adds `no-new-privileges`, `cap_drop: ALL`, read-only rootfs, tmpfs
  `/tmp`, and log rotation.
- Model code is loaded with `trust_remote_code=True`; `MODEL_REVISION` is a deployment
  variable and must be pinned before production.
- PyTorch, torchvision, and Transformers versions are pinned in the Dockerfile.
- Model cache is a named volume (`uocr_models`), not committed.
- OCR output is treated as untrusted text; no code execution or prompt injection path is
  introduced by the parser.

## Model-code review

See [`step11-security-review.md`](step11-security-review.md). The pinned modeling code
contains `eval()` only in the result-saving/drawing helpers; the sidecar uses
`eval_mode=True` and never calls them. No subprocess/network operations were found in
the reviewed files. `trust_remote_code=True` remains a compensating-control surface.

## Image scan

Trivy reported HIGH findings in `transformers==4.57.1` (fixed only in 5.x) plus
`setuptools`, `wheel`, and `jaraco.context`. The Dockerfile now upgrades the build tools;
the Transformers finding remains mitigated by revision pinning, network isolation, no
`save_pretrained` on untrusted templates, and the planned v2 compatibility evaluation.

## Required before production

- [x] Pin `OCR_UNLIMITED_MODEL_REVISION` to a verified commit (`8249d20b...`).
- [x] Pin the sidecar base image digest.
- [ ] Review the vendored `modeling_unlimitedocr.py`/DeepEncoder code at the pinned
      revision. (Revision is pinned; manual code review still pending.)
- [x] Add `THIRD_PARTY_NOTICES` for MIT and any vendored DeepSeek/PaddleOCR components.
- [x] Run an image vulnerability scan and record results.
- [x] Re-run Trivy after the build-tool upgrade; residual risk is 3 HIGH findings in `transformers==4.57.1`.
- [ ] Confirm the deployment host has no public route to port `10000`.
- [ ] Confirm the model cache volume has restrictive host permissions.
