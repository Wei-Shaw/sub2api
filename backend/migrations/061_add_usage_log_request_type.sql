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

-- Backfill from legacy fields. openai_ws_mode has higher priority than stream.
UPDATE usage_logs
SET request_type = CASE
    WHEN openai_ws_mode = TRUE THEN 3
    WHEN stream = TRUE THEN 2
    ELSE 1
END
WHERE request_type = 0;
