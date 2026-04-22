-- 在已有的"用户专属分组倍率表"上扩展 rpm_override 列；同时放宽 rate_multiplier 为可空，
-- 使一行记录可以只覆盖 rate、只覆盖 rpm，或同时覆盖两者。
-- 语义：
--   - rate_multiplier NULL  → 该用户在此分组使用 groups.rate_multiplier 默认值
--   - rate_multiplier 非 NULL → 覆盖分组默认计费倍率
--   - rpm_override NULL     → 该用户在此分组使用 groups.rpm_limit 默认值
--   - rpm_override 非 NULL  → 覆盖分组默认 RPM（0 = 不限制）
-- 用户级 users.rpm_limit 仍独立生效（跨分组总配额）。
ALTER TABLE user_group_rate_multipliers
    ADD COLUMN IF NOT EXISTS rpm_override integer NULL;

ALTER TABLE user_group_rate_multipliers
    ALTER COLUMN rate_multiplier DROP NOT NULL;

COMMENT ON COLUMN user_group_rate_multipliers.rate_multiplier IS '专属计费倍率；NULL 表示沿用分组默认倍率。';
COMMENT ON COLUMN user_group_rate_multipliers.rpm_override IS '专属 RPM 上限；NULL 表示沿用分组默认；0 表示该用户在此分组不受 RPM 限制。';
