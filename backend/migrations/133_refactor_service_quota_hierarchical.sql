-- Migration 133: Service Quota 扁平模型 → 层级模型
--
-- 旧模型：service_quota_rules 一行 = 一种 limiter + 一条 scope 路径。
--         多 limiter / 多路径需重复建行（用 batch_id 串起来），操作繁琐。
-- 新模型：一条规则 = N 个限流器 × M 条路径 × 用户绑定，每个 path×limiter 独立计数。
--   service_quota_rules        : 主表（共享属性 enabled/name/counter_mode/is_fallback）
--   service_quota_limiters     : 限流器子表 (rule_id, limiter_type, window_mode, limit_value)
--   service_quota_paths        : 路径子表 (rule_id, platform, channel_id, group_id, account_id, model_pattern)
--   service_quota_rule_users   : 用户绑定（保留）
--
-- 迁移策略：
--   1. 旧表重命名为 _old（保留可回滚，不删除）
--   2. 创建新表（结构如上）
--   3. 从 _old 迁移：
--        batch_id IS NULL  → 每行映射为 1 条新规则 + 1 个 limiter + 1 条 path
--        batch_id IS NOT NULL → 同 batch_id 归并为 1 条规则，limiters 取 DISTINCT(limiter_type)，
--                               paths 取每行 (platform, channel_id, group_id, account_id, model_pattern)
--
-- Redis 计数 key 在 service 层加 v2 前缀，旧 key 按 TTL 自然过期。
--
-- 幂等：通过判断新表 service_quota_limiters 是否已存在来控制重复执行。

