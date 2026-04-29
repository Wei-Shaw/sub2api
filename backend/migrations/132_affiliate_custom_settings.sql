-- 邀请返利：用户专属配置增强
-- 1) aff_rebate_rate_percent: 用户作为邀请人时的专属返利比例（百分比，NULL 表示沿用全局比例）
-- 2) aff_code_custom: 标记当前 aff_code 是否被管理员手动改写过（用于"专属用户"列表筛选）

ALTER TABLE user_affiliates
    ADD COLUMN IF NOT EXISTS aff_rebate_rate_percent DECIMAL(5,2);

ALTER TABLE user_affiliates
    ADD COLUMN IF NOT EXISTS aff_code_custom BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_user_affiliates_admin_settings
    ON user_affiliates (updated_at)
    WHERE aff_code_custom = true OR aff_rebate_rate_percent IS NOT NULL;

COMMENT ON COLUMN user_affiliates.aff_rebate_rate_percent IS '专属返利比例（百分比 0-100，NULL 表示沿用全局）';
COMMENT ON COLUMN user_affiliates.aff_code_custom IS '邀请码是否由管理员改写过（用于专属用户筛选）';
