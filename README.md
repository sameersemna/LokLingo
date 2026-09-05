# LokLingo

A fully self-hosted, high-performance multilingual translation platform designed as a local-first alternative to DeepL.

## ✨ Features

* 🌍 Multilingual translation (EN, DE, FR, HI, UR, AR, BN)
* 🖼️ Image OCR extraction
* ⚙️ Environment-driven configuration (`APP_ENV`)
* ⚡ Unlimited usage (local deployment)
* 🔌 API-first architecture
* 🧠 Model routing via LiteLLM

---

## 🏗️ Architecture

```mermaid
flowchart TD
    FE[React Frontend] --> BE[Go Fiber API]

    BE --> LLM[LiteLLM Gateway (latitude)]
    BE --> OCR[OCR Service (PaddleOCR)]
    BE --> REDIS[(Redis)]

    OCR --> BE
    LLM --> BE
```

---

## 🧱 Tech Stack

| Layer       | Tech         |
| ----------- | ------------ |
| Frontend    | Vite + React |
| Backend     | Go (Fiber)   |
| OCR         | PaddleOCR    |
| LLM Gateway | LiteLLM      |
| Queue       | Redis        |

---

## 🚀 Development Setup

1. Copy the development environment file:

```bash
cp .env.development .env
```

2. Start the app with the development overlay:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
```

3. Open the running services:

```text
Frontend: http://localhost:3000
Backend health: http://localhost:28080/health
Backend readiness: http://localhost:28080/ready
OCR health: http://localhost:8000/health
```

4. Stop the stack when finished:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml down
```

## 🌐 LAN Access Setup

LokLingo is configured for LAN access using hostname `promaxgb10-6116` and these endpoints:

```text
Frontend: http://promaxgb10-6116:3000
Backend: http://promaxgb10-6116:28080
OCR: http://promaxgb10-6116:8000
```

Use the validation helper after bringing the stack up:

```bash
bash scripts/check-lan-access.sh
```

Firewall notes:

* Ensure inbound TCP ports `3000`, `28080`, and `8000` are allowed on the host.
* On Linux hosts with `ufw`, allow access with:

```bash
sudo ufw allow 3000/tcp
sudo ufw allow 28080/tcp
sudo ufw allow 8000/tcp
```

Hostname troubleshooting:

* Verify hostname resolution from another device:

```bash
ping promaxgb10-6116
```

* If hostname lookup fails, use the host LAN IP shown by `scripts/check-lan-access.sh`.
* If DNS/mDNS is restricted on your network, add a local hosts entry on test devices:

```text
<LAN_IP> promaxgb10-6116
```

Android testing instructions:

* Connect Android device to the same WiFi network as the host machine.
* Open Chrome and navigate to `http://promaxgb10-6116:3000`.
* If hostname does not resolve on Android, open `http://<LAN_IP>:3000` instead.
* Confirm backend and OCR health pages load:
    * `http://promaxgb10-6116:28080/health`
    * `http://promaxgb10-6116:8000/health`

## 🧭 LAN Operations

Recommended production-style LAN workflow:

```bash
bash scripts/start-lan.sh
bash scripts/check-health.sh
bash scripts/monitor-lan.sh
bash scripts/stop-lan.sh
```

