-- MySQL 方言版本（翻译自 ../028_add_usage_logs_user_agent.sql）
-- Add user_agent column to usage_logs table
-- Records the User-Agent header from API requests for analytics and debugging
--
-- 翻译说明：ADD COLUMN IF NOT EXISTS → 去掉修饰词；COMMENT ON COLUMN → 整句跳过。

ALTER TABLE usage_logs
    ADD COLUMN user_agent VARCHAR(512);

-- Optional: Add index for user_agent queries (uncomment if needed for analytics)
-- CREATE INDEX idx_usage_logs_user_agent ON usage_logs(user_agent);

-- (comment skipped) COMMENT ON COLUMN usage_logs.user_agent IS 'User-Agent header from the API request'
