-- MySQL 方言版本（翻译自 ../117_add_payment_order_provider_snapshot.sql）
-- JSONB → JSON；可空列显式标注 NULL；ADD COLUMN IF NOT EXISTS → ADD COLUMN
-- （迁移执行器保证单次执行）。

ALTER TABLE payment_orders
ADD COLUMN provider_snapshot JSON NULL;
