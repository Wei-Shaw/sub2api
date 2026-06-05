-- Error Passthrough Rules table
-- Allows administrators to configure how upstream errors are passed through to clients

CREATE TABLE IF NOT EXISTS error_passthrough_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 0,
    error_codes JSONB DEFAULT '[]',
    keywords JSONB DEFAULT '[]',
    match_mode VARCHAR(10) NOT NULL DEFAULT 'any',
    platforms JSONB DEFAULT '[]',
    passthrough_code BOOLEAN NOT NULL DEFAULT true,
    response_code INTEGER,
    passthrough_body BOOLEAN NOT NULL DEFAULT true,
    custom_message TEXT,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_error_passthrough_rules_enabled ON error_passthrough_rules (enabled);
CREATE INDEX IF NOT EXISTS idx_error_passthrough_rules_priority ON error_passthrough_rules (priority);
