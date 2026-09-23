ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS notify_wecom BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS wecom_sent BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN ops_alert_rules.notify_wecom IS 'Whether this rule sends WeCom bot notifications';
COMMENT ON COLUMN ops_alert_events.wecom_sent IS 'Whether a WeCom bot notification was sent for this event';
