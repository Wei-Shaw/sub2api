-- Migration 135: 5 维度关联表 → service_quota_paths（恢复多路径独立模型）
--
-- 134 把 paths 折叠成 5 维 AND-of-sets，"platforms={A,B} ∧ accounts={7,8}" 会笛卡尔积命中
-- (A,7)/(A,8)/(B,7)/(B,8) 4 种组合，无法表达"只想要 (A,7)∨(B,8) 这两条独立线路"。
-- 现在回滚回 187aa2d8 的多路径模型：每条 path 是 5 字段独立组合，路径之间互不关联，
-- N 个限流器 × M 条路径 = N×M 个独立计数上下文。
--
-- 数据迁移：5 维 AND 语义不能等价表达"M 条独立组合"，所以不做搬迁，直接 TRUNCATE 所有规则
-- 让用户重建。Redis 计数 key 在 service 层回到 v2:{rule_id}:{path_id}:{limiter_type}:{target}，
-- 旧 v3 key 按 TTL 自然过期。
--
-- 幂等：通过判断 service_quota_paths 是否已存在来控制重复执行。

DO $migration$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = current_schema() AND table_name = 'service_quota_paths'
    ) THEN
        RAISE NOTICE 'Migration 135 already applied, skipping';
        RETURN;
    END IF;

    -- Step 1: 清空所有规则数据。CASCADE 会同时清空所有 FK 引用 service_quota_rules 的子表
    -- （limiters / rule_users / rule_platforms / rule_channels / rule_groups / rule_accounts / rule_models 等）
    TRUNCATE service_quota_rules RESTART IDENTITY CASCADE;

    -- Step 2: DROP 5 张维度关联表
    DROP TABLE IF EXISTS service_quota_rule_platforms;
    DROP TABLE IF EXISTS service_quota_rule_channels;
    DROP TABLE IF EXISTS service_quota_rule_groups;
    DROP TABLE IF EXISTS service_quota_rule_accounts;
    DROP TABLE IF EXISTS service_quota_rule_models;

    -- Step 3: 重建 service_quota_paths（schema 同 133）
    CREATE TABLE service_quota_paths (
        id            BIGSERIAL PRIMARY KEY,
        rule_id       bigint       NOT NULL REFERENCES service_quota_rules(id) ON DELETE CASCADE,
        platform      varchar(32)  NULL,
        channel_id    bigint       NULL REFERENCES channels(id) ON DELETE CASCADE,
        group_id      bigint       NULL REFERENCES groups(id)   ON DELETE CASCADE,
        account_id    bigint       NULL REFERENCES accounts(id) ON DELETE CASCADE,
        model_pattern varchar(255) NULL
    );
    CREATE INDEX idx_service_quota_paths_rule ON service_quota_paths(rule_id);
    -- 同规则内路径不可重复（COALESCE 把 NULL 折叠为零值参与去重）
    CREATE UNIQUE INDEX idx_service_quota_paths_unique ON service_quota_paths (
        rule_id,
        COALESCE(platform, ''),
        COALESCE(channel_id, 0),
        COALESCE(group_id, 0),
        COALESCE(account_id, 0),
        COALESCE(model_pattern, '')
    );

    RAISE NOTICE 'Migration 135 completed: rules truncated, dimension tables dropped, service_quota_paths recreated';
END
$migration$;
