# LokLingo Hardened Architecture

Last updated: 2026-07-17

## Runtime topology (LAN-first)

```mermaid
flowchart TB
  subgraph Clients
    Browser[LAN Browser / Android]
    Ops[Ops scripts / Grafana]
  end

  subgraph Edge
    FE[Frontend nginx non-root<br/>CSP · body limits · dynamic DNS]
  end

  subgraph API
    BE[Go Fiber Backend non-root<br/>recover · security headers · rate limits<br/>WRITE_API_TOKEN · BodyLimit]
    Worker[Job Worker<br/>adaptive concurrency · checkpoints]
    CB[LiteLLM Circuit Breaker]
    OCRChain[OCR Provider Chain<br/>paddle → tesseract → ollama]
  end

  subgraph Data
    Redis[(Redis Queue + DLQ)]
    PG[(Postgres analytics optional)]
    Vol[pdf_uploads volume]
  end

  subgraph AI
    LiteLLM[LiteLLM Gateway]
    Ollama[Ollama]
    OCR[OCR Service non-root<br/>PaddleOCR]
  end

  subgraph Observability
    Proxy[Metrics Proxy + INTERNAL_TOKEN]
    Prom[Prometheus + alert rules]
    Graf[Grafana dashboards]
  end

  subgraph Healing
    SelfHeal[scripts/self-heal.sh]
    Backup[scripts/backup-restore.sh]
  end

  Browser --> FE
  FE -->|/api /health /ocr| BE
  FE --> OCR
  Browser -->|optional direct| BE
  Ops --> BE
  Ops --> Prom

  BE --> Worker
  Worker --> CB
  CB --> LiteLLM
  CB --> Ollama
  Worker --> OCRChain
  OCRChain --> OCR
  OCR --> Ollama
  Worker --> Redis
  BE --> Redis
  BE --> PG
  BE --> Vol
  OCR --> Vol

  BE --> Proxy
  Proxy --> Prom
  Prom --> Graf

  SelfHeal -->|restart unhealthy| BE
  SelfHeal --> OCR
  SelfHeal --> FE
  SelfHeal -->|optional DLQ replay| Redis
  Backup --> Redis
  Backup --> PG
  Backup --> Vol
```

## Security control plane

```mermaid
flowchart LR
  Req[HTTP Request] --> Recover[Panic recover]
  Recover --> RID[X-Request-Id]
  RID --> Sec[Security headers]
  Sec --> CORS[LAN-only CORS]
  CORS --> GRL[Global rate limit]
  GRL --> Route{Route class}

  Route -->|POST write| WAuth[WRITE_API_TOKEN]
  WAuth --> WRL[Write rate limit]
  WRL --> URL[Upload rate limit]
  URL --> Gate[Inflight gate image]
  Gate --> Handler[Handler + validation]

  Route -->|metrics/DLQ| IAuth[INTERNAL_TOKEN]
  IAuth --> Internal[Internal handlers]

  Route -->|health/ready| Probe[Liveness / readiness]
  Route -->|GET job| Read[Job read / stream]
```

## Production container posture

| Service | User | Root FS | Caps | Notes |
| --- | --- | --- | --- | --- |
| backend | `loklingo` | read-only (prod) | drop ALL | tmpfs `/tmp`; pdf volume RW |
| frontend | `nginx` | read-only (prod) | drop ALL | listens 8080; dynamic resolver |
| ocr | `ocr` | RW (models) | drop ALL | shared volume RO for inputs |
| ollama | image default | RW data vol | no-new-privileges | host port optional |

## Fallback & self-healing matrix

| Failure | Detection | Automatic response | Manual |
| --- | --- | --- | --- |
| Backend panic | recover middleware | 500, process lives | logs |
| Backend down | healthcheck + self-heal | container restart | compose logs |
| OCR down | ready + OCR health | OCR chain fallback; self-heal restart | ocr-crash.md |
| LLM timeouts | circuit breaker metrics | open circuit, reject/failover | provider health API |
| Stuck jobs | `loklingo_queue_stuck_jobs` | RecoverStale on dequeue; self-heal restart | stuck-jobs.md |
| Dead letters | DLQ volume alert | optional `SELF_HEAL_REPLAY_DEAD` | reliability-recovery.sh |
| Frontend DNS race | nginx dynamic resolver | re-resolve every 10s | rebuild FE image |

## Related files

- `docker-compose.prod.yml` — security opts, limits, required tokens
- `frontend/nginx.conf` — CSP + dynamic upstreams
- `backend/middleware/security_headers.go` — API headers
- `backend/config/config.go` — production secret validation
- `e2e/` — Playwright security + flow tests
- `scripts/self-heal.sh`, `scripts/backup-restore.sh`
