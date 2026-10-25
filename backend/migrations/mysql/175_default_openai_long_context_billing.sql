-- MySQL 方言版本（翻译自 ../175_default_openai_long_context_billing.sql）
-- Keep mixed-version writers consistent before backfilling rows that already exist.
--
-- 翻译决策：
--   - enforce 函数 → BEFORE INSERT / BEFORE UPDATE 两个行级触发器（MySQL 无 INSERT OR UPDATE、
--     无 UPDATE OF 列清单，任何 UPDATE 都会触发，由触发器体条件保证幂等）；RAISE EXCEPTION
--     → SIGNAL SQLSTATE '45000'；jsonb_set → JSON_SET；`?` → JSON_CONTAINS_PATH。
--   - PG 的 spark 影子分支会读取 accounts 本表（母账号行）强制继承取值；MySQL 触发器禁止读写
--     触发表自身（错误 1442），该分支退化为「UPDATE 时 NEW 缺键且 OLD 有布尔键则保留 OLD，
--     否则置 false」。母账号→影子的取值继承由应用层保证（CreateShadow 复制母账号取值、
--     批量设置按母账号下发），存量对齐由本文件末尾回填语句完成。
--   - PG 的 AFTER UPDATE 传播触发器在库内 UPDATE 影子行并为每个变化的影子入队；MySQL 因 1442
--     无法实现，退化为：母账号 platform/开关变化时对母账号入队 scheduler_outbox 的
--     account_changed 事件（跨实例快照失效）；影子行迁移与影子事件由文件末尾两条语句一次性回填。
--   - PG 的「UPDATE + RETURNING + 入队」CTE 拆为两条语句：先按同一变更谓词入队，再执行 UPDATE。

DROP TRIGGER IF EXISTS accounts_enforce_openai_long_context_billing_extra_ins;
CREATE TRIGGER accounts_enforce_openai_long_context_billing_extra_ins
BEFORE INSERT ON accounts FOR EACH ROW
BEGIN
    IF NEW.platform = 'openai' THEN
        IF NEW.extra IS NULL THEN
            SET NEW.extra = CAST('{}' AS JSON);
        END IF;
        IF NOT JSON_CONTAINS_PATH(NEW.extra, 'one', '$.openai_long_context_billing_enabled') THEN
            SET NEW.extra = JSON_SET(
                NEW.extra,
                '$.openai_long_context_billing_enabled',
                CAST('false' AS JSON)
            );
        END IF;
        IF JSON_TYPE(JSON_EXTRACT(NEW.extra, '$.openai_long_context_billing_enabled')) <> 'BOOLEAN' THEN
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'openai_long_context_billing_enabled must be a boolean';
        END IF;
    END IF;
END;

DROP TRIGGER IF EXISTS accounts_enforce_openai_long_context_billing_extra_upd;
CREATE TRIGGER accounts_enforce_openai_long_context_billing_extra_upd
BEFORE UPDATE ON accounts FOR EACH ROW
BEGIN
    IF NEW.platform = 'openai' THEN
        IF NEW.extra IS NULL THEN
            SET NEW.extra = CAST('{}' AS JSON);
        END IF;
        IF NOT JSON_CONTAINS_PATH(NEW.extra, 'one', '$.openai_long_context_billing_enabled') THEN
            IF OLD.platform = 'openai'
               AND JSON_TYPE(JSON_EXTRACT(COALESCE(OLD.extra, CAST('{}' AS JSON)), '$.openai_long_context_billing_enabled')) = 'BOOLEAN' THEN
                -- 保留 OLD 的既有布尔值（对应 PG 版 TG_OP = 'UPDATE' 分支）
                SET NEW.extra = JSON_SET(
                    NEW.extra,
                    '$.openai_long_context_billing_enabled',
                    JSON_EXTRACT(OLD.extra, '$.openai_long_context_billing_enabled')
                );
            ELSE
                SET NEW.extra = JSON_SET(
                    NEW.extra,
                    '$.openai_long_context_billing_enabled',
                    CAST('false' AS JSON)
                );
            END IF;
        END IF;
        IF JSON_TYPE(JSON_EXTRACT(NEW.extra, '$.openai_long_context_billing_enabled')) <> 'BOOLEAN' THEN
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'openai_long_context_billing_enabled must be a boolean';
        END IF;
    END IF;
END;

