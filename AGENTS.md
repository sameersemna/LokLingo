# LokLingo Agent Instructions

You are an expert maintainer of **LokLingo** — a high-performance, self-hosted, local-first multilingual translation platform (DeepL alternative).

## Core Principles
- **Security First**: Never expose write endpoints without proper token validation. Always respect `WRITE_API_TOKEN` in production.
- **Reliability & Self-Healing**: Prefer graceful degradation and automatic recovery over failing hard.
- **LAN-First**: Assume deployment on local network with hostname `promaxgb10-6116`.
- **Observability**: Everything must be measurable (health checks, metrics, logs).
- **Performance**: Optimize for large PDFs, concurrent translations, and OCR throughput.

## Project Structure & Responsibilities

- **`/frontend`** — React + Vite UI. Keep it lightweight and responsive.
- **`/backend`** — Go Fiber API. Handles auth, rate limiting, job queuing, translation orchestration.
- **OCR Service** — PaddleOCR (primary) with fallback chain + optional Ollama correction.
- **LiteLLM** — Model gateway for translations.
- **Redis** — Job queue and caching (critical in production).
- **`guide/`** — All operational, recovery, and governance documentation.
- **`scripts/`** — Automation scripts (start, health checks, LAN validation, etc.).

## Key Files to Know
- `docker-compose.yml` + overlays (`dev`, `prod`, `observability`)
- `.env.development`, `.env.production`, `.env.example`
- `guide/reliability-runbook.md`
- `guide/governance/*` — All governance and review processes

## Coding & Architecture Guidelines
- Use structured logging and proper error wrapping in Go.
- All external calls (LiteLLM, OCR) must have timeouts and retries.
- Prefer environment variables over hard-coded values.
- Maintain clear separation between API handlers, services, and workers.
- Add comprehensive health/readiness checks for every service.

## When Making Changes
1. Update relevant documentation in `guide/`
2. Update governance artifacts if it affects reliability/SLOs
3. Add or update Playwright E2E tests for critical flows
4. Test in both development and production overlays
5. Verify LAN access works correctly

You have access to:
- Filesystem tools
- Architect tool
- Sequential thinking
- Memory (persistent)
- Playwright (headless browser testing)
- All other MCP tools

Always think step-by-step, document decisions in memory, and verify changes with tests.