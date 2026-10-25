-- MySQL 方言版本（翻译自 ../137_redeem_code_expires_at.sql）
-- 翻译说明：TIMESTAMPTZ → DATETIME(3) NULL；CREATE INDEX IF NOT EXISTS → CREATE INDEX
-- （IF NOT EXISTS 按迁移执行器单次执行语义去掉）。

-- Add optional expiry time for redeem codes themselves.
-- `validity_days` remains the subscription duration granted after redeeming.

ALTER TABLE redeem_codes
    ADD COLUMN expires_at DATETIME(3) NULL;

CREATE INDEX idx_redeem_codes_expires_at
    ON redeem_codes (expires_at);
