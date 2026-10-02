-- 订单快照过去只给 SePay 记 currency，其余网关（如按 USD 计价的 NOWPayments）缺这个键，
-- PaymentOrderCurrency 便回落为 VND，后台统计把 USD 金额当 VND 展示。
-- 从下单实例的配置回填；只解析明文 JSON 配置（CASE 保证旧的加密配置不会被强转），
-- 已有 currency 的不动，可重入。
UPDATE payment_orders o
SET provider_snapshot = o.provider_snapshot || jsonb_build_object('currency', upper(c.currency))
FROM (
    SELECT id::text AS instance_id,
           CASE WHEN left(btrim(config), 1) = '{' THEN config::jsonb ->> 'currency' END AS currency
    FROM payment_provider_instances
    WHERE provider_key = 'nowpayments'
) c
WHERE o.provider_snapshot IS NOT NULL
  AND NOT (o.provider_snapshot ? 'currency')
  AND o.provider_key = 'nowpayments'
  AND o.provider_instance_id = c.instance_id
  AND c.currency ~ '^[A-Za-z]{3}$';
