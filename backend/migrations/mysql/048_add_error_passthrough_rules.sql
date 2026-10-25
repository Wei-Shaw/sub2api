-- MySQL 方言版本（翻译自 ../048_add_error_passthrough_rules.sql）
-- Error Passthrough Rules table
-- Allows administrators to configure how upstream errors are passed through to clients

CREATE TABLE IF NOT EXISTS error_passthrough_rules (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INTEGER NOT NULL DEFAULT 0,
    error_codes JSON DEFAULT ('[]'),
    keywords JSON DEFAULT ('[]'),
    match_mode VARCHAR(10) NOT NULL DEFAULT 'any',
    platforms JSON DEFAULT ('[]'),
    passthrough_code BOOLEAN NOT NULL DEFAULT TRUE,
    response_code INTEGER,
    passthrough_body BOOLEAN NOT NULL DEFAULT TRUE,
    custom_message TEXT,
    description TEXT,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Indexes for efficient queries
CREATE INDEX idx_error_passthrough_rules_enabled ON error_passthrough_rules (enabled);
CREATE INDEX idx_error_passthrough_rules_priority ON error_passthrough_rules (priority);
