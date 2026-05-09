-- Reliability telemetry analytics table.
-- One row per reliability event emitted from upstream integration paths
-- (LiteLLM and OCR), persisted via pglog handler.

CREATE TABLE IF NOT EXISTS reliability_events (
    id          BIGSERIAL PRIMARY KEY,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- integration and event identity
    integration TEXT        NOT NULL, -- litellm | ocr
    event_name  TEXT        NOT NULL,

    -- optional dimensions for filtering/aggregation
    reason      TEXT        NOT NULL DEFAULT '',
    endpoint    TEXT        NOT NULL DEFAULT '',
    attempt     BIGINT      NOT NULL DEFAULT 0,
    status      BIGINT      NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS reliability_events_recorded_at_idx
    ON reliability_events (recorded_at DESC);

CREATE INDEX IF NOT EXISTS reliability_events_integration_idx
    ON reliability_events (integration);

CREATE INDEX IF NOT EXISTS reliability_events_event_name_idx
    ON reliability_events (event_name);

CREATE INDEX IF NOT EXISTS reliability_events_reason_idx
    ON reliability_events (reason);
