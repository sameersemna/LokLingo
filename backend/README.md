# LokLingo Backend

Go Fiber API server for LokLingo.

## Setup

```bash
go mod tidy
go run main.go
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | /health | Health check |
| POST | /api/v1/translate | Translate text |

## Translate request

```json
{
  "source_text": "Hello",
  "source_lang": "en",
  "target_lang": "de"
}
```

## Environment

| Variable | Default | Description |
|----------|---------|-------------|
| PORT | 8080 | Server port |
