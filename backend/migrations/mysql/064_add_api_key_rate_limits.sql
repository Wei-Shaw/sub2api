-- MySQL 方言版本（翻译自 ../064_add_api_key_rate_limits.sql）
-- Add rate limit fields to api_keys table
-- Rate limit configuration (0 = unlimited)
ALTER TABLE api_keys ADD COLUMN rate_limit_5h DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN rate_limit_1d DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN rate_limit_7d DECIMAL(20,8) NOT NULL DEFAULT 0;

-- Rate limit usage tracking
ALTER TABLE api_keys ADD COLUMN usage_5h DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN usage_1d DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN usage_7d DECIMAL(20,8) NOT NULL DEFAULT 0;

-- Window start times (nullable)
ALTER TABLE api_keys ADD COLUMN window_5h_start DATETIME(3) NULL;
ALTER TABLE api_keys ADD COLUMN window_1d_start DATETIME(3) NULL;
ALTER TABLE api_keys ADD COLUMN window_7d_start DATETIME(3) NULL;
