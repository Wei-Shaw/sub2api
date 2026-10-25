-- MySQL 方言版本（翻译自 ../102_add_balance_notify_threshold_type.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（执行器保证单次执行）。

-- Add threshold type support (fixed / percentage) to balance notification
ALTER TABLE users ADD COLUMN balance_notify_threshold_type VARCHAR(10) NOT NULL DEFAULT 'fixed';
-- Track cumulative recharge amount for percentage threshold calculation
ALTER TABLE users ADD COLUMN total_recharged DECIMAL(20,8) NOT NULL DEFAULT 0;
