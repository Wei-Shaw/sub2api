-- 支付服务商全量切换到 SePay（越南）。
--
-- 旧的服务商实现（易支付、支付宝、微信支付、Stripe、空中云汇）已从代码中移除，
-- 它们对应的 provider_instance 行现在既无法构造服务商，也无法处理回调：留着只会
-- 让下单在运行时报 "unknown provider key"。这里把它们停用并清理相关设置项。
--
-- 停用而不是删除：payment_orders 通过 provider_instance_id / provider_snapshot
-- 指向这些行，删除会让历史订单的服务商实例变成悬空引用，管理端订单详情随之报错。

-- 停用前先快照，便于回滚以及事后核对当时的配置。
-- CREATE TABLE IF NOT EXISTS ... AS SELECT 在重跑时是空操作，保持幂等。
CREATE TABLE IF NOT EXISTS payment_provider_instances_backup_240 AS
SELECT id,
       provider_key,
       name,
       supported_types,
       enabled,
       sort_order,
       now() AS backed_up_at
FROM payment_provider_instances
WHERE provider_key <> 'sepay';

COMMENT ON TABLE payment_provider_instances_backup_240 IS
    '迁移 240 停用非 SePay 支付服务商实例前的快照（不含 config 密文）。确认无需回滚后可安全 DROP；回滚方式：UPDATE payment_provider_instances p SET enabled = b.enabled FROM payment_provider_instances_backup_240 b WHERE p.id = b.id';

UPDATE payment_provider_instances
SET enabled = false
WHERE provider_key <> 'sepay'
  AND enabled = true;

-- 已启用的支付方式里若还留着旧网关的方式码，收银台会渲染出永远下不了单的按钮。
CREATE TABLE IF NOT EXISTS settings_payment_backup_240 AS
SELECT key,
       value,
       now() AS backed_up_at
FROM settings
WHERE key IN (
    'ENABLED_PAYMENT_TYPES',
    'ALIPAY_FORCE_QRCODE',
    'ALIPAY_MOBILE_PRECREATE_DEEP_LINK',
    'payment_visible_method_alipay_source',
    'payment_visible_method_wxpay_source',
    'payment_visible_method_alipay_enabled',
    'payment_visible_method_wxpay_enabled'
);

COMMENT ON TABLE settings_payment_backup_240 IS
    '迁移 240 清理旧支付服务商相关设置前的快照。确认无需回滚后可安全 DROP；回滚方式：UPDATE settings s SET value = b.value FROM settings_payment_backup_240 b WHERE s.key = b.key，已被移除的键从备份表 INSERT 补回';

UPDATE settings
SET value = ''
WHERE key = 'ENABLED_PAYMENT_TYPES'
  AND value <> '';

-- 这些设置项对应的功能（支付宝二维码强制、支付宝当面付深链、可见支付方式来源
-- 路由）已随旧服务商一并删除，后端不再读取，留在表里只会让管理端读到无意义的值。
DELETE FROM settings
WHERE key IN (
    'ALIPAY_FORCE_QRCODE',
    'ALIPAY_MOBILE_PRECREATE_DEEP_LINK',
    'payment_visible_method_alipay_source',
    'payment_visible_method_wxpay_source',
    'payment_visible_method_alipay_enabled',
    'payment_visible_method_wxpay_enabled'
);
