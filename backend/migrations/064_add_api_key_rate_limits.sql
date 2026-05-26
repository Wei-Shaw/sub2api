-- Add rate limit fields to api_keys table
-- Rate limit configuration (0 = unlimited)
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS rate_limit_5h decimal(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS rate_limit_1d decimal(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS rate_limit_7d decimal(20,8) NOT NULL DEFAULT 0;

-- Rate limit usage tracking
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS usage_5h decimal(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS usage_1d decimal(20,8) NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS usage_7d decimal(20,8) NOT NULL DEFAULT 0;

-- Window start times (nullable)
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS window_5h_start timestamptz;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS window_1d_start timestamptz;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS window_7d_start timestamptz;
