-- Channel monitor exclusive mode: v1 (active probes) or v2 (passive aggregation).
-- Default v2 keeps existing V2-first deployments unchanged.
INSERT INTO settings (key, value)
VALUES ('channel_monitor_mode', 'v2')
ON CONFLICT (key) DO NOTHING;
