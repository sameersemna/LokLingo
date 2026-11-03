# Step 6 — Deployment And Configuration

## Delivered

- `docker-compose.yml` optional `ocr-vlm` profile with GPU reservation, model cache, internal
  network only, and health check.
- `docker-compose.prod.yml` hardening override.
- `uocr_models` volume.
- Backend and OCR service environment variables in `.env.example`.
- `OCR_UNLIMITED_OFFLINE=true` uses the cached model without Hugging Face DNS/network
  access; set it to `false` only for the first download.
- README and API documentation updates.

## Validated

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml config
docker compose -f docker-compose.yml -f docker-compose.dev.yml --profile ocr-vlm config
WRITE_API_TOKEN=... INTERNAL_TOKEN=... \
  docker compose -f docker-compose.yml -f docker-compose.prod.yml --profile production config
```

All three render successfully. The `ocr-vlm` compose profile was launched against the
built image; the container reached Docker `healthy` status and reported the pinned model
revision.

## Pending

- Canary/rollback rehearsal.
- Production `.env` confirmation of the pinned revision (the default is already pinned in
  `.env.example` and compose).
