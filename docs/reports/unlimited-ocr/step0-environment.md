# Step 0 — Environment And Baseline

Status: complete.

## Environment

- Branch/worktree: `main`, uncommitted documentation only.
- CPU/GPU architecture: GB10 ARM64 (aarch64), CUDA 13.0, driver 580.178.04.
- Docker: 29.6.2 with NVIDIA runtime available.
- GPU passthrough: `docker run --rm --gpus all nvidia/cuda:13.0.0-base-ubuntu24.04 nvidia-smi -L` returns `GPU 0: NVIDIA GB10`.
- Unified memory: approximately 121 GiB total, 70 GiB available at research time.

## Baseline commands

- `cd backend && go test ./...`: PASS.
- `cd ocr-service && python3 -m pytest test_app.py -q`: PASS, 86 tests.
- `cd frontend && npm run build`: PASS.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`: PASS.
- `docker compose -f docker-compose.yml -f docker-compose.prod.yml --profile production config`: PASS.
- Frontend tests/lint are also expected to run before frontend changes.

## Intentionally not started

- No source files were changed in this step.
- Feature branch creation and committed work remains operator-controlled.
