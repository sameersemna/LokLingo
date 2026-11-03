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

## Required before production

- [x] Pin `OCR_UNLIMITED_MODEL_REVISION` to a verified commit (`8249d20b...`).
- [x] Pin the sidecar base image digest.
- [ ] Review the vendored `modeling_unlimitedocr.py`/DeepEncoder code at the pinned
      revision. (Revision is pinned; manual code review still pending.)
- [x] Add `THIRD_PARTY_NOTICES` for MIT and any vendored DeepSeek/PaddleOCR components.
- [ ] Run an image vulnerability scan and record results.
- [ ] Confirm the deployment host has no public route to port `10000`.
- [ ] Confirm the model cache volume has restrictive host permissions.
