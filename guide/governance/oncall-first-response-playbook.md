# On-Call First Response Playbook

Governance Version: 1.0
Review Owner: On-Call Rotation Owner
Last Review: 2026-05-12
Next Review: 2026-08-12
Operational Scope: First 60 minutes for production reliability incidents

Use this playbook to reduce time-to-mitigation during incident response.

## 0) Classify Incident in 2 Minutes

- Determine provisional severity using [severity-matrix.md](severity-matrix.md).
- Open incident thread and assign incident commander for Sev 1 or Sev 2.
- Record incident ID, start timestamp (UTC), and suspected impacted flow.

## 1) Run Core Health Queries (First 5 Minutes)

Run from repository root.

```bash
curl -sS "http://localhost:28080/ready" | jq .
curl -sS "http://localhost:28080/api/v1/metrics/providers" -H "X-Internal-Token: $INTERNAL_TOKEN" | jq .
curl -sS "http://localhost:28080/api/v1/metrics/ocr?window=1h" -H "X-Internal-Token: $INTERNAL_TOKEN" | jq .
```

Expected outcome:

- readiness dependencies are healthy.
- no obvious error/timeout spikes in 1h window.

## 2) Flow-Specific Triage Queries

### A) OCR outage or degradation

```bash
curl -sS "http://localhost:28080/api/v1/metrics/ocr?window=1h" -H "X-Internal-Token: $INTERNAL_TOKEN" | jq .
curl -sS "http://localhost:28080/api/v1/metrics/ocr?window=24h" -H "X-Internal-Token: $INTERNAL_TOKEN" | jq .
rg -n "ocr|timeout|retry|response_rejected" backend -g"*.go"
```

Decision hints:

- If 1h failure/timeout trend is sharply above 24h baseline, escalate to Sev 1 or Sev 2.
- If OCR service is unhealthy and no fallback path is active, classify as Sev 1.

Fallback-budget exhaustion triage checklist (OCR provider chain):

- Check `increase(loklingo_ocr_provider_fallback_budget_exhausted_total[10m])` from Prometheus metrics.
- Treat `> 0` as warning and `>= 3` as critical in the same 10-minute window.
- Verify `OCR_PROVIDER_TIMEOUT_SECONDS` and `OCR_MAX_FALLBACKS` are aligned with current provider latency behavior.
- Correlate with `loklingo_provider_timeout_total` and queue pressure before changing concurrency.
- If critical threshold repeats for two windows, page OCR owner and open Sev 2+ incident workflow.

### B) Translation provider outage or failover instability

```bash
curl -sS "http://localhost:28080/api/v1/metrics/providers" -H "X-Internal-Token: $INTERNAL_TOKEN" | jq .
rg -n "provider_failover|retry|timeout|circuit" backend -g"*.go"
```

Decision hints:

- If failover succeeds with degraded latency, treat as Sev 2.
- If all configured providers fail, treat as Sev 1.

### C) Queue backlog and worker pressure

```bash
curl -sS "http://localhost:28080/api/v1/metrics/providers" -H "X-Internal-Token: $INTERNAL_TOKEN" | jq .
rg -n "queue|retry|dead|worker" backend/jobs backend/handlers -g"*.go"
```

Decision hints:

- Backlog beyond threshold for sustained window indicates Sev 2 risk.
- If queue growth blocks completion across core flows, escalate to Sev 1.

### D) Rendering degradation or export failures

```bash
rg -n "render|export|failed|timeout" backend frontend -g"*.go" -g"*.ts" -g"*.tsx"
curl -sS "http://localhost:28080/api/v1/metrics/providers" -H "X-Internal-Token: $INTERNAL_TOKEN" | jq .
```

Decision hints:

- Partial degradation with retries succeeding usually maps to Sev 3.
- Sustained render/export failure with customer impact maps to Sev 2.

## 3) First Mitigation Options (Low Risk)

- Reduce pressure by temporarily lowering worker concurrency.
- Shift traffic to healthy providers if failover supports controlled preference.
- Pause non-critical batch jobs if queue starvation is observed.
- Roll back recent high-risk deploy if clear time-correlation exists.

Always log: action, owner, timestamp, and observed effect after 10-15 minutes.

## 4) Communication Cadence

- Sev 1: update every 15 minutes.
- Sev 2: update every 30 minutes.
- Sev 3: update every 60 minutes.
- Sev 4: include in standard daily reliability report unless promoted.

Use templates in [incident-templates.md](incident-templates.md).

## 5) Exit Criteria for First-Hour Response

- Severity confirmed and explicitly recorded.
- Mitigation started and outcome observed at least once.
- Incident artifact folder initialized from [incidents-template/README.md](incidents-template/README.md).
- Next decision checkpoint scheduled with owner and UTC timestamp.
