-- OCR analytics dashboard queries for Postgres.
-- Run in psql against the LokLingo database.

-- 1) OCR usage volume over time (hourly)
SELECT
  date_trunc('hour', recorded_at) AS hour,
  COUNT(*) AS ocr_events
FROM ocr_events
GROUP BY 1
ORDER BY 1 DESC
LIMIT 168;

-- 2) Outcome mix
SELECT
  outcome,
  COUNT(*) AS events,
  ROUND(100.0 * COUNT(*) / NULLIF(SUM(COUNT(*)) OVER (), 0), 2) AS pct
FROM ocr_events
GROUP BY outcome
ORDER BY events DESC;

-- 3) p50/p95/p99 OCR latency by language
SELECT
  NULLIF(lang, '') AS lang,
  COUNT(*) AS n,
  percentile_cont(0.50) WITHIN GROUP (ORDER BY ocr_ms) AS p50_ocr_ms,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY ocr_ms) AS p95_ocr_ms,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY ocr_ms) AS p99_ocr_ms
FROM ocr_events
WHERE ocr_ms > 0
GROUP BY lang
HAVING COUNT(*) >= 5
ORDER BY p95_ocr_ms DESC NULLS LAST;

-- 4) End-to-end latency percentiles by target language
SELECT
  NULLIF(target, '') AS target,
  COUNT(*) AS n,
  percentile_cont(0.50) WITHIN GROUP (ORDER BY total_ms) AS p50_total_ms,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY total_ms) AS p95_total_ms,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY total_ms) AS p99_total_ms
FROM ocr_events
WHERE total_ms > 0
GROUP BY target
HAVING COUNT(*) >= 5
ORDER BY p95_total_ms DESC NULLS LAST;

-- 5) Top extraction failure reasons (what triggers fallback)
SELECT
  extraction_failure_reason,
  COUNT(*) AS events
FROM ocr_events
GROUP BY extraction_failure_reason
ORDER BY events DESC
LIMIT 20;

-- 6) Slowest recent events for debugging
SELECT
  recorded_at,
  job_id,
  outcome,
  lang,
  target,
  extract_ms,
  ocr_ms,
  translate_ms,
  total_ms,
  extraction_failure_reason,
  file
FROM ocr_events
ORDER BY total_ms DESC, recorded_at DESC
LIMIT 100;

-- 7) Rolling 24h SLO-like summary
SELECT
  COUNT(*) FILTER (WHERE recorded_at >= now() - interval '24 hours') AS events_24h,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY total_ms)
    FILTER (WHERE recorded_at >= now() - interval '24 hours' AND total_ms > 0) AS p95_total_ms_24h,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY total_ms)
    FILTER (WHERE recorded_at >= now() - interval '24 hours' AND total_ms > 0) AS p99_total_ms_24h,
  ROUND(100.0 * COUNT(*) FILTER (WHERE recorded_at >= now() - interval '24 hours' AND outcome = 'succeeded')
        / NULLIF(COUNT(*) FILTER (WHERE recorded_at >= now() - interval '24 hours'), 0), 2) AS success_rate_pct_24h
FROM ocr_events;

-- 8) Reliability event volume over time (hourly, last 7d)
SELECT
  date_trunc('hour', recorded_at) AS hour,
  integration,
  event_name,
  COUNT(*) AS events
FROM reliability_events
WHERE recorded_at >= now() - interval '7 days'
GROUP BY 1, 2, 3
ORDER BY hour DESC, integration, event_name;

-- 9) Top reliability failure reasons (last 24h)
SELECT
  integration,
  event_name,
  NULLIF(reason, '') AS reason,
  COUNT(*) AS events
FROM reliability_events
WHERE recorded_at >= now() - interval '24 hours'
GROUP BY integration, event_name, reason
ORDER BY events DESC
LIMIT 50;

-- 10) LiteLLM circuit-breaker and retry signal counts (last 24h)
SELECT
  event_name,
  COUNT(*) AS events
FROM reliability_events
WHERE recorded_at >= now() - interval '24 hours'
  AND integration = 'litellm'
  AND event_name IN (
    'litellm_retry',
    'litellm_retry_cancelled',
    'litellm_circuit_reject',
    'litellm_circuit_open',
    'litellm_circuit_opened',
    'litellm_circuit_recovered',
    'litellm_response_rejected'
  )
GROUP BY event_name
ORDER BY events DESC;

-- 11) OCR reliability signal counts by endpoint (last 24h)
SELECT
  event_name,
  NULLIF(endpoint, '') AS endpoint,
  COUNT(*) AS events
FROM reliability_events
WHERE recorded_at >= now() - interval '24 hours'
  AND integration = 'ocr'
GROUP BY event_name, endpoint
ORDER BY events DESC;
