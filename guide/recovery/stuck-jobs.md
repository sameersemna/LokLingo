# Stuck Jobs Recovery

Symptoms:
- `/api/v1/health/dashboard` reports stuck jobs
- Jobs remain in `processing` for too long
- `loklingo:jobs:inflight:zset` grows without clearing

Recovery steps:
1. Confirm whether the worker is still running.
2. Check worker logs for provider timeouts, OCR failures, or dead-letter events.
3. Requeue or replay the affected jobs from the dead-letter API if appropriate.
4. If the job is genuinely stuck, restart the worker container to trigger stale inflight recovery.
5. Reduce job size or concurrency if the host is under pressure.

Escalation:
- If jobs repeatedly get stuck, lower upload rate limits and inspect provider latency in `/api/v1/metrics/prometheus`.
