-- Keep the historical behavior by default; administrators may opt out for future captures.
ALTER TABLE conversation_log_config
    ADD COLUMN IF NOT EXISTS redact_sensitive BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE conversation_logs
    ADD COLUMN IF NOT EXISTS redacted BOOLEAN NOT NULL DEFAULT TRUE;
