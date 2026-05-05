-- OCR fallback analytics table.
-- One row per job where native PDF text extraction failed and the OCR
-- service was invoked.  Every column maps 1-to-1 to a structured log
-- field emitted by the worker so that log shippers (or the pglog handler)
-- can write rows with no string parsing.

CREATE TABLE IF NOT EXISTS ocr_events (
    id                       BIGSERIAL PRIMARY KEY,
    recorded_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- job identity
    job_id                   TEXT        NOT NULL,
    file                     TEXT        NOT NULL,

    -- extraction context
    lang                     TEXT        NOT NULL DEFAULT '',
    target                   TEXT        NOT NULL DEFAULT '',
    extraction_failure_reason TEXT       NOT NULL DEFAULT '',

    -- outcome  ("succeeded" | "also_failed")
    outcome                  TEXT        NOT NULL DEFAULT 'succeeded'
);

CREATE INDEX IF NOT EXISTS ocr_events_job_id_idx    ON ocr_events (job_id);
CREATE INDEX IF NOT EXISTS ocr_events_recorded_at_idx ON ocr_events (recorded_at DESC);
CREATE INDEX IF NOT EXISTS ocr_events_lang_idx      ON ocr_events (lang);
CREATE INDEX IF NOT EXISTS ocr_events_target_idx    ON ocr_events (target);
