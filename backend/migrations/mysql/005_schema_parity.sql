-- MySQL 方言版本（翻译自 ../005_schema_parity.sql）
-- Align SQL migrations with current GORM persistence models.
-- This file is designed to be safe on both fresh installs and existing databases.
--
-- 翻译说明：
--   - ADD COLUMN/CREATE INDEX IF NOT EXISTS → 去掉修饰词（执行器保证单次执行）；
--   - ALTER COLUMN key TYPE VARCHAR(128) → MODIFY COLUMN 补全原始定义
--     （001 中为 `key` VARCHAR(64) NOT NULL UNIQUE；MODIFY 不影响已存在的唯一索引，
--     故列定义只保留 NOT NULL，不加 UNIQUE 以免重复建索引）；
--   - `key` 为 MySQL 保留字，全程反引号；
--   - TEXT 列的 NOT NULL DEFAULT '' → 表达式默认值 DEFAULT ('')（MySQL 8.0.13+）。

-- users: add fields added after initial migration
ALTER TABLE users ADD COLUMN username VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN wechat VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN notes TEXT NOT NULL DEFAULT ('');

-- api_keys: allow longer keys (GORM model uses size:128)
ALTER TABLE api_keys MODIFY COLUMN `key` VARCHAR(128) NOT NULL;

-- accounts: scheduling and rate-limit fields used by repository queries
ALTER TABLE accounts ADD COLUMN schedulable BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE accounts ADD COLUMN rate_limited_at DATETIME(3) NULL;
ALTER TABLE accounts ADD COLUMN rate_limit_reset_at DATETIME(3) NULL;
ALTER TABLE accounts ADD COLUMN overload_until DATETIME(3) NULL;
ALTER TABLE accounts ADD COLUMN session_window_start DATETIME(3) NULL;
ALTER TABLE accounts ADD COLUMN session_window_end DATETIME(3) NULL;
ALTER TABLE accounts ADD COLUMN session_window_status VARCHAR(20) NULL;

CREATE INDEX idx_accounts_schedulable ON accounts(schedulable);
CREATE INDEX idx_accounts_rate_limited_at ON accounts(rate_limited_at);
CREATE INDEX idx_accounts_rate_limit_reset_at ON accounts(rate_limit_reset_at);
CREATE INDEX idx_accounts_overload_until ON accounts(overload_until);

-- redeem_codes: subscription redeem fields
-- （PG 内联 REFERENCES → 先加列/建索引，再显式命名 FK 约束）
ALTER TABLE redeem_codes ADD COLUMN group_id BIGINT NULL;
ALTER TABLE redeem_codes ADD COLUMN validity_days INT NOT NULL DEFAULT 30;
CREATE INDEX idx_redeem_codes_group_id ON redeem_codes(group_id);
ALTER TABLE redeem_codes ADD CONSTRAINT fk_redeem_codes_group_id
    FOREIGN KEY (group_id) REFERENCES `groups`(id) ON DELETE SET NULL;

-- usage_logs: billing type used by filters and stats
ALTER TABLE usage_logs ADD COLUMN billing_type SMALLINT NOT NULL DEFAULT 0;
CREATE INDEX idx_usage_logs_billing_type ON usage_logs(billing_type);

-- settings: key-value store
CREATE TABLE IF NOT EXISTS settings (
    id          BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `key`       VARCHAR(100) NOT NULL UNIQUE,
    value       TEXT NOT NULL,
    updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