DO $migration$
DECLARE
    new_rule_id bigint;
    rec_batch RECORD;
    rec_row   RECORD;
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = current_schema() AND table_name = 'service_quota_limiters'
    ) THEN
        RAISE NOTICE 'Migration 133 already applied, skipping';
        RETURN;
    END IF;

    -- Step 1: 旧表重命名为 _old，保留可回滚。
    -- service_quota_rule_users.rule_id 的 FK 仍然按 OID 引用旧表，rename 后自动跟随到 *_old。
    ALTER TABLE service_quota_rule_users RENAME TO service_quota_rule_users_old;
    ALTER TABLE service_quota_rules      RENAME TO service_quota_rules_old;

    -- 旧索引也跟着改名，避免和新建的索引同名冲突
    ALTER INDEX IF EXISTS idx_service_quota_rules_enabled        RENAME TO idx_service_quota_rules_old_enabled;
    ALTER INDEX IF EXISTS idx_service_quota_rules_unique         RENAME TO idx_service_quota_rules_old_unique;
    ALTER INDEX IF EXISTS idx_service_quota_rules_unique_scope_type RENAME TO idx_service_quota_rules_old_unique_scope_type;
    ALTER INDEX IF EXISTS idx_service_quota_rules_channel_id     RENAME TO idx_service_quota_rules_old_channel_id;
    ALTER INDEX IF EXISTS idx_service_quota_rules_batch_id       RENAME TO idx_service_quota_rules_old_batch_id;
    ALTER INDEX IF EXISTS idx_service_quota_rule_users_user      RENAME TO idx_service_quota_rule_users_old_user;

    -- Step 2: 创建新表
    CREATE TABLE service_quota_rules (
        id           BIGSERIAL PRIMARY KEY,
        enabled      boolean      NOT NULL DEFAULT true,
        name         varchar(128) NULL,
        counter_mode varchar(16)  NOT NULL DEFAULT 'per_user',
        is_fallback  boolean      NOT NULL DEFAULT false,
        created_at   timestamptz  NOT NULL DEFAULT now(),
        updated_at   timestamptz  NOT NULL DEFAULT now(),
        CONSTRAINT service_quota_rules_counter_mode_check
            CHECK (counter_mode IN ('user','per_user','shared'))
    );
    CREATE INDEX idx_service_quota_rules_enabled ON service_quota_rules(enabled);

    CREATE TABLE service_quota_limiters (
        id           BIGSERIAL PRIMARY KEY,
        rule_id      bigint        NOT NULL REFERENCES service_quota_rules(id) ON DELETE CASCADE,
        limiter_type varchar(32)   NOT NULL,
        window_mode  varchar(16)   NOT NULL DEFAULT 'fixed',
        limit_value  numeric(20,8) NOT NULL,
        CONSTRAINT service_quota_limiters_type_check
            CHECK (limiter_type IN ('rpm','tpm','tpd','daily_usd','concurrency')),
        CONSTRAINT service_quota_limiters_window_check
            CHECK (window_mode IN ('fixed','rolling')),
        CONSTRAINT service_quota_limiters_value_check
            CHECK (limit_value > 0),
        CONSTRAINT service_quota_limiters_unique UNIQUE (rule_id, limiter_type)
    );
    CREATE INDEX idx_service_quota_limiters_rule ON service_quota_limiters(rule_id);

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

    CREATE TABLE service_quota_rule_users (
        rule_id bigint NOT NULL REFERENCES service_quota_rules(id) ON DELETE CASCADE,
        user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        PRIMARY KEY (rule_id, user_id)
    );
    CREATE INDEX idx_service_quota_rule_users_user ON service_quota_rule_users(user_id);

    -- Step 3: 数据迁移
    -- 3a. batch 行（同 batch_id 归并为一条规则）
    FOR rec_batch IN
        SELECT
            batch_id,
            bool_and(enabled)             AS enabled,
            MIN(counter_mode)             AS counter_mode,
            bool_or(is_fallback)          AS is_fallback,
            MIN(created_at)               AS created_at,
            MAX(updated_at)               AS updated_at,
            'batch-' || batch_id::text    AS rule_name
        FROM service_quota_rules_old
        WHERE batch_id IS NOT NULL
        GROUP BY batch_id
    LOOP
        INSERT INTO service_quota_rules (enabled, name, counter_mode, is_fallback, created_at, updated_at)
        VALUES (rec_batch.enabled, rec_batch.rule_name, rec_batch.counter_mode, rec_batch.is_fallback,
                rec_batch.created_at, rec_batch.updated_at)
        RETURNING id INTO new_rule_id;

        -- 同 batch 内同 limiter_type 的旧行通常配置一致，DISTINCT ON 取最早一条
        INSERT INTO service_quota_limiters (rule_id, limiter_type, window_mode, limit_value)
        SELECT DISTINCT ON (limiter_type)
               new_rule_id, limiter_type, window_mode, limit_value
        FROM service_quota_rules_old
        WHERE batch_id = rec_batch.batch_id
        ORDER BY limiter_type, id;

        INSERT INTO service_quota_paths (rule_id, platform, channel_id, group_id, account_id, model_pattern)
        SELECT DISTINCT
               new_rule_id, platform, channel_id, group_id, account_id, model_pattern
        FROM service_quota_rules_old
        WHERE batch_id = rec_batch.batch_id;

        -- 用户绑定：从 batch 任一旧规则的 rule_users 取并集
        INSERT INTO service_quota_rule_users (rule_id, user_id)
        SELECT DISTINCT new_rule_id, ru.user_id
        FROM service_quota_rule_users_old ru
        JOIN service_quota_rules_old r ON r.id = ru.rule_id
        WHERE r.batch_id = rec_batch.batch_id;
    END LOOP;

    -- 3b. 非 batch 行（每行 → 1 条规则）
    FOR rec_row IN
        SELECT id, enabled, platform, channel_id, group_id, account_id, model_pattern,
               limiter_type, counter_mode, is_fallback, window_mode, limit_value,
               created_at, updated_at
        FROM service_quota_rules_old
        WHERE batch_id IS NULL
        ORDER BY id
    LOOP
        INSERT INTO service_quota_rules (enabled, counter_mode, is_fallback, created_at, updated_at)
        VALUES (rec_row.enabled, rec_row.counter_mode, rec_row.is_fallback,
                rec_row.created_at, rec_row.updated_at)
        RETURNING id INTO new_rule_id;

        INSERT INTO service_quota_limiters (rule_id, limiter_type, window_mode, limit_value)
        VALUES (new_rule_id, rec_row.limiter_type, rec_row.window_mode, rec_row.limit_value);

        INSERT INTO service_quota_paths (rule_id, platform, channel_id, group_id, account_id, model_pattern)
        VALUES (new_rule_id, rec_row.platform, rec_row.channel_id, rec_row.group_id,
                rec_row.account_id, rec_row.model_pattern);

        INSERT INTO service_quota_rule_users (rule_id, user_id)
        SELECT new_rule_id, user_id
        FROM service_quota_rule_users_old
        WHERE rule_id = rec_row.id;
    END LOOP;

    RAISE NOTICE 'Migration 133 completed: % rules migrated',
        (SELECT count(*) FROM service_quota_rules);
END
$migration$;
