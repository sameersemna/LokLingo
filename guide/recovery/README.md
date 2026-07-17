# LokLingo Recovery Index

Operational recovery playbooks for the self-hosted stack.

## Automated self-healing

```bash
# One-shot diagnose + heal (reads host ports from .env)
bash scripts/self-heal.sh

# Continuous loop (every 60s)
bash scripts/self-heal.sh --watch

# Report only
bash scripts/self-heal.sh --dry-run

# Opt-in dead-letter auto-replay (use carefully — poison messages)
SELF_HEAL_REPLAY_DEAD=1 bash scripts/self-heal.sh
```

Env overrides: `LOKLINGO_BASE_URL`, `OCR_URL`, `FRONTEND_URL`, `INTERNAL_TOKEN`, `COMPOSE_FILES`.

Manual recovery tooling:

```bash
bash guide/ops/reliability-recovery.sh queue-status
bash guide/ops/reliability-recovery.sh list-dead
bash guide/ops/reliability-recovery.sh replay-dead <job-id>
```

## Scenario playbooks

| Scenario | Doc | First actions |
| --- | --- | --- |
| OCR container crash / unhealthy | [ocr-crash.md](ocr-crash.md) | `docker restart loklingo-ocr`; check shared volume; fallback providers |
| Ollama down (correction / OCR chain) | [ollama-unavailable.md](ollama-unavailable.md) | restart ollama; disable correction; use paddle/tesseract only |
| Redis queue corruption | [queue-corruption.md](queue-corruption.md) | inspect keys; drain DLQ; restore from backup if needed |
| Stuck inflight jobs | [stuck-jobs.md](stuck-jobs.md) | RecoverStale on dequeue; backend restart; lower upload rate limits |
| LiteLLM / translation outage | reliability runbook + circuit breaker alerts | wait for circuit recovery; enable Ollama/OpenAI-compat failover providers |
| Secrets rotation / auth failures | [../security-hardening.md](../security-hardening.md) | rotate tokens; restart backend; update clients |

## Backup / restore

```bash
bash scripts/backup-restore.sh backup
CONFIRM_RESTORE=yes bash scripts/backup-restore.sh restore .backups/<stamp>
```

## Health probes

```text
Backend liveness:  http://localhost:${BACKEND_HOST_PORT:-28080}/health
Backend readiness: http://localhost:${BACKEND_HOST_PORT:-28080}/ready
Health dashboard:  http://localhost:${BACKEND_HOST_PORT:-28080}/api/v1/health/dashboard
OCR health:        http://localhost:${OCR_HOST_PORT:-8000}/health
Frontend:          http://localhost:${FRONTEND_HOST_PORT:-3000}/
LAN hostname:      http://promaxgb10-6116:3000
```

## Fallback chains (design)

```text
Translation: LiteLLM → Ollama → OpenAI-compatible (registry order)
OCR:         primary OCR_PROVIDER → next engines (paddle/tesseract/ollama) up to OCR_MAX_FALLBACKS
Render:      primary fonts → degraded render fallback metrics
Queue:       inflight → RecoverStale requeue → dead-letter → manual/auto replay
Circuit:     consecutive LLM failures open breaker → reject with backoff → half-open recovery
```

See also: [reliability-runbook.md](../reliability-runbook.md), [security-hardening.md](../security-hardening.md).
