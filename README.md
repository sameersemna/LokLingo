# LokLingo

A fully self-hosted, high-performance multilingual translation platform designed as a local-first alternative to DeepL.

## ✨ Features

* 🌍 Multilingual translation (EN, DE, FR, HI, UR, AR, BN)
* 📄 PDF translation (with OCR fallback)
* 🖼️ Image translation with layout preservation
* 🎬 Subtitle (SRT) translation
* ⚡ Unlimited usage (local deployment)
* 🔌 API-first architecture
* 🧠 Model routing via LiteLLM
* 🧾 Translation memory (Qdrant)

---

## 🏗️ Architecture

```mermaid
flowchart TD
    FE[React Frontend] --> BE[Go Fiber API]

    BE --> LLM[LiteLLM Gateway (latitude)]
    BE --> OCR[OCR Service (PaddleOCR)]
    BE --> REDIS[(Redis - latitude)]
    BE --> POSTGRES[(Postgres - latitude)]
    BE --> QDRANT[(Qdrant - local)]

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
| Database    | Postgres     |
| Vector DB   | Qdrant       |

---

## 🚀 Getting Started (Planned)

1. Setup LiteLLM + models
2. Configure `.env`
3. Run Docker Compose
4. Access frontend

---

## 📌 Roadmap

* [ ] Core translation API
* [ ] Frontend MVP
* [ ] PDF + OCR pipeline
* [ ] Image translation
* [ ] Subtitle translation
* [ ] Mobile app (Flutter)

---

## ⚠️ Notes

* External services (Postgres, Redis, Neo4j) run on `latitude`
* Only app services are dockerized locally
