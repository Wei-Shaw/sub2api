-- Migration 134: service_quota_paths → 5 张维度关联表（AND-of-sets 语义）
--
-- 旧设计：service_quota_paths 一行 = 一条"组合路径"，5 字段（platform/channel/group/account/model_pattern）按 AND 命中。
--         一条规则可挂 M 条这样的路径，rule × paths × limiters 计数器维度爆炸，UI 也要做"M 条 5 字段编辑器"。
-- 新设计：每个维度独立成表，rule 命中条件 = 「每个非空维度都包含本次请求的对应字段」。
--         一条规则只对应 N 个 limiter × 1 套维度集合，UI 用 5 个 multi-select 即可。
--
-- 语义对照：
--   旧：path1=(anthropic, account=7) ∨ path2=(openai, account=8)   ← 两条独立组合
--   新：platforms={anthropic,openai} ∧ accounts={7,8}             ← 4 个组合都会命中（含 anthropic+8、openai+7）
-- 业务上单次请求 platform/account 是绑定的（账号只属于一个平台），AND-of-sets 落到现实里不会产生跨平台串台。
--
-- Redis counter key 由 v2:{rule_id}:{path_id}:{limiter_type}:{target} 升级到 v3:{rule_id}:{limiter_type}:{target}，
-- 旧 key 按 TTL 自然过期。
--
-- 数据迁移：把每条 path 的非空字段并入对应维度表（DISTINCT 去重）。
--          eg. 一条规则有 paths=[{platform:A, account:7}, {platform:B}] →
--              platforms={A,B}, accounts={7}
--
-- 幂等：通过判断 service_quota_rule_platforms 是否已存在来控制重复执行。

DO $migration$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = current_schema() AND table_name = 'service_quota_rule_platforms'
    ) THEN
        RAISE NOTICE 'Migration 134 already applied, skipping';
        RETURN;
    END IF;

    -- Step 1: 创建 5 张维度关联表
    CREATE TABLE service_quota_rule_platforms (
        rule_id  bigint      NOT NULL REFERENCES service_quota_rules(id) ON DELETE CASCADE,
        platform varchar(32) NOT NULL,
        PRIMARY KEY (rule_id, platform)
    );

    CREATE TABLE service_quota_rule_channels (
        rule_id    bigint NOT NULL REFERENCES service_quota_rules(id) ON DELETE CASCADE,
        channel_id bigint NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
        PRIMARY KEY (rule_id, channel_id)
    );
    CREATE INDEX idx_service_quota_rule_channels_channel ON service_quota_rule_channels(channel_id);

    CREATE TABLE service_quota_rule_groups (
        rule_id  bigint NOT NULL REFERENCES service_quota_rules(id) ON DELETE CASCADE,
        group_id bigint NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
        PRIMARY KEY (rule_id, group_id)
    );
    CREATE INDEX idx_service_quota_rule_groups_group ON service_quota_rule_groups(group_id);

    CREATE TABLE service_quota_rule_accounts (
        rule_id    bigint NOT NULL REFERENCES service_quota_rules(id) ON DELETE CASCADE,
        account_id bigint NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
        PRIMARY KEY (rule_id, account_id)
    );
    CREATE INDEX idx_service_quota_rule_accounts_account ON service_quota_rule_accounts(account_id);

    CREATE TABLE service_quota_rule_models (
        rule_id       bigint       NOT NULL REFERENCES service_quota_rules(id) ON DELETE CASCADE,
        model_pattern varchar(255) NOT NULL,
        PRIMARY KEY (rule_id, model_pattern)
    );

    -- Step 2: 从 service_quota_paths 把非空字段并入维度表
    INSERT INTO service_quota_rule_platforms (rule_id, platform)
    SELECT DISTINCT rule_id, platform FROM service_quota_paths WHERE platform IS NOT NULL;

    INSERT INTO service_quota_rule_channels (rule_id, channel_id)
    SELECT DISTINCT rule_id, channel_id FROM service_quota_paths WHERE channel_id IS NOT NULL;

    INSERT INTO service_quota_rule_groups (rule_id, group_id)
    SELECT DISTINCT rule_id, group_id FROM service_quota_paths WHERE group_id IS NOT NULL;

    INSERT INTO service_quota_rule_accounts (rule_id, account_id)
    SELECT DISTINCT rule_id, account_id FROM service_quota_paths WHERE account_id IS NOT NULL;

    INSERT INTO service_quota_rule_models (rule_id, model_pattern)
    SELECT DISTINCT rule_id, model_pattern FROM service_quota_paths WHERE model_pattern IS NOT NULL;

    -- Step 3: 删除旧表（service_quota_paths 已不再使用）
    DROP TABLE service_quota_paths;

    RAISE NOTICE 'Migration 134 completed: % rules with platforms / % channels / % groups / % accounts / % models',
        (SELECT count(DISTINCT rule_id) FROM service_quota_rule_platforms),
        (SELECT count(*) FROM service_quota_rule_channels),
        (SELECT count(*) FROM service_quota_rule_groups),
        (SELECT count(*) FROM service_quota_rule_accounts),
        (SELECT count(*) FROM service_quota_rule_models);
END
$migration$;
