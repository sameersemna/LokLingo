-- Reliability telemetry retention policy helper.
-- Creates a reusable cleanup function so operators can run retention via
-- external cron, scheduled jobs, or ad-hoc maintenance.

CREATE OR REPLACE FUNCTION prune_reliability_events(retention INTERVAL DEFAULT INTERVAL '30 days')
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
    deleted_rows BIGINT;
BEGIN
    DELETE FROM reliability_events
    WHERE recorded_at < now() - retention;

    GET DIAGNOSTICS deleted_rows = ROW_COUNT;
    RETURN deleted_rows;
END;
$$;

COMMENT ON FUNCTION prune_reliability_events(INTERVAL)
IS 'Deletes reliability_events older than the provided retention interval and returns deleted row count.';
