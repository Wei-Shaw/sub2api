-- M2: 为 groups 表添加分组级策略默认值字段。
-- 这些字段是账号级覆盖（account.Extra）与系统级默认（system settings）之间的中间层：
--   account > group > system
--
-- default_account_concurrency: 该分组内账号的并发默认值（0 = 继承系统）
-- default_account_rpm:         该分组内账号的 RPM 默认值（0 = 继承系统）
-- default_passthrough_profile: 该分组的透传 profile（'' = 继承系统）
-- default_429_cooldown_sec:    该分组的 429 冷却时长（0 = 继承系统）

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS default_account_concurrency INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS default_account_rpm INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS default_passthrough_profile VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS default_429_cooldown_sec INT NOT NULL DEFAULT 0;
