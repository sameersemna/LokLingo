# Queue Corruption Recovery

Symptoms:
- Queue depth in `/api/v1/health/dashboard` looks wrong
- Jobs disappear, repeat, or never leave inflight/retry sets
- The worker logs `recover_stale_inflight_failed` or Redis errors

Recovery steps:
1. Check Redis logs and confirm the container is reachable.
2. Restart Redis if the store is unavailable.
3. Use the dead-letter and replay endpoints to recover valid jobs.
4. If queue keys are inconsistent, flush only the LokLingo queue namespace after confirming no live work is in progress.
5. Restart backend and worker after Redis recovers.

Escalation:
- If queue corruption repeats, inspect Redis persistence and host disk pressure.
