# Ollama Unavailable Recovery

Symptoms:
- `/api/v1/health/dashboard` shows Ollama down
- OCR correction or translation requests lose quality or fail over
- `ollama list` fails inside the Ollama container

Recovery steps:
1. Check `loklingo-ollama` logs and confirm the service is running.
2. Confirm the configured model is present with `ollama list`.
3. If the model is missing, pull it again.
4. Verify the backend `OLLAMA_BASE_URL` points at the LAN service or Docker hostname expected by the compose overlay.
5. Restart the stack if the service is healthy but the backend has stale connection state.

Escalation:
- If Ollama cannot start, free disk space and validate the model cache volume.