The stack is started with the production compose overlay:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml --profile production up -d --build
```

The centralized health dashboard is exposed at:

```text
http://promaxgb10-6116:28080/api/v1/health/dashboard
```

Recovery docs:

- [OCR crash](guide/recovery/ocr-crash.md)
- [Ollama unavailable](guide/recovery/ollama-unavailable.md)
- [Queue corruption](guide/recovery/queue-corruption.md)
- [Stuck jobs](guide/recovery/stuck-jobs.md)
- [Security hardening](guide/security-hardening.md)

Self-heal / backup:

```bash
bash scripts/self-heal.sh              # one-shot recovery
bash scripts/self-heal.sh --watch      # continuous healing loop
bash scripts/backup-restore.sh backup  # Redis/Postgres/uploads snapshot
```

Playwright E2E (stack must be up):

```bash
cd e2e && npm install && npx playwright install chromium
npx playwright test
```

## ⚙️ Configuration

LokLingo reads all runtime settings from environment variables.

Important variables:

* `APP_ENV`: active environment, defaults to `development`
* `REDIS_URL`: Redis connection string used by the backend
* `LITELLM_BASE_URL`: LiteLLM base URL
* `LITELLM_API_KEY`: LiteLLM API key
* `LITELLM_MODEL`: model name available on the configured LiteLLM host
* `BACKEND_HOST_PORT`: host port mapped to backend container port `8080`
* `FRONTEND_HOST_PORT`: host port mapped to frontend container port `80`
* `OCR_HOST_PORT`: host port mapped to OCR container port `8000`
* `OCR_PROVIDER`: primary OCR engine for backend fallback chain (`paddle`, `tesseract`, `ollama`), defaults to `paddle`
* `OCR_PROVIDER_TIMEOUT_SECONDS`: timeout in seconds for each provider attempt before fallback, defaults to `120`
* `OCR_MAX_FALLBACKS`: max number of provider failovers allowed per OCR request, defaults to `2`
* `OCR_SHARED_STORAGE_DIR`: shared directory mounted into backend and OCR containers for zero-copy OCR requests, defaults to `/tmp/loklingo`
* `OCR_OLLAMA_BASE_URL`: OCR service Ollama endpoint for correction (`http://loklingo-ollama:11434` in compose)
* `OCR_CORRECTION_ENABLED`: enables LLM post-correction for low-confidence OCR segments (default `false`)
* `OCR_CORRECTION_MODEL`: correction model name, default `Keyvan/german-ocr`
* `OCR_CORRECTION_TIMEOUT`: per-correction call timeout in seconds (default `8.0`)
* `OCR_CORRECTION_RETRIES`: retry attempts for correction calls (default `1`)
* `OCR_CORRECTION_LOW_CONFIDENCE_THRESHOLD`: confidence threshold used to select candidate blocks for correction (default `0.78`)
* `OCR_CORRECTION_MAX_CHUNK_BLOCKS`: max OCR blocks per correction chunk (default `24`)
* `OCR_CORRECTION_LANGS`: comma-separated ISO 639-1 codes eligible for LLM post-correction; `auto` covers requests without a language hint (default `de,auto`)
* `OCR_CORRECTION_MIN_SIMILARITY`: minimum character-level similarity ratio between an OCR line and its LLM correction; lower-similarity lines are rejected as likely hallucinations (default `0.55`)
* `OCR_VLM_FALLBACK_ENABLED`: re-OCR low-confidence pages with a local vision-language model via Ollama (default `false`)
* `OCR_VLM_MODEL`: VLM used for the fallback (default `qwen2.5-vl`)
* `OCR_VLM_LANGS`: languages eligible for VLM fallback (default `ar,ur,fa,he`)
* `OCR_VLM_CONFIDENCE_THRESHOLD`: page-level PaddleOCR confidence below which the VLM fallback triggers (default `0.72`)
* `OCR_VLM_TIMEOUT`: per-call VLM timeout in seconds (default `60`)
* `WRITE_API_TOKEN`: protects write routes (`POST /api/v1/translate*`, `POST /api/v1/jobs*`); required in `production`, optional in `development`
* `GLOBAL_RATE_LIMIT_PER_MINUTE`: per-IP request cap for non-health routes, defaults to `120`
* `WRITE_RATE_LIMIT_PER_MINUTE`: per-IP cap for authenticated write requests, defaults to `40`
* `UPLOAD_RATE_LIMIT_PER_MINUTE`: stricter per-IP cap for upload-heavy routes (`/translate/image`, `/jobs/pdf`, `/jobs/image`), defaults to `12`
* `SYNC_IMAGE_MAX_INFLIGHT`: max concurrent in-flight sync image translations, defaults to `8`
* `MAX_PDF_UPLOAD_BYTES`: max accepted PDF upload size for backend and OCR guards, defaults to `104857600` (100 MB)
* `MAX_PDF_PAGES`: max accepted PDF page count for backend and OCR guards, defaults to `200`
* `TRANSLATE_CONCURRENCY`: max concurrent LLM translation requests in the worker, defaults to `3`
* `TRANSLATE_CHUNK_MIN_WORDS`: preferred minimum words per PDF translation chunk, defaults to `500`
* `TRANSLATE_CHUNK_MAX_WORDS`: hard cap words per PDF translation chunk, defaults to `1000`

