-- MySQL 方言版本（翻译自 ../112_add_payment_order_provider_key_snapshot.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（迁移执行器保证单次执行）。
--   - CAST(id AS TEXT) → CAST(id AS CHAR)（MySQL 无 TEXT cast 类型；BIGINT→CHAR 与 PG
--     BIGINT→TEXT 同为无前导零的十进制字符串，比较语义一致）。

ALTER TABLE payment_orders ADD COLUMN provider_key VARCHAR(30) NULL;

UPDATE payment_orders
SET provider_key = (
    SELECT provider_key
    FROM payment_provider_instances
    WHERE CAST(id AS CHAR) = payment_orders.provider_instance_id
)
WHERE provider_key IS NULL
  AND provider_instance_id IS NOT NULL;
