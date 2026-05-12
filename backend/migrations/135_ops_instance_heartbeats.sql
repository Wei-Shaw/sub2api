CREATE TABLE IF NOT EXISTS ops_instance_heartbeats (
    instance_id VARCHAR(128) PRIMARY KEY,
    role VARCHAR(32) NOT NULL,
    hostname VARCHAR(255) NOT NULL DEFAULT '',
    autonomous_background_enabled BOOLEAN NOT NULL DEFAULT false,
    started_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL
);

COMMENT ON TABLE ops_instance_heartbeats IS 'Per-instance heartbeat snapshots for deployment cluster monitoring.';
COMMENT ON COLUMN ops_instance_heartbeats.instance_id IS 'Stable deployment.instance_id used to identify one running instance across heartbeats.';
COMMENT ON COLUMN ops_instance_heartbeats.role IS 'Deployment role: standalone/master/slave.';
COMMENT ON COLUMN ops_instance_heartbeats.hostname IS 'Best-effort hostname for operator debugging.';
COMMENT ON COLUMN ops_instance_heartbeats.autonomous_background_enabled IS 'Whether this instance is expected to run master-only autonomous background services.';
COMMENT ON COLUMN ops_instance_heartbeats.started_at IS 'Process start time of the current instance lifecycle.';
COMMENT ON COLUMN ops_instance_heartbeats.last_seen_at IS 'Latest heartbeat timestamp from the instance.';

CREATE INDEX IF NOT EXISTS idx_ops_instance_heartbeats_role_last_seen
    ON ops_instance_heartbeats (role, last_seen_at DESC);
