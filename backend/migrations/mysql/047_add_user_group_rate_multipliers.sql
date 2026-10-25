-- MySQL 方言版本（翻译自 ../047_add_user_group_rate_multipliers.sql）
-- 用户专属分组倍率表
-- 允许管理员为特定用户设置分组的专属计费倍率，覆盖分组默认倍率
CREATE TABLE IF NOT EXISTS user_group_rate_multipliers (
    user_id         BIGINT NOT NULL,
    group_id        BIGINT NOT NULL,
    rate_multiplier DECIMAL(10,4) NOT NULL,
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (user_id, group_id),
    CONSTRAINT fk_user_group_rate_multipliers_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_user_group_rate_multipliers_group_id FOREIGN KEY (group_id) REFERENCES `groups`(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 按 group_id 查询索引（删除分组时清理关联记录）
CREATE INDEX idx_user_group_rate_multipliers_group_id
    ON user_group_rate_multipliers(group_id);

-- (comment skipped: 表 user_group_rate_multipliers 注释)
-- (comment skipped: 列 user_group_rate_multipliers.user_id)
-- (comment skipped: 列 user_group_rate_multipliers.group_id)
-- (comment skipped: 列 user_group_rate_multipliers.rate_multiplier)
