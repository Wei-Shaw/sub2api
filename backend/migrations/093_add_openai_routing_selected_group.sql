ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS routing_selected_group VARCHAR(32);

ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS routing_selected_group VARCHAR(32);

CREATE INDEX IF NOT EXISTS idx_usage_logs_created_routing_selected_group
    ON usage_logs (created_at, routing_selected_group);

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_created_routing_selected_group
    ON ops_error_logs (created_at, routing_selected_group);

COMMENT ON COLUMN usage_logs.routing_selected_group IS 'Final OpenAI routing subgroup actually selected for the request: active, exhausted, or reserve.';
COMMENT ON COLUMN ops_error_logs.routing_selected_group IS 'Final OpenAI routing subgroup actually selected before the request failed: active, exhausted, or reserve.';
