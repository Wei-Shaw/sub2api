-- One finalized gateway HTTP request, independent of billing/error logging.
-- No payloads, headers, client identity, or credentials are retained here.
-- Application-generated event IDs, not client request IDs, make write retries
-- idempotent without collapsing different requests sharing X-Request-ID.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS ops_traffic_events (
    event_id UUID PRIMARY KEY,
    request_id TEXT NOT NULL DEFAULT '',
    group_id BIGINT,
    account_id BIGINT,
    platform TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    traffic JSONB NOT NULL CHECK (jsonb_typeof(traffic) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_ops_traffic_events_created_at
    ON ops_traffic_events (created_at, event_id);
CREATE INDEX IF NOT EXISTS idx_ops_traffic_events_group_created_at
    ON ops_traffic_events (group_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ops_traffic_events_platform_created_at
    ON ops_traffic_events (platform, created_at);

COMMENT ON TABLE ops_traffic_events IS
    'Append-only finalized gateway HTTP entity-body ledger; failures included, WebSocket frames excluded; retained with Ops raw error-log retention';
COMMENT ON COLUMN ops_traffic_events.created_at IS
    'Request completion time after response compressor close, not request start';
COMMENT ON COLUMN ops_traffic_events.platform IS
    'Final resolved request route platform; all retry attempts belong to this request, not independently attributed upstream accounts';