Environment files:

* `.env.development`: local development defaults, including non-conflicting host ports and a working LiteLLM model for the `latitude` host
* `.env.production`: production-oriented defaults; does not start Redis locally
* `.env.example`: template for custom environments

## 🐳 Compose Layout

Base compose file:

* `docker-compose.yml` runs backend, frontend, and OCR
* Redis is not included in the base file

Development overlay:

* `docker-compose.dev.yml` pins LAN-facing host ports: frontend `3000`, backend `28080`, OCR `8000`
* It forces backend-to-OCR traffic through the internal Docker DNS name `http://loklingo-ocr:8000`

Observability overlay:

* `docker-compose.observability.yml` adds Prometheus and Grafana with auto-provisioned datasource and dashboards
* It scrapes backend metrics through a token-aware metrics proxy that injects `X-Internal-Token` and loads dashboards from `guide/governance/dashboards`
* The metrics proxy is exposed on `${METRICS_PROXY_HOST_PORT:-18081}` for smoke validation and local troubleshooting

Development command:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d
```

Production command:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml --profile production up -d
```

Development with observability:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.observability.yml up -d --build
```

Observability endpoints:

```text
Prometheus: http://localhost:19090
Grafana: http://localhost:13030
Metrics proxy: http://localhost:18081
```

Bundled smoke test:

```bash
sh guide/smoke-observability.sh
```

In production, `REDIS_URL` must point to an external Redis instance.

## 🤖 Local OCR Correction With Ollama

OCR correction is local-only and designed to improve scanned document quality by correcting only low-confidence OCR regions. It ships with a German fine-tuned model by default; additional languages can be enabled via `OCR_CORRECTION_LANGS` with a suitable per-language model. Corrected lines that diverge too far from the raw OCR output are rejected automatically (`OCR_CORRECTION_MIN_SIMILARITY`) to guard against LLM hallucination. Arabic-script languages (`ar`, `ur`, `fa`, `he`) use right-to-left reading order automatically.

Pull required local models:

```bash
docker compose exec loklingo-ollama ollama pull Keyvan/german-ocr
docker compose exec loklingo-ollama ollama pull Keyvan/german-ocr-3.1
docker compose exec loklingo-ollama ollama pull qwen2.5-vl
docker compose exec loklingo-ollama ollama pull minicpm-v
```

Enable correction in `.env`:

```bash
OCR_CORRECTION_ENABLED=true
OCR_CORRECTION_MODEL=Keyvan/german-ocr
OCR_OLLAMA_BASE_URL=http://loklingo-ollama:11434
```

Quick model availability check:

```bash
docker compose exec loklingo-ollama ollama list
curl http://localhost:8000/health
```

The OCR health payload includes correction status, selected model, and cumulative correction metrics. If Ollama is unavailable or the model is missing, OCR falls back to raw PaddleOCR output without failing the request.

### VLM OCR fallback for Arabic-script documents

PaddleOCR's Arabic recognition is weak (KITAB-Bench, ACL 2025: ~0.79 CER vs. ~0.46 for Qwen2.5-VL). When `OCR_VLM_FALLBACK_ENABLED=true`, pages in `OCR_VLM_LANGS` whose PaddleOCR confidence stays below `OCR_VLM_CONFIDENCE_THRESHOLD` after all preprocessing variants are re-OCR'd with the local VLM (`qwen2.5-vl` by default — already pulled above). The VLM returns plain text without geometry, so blocks get full-width pseudo-bboxes and fixed confidence `0.85`; RTL reading order is applied automatically. Any VLM failure (Ollama down, model missing, timeout, empty response) degrades gracefully to the best PaddleOCR result.

```bash
OCR_VLM_FALLBACK_ENABLED=true
OCR_VLM_MODEL=qwen2.5-vl
```

## ✅ Smoke Test

Translate through the frontend edge:

```bash
curl http://localhost:13000/translate \
    -H 'X-API-Token: $WRITE_API_TOKEN' \
    -H 'Content-Type: application/json' \
    --data-raw '{"text":"hello world","source":"en","target":"de"}'
