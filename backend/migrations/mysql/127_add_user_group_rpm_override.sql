-- MySQL 方言版本（翻译自 ../127_add_user_group_rpm_override.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN。
--   - ALTER COLUMN rate_multiplier DROP NOT NULL → MODIFY COLUMN rate_multiplier DECIMAL(10,4) NULL
--     （回溯原始定义 ../047_add_user_group_rate_multipliers.sql：DECIMAL(10,4) NOT NULL，无默认值）。
--   - COMMENT ON COLUMN 整句跳过。

-- 在已有的"用户专属分组倍率表"上扩展 rpm_override 列；同时放宽 rate_multiplier 为可空，
-- 使一行记录可以只覆盖 rate、只覆盖 rpm，或同时覆盖两者。
-- 语义：
--   - rate_multiplier NULL  → 该用户在此分组使用 groups.rate_multiplier 默认值
--   - rate_multiplier 非 NULL → 覆盖分组默认计费倍率
--   - rpm_override NULL     → 该用户在此分组使用 groups.rpm_limit 默认值
--   - rpm_override 非 NULL  → 覆盖分组默认 RPM（0 = 不限制）
-- 用户级 users.rpm_limit 仍独立生效（跨分组总配额）。
ALTER TABLE user_group_rate_multipliers
    ADD COLUMN rpm_override INT NULL;

ALTER TABLE user_group_rate_multipliers
    MODIFY COLUMN rate_multiplier DECIMAL(10,4) NULL;

-- (comment skipped)
-- (comment skipped)
