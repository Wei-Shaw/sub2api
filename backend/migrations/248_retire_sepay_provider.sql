-- SePay 网关已从代码中移除（由 GPM Pay 取代）。
--
-- 后端不再能为 provider_key = 'sepay' 的实例构建网关：继续启用会让负载均衡选中它，
-- 用户建单直接失败；ENABLED_PAYMENT_TYPES 里残留的 sepay_* 也会让充值页继续列出它。
-- 实例只停用不删除：历史订单通过 provider_instance_id 引用它们。

-- 清理前先快照，便于回滚以及事后核对。CREATE TABLE IF NOT EXISTS ... AS SELECT 重跑时是空操作。
CREATE TABLE IF NOT EXISTS payment_provider_instances_backup_248 AS
SELECT id,
       provider_key,
       name,
       enabled,
       supported_types,
       now() AS backed_up_at
FROM payment_provider_instances
WHERE provider_key = 'sepay';

COMMENT ON TABLE payment_provider_instances_backup_248 IS
    '迁移 248 停用 SePay 实例前的快照。确认无需回滚后可安全 DROP；回滚需同时回退移除 SePay 的代码。';

CREATE TABLE IF NOT EXISTS settings_payment_backup_248 AS
SELECT key,
       value,
       now() AS backed_up_at
FROM settings
WHERE key = 'ENABLED_PAYMENT_TYPES';

COMMENT ON TABLE settings_payment_backup_248 IS
    '迁移 248 清理 ENABLED_PAYMENT_TYPES 前的快照。确认无需回滚后可安全 DROP；回滚方式：UPDATE settings s SET value = b.value FROM settings_payment_backup_248 b WHERE s.key = b.key';

UPDATE payment_provider_instances
SET enabled = false
WHERE provider_key = 'sepay'
  AND enabled;

-- 逐项过滤而不是整串置空：同一列还装着 GPM Pay / NOWPayments 的方式。
UPDATE settings
SET value = (
        SELECT COALESCE(string_agg(t, ','), '')
        FROM unnest(string_to_array(value, ',')) AS t
        WHERE btrim(t) NOT LIKE 'sepay%'
          AND btrim(t) <> ''
    )
WHERE key = 'ENABLED_PAYMENT_TYPES'
  AND value LIKE '%sepay%';

-- 挂起中的 SePay 订单不动：网关已不可查询，到期后按未支付正常过期。
