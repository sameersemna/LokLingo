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
* `MAX_PDF_UPLOAD_BYTES`: max accepted PDF upload size for backend and OCR guards, defaults to `104857600` (100 MB)
* `MAX_PDF_PAGES`: max accepted PDF page count for backend and OCR guards, defaults to `200`

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
