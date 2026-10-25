-- MySQL 方言版本（翻译自 ../184_auth_cache_invalidation_outbox.sql）
-- Durable, transactionally-enqueued API-key auth cache invalidation.
-- cache_key is always SHA-256 hex; plaintext credentials never leave api_keys.
--
-- 翻译决策：
--   - PG 的 4 个触发器函数 + 4 个触发器展开为 9 个行级 MySQL 触发器（MySQL 单触发器
--     只能绑定一个事件，UPDATE/DELETE 拆分，_ins/_upd/_del 后缀；跨表 INSERT ... SELECT
--     在触发器内允许，修改目标为 outbox 表而非触发表本身）。
--   - `IS DISTINCT FROM` → `NOT (... <=> ...)`；sha256(key::bytea) → SHA2(`key`, 256)；
--     辅助函数 enqueue_auth_cache_invalidation（应用代码未直接调用）内联为 IF 守卫。
--   - 两个部分索引（WHERE claimed_at IS NULL / IS NOT NULL，均非唯一）按单文件指引以
--     IF() 函数键部分翻译（仅影响索引体积，不影响语义）。
--   - MySQL 的外键级联动作不触发子表触发器，且级联先于 AFTER 触发器执行：
--     users/groups 的 DELETE 触发器因此采用 BEFORE DELETE，硬删除父行时仍能读到
--     未被级联清空的 api_keys 行，入队结果与 PG 版等价。

