-- 用户平台维度配额表。每个 (user_id, platform) 对独立记录日/周/月三层 USD 限额与用量。
-- nil limit = 不限制（沿用上层默认），0 = 显式禁用，>0 = USD 上限。
-- 软删除：deleted_at IS NULL 的记录为活跃记录，部分唯一索引保证同用户同平台只有一条活跃配额。

CREATE TABLE IF NOT EXISTS user_platform_quotas (
    id                   BIGSERIAL PRIMARY KEY,
    user_id              BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform             VARCHAR(32) NOT NULL CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity')),

    -- 日 / 周 / 月 USD 上限：NULL = 不限制，0 = 显式禁用，>0 = 上限
    daily_limit_usd      DECIMAL(20,10),
    weekly_limit_usd     DECIMAL(20,10),
    monthly_limit_usd    DECIMAL(20,10),

    -- 当前窗口已用量
    daily_usage_usd      DECIMAL(20,10) NOT NULL DEFAULT 0,
    weekly_usage_usd     DECIMAL(20,10) NOT NULL DEFAULT 0,
    monthly_usage_usd    DECIMAL(20,10) NOT NULL DEFAULT 0,

    -- 窗口起点（NULL = 首次尚未初始化）
    daily_window_start   TIMESTAMPTZ,
    weekly_window_start  TIMESTAMPTZ,
    monthly_window_start TIMESTAMPTZ,

    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ
);

-- 软删除友好唯一索引：同用户同平台只允许一条未删除记录
CREATE UNIQUE INDEX IF NOT EXISTS userplatformquota_user_id_platform_uq
    ON user_platform_quotas (user_id, platform)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS userplatformquota_user_id
    ON user_platform_quotas (user_id);
