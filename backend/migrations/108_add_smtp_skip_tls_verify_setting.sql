-- Add smtp_skip_tls_verify setting with a safe default for existing deployments.
-- Default remains false, meaning SMTP TLS certificates are validated.

INSERT INTO settings (key, value)
VALUES ('smtp_skip_tls_verify', 'false')
ON CONFLICT (key) DO NOTHING;