CREATE TABLE IF NOT EXISTS auth_cache_invalidation_outbox (
    id             BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    cache_key      CHAR(64) NOT NULL,
    created_at     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    available_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    delivery_stage SMALLINT NOT NULL DEFAULT 0,
    attempts       INT NOT NULL DEFAULT 0,
    last_error     TEXT,
    claimed_at     DATETIME(3) NULL,
    claimed_by     TEXT,
    CONSTRAINT chk_auth_cache_invalidation_outbox_cache_key
        CHECK (REGEXP_LIKE(cache_key, '^[0-9a-f]{64}$')),
    CONSTRAINT chk_auth_cache_invalidation_outbox_delivery_stage
        CHECK (delivery_stage IN (0, 1)),
    CONSTRAINT chk_auth_cache_invalidation_outbox_attempts
        CHECK (attempts >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_auth_cache_invalidation_outbox_available
    ON auth_cache_invalidation_outbox (available_at, id, (IF(claimed_at IS NULL, 0, NULL)));
CREATE INDEX idx_auth_cache_invalidation_outbox_lease
    ON auth_cache_invalidation_outbox (claimed_at, (IF(claimed_at IS NOT NULL, 0, NULL)));
CREATE INDEX idx_auth_cache_invalidation_outbox_cache_key
    ON auth_cache_invalidation_outbox (cache_key);
CREATE INDEX idx_auth_cache_invalidation_outbox_created_at
    ON auth_cache_invalidation_outbox (created_at);

-- api_keys：快照相关字段变化 → 失效旧 key；软删除状态下换 key → 同时失效新 key。
DROP TRIGGER IF EXISTS trg_api_keys_auth_cache_invalidation_upd;
CREATE TRIGGER trg_api_keys_auth_cache_invalidation_upd
AFTER UPDATE ON api_keys FOR EACH ROW
BEGIN
    IF NOT (OLD.`key` <=> NEW.`key`)
       OR NOT (OLD.status <=> NEW.status)
       OR NOT (OLD.deleted_at <=> NEW.deleted_at)
       OR NOT (OLD.user_id <=> NEW.user_id)
       OR NOT (OLD.group_id <=> NEW.group_id)
       OR NOT (OLD.ip_whitelist <=> NEW.ip_whitelist)
       OR NOT (OLD.ip_blacklist <=> NEW.ip_blacklist)
       OR NOT (OLD.expires_at <=> NEW.expires_at) THEN
        IF OLD.`key` IS NOT NULL AND OLD.`key` <> '' THEN
            INSERT INTO auth_cache_invalidation_outbox (cache_key)
            VALUES (SHA2(OLD.`key`, 256));
        END IF;
        IF NEW.deleted_at IS NULL AND NOT (NEW.`key` <=> OLD.`key`) THEN
            IF NEW.`key` IS NOT NULL AND NEW.`key` <> '' THEN
                INSERT INTO auth_cache_invalidation_outbox (cache_key)
                VALUES (SHA2(NEW.`key`, 256));
            END IF;
        END IF;
    END IF;
END;

DROP TRIGGER IF EXISTS trg_api_keys_auth_cache_invalidation_del;
CREATE TRIGGER trg_api_keys_auth_cache_invalidation_del
AFTER DELETE ON api_keys FOR EACH ROW
BEGIN
    IF OLD.`key` IS NOT NULL AND OLD.`key` <> '' THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        VALUES (SHA2(OLD.`key`, 256));
    END IF;
END;

-- users：status/role/deleted_at 任一变化 → 失效该用户全部未删除 key。
DROP TRIGGER IF EXISTS trg_users_auth_cache_invalidation_upd;
CREATE TRIGGER trg_users_auth_cache_invalidation_upd
AFTER UPDATE ON users FOR EACH ROW
BEGIN
    IF NOT ((OLD.status <=> NEW.status)
            AND (OLD.role <=> NEW.role)
            AND (OLD.deleted_at <=> NEW.deleted_at)) THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT SHA2(k.`key`, 256)
        FROM api_keys AS k
        WHERE k.user_id = OLD.id
          AND k.deleted_at IS NULL
          AND k.`key` <> '';
    END IF;
END;

-- BEFORE DELETE：外键级联（api_keys 随用户删除）发生在 AFTER 触发器之前，
-- AFTER 阶段已读不到 key，故用 BEFORE 在级联前取出。
DROP TRIGGER IF EXISTS trg_users_auth_cache_invalidation_del;
CREATE TRIGGER trg_users_auth_cache_invalidation_del
BEFORE DELETE ON users FOR EACH ROW
BEGIN
    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT SHA2(k.`key`, 256)
    FROM api_keys AS k
    WHERE k.user_id = OLD.id
      AND k.deleted_at IS NULL
      AND k.`key` <> '';
END;

-- groups：status/is_exclusive/deleted_at 任一变化 → 失效该分组全部未删除 key。
DROP TRIGGER IF EXISTS trg_groups_auth_cache_invalidation_upd;
CREATE TRIGGER trg_groups_auth_cache_invalidation_upd
AFTER UPDATE ON `groups` FOR EACH ROW
BEGIN
    IF NOT ((OLD.status <=> NEW.status)
            AND (OLD.is_exclusive <=> NEW.is_exclusive)
            AND (OLD.deleted_at <=> NEW.deleted_at)) THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT SHA2(k.`key`, 256)
        FROM api_keys AS k
        WHERE k.group_id = OLD.id
          AND k.deleted_at IS NULL
          AND k.`key` <> '';
    END IF;
END;

-- BEFORE DELETE：外键级联（api_keys.group_id 置空、user_allowed_groups 级联删除）
-- 发生在 AFTER 触发器之前，AFTER 阶段已读不到该分组的 key，故用 BEFORE 在级联前取出。
DROP TRIGGER IF EXISTS trg_groups_auth_cache_invalidation_del;
CREATE TRIGGER trg_groups_auth_cache_invalidation_del
BEFORE DELETE ON `groups` FOR EACH ROW
BEGIN
    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT SHA2(k.`key`, 256)
    FROM api_keys AS k
    WHERE k.group_id = OLD.id
      AND k.deleted_at IS NULL
      AND k.`key` <> '';
END;

-- user_allowed_groups：仅当分组为专属（is_exclusive）时相关快照才含分组绑定，才需要失效。
DROP TRIGGER IF EXISTS trg_user_allowed_groups_auth_cache_invalidation_ins;
CREATE TRIGGER trg_user_allowed_groups_auth_cache_invalidation_ins
AFTER INSERT ON user_allowed_groups FOR EACH ROW
BEGIN
    IF EXISTS (
        SELECT 1 FROM `groups` g
        WHERE g.id = NEW.group_id AND g.is_exclusive = TRUE
    ) THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT SHA2(k.`key`, 256)
        FROM api_keys AS k
        WHERE k.user_id = NEW.user_id
          AND k.group_id = NEW.group_id
          AND k.deleted_at IS NULL
          AND k.`key` <> '';
    END IF;
END;

DROP TRIGGER IF EXISTS trg_user_allowed_groups_auth_cache_invalidation_upd;
CREATE TRIGGER trg_user_allowed_groups_auth_cache_invalidation_upd
AFTER UPDATE ON user_allowed_groups FOR EACH ROW
BEGIN
    IF NOT (OLD.user_id <=> NEW.user_id)
       OR NOT (OLD.group_id <=> NEW.group_id) THEN
        -- 旧绑定：专属分组则失效旧 (user, group) 下的 key
        IF EXISTS (
            SELECT 1 FROM `groups` g
            WHERE g.id = OLD.group_id AND g.is_exclusive = TRUE
        ) THEN
            INSERT INTO auth_cache_invalidation_outbox (cache_key)
            SELECT SHA2(k.`key`, 256)
            FROM api_keys AS k
            WHERE k.user_id = OLD.user_id
              AND k.group_id = OLD.group_id
              AND k.deleted_at IS NULL
              AND k.`key` <> '';
        END IF;
        -- 新绑定：专属分组则失效新 (user, group) 下的 key
        IF EXISTS (
            SELECT 1 FROM `groups` g
            WHERE g.id = NEW.group_id AND g.is_exclusive = TRUE
        ) THEN
            INSERT INTO auth_cache_invalidation_outbox (cache_key)
            SELECT SHA2(k.`key`, 256)
            FROM api_keys AS k
            WHERE k.user_id = NEW.user_id
              AND k.group_id = NEW.group_id
              AND k.deleted_at IS NULL
              AND k.`key` <> '';
        END IF;
    END IF;
    -- user_id/group_id 均未变化时 no-op（对应 PG 版 ELSIF TG_OP = 'UPDATE' THEN RETURN）
END;

DROP TRIGGER IF EXISTS trg_user_allowed_groups_auth_cache_invalidation_del;
CREATE TRIGGER trg_user_allowed_groups_auth_cache_invalidation_del
AFTER DELETE ON user_allowed_groups FOR EACH ROW
BEGIN
    IF EXISTS (
        SELECT 1 FROM `groups` g
        WHERE g.id = OLD.group_id AND g.is_exclusive = TRUE
    ) THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT SHA2(k.`key`, 256)
        FROM api_keys AS k
        WHERE k.user_id = OLD.user_id
          AND k.group_id = OLD.group_id
          AND k.deleted_at IS NULL
          AND k.`key` <> '';
    END IF;
END;

-- (comment skipped) COMMENT ON TABLE auth_cache_invalidation_outbox