DROP TRIGGER IF EXISTS accounts_propagate_openai_long_context_billing_extra;
CREATE TRIGGER accounts_propagate_openai_long_context_billing_extra
AFTER UPDATE ON accounts FOR EACH ROW
BEGIN
    IF NEW.platform = 'openai'
       AND NEW.parent_account_id IS NULL
       AND (NOT (OLD.platform <=> NEW.platform)
            OR NOT (JSON_EXTRACT(OLD.extra, '$.openai_long_context_billing_enabled')
                    <=> JSON_EXTRACT(NEW.extra, '$.openai_long_context_billing_enabled'))) THEN
        INSERT INTO scheduler_outbox (event_type, account_id)
        VALUES ('account_changed', NEW.id);
    END IF;
END;

-- 回填 1：openai 账号上非布尔的存量值强制为 false。
UPDATE accounts
SET extra = JSON_SET(
    COALESCE(extra, CAST('{}' AS JSON)),
    '$.openai_long_context_billing_enabled',
    CAST('false' AS JSON)
)
WHERE platform = 'openai'
  AND JSON_CONTAINS_PATH(COALESCE(extra, CAST('{}' AS JSON)), 'one', '$.openai_long_context_billing_enabled')
  AND NOT (JSON_TYPE(JSON_EXTRACT(COALESCE(extra, CAST('{}' AS JSON)), '$.openai_long_context_billing_enabled')) <=> 'BOOLEAN');

-- 回填 2：缺键的 openai 母账号补 false。
UPDATE accounts
SET extra = JSON_SET(
    COALESCE(extra, CAST('{}' AS JSON)),
    '$.openai_long_context_billing_enabled',
    CAST('false' AS JSON)
)
WHERE platform = 'openai'
  AND parent_account_id IS NULL
  AND NOT JSON_CONTAINS_PATH(COALESCE(extra, CAST('{}' AS JSON)), 'one', '$.openai_long_context_billing_enabled');

-- 回填 3a：spark 影子的有效值与母账号不一致者入队 account_changed。
INSERT INTO scheduler_outbox (event_type, account_id)
SELECT 'account_changed', shadow.id
FROM accounts AS shadow
JOIN accounts AS parent ON parent.id = shadow.parent_account_id
WHERE shadow.platform = 'openai'
  AND shadow.quota_dimension = 'spark'
  AND NOT (JSON_EXTRACT(COALESCE(shadow.extra, CAST('{}' AS JSON)), '$.openai_long_context_billing_enabled') <=>
           CASE
               WHEN NOT (parent.platform <=> 'openai') THEN CAST('false' AS JSON)
               WHEN NOT JSON_CONTAINS_PATH(COALESCE(parent.extra, CAST('{}' AS JSON)), 'one', '$.openai_long_context_billing_enabled') THEN CAST('false' AS JSON)
               WHEN JSON_TYPE(JSON_EXTRACT(COALESCE(parent.extra, CAST('{}' AS JSON)), '$.openai_long_context_billing_enabled')) = 'BOOLEAN'
                   THEN JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled')
               ELSE CAST('false' AS JSON)
           END);

-- 回填 3b：spark 影子对齐母账号有效值。
UPDATE accounts AS shadow
JOIN accounts AS parent ON parent.id = shadow.parent_account_id
SET shadow.extra = JSON_SET(
    COALESCE(shadow.extra, CAST('{}' AS JSON)),
    '$.openai_long_context_billing_enabled',
    CASE
        WHEN NOT (parent.platform <=> 'openai') THEN CAST('false' AS JSON)
        WHEN NOT JSON_CONTAINS_PATH(COALESCE(parent.extra, CAST('{}' AS JSON)), 'one', '$.openai_long_context_billing_enabled') THEN CAST('false' AS JSON)
        WHEN JSON_TYPE(JSON_EXTRACT(COALESCE(parent.extra, CAST('{}' AS JSON)), '$.openai_long_context_billing_enabled')) = 'BOOLEAN'
            THEN JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled')
        ELSE CAST('false' AS JSON)
    END)
WHERE shadow.platform = 'openai'
  AND shadow.quota_dimension = 'spark'
  AND NOT (JSON_EXTRACT(COALESCE(shadow.extra, CAST('{}' AS JSON)), '$.openai_long_context_billing_enabled') <=>
           CASE
               WHEN NOT (parent.platform <=> 'openai') THEN CAST('false' AS JSON)
               WHEN NOT JSON_CONTAINS_PATH(COALESCE(parent.extra, CAST('{}' AS JSON)), 'one', '$.openai_long_context_billing_enabled') THEN CAST('false' AS JSON)
               WHEN JSON_TYPE(JSON_EXTRACT(COALESCE(parent.extra, CAST('{}' AS JSON)), '$.openai_long_context_billing_enabled')) = 'BOOLEAN'
                   THEN JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled')
               ELSE CAST('false' AS JSON)
           END);
