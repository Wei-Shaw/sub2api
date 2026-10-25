-- MySQL 方言版本（翻译自 ../132_affiliate_custom_settings.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN。
--   - 部分索引 WHERE aff_code_custom = true OR aff_rebate_rate_percent IS NOT NULL
--     为非唯一部分索引 → 去掉 WHERE 子句（普通索引，仅性能差异）。
--   - COMMENT ON COLUMN 整句跳过。

-- 邀请返利：用户专属配置增强
-- 1) aff_rebate_rate_percent: 用户作为邀请人时的专属返利比例（百分比，NULL 表示沿用全局比例）
-- 2) aff_code_custom: 标记当前 aff_code 是否被管理员手动改写过（用于"专属用户"列表筛选）

ALTER TABLE user_affiliates
    ADD COLUMN aff_rebate_rate_percent DECIMAL(5,2) NULL;

ALTER TABLE user_affiliates
    ADD COLUMN aff_code_custom BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX idx_user_affiliates_admin_settings
    ON user_affiliates (updated_at);

-- (comment skipped)
-- (comment skipped)
