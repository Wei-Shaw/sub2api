-- MySQL 方言版本（翻译自 ../101_add_payment_mode.sql）
-- （原语句均为 MySQL 兼容语法，原样保留；TRIM(BOTH ',' FROM x) 与 REPLACE 语义一致。）

-- Add payment_mode field to payment_provider_instances
-- Values: 'redirect' (hosted page redirect), 'api' (API call for QR/payurl), '' (default/N/A)
ALTER TABLE payment_provider_instances ADD COLUMN payment_mode VARCHAR(20) NOT NULL DEFAULT '';

-- Migrate existing data: easypay instances with 'easypay' in supported_types → redirect mode
-- Remove 'easypay' from supported_types and set payment_mode = 'redirect'
UPDATE payment_provider_instances
SET payment_mode = 'redirect',
    supported_types = TRIM(BOTH ',' FROM REPLACE(REPLACE(REPLACE(
      supported_types, 'easypay,', ''), ',easypay', ''), 'easypay', ''))
WHERE provider_key = 'easypay' AND supported_types LIKE '%easypay%';

-- EasyPay instances without 'easypay' in supported_types → api mode
UPDATE payment_provider_instances
SET payment_mode = 'api'
WHERE provider_key = 'easypay' AND payment_mode = '';
