-- MySQL 方言版本（翻译自 ../142_user_platform_quotas.sql）
-- 用户平台维度配额表。每个 (user_id, platform) 对独立记录日/周/月三层 USD 限额与用量。
-- nil limit = 不限制（沿用上层默认），0 = 显式禁用，>0 = USD 上限。
-- 软删除：deleted_at IS NULL 的记录为活跃记录，函数唯一索引保证同用户同平台只有一条活跃配额。

CREATE TABLE IF NOT EXISTS user_platform_quotas (
    id                   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id              BIGINT NOT NULL,
    platform             VARCHAR(32) NOT NULL,

    -- 日 / 周 / 月 USD 上限：NULL = 不限制，0 = 显式禁用，>0 = 上限
    daily_limit_usd      DECIMAL(20,10),
    weekly_limit_usd     DECIMAL(20,10),
    monthly_limit_usd    DECIMAL(20,10),

    -- 当前窗口已用量
    daily_usage_usd      DECIMAL(20,10) NOT NULL DEFAULT 0,
    weekly_usage_usd     DECIMAL(20,10) NOT NULL DEFAULT 0,
    monthly_usage_usd    DECIMAL(20,10) NOT NULL DEFAULT 0,

    -- 窗口起点（NULL = 首次尚未初始化）
    daily_window_start   DATETIME(3) NULL,
    weekly_window_start  DATETIME(3) NULL,
    monthly_window_start DATETIME(3) NULL,

    created_at           DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at           DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    deleted_at           DATETIME(3) NULL,

    CONSTRAINT user_platform_quotas_platform_check CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity')),
    CONSTRAINT fk_user_platform_quotas_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 软删除友好唯一索引：同用户同平台只允许一条未删除记录
-- （PG 版部分索引 WHERE deleted_at IS NULL → 函数键部分）
CREATE UNIQUE INDEX userplatformquota_user_id_platform_uq
    ON user_platform_quotas (user_id, platform, (IF(deleted_at IS NULL, 0, NULL)));

CREATE INDEX userplatformquota_user_id
    ON user_platform_quotas (user_id);
