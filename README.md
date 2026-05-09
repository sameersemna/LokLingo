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

LokLingo can persist OCR and LiteLLM reliability events into Postgres for
time-windowed operations dashboards.

### 1) Apply analytics migrations

If `POSTGRES_DSN` is configured for backend analytics logging, apply SQL
migrations in order:

```bash
for f in backend/migrations/*.sql; do
    psql "$POSTGRES_DSN" -f "$f"
done
```

The new reliability telemetry table is created by:

* `backend/migrations/003_create_reliability_events.sql`
* `backend/migrations/004_add_reliability_retention_policy.sql`

Retention cleanup helper:

```sql
SELECT prune_reliability_events(INTERVAL '30 days');
```

Recommended: run that SQL daily from an external scheduler (system cron,
Kubernetes CronJob, CI maintenance job, etc.).

### 2) Query API-level metrics

The internal metrics endpoint now returns both:

* `reliability`: in-process counters since backend start
* `reliability_windowed.events`: Postgres-aggregated reliability events for the requested window

Example:

```bash
curl "http://localhost:28080/api/v1/metrics/ocr?window=24h" \
    -H "X-Internal-Token: $INTERNAL_TOKEN"
```

Accepted windows: `1h`, `6h`, `24h`, `7d`, `30d`.

### 3) Query dashboard SQL directly

Use:

* `backend/analytics/ocr_dashboard.sql`

This file now includes reliability-focused queries such as top failure reasons,
retry/circuit event volumes, and integration/event trends over time.

### 4) Frontend reliability severity thresholds

The readiness popover mini-widget and sparkline support configurable severity
thresholds through Vite environment variables:

* `VITE_RELIABILITY_PRESSURE_WARN`: warn threshold for pressure score (default `8`)
* `VITE_RELIABILITY_PRESSURE_CRITICAL`: critical threshold for pressure score (default `20`)

Pressure score is computed from the latest 1h metrics as:

```text
litellm.retry_attempts_total
+ litellm.circuit_opened_total
+ ocr.retry_attempts_total
+ ocr.response_rejected_total
```

Set these in the frontend environment used at build time to tune alert
sensitivity per environment (dev/staging/prod).

---

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