```

Expected response:

```json
{"translated_text":"Hallo Welt","source":"en","target":"de"}
```

Or run the bundled smoke test:

```bash
sh guide/smoke-dev.sh
```

OCR PDF memory check (page-by-page processing):

```bash
# run inside the OCR container after compose is up
docker compose exec loklingo-ocr python bench_pdf_memory.py /tmp/sample.pdf --dpi 200 --lang auto
```

This script reports pages processed, elapsed time, and process peak RSS so large-PDF memory behavior can be tracked.

The readiness endpoint reports dependency status without sending a translation request:

```bash
curl http://localhost:28080/ready
```

Write-route authentication:

Use either:

* `X-API-Token: <WRITE_API_TOKEN>`
* `Authorization: Bearer <WRITE_API_TOKEN>`

In production, backend startup validation requires `WRITE_API_TOKEN`.

Async job failure handling:

* Failed jobs now track attempt metadata (`attempt`, `max_attempts`, `last_error`, `next_retry_at`, `dead_lettered_at`).
* Transient failures (rate limit/timeouts/network/service unavailable) are retried with bounded exponential backoff.
* Terminal failures are moved to a Redis dead-letter queue for later inspection (`loklingo:jobs:dead:queue`).
* Delayed retries are staged in Redis and promoted back to the active queue when due (`loklingo:jobs:retry:zset`).
* Operator endpoints (guarded by `X-Internal-Token`) are available for dead-letter operations:
    * `GET /api/v1/jobs/dead?limit=25`
    * `POST /api/v1/jobs/:id/replay`

## 📊 Reliability Telemetry

Operational reliability guidance has moved to a dedicated runbook:

* [guide/reliability-runbook.md](guide/reliability-runbook.md)
* [guide/governance/README.md](guide/governance/README.md)

Quick access:

* Severity matrix: [guide/governance/severity-matrix.md](guide/governance/severity-matrix.md)
* Incident templates: [guide/governance/incident-templates.md](guide/governance/incident-templates.md)
* SLO/SLA definitions: [guide/governance/slo-sla.md](guide/governance/slo-sla.md)
* Dashboard spec: [guide/governance/dashboard-spec.md](guide/governance/dashboard-spec.md)
* Observability instrumentation spec: [guide/governance/observability-reliability-spec.md](guide/governance/observability-reliability-spec.md)
* Golden-path smoke tests: [guide/governance/golden-path-smoke-tests.md](guide/governance/golden-path-smoke-tests.md)
* Lifecycle events standard: [guide/governance/lifecycle-events.md](guide/governance/lifecycle-events.md)
* On-call first response playbook: [guide/governance/oncall-first-response-playbook.md](guide/governance/oncall-first-response-playbook.md)
* Incident artifact template bundle: [guide/governance/incidents-template/README.md](guide/governance/incidents-template/README.md)
* Governance archive bootstrap script: [guide/governance/create-governance-archive-structure.sh](guide/governance/create-governance-archive-structure.sh)
* Governance archive index generator: [guide/governance/generate-governance-archive-index.sh](guide/governance/generate-governance-archive-index.sh)
* Governance current pointers updater: [guide/governance/update-governance-current-pointers.sh](guide/governance/update-governance-current-pointers.sh)
* Governance drift snapshot generator: [guide/governance/generate-governance-drift-snapshot.sh](guide/governance/generate-governance-drift-snapshot.sh)
* Governance drift summary generator: [guide/governance/generate-governance-drift-summary.sh](guide/governance/generate-governance-drift-summary.sh)
* Governance drift trend updater: [guide/governance/update-governance-drift-trend.sh](guide/governance/update-governance-drift-trend.sh)
* Governance maintenance runner: [guide/governance/run-governance-maintenance.sh](guide/governance/run-governance-maintenance.sh)
* Governance metadata check script: [guide/governance/check-governance-metadata.sh](guide/governance/check-governance-metadata.sh)
* Governance freshness check script: [guide/governance/check-governance-freshness.sh](guide/governance/check-governance-freshness.sh)
* Governance changelog discipline script: [guide/governance/check-governance-changelog-discipline.sh](guide/governance/check-governance-changelog-discipline.sh)
* Governance change log: [guide/governance/governance-change-log.md](guide/governance/governance-change-log.md)
* Governance archive layout: [guide/governance/governance-archive-layout.md](guide/governance/governance-archive-layout.md)
* Governance operating calendar: [guide/governance/governance-operating-calendar.md](guide/governance/governance-operating-calendar.md)
* Governance current pointers: [guide/governance/current-pointers.md](guide/governance/current-pointers.md)
* Governance release checklist: [guide/governance/governance-release-checklist.md](guide/governance/governance-release-checklist.md)
* Governance drift dashboard template: [guide/governance/governance-drift-dashboard-template.md](guide/governance/governance-drift-dashboard-template.md)
* Governance maintainer quickstart: [guide/governance/MAINTAINER-QUICKSTART.md](guide/governance/MAINTAINER-QUICKSTART.md)
* Monthly governance signoff template: [guide/governance/reviews/monthly-governance-signoff-template.md](guide/governance/reviews/monthly-governance-signoff-template.md)
* Quarterly governance review template: [guide/governance/reviews/quarterly-governance-review-template.md](guide/governance/reviews/quarterly-governance-review-template.md)
* Annual governance summary template: [guide/governance/reviews/annual-governance-summary-template.md](guide/governance/reviews/annual-governance-summary-template.md)
* Incident example index: [guide/governance/examples/README.md](guide/governance/examples/README.md)
* Example Sev 1 OCR outage packet: [guide/governance/examples/sev1-ocr-outage/summary.md](guide/governance/examples/sev1-ocr-outage/summary.md)
* Example Sev 2 translation failover packet: [guide/governance/examples/sev2-translation-failover/summary.md](guide/governance/examples/sev2-translation-failover/summary.md)
* Example Sev 2 queue backlog packet: [guide/governance/examples/sev2-queue-backlog/summary.md](guide/governance/examples/sev2-queue-backlog/summary.md)
* Example Sev 3 export failure packet: [guide/governance/examples/sev3-export-failure/summary.md](guide/governance/examples/sev3-export-failure/summary.md)
* On-call runbook order: [guide/reliability-runbook.md](guide/reliability-runbook.md#on-call-runbook-order-checkpoint-reliability)
* Incident handoff template: [guide/reliability-runbook.md](guide/reliability-runbook.md#incident-handoff-template-unresolved-reliability-issues)
* Monthly governance signoff checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#monthly-governance-signoff-checklist)

Governance doc validation is enforced in CI via the `governance-check` job and can be run locally with `bash guide/governance/check-governance-metadata.sh`.
Governance review freshness is enforced in CI via the `governance-freshness-check` job and can be run locally with `bash guide/governance/check-governance-freshness.sh`.
Governance changelog discipline is enforced in CI via the `governance-changelog-check` job and can be run locally with `bash guide/governance/check-governance-changelog-discipline.sh`.
Governance check outputs are uploaded in CI as artifacts (`governance-metadata-report`, `governance-freshness-report`, `governance-changelog-report`).
Weekly scheduled governance maintenance uploads archive and drift artifacts (`governance-maintenance-artifacts`).
Governance drift trend history is stored at [guide/governance/archive/drift/trend.csv](guide/governance/archive/drift/trend.csv).
Governance changes that alter operational expectations should also update [guide/governance/governance-change-log.md](guide/governance/governance-change-log.md).

## 📌 Roadmap

* [ ] Core translation API
* [ ] Frontend MVP
* [ ] PDF + OCR pipeline
* [ ] Image translation workflow
* [ ] Subtitle translation
* [ ] Mobile app (Flutter)

---

## ⚠️ Notes

* The backend refuses to start in `production` if `REDIS_URL` points to `localhost`
* The backend fails fast on missing `LITELLM_BASE_URL`, `LITELLM_MODEL`, `OCR_SERVICE_URL`, or `REDIS_URL`
* The development environment uses safe host-port defaults to avoid collisions with other local projects
* Redis may log a host `vm.overcommit_memory` warning locally; that does not block startup
