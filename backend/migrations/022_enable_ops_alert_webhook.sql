-- Enable webhook notifications for rules with webhook_url configured

UPDATE ops_alert_rules
SET notify_webhook = TRUE
WHERE webhook_url IS NOT NULL
  AND webhook_url <> ''
  AND notify_webhook IS DISTINCT FROM TRUE;
