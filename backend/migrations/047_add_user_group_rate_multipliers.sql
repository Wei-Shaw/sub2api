-- 用户专属分组倍率表
-- 允许管理员为特定用户设置分组的专属计费倍率，覆盖分组默认倍率
CREATE TABLE IF NOT EXISTS user_group_rate_multipliers (
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id        BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    rate_multiplier DECIMAL(10,4) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, group_id)
);

-- 按 group_id 查询索引（删除分组时清理关联记录）
CREATE INDEX IF NOT EXISTS idx_user_group_rate_multipliers_group_id
    ON user_group_rate_multipliers(group_id);

COMMENT ON TABLE user_group_rate_multipliers IS '用户专属分组倍率配置';
COMMENT ON COLUMN user_group_rate_multipliers.user_id IS '用户ID';
COMMENT ON COLUMN user_group_rate_multipliers.group_id IS '分组ID';
COMMENT ON COLUMN user_group_rate_multipliers.rate_multiplier IS '专属计费倍率（覆盖分组默认倍率）';
