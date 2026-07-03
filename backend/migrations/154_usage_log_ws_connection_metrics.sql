ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS ws_conn_reused BOOLEAN,
    ADD COLUMN IF NOT EXISTS ws_preflight_fail_count INTEGER,
    ADD COLUMN IF NOT EXISTS ws_conn_pick_ms INTEGER,
    ADD COLUMN IF NOT EXISTS ws_payload_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS ws_event_count INTEGER,
    ADD COLUMN IF NOT EXISTS ws_queue_wait_ms INTEGER;

COMMENT ON COLUMN usage_logs.ws_conn_reused IS 'Whether the OpenAI WS upstream connection was reused for this request.';
COMMENT ON COLUMN usage_logs.ws_preflight_fail_count IS 'Number of WS preflight ping failures observed before serving this request.';
COMMENT ON COLUMN usage_logs.ws_conn_pick_ms IS 'Time spent acquiring an upstream WS connection in milliseconds.';
COMMENT ON COLUMN usage_logs.ws_payload_bytes IS 'Approximate upstream WS payload size in bytes.';
COMMENT ON COLUMN usage_logs.ws_event_count IS 'Number of upstream WS events observed for this request.';
COMMENT ON COLUMN usage_logs.ws_queue_wait_ms IS 'Time spent waiting for an upstream WS connection slot in milliseconds.';
