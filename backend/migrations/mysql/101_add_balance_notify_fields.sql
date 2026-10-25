-- MySQL 方言版本（翻译自 ../101_add_balance_notify_fields.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（执行器保证单次执行）；
--           TEXT NOT NULL DEFAULT '[]' → TEXT NOT NULL DEFAULT ('[]')（表达式默认值）。

-- Balance notification user preferences
ALTER TABLE users ADD COLUMN balance_notify_enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE users ADD COLUMN balance_notify_threshold DECIMAL(20,8) DEFAULT NULL;
ALTER TABLE users ADD COLUMN balance_notify_extra_emails TEXT NOT NULL DEFAULT ('[]');
