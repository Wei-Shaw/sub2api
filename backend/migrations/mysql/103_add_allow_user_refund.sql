-- MySQL 方言版本（翻译自 ../103_add_allow_user_refund.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（执行器保证单次执行）。

ALTER TABLE payment_provider_instances ADD COLUMN allow_user_refund BOOLEAN NOT NULL DEFAULT FALSE;
