-- 本次重构把 service_quota_rules 的 scope 表达从「单级枚举 + 纯过滤字段」改成「可组合过滤字段」：
--
-- 1. 把 target_mode 单枚举拆成两个正交维度：
--      - counter_mode (user | per_user | shared) —— 决定 Redis 计数 key 的分片方式
--      - is_fallback                             —— 是否为兜底规则（被同 limiter 的非兜底规则覆盖）
-- 2. 把 target_user_id 的单用户绑定改成 service_quota_rule_users 关联表，允许一条规则绑定多个用户。
-- 3. 删除 scope_level 列：它是一个强迫用户选单一级别的枚举，无法表达像
--      「分组 A 下的账号 X」「账号 X 全局」这类由多个 scope 字段组合出的链路级限制。
--    重构后 scope 表达完全由 platform / group_id / account_id / model_pattern 是否 NULL 决定。
--
-- 旧字段 → 新字段映射：
--   target_mode='user'     → counter_mode='user',     is_fallback=false, target_user_id 搬进关联表
--   target_mode='per_user' → counter_mode='per_user', is_fallback=false
--   target_mode='shared'   → counter_mode='shared',   is_fallback=false
--   target_mode='default'  → counter_mode='per_user', is_fallback=true
--
-- 幂等：
--   - 各 ALTER / CREATE 都带 IF [NOT] EXISTS
--   - 数据回填用 DO 块先判断旧列是否仍在，避免二次执行报错
--   - 线上环境 service_quota_enabled 默认 false，存量数据极少

ALTER TABLE service_quota_rules
    ADD COLUMN IF NOT EXISTS counter_mode varchar(16) NOT NULL DEFAULT 'per_user';

ALTER TABLE service_quota_rules
    ADD COLUMN IF NOT EXISTS is_fallback boolean NOT NULL DEFAULT false;

-- 回填 counter_mode / is_fallback（仅当旧 target_mode 列仍在时执行）。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'service_quota_rules' AND column_name = 'target_mode'
    ) THEN
        UPDATE service_quota_rules SET counter_mode = 'user',     is_fallback = false WHERE target_mode = 'user';
        UPDATE service_quota_rules SET counter_mode = 'per_user', is_fallback = false WHERE target_mode = 'per_user';
        UPDATE service_quota_rules SET counter_mode = 'shared',   is_fallback = false WHERE target_mode = 'shared';
        UPDATE service_quota_rules SET counter_mode = 'per_user', is_fallback = true  WHERE target_mode = 'default';
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS service_quota_rule_users (
    rule_id bigint NOT NULL REFERENCES service_quota_rules(id) ON DELETE CASCADE,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (rule_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_service_quota_rule_users_user ON service_quota_rule_users(user_id);

-- 旧 target_user_id 数据搬进关联表。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'service_quota_rules' AND column_name = 'target_user_id'
    ) THEN
        INSERT INTO service_quota_rule_users (rule_id, user_id)
        SELECT id, target_user_id FROM service_quota_rules WHERE target_user_id IS NOT NULL
        ON CONFLICT DO NOTHING;
    END IF;
END$$;

-- 换掉旧的 CHECK 约束。
ALTER TABLE service_quota_rules DROP CONSTRAINT IF EXISTS service_quota_rules_target_mode_check;
ALTER TABLE service_quota_rules DROP CONSTRAINT IF EXISTS service_quota_rules_user_mode_check;

ALTER TABLE service_quota_rules
    ADD CONSTRAINT service_quota_rules_counter_mode_check
        CHECK (counter_mode IN ('user','per_user','shared'));

-- 删除旧列（DROP COLUMN 会级联删掉依赖的索引 idx_service_quota_rules_target_user）。
ALTER TABLE service_quota_rules DROP COLUMN IF EXISTS target_mode;
ALTER TABLE service_quota_rules DROP COLUMN IF EXISTS target_user_id;
DROP INDEX IF EXISTS idx_service_quota_rules_target_user;

-- 重建唯一索引：去掉 scope_level；把新增的 counter_mode / is_fallback 纳入去重维度，
-- 这样「同 scope + 同 limiter + 同计数模式 + 同兜底」才算重复规则。
DROP INDEX IF EXISTS idx_service_quota_rules_unique_scope_type;

CREATE UNIQUE INDEX IF NOT EXISTS idx_service_quota_rules_unique_scope_type
ON service_quota_rules (
    COALESCE(platform, ''),
    COALESCE(group_id, 0),
    COALESCE(account_id, 0),
    COALESCE(model_pattern, ''),
    limiter_type,
    counter_mode,
    is_fallback
);

-- 删除 scope_level 列及其 CHECK 约束。
ALTER TABLE service_quota_rules DROP CONSTRAINT IF EXISTS service_quota_rules_scope_level_check;
ALTER TABLE service_quota_rules DROP COLUMN IF EXISTS scope_level;
