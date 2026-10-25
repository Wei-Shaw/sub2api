-- MySQL 方言版本（翻译自 ../174_add_usage_log_long_context_billing.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。

-- Snapshot whether long-context pricing changed token prices for a request so
-- usage history can explain the applied charge without inferring from totals.
ALTER TABLE usage_logs
    ADD COLUMN long_context_billing_applied BOOLEAN NOT NULL DEFAULT FALSE;
