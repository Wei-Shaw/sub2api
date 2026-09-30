-- #6976: usage_logs INSERT trigger took FOR KEY SHARE on
-- usage_group_rollup_state, which conflicts with the background rollup's
-- FOR UPDATE watermark lock held during long historical aggregation.
-- Real-time inserts stalled until the rollup committed, causing
-- best-effort batch insert timeouts and lost usage_logs rows.
--
-- Fix: read the watermark without a row lock and only take a brief row
-- lock inside the conditional UPDATE (WHERE clause guards the write).
-- Lock wait time drops from minutes (full rebuild) to milliseconds.

CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state_after_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected_date DATE;
    published_before DATE;
    configured_timezone TEXT := current_setting('TimeZone');
BEGIN
    SELECT MIN((created_at AT TIME ZONE configured_timezone)::date)
    INTO affected_date
    FROM inserted_usage_logs
    WHERE group_id IS NOT NULL;

    IF affected_date IS NULL THEN
        RETURN NULL;
    END IF;

    SELECT closed_before
    INTO published_before
    FROM usage_group_rollup_state
    WHERE id = 1;

    IF published_before > affected_date THEN
        UPDATE usage_group_rollup_state
        SET closed_before = LEAST(closed_before, affected_date),
            updated_at = NOW()
        WHERE id = 1
          AND closed_before > affected_date;
    END IF;

    RETURN NULL;
END;
$$;
