# OCR Crash Recovery

Symptoms:
- `/api/v1/health/dashboard` shows OCR down
- OCR requests return 502/503
- `loklingo-ocr` container exits or restarts repeatedly

Recovery steps:
1. Check the OCR container logs.
2. Confirm the OCR service starts and `/health` returns 200.
3. Verify the OCR shared storage directory is writable and mounted.
4. If the failure is tied to a bad document, remove the offending file from `/tmp/loklingo` and retry.
5. Restart the stack with `scripts/start-lan.sh` if needed.

Escalation:
- If the OCR container keeps crashing, rebuild the OCR image and inspect the Python dependency set.
