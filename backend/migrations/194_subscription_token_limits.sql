-- 194_subscription_token_limits.sql
-- 新增订阅(token)计费类型所需的 token 限额与用量列。

-- 分组级 token 限额（订阅 token 型分组使用；nullable = 不限）
ALTER TABLE groups ADD COLUMN IF NOT EXISTS daily_limit_tokens BIGINT;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS weekly_limit_tokens BIGINT;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS monthly_limit_tokens BIGINT;

-- 用户订阅级 token 用量（默认 0；与 *_usage_usd 共享 *_window_start 窗口）
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS daily_usage_tokens BIGINT NOT NULL DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS weekly_usage_tokens BIGINT NOT NULL DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS monthly_usage_tokens BIGINT NOT NULL DEFAULT 0;
