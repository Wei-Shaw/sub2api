-- MySQL 方言版本（翻译自 ../193_group_profit_control_auth_cache_invalidation.sql）
-- Profit-control fields are part of the API-key auth snapshot and gate the
-- scheduling admission filter; the profit threshold D additionally depends on
-- group pricing and peak-window fields. Extend the durable invalidation
-- trigger so out-of-band group edits (direct SQL, crash between update and
-- app-level invalidation) cannot leave cached snapshots using stale
-- profit-control inputs. Normal admin saves already invalidate via
-- InvalidateAuthCacheByGroupID; this trigger is the durable backstop. Based on
-- the latest function body from 186_group_auth_cache_image_generation.sql.
--
-- 翻译决策：同 186 —— DROP + CREATE 重建 `groups` 上的两个失效触发器，
-- 变更检测字段补齐 platform / subscription_type / rate_multiplier /
-- peak_rate_enabled / peak_start / peak_end / peak_rate_multiplier /
-- profit_control_enabled / profit_min_margin / profit_safety_buffer。

DROP TRIGGER IF EXISTS trg_groups_auth_cache_invalidation_upd;
CREATE TRIGGER trg_groups_auth_cache_invalidation_upd
AFTER UPDATE ON `groups` FOR EACH ROW
BEGIN
    IF NOT ((OLD.status <=> NEW.status)
            AND (OLD.is_exclusive <=> NEW.is_exclusive)
            AND (OLD.allow_image_generation <=> NEW.allow_image_generation)
            AND (OLD.platform <=> NEW.platform)
            AND (OLD.subscription_type <=> NEW.subscription_type)
            AND (OLD.rate_multiplier <=> NEW.rate_multiplier)
            AND (OLD.peak_rate_enabled <=> NEW.peak_rate_enabled)
            AND (OLD.peak_start <=> NEW.peak_start)
            AND (OLD.peak_end <=> NEW.peak_end)
            AND (OLD.peak_rate_multiplier <=> NEW.peak_rate_multiplier)
            AND (OLD.profit_control_enabled <=> NEW.profit_control_enabled)
            AND (OLD.profit_min_margin <=> NEW.profit_min_margin)
            AND (OLD.profit_safety_buffer <=> NEW.profit_safety_buffer)
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
