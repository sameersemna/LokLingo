# LokLingo Security Hardening Guide

Governance Version: 1.1  
Last Review: 2026-07-17  
Scope: LAN-first self-hosted production on hostname `promaxgb10-6116`

## Threat model (LAN-first)

| Threat | Mitigation |
| --- | --- |
| Untrusted LAN clients flooding API | Tiered rate limits + body limits + inflight gates |
| Unauthorized writes / job injection | `WRITE_API_TOKEN` required in production |
| Metrics / DLQ / replay abuse | `INTERNAL_TOKEN` required in production |
| Weak/default secrets | Startup validation rejects placeholders and short tokens |
| XSS / clickjacking on UI | CSP + `X-Frame-Options: DENY` on nginx edge |
| Container breakout | non-root images, `no-new-privileges`, `cap_drop: ALL`, read-only rootfs (prod overlay) |
| Supply-chain / misconfig in CI | CI security gates (secret scan, compose config, unit tests, Playwright smoke) |
| Data loss | `scripts/backup-restore.sh` + Redis/Postgres/volume export |

## Required production secrets

```bash
cp .env.production .env   # or copy from .env.example and set APP_ENV=production
openssl rand -hex 32      # WRITE_API_TOKEN
openssl rand -hex 32      # INTERNAL_TOKEN
# Also rotate: REDIS password, Postgres password, LITELLM_API_KEY, GRAFANA_ADMIN_PASSWORD
```

Production boot **fails closed** when:

- `WRITE_API_TOKEN` or `INTERNAL_TOKEN` is missing
- Tokens are shorter than 24 characters
- Tokens match documented placeholders (`replace-*`, `CHANGE_ME*`, `dev-internal-token`, …)
- `LITELLM_API_KEY` is the example value `sk-loklingo`

## Auth headers

| Header | Routes |
| --- | --- |
| `X-API-Token` or `Authorization: Bearer <WRITE_API_TOKEN>` | `POST /api/v1/translate*`, `POST /api/v1/jobs*` |
| `X-Internal-Token: <INTERNAL_TOKEN>` | metrics, dead-letter list/replay, lifecycle events |

CORS allows: `Content-Type`, `Authorization`, `X-API-Token`, `X-Internal-Token`, `X-Request-Id`.  
Origins: `localhost`, `promaxgb10-6116`, and private/loopback IPs only.

## Security headers

**Backend (Fiber):** `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, `Cross-Origin-Resource-Policy`, `Cache-Control: no-store` (non-health).

**Frontend (nginx):** CSP (self-only scripts/connect), frame deny, nosniff, Permissions-Policy, COOP/CORP, `server_tokens off`.

## Docker production posture

```bash
# Fill secrets first
cp .env.production .env
# edit .env

docker compose -f docker-compose.yml -f docker-compose.prod.yml --profile production up -d --build
```

Prod overlay applies:

- `security_opt: no-new-privileges:true`
- `cap_drop: ALL`
- backend/frontend `read_only: true` + tmpfs where needed
- stricter rate-limit defaults
- log rotation
- resource limits (Compose `deploy.resources`; enforced under Swarm / some runtimes)

Frontend nginx runs as non-root user and listens on container port **8080** (host still maps `FRONTEND_HOST_PORT`).

## Token rotation procedure

1. Generate new tokens with `openssl rand -hex 32`.
2. Update `.env` on the host (do not commit).
3. Rolling restart: `docker compose ... up -d loklingo-backend`.
4. Update any clients/scripts (`INTERNAL_TOKEN`, `WRITE_API_TOKEN`).
5. Invalidate old tokens (single-token model — old value stops working immediately).
6. Record rotation in ops log / incident notes (no secret values in git).

## Network exposure checklist

- [ ] Firewall restricts `3000`, `28080`, `8000` to trusted LAN only
- [ ] Prefer not exposing OCR (`8000`) and Ollama (`11434`) beyond the Docker network when possible
- [ ] Grafana password is not `admin`
- [ ] Postgres uses `sslmode=require` when off-host
- [ ] Redis requires a password and is not bound to `0.0.0.0` without ACL

## Ollama port exposure (LAN risk)

The `loklingo-ollama` container is published on the host at
`${OLLAMA_HOST_PORT:-11434}:11434`. By default it is reachable from any
host that can reach the LAN.

**Why this matters**

- Ollama's HTTP API has **no built-in authentication**. Anyone who can
  reach the port can list models, pull new ones, run inference, and
  consume GPU/CPU.
- Pulling a model can be expensive (multi-GB downloads) and can be used
  to exhaust disk or bandwidth.
- Running arbitrary prompts on your GPU leaks hardware to third-party
  traffic and may be used as a pivot for further abuse.

**Recommended mitigations** (apply in order of preference)

1. **LAN-only is the baseline.** Bind Ollama to a private IP only:
   `OLLAMA_HOST_PORT` should not be published beyond the LAN, and the
   host firewall should block external traffic to it.
2. **Drop the host port entirely if not needed.** Edit
   `docker-compose.yml` and remove the `ports:` mapping for
   `loklingo-ollama`. The OCR service and backend will still reach it
   over the `loklingo-net` Docker network. The frontend never needs to
   talk to Ollama directly.
3. **Restrict with iptables/nftables.** Example for an internal-only
   `11434`:
   ```bash
   iptables -A INPUT -p tcp --dport 11434 -s 192.168.0.0/16 -j ACCEPT
   iptables -A INPUT -p tcp --dport 11434 -j DROP
   ```
4. **Reverse proxy with auth.** Front Ollama with Caddy/nginx and
   require HTTP basic auth. (LokLingo does not ship this; document any
   custom addition in `guide/reliability-runbook.md`.)
5. **Move Ollama off the LAN host entirely.** Run on a separate
   workstation that is only reachable over a private link.

**Detection**

- `scripts/check-lan-access.sh` confirms the LAN reachability of
  published ports; integrate it into your change-control.
- Watch for unexpected `ollama list` or `ollama pull` activity in
  container logs (`docker logs loklingo-ollama`). Ollama logs every
  request to stdout.

**Recovery if exposed**

1. Block external traffic at the host firewall.
2. Audit `ollama list` output to confirm no unauthorized model was
   pulled.
3. Rotate any model caches that may have been overwritten.
4. Document the incident under `guide/incidents/` per the governance
   flow.

## Related runbooks

- [Reliability runbook](reliability-runbook.md)
- [Self-heal script](../scripts/self-heal.sh)
- [Backup/restore](../scripts/backup-restore.sh)
- [OCR crash recovery](recovery/ocr-crash.md)
- [Stuck jobs](recovery/stuck-jobs.md)
