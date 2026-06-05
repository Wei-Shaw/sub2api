-- Add user_agent column to usage_logs table
-- Records the User-Agent header from API requests for analytics and debugging

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS user_agent VARCHAR(512);

-- Optional: Add index for user_agent queries (uncomment if needed for analytics)
-- CREATE INDEX IF NOT EXISTS idx_usage_logs_user_agent ON usage_logs(user_agent);

COMMENT ON COLUMN usage_logs.user_agent IS 'User-Agent header from the API request';
