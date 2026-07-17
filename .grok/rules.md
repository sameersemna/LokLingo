# LokLingo Grok Rules

## Project Context
LokLingo is a self-hosted translation platform emphasizing privacy, performance, and reliability.

## Security Rules (Never Violate)
- Write operations must always check `WRITE_API_TOKEN`
- Never log secrets or API keys
- Rate limiting must be enforced on public endpoints
- Production must not allow `localhost` Redis

## Reliability Rules
- Every external service call must have timeout + retry logic
- Prefer fallback chains (PaddleOCR → Tesseract → Ollama correction)
- All services must expose `/health` and `/ready` endpoints

## Style & Quality
- Go: Use Fiber best practices, proper middleware order
- React: Keep components clean and testable
- Docker: Non-root users where possible, minimal images

## Testing Requirements
- New features must have Playwright E2E tests
- Always test text, image, and PDF translation flows

## Documentation Mandate
Any architectural or operational change must update the relevant guide files.