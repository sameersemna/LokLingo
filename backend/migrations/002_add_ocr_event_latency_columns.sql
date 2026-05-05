-- Add latency telemetry columns for OCR fallback analytics.
-- All values are milliseconds measured in the worker process.

ALTER TABLE ocr_events
    ADD COLUMN IF NOT EXISTS extract_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ocr_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS translate_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_ms BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS ocr_events_total_ms_idx ON ocr_events (total_ms DESC);
CREATE INDEX IF NOT EXISTS ocr_events_outcome_idx ON ocr_events (outcome);
