-- MySQL 方言版本（翻译自 ../186_group_auth_cache_image_generation.sql）
-- Group image-generation permission is part of the API-key auth snapshot.
-- Extend the existing durable invalidation trigger without changing migration 184.
--
-- 翻译决策：PG 通过 CREATE OR REPLACE FUNCTION 热替换 groups 触发器函数体；
-- MySQL 触发器体是内联的，等价做法为 DROP + CREATE 重建 `groups` 上的两个失效触发器
-- （184 建立的其余触发器不受影响），变更检测字段增加 allow_image_generation。

DROP TRIGGER IF EXISTS trg_groups_auth_cache_invalidation_upd;
CREATE TRIGGER trg_groups_auth_cache_invalidation_upd
AFTER UPDATE ON `groups` FOR EACH ROW
BEGIN
    IF NOT ((OLD.status <=> NEW.status)
            AND (OLD.is_exclusive <=> NEW.is_exclusive)
            AND (OLD.allow_image_generation <=> NEW.allow_image_generation)
            AND (OLD.deleted_at <=> NEW.deleted_at)) THEN
        INSERT INTO auth_cache_invalidation_outbox (cache_key)
        SELECT SHA2(k.`key`, 256)
        FROM api_keys AS k
        WHERE k.group_id = OLD.id
          AND k.deleted_at IS NULL
          AND k.`key` <> '';
    END IF;
END;

-- BEFORE DELETE：外键级联先于 AFTER 触发器执行（同 184），故用 BEFORE 在级联前取出 key。
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
