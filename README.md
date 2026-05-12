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
Frontend: http://localhost:13000
Backend health: http://localhost:28080/health
Backend readiness: http://localhost:28080/ready
OCR health: http://localhost:18000/health
```

4. Stop the stack when finished:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml down
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
* `OCR_SHARED_STORAGE_DIR`: shared directory mounted into backend and OCR containers for zero-copy OCR requests, defaults to `/tmp/loklingo`
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

* `docker-compose.dev.yml` adds a local Redis container
* It wires the backend to `redis://loklingo-redis:6379`

Development command:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d
```

Production command:

```bash
docker compose up -d
```

In production, `REDIS_URL` must point to an external Redis instance.

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

Quick access:

* Checkpoint counters and thresholds: [guide/reliability-runbook.md](guide/reliability-runbook.md#5-checkpoint-reliability-counters-and-thresholds)
* On-call runbook order: [guide/reliability-runbook.md](guide/reliability-runbook.md#on-call-runbook-order-checkpoint-reliability)
* Reliability ownership model: [guide/reliability-runbook.md](guide/reliability-runbook.md#reliability-ownership-model)
* Communication cadence guidance: [guide/reliability-runbook.md](guide/reliability-runbook.md#communication-cadence-guidance)
* Incident severity matrix: [guide/reliability-runbook.md](guide/reliability-runbook.md#incident-severity-matrix-response-and-escalation)
* First 60 minutes timeline: [guide/reliability-runbook.md](guide/reliability-runbook.md#first-60-minutes-incident-timeline)
* Incident handoff template: [guide/reliability-runbook.md](guide/reliability-runbook.md#incident-handoff-template-unresolved-reliability-issues)
* Incident evidence checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#incident-evidence-checklist)
* Incident artifact bundle template: [guide/reliability-runbook.md](guide/reliability-runbook.md#incident-artifact-bundle-template)
* Incident closure definition of done: [guide/reliability-runbook.md](guide/reliability-runbook.md#incident-closure-definition-of-done-reliability)
* Closure signoff checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#closure-signoff-checklist)
* Closure quality scorecard: [guide/reliability-runbook.md](guide/reliability-runbook.md#closure-quality-scorecard)
* Post-incident follow-up SLA defaults: [guide/reliability-runbook.md](guide/reliability-runbook.md#post-incident-follow-up-sla-defaults)
* Follow-up tracking template: [guide/reliability-runbook.md](guide/reliability-runbook.md#follow-up-tracking-template)
* Follow-up review cadence: [guide/reliability-runbook.md](guide/reliability-runbook.md#follow-up-review-cadence)
* Follow-up completion evidence checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#follow-up-completion-evidence-checklist)
* Follow-up status taxonomy: [guide/reliability-runbook.md](guide/reliability-runbook.md#follow-up-status-taxonomy)
* Blocked-item escalation playbook: [guide/reliability-runbook.md](guide/reliability-runbook.md#blocked-item-escalation-playbook)
* Blocked escalation exception matrix: [guide/reliability-runbook.md](guide/reliability-runbook.md#blocked-escalation-exception-matrix)
* Exception expiry sweep checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#exception-expiry-sweep-checklist)
* Weekly exception audit checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#weekly-exception-audit-checklist)
* Monthly exception trend review: [guide/reliability-runbook.md](guide/reliability-runbook.md#monthly-exception-trend-review)
* Exception reduction action plan template: [guide/reliability-runbook.md](guide/reliability-runbook.md#exception-reduction-action-plan-template)
* Action plan outcome review block: [guide/reliability-runbook.md](guide/reliability-runbook.md#action-plan-outcome-review-block)
* Monthly governance signoff checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#monthly-governance-signoff-checklist)
* Quarter-end exception governance snapshot: [guide/reliability-runbook.md](guide/reliability-runbook.md#quarter-end-exception-governance-snapshot)
* Annual exception governance summary: [guide/reliability-runbook.md](guide/reliability-runbook.md#annual-exception-governance-summary)
* Annual leadership review checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#annual-leadership-review-checklist)
* Governance archive and retrieval checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#governance-archive-and-retrieval-checklist)
* Governance artifact retention policy: [guide/reliability-runbook.md](guide/reliability-runbook.md#governance-artifact-retention-policy)
* Governance index template: [guide/reliability-runbook.md](guide/reliability-runbook.md#governance-index-template)
* Governance index maintenance cadence: [guide/reliability-runbook.md](guide/reliability-runbook.md#governance-index-maintenance-cadence)
* Governance ownership RACI: [guide/reliability-runbook.md](guide/reliability-runbook.md#governance-ownership-raci)
* Governance change log entry template: [guide/reliability-runbook.md](guide/reliability-runbook.md#governance-change-log-entry-template)
* Weekly reliability KPI scorecard: [guide/reliability-runbook.md](guide/reliability-runbook.md#weekly-reliability-kpi-scorecard)
* Reliability review meeting agenda: [guide/reliability-runbook.md](guide/reliability-runbook.md#reliability-review-meeting-agenda-template)
* Severity-to-channel communication policy: [guide/reliability-runbook.md](guide/reliability-runbook.md#severity-to-channel-communication-policy)
* Communication escalation exception policy: [guide/reliability-runbook.md](guide/reliability-runbook.md#communication-escalation-exception-policy)
* Escalation decision log template: [guide/reliability-runbook.md](guide/reliability-runbook.md#escalation-decision-log-template)
* Escalation decision quality checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#escalation-decision-quality-checklist)
* Decision outcome review block: [guide/reliability-runbook.md](guide/reliability-runbook.md#decision-outcome-review-block)
* Stability confirmation checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#stability-confirmation-checklist)
* Post-closure watchback checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#post-closure-watchback-checklist)
* Reopen readiness checklist: [guide/reliability-runbook.md](guide/reliability-runbook.md#reopen-readiness-checklist)
* New on-call quick start: [guide/reliability-runbook.md](guide/reliability-runbook.md#new-on-call-quick-start-reliability)
* Common pitfalls: [guide/reliability-runbook.md](guide/reliability-runbook.md#common-pitfalls-on-call-reliability)

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
