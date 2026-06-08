-- Add request_type enum for usage_logs while keeping legacy stream/openai_ws_mode compatibility.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS request_type SMALLINT NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_logs_request_type_check'
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_request_type_check
            CHECK (request_type IN (0, 1, 2, 3));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_usage_logs_request_type_created_at
    ON usage_logs (request_type, created_at);

-- Backfill from legacy fields in bounded batches.
-- Why bounded:
-- 1) Full-table UPDATE on large usage_logs can block startup for a long time.
-- 2) request_type=0 rows remain query-compatible via legacy fallback logic
--    (stream/openai_ws_mode) in repository filters.
-- 3) Subsequent writes will use explicit request_type and gradually dilute
--    historical unknown rows.
--
-- openai_ws_mode has higher priority than stream.
DO $$
DECLARE
    v_rows         INTEGER := 0;
    v_total_rows   INTEGER := 0;
    v_batch_size   INTEGER := 5000;
    v_started_at   TIMESTAMPTZ := clock_timestamp();
    v_max_duration INTERVAL := INTERVAL '8 seconds';
BEGIN
    LOOP
        WITH batch AS (
            SELECT id
            FROM usage_logs
            WHERE request_type = 0
            ORDER BY id
            LIMIT v_batch_size
        )
        UPDATE usage_logs ul
        SET request_type = CASE
            WHEN ul.openai_ws_mode = TRUE THEN 3
            WHEN ul.stream = TRUE THEN 2
            ELSE 1
        END
        FROM batch
        WHERE ul.id = batch.id;

        GET DIAGNOSTICS v_rows = ROW_COUNT;
        EXIT WHEN v_rows = 0;

        v_total_rows := v_total_rows + v_rows;
        EXIT WHEN clock_timestamp() - v_started_at >= v_max_duration;
    END LOOP;

    RAISE NOTICE 'usage_logs.request_type startup backfill rows=%', v_total_rows;
END
$$;
