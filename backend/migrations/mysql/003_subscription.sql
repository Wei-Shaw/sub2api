-- MySQL 方言版本（翻译自 ../003_subscription.sql）
-- Sub2API 订阅功能迁移脚本
-- 添加订阅分组和用户订阅功能
--
-- 翻译说明：
--   - ADD COLUMN/CREATE INDEX IF NOT EXISTS → 去掉修饰词（执行器保证单次执行）；
--   - BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；
--   - TIMESTAMPTZ → DATETIME(3)，DEFAULT NOW() → DEFAULT CURRENT_TIMESTAMP(3)；
--   - 内联 REFERENCES → 显式命名 FK 约束（fk_<表>_<列>）；
--   - 内联 UNIQUE(user_id, group_id) 保持不命名：MySQL 索引名默认取首列 `user_id`，
--     后续 016 迁移按 `DROP INDEX user_id` 处理，故不得显式重命名。

-- 1. 扩展 groups 表添加订阅相关字段（`groups` 为 MySQL 8 保留字）
ALTER TABLE `groups` ADD COLUMN platform VARCHAR(50) NOT NULL DEFAULT 'anthropic';
ALTER TABLE `groups` ADD COLUMN subscription_type VARCHAR(20) NOT NULL DEFAULT 'standard';
ALTER TABLE `groups` ADD COLUMN daily_limit_usd DECIMAL(20, 8) DEFAULT NULL;
ALTER TABLE `groups` ADD COLUMN weekly_limit_usd DECIMAL(20, 8) DEFAULT NULL;
ALTER TABLE `groups` ADD COLUMN monthly_limit_usd DECIMAL(20, 8) DEFAULT NULL;
ALTER TABLE `groups` ADD COLUMN default_validity_days INT NOT NULL DEFAULT 30;

-- 添加索引
CREATE INDEX idx_groups_platform ON `groups`(platform);
CREATE INDEX idx_groups_subscription_type ON `groups`(subscription_type);

-- 2. 创建 user_subscriptions 用户订阅表
CREATE TABLE IF NOT EXISTS user_subscriptions (
    id                      BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id                 BIGINT NOT NULL,
    group_id                BIGINT NOT NULL,

    -- 订阅有效期
    starts_at               DATETIME(3) NOT NULL,
    expires_at              DATETIME(3) NOT NULL,
    status                  VARCHAR(20) NOT NULL DEFAULT 'active',  -- active/expired/suspended

    -- 滑动窗口起始时间（NULL=未激活）
    daily_window_start      DATETIME(3) NULL,
    weekly_window_start     DATETIME(3) NULL,
    monthly_window_start    DATETIME(3) NULL,

    -- 当前窗口已用额度（USD，基于 total_cost 计算）
    daily_usage_usd         DECIMAL(20, 10) NOT NULL DEFAULT 0,
    weekly_usage_usd        DECIMAL(20, 10) NOT NULL DEFAULT 0,
    monthly_usage_usd       DECIMAL(20, 10) NOT NULL DEFAULT 0,

    -- 管理员分配信息
    assigned_by             BIGINT NULL,
    assigned_at             DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    notes                   TEXT,

    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    -- 唯一约束：每个用户对每个分组只能有一个订阅
    -- （不命名，MySQL 索引名默认为首列名 user_id，与 016 的 DROP INDEX user_id 对齐）
    UNIQUE (user_id, group_id),
    CONSTRAINT fk_user_subscriptions_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_user_subscriptions_group_id FOREIGN KEY (group_id) REFERENCES `groups`(id) ON DELETE CASCADE,
    CONSTRAINT fk_user_subscriptions_assigned_by FOREIGN KEY (assigned_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- user_subscriptions 索引
CREATE INDEX idx_user_subscriptions_user_id ON user_subscriptions(user_id);
CREATE INDEX idx_user_subscriptions_group_id ON user_subscriptions(group_id);
CREATE INDEX idx_user_subscriptions_status ON user_subscriptions(status);
CREATE INDEX idx_user_subscriptions_expires_at ON user_subscriptions(expires_at);
CREATE INDEX idx_user_subscriptions_assigned_by ON user_subscriptions(assigned_by);

-- 3. 扩展 usage_logs 表添加分组和订阅关联
-- （PG 内联 REFERENCES → 先加列/建索引，再显式命名 FK 约束，InnoDB 复用已有索引）
ALTER TABLE usage_logs ADD COLUMN group_id BIGINT NULL;
ALTER TABLE usage_logs ADD COLUMN subscription_id BIGINT NULL;
ALTER TABLE usage_logs ADD COLUMN rate_multiplier DECIMAL(10, 4) NOT NULL DEFAULT 1;
ALTER TABLE usage_logs ADD COLUMN first_token_ms INT NULL;

-- usage_logs 新索引
CREATE INDEX idx_usage_logs_group_id ON usage_logs(group_id);
CREATE INDEX idx_usage_logs_subscription_id ON usage_logs(subscription_id);
CREATE INDEX idx_usage_logs_sub_created ON usage_logs(subscription_id, created_at);

ALTER TABLE usage_logs ADD CONSTRAINT fk_usage_logs_group_id
    FOREIGN KEY (group_id) REFERENCES `groups`(id) ON DELETE SET NULL;
ALTER TABLE usage_logs ADD CONSTRAINT fk_usage_logs_subscription_id
    FOREIGN KEY (subscription_id) REFERENCES user_subscriptions(id) ON DELETE SET NULL;
