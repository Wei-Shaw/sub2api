-- MySQL 方言版本（翻译自 ../027_usage_billing_consistency.sql）
-- Ensure usage_logs idempotency (request_id, api_key_id) and add reconciliation infrastructure.
--
-- 说明：PG 版 CTE（WITH ranked AS ... UPDATE ... FROM）改写为
--       UPDATE ... JOIN (派生表) 的多表更新语法。

-- -----------------------------------------------------------------------------
-- 1) Normalize legacy request_id values
-- -----------------------------------------------------------------------------
-- Historically request_id may be inserted as empty string. Convert it to NULL so
-- the upcoming unique index does not break on repeated "" values.
UPDATE usage_logs
SET request_id = NULL
WHERE request_id = '';

-- If duplicates already exist for the same (request_id, api_key_id), keep the
-- first row and NULL-out request_id for the rest so the unique index can be
-- created without deleting historical logs.
UPDATE usage_logs ul
JOIN (
    SELECT
        id,
        ROW_NUMBER() OVER (PARTITION BY api_key_id, request_id ORDER BY id) AS rn
    FROM usage_logs
    WHERE request_id IS NOT NULL
) r ON ul.id = r.id
    AND r.rn > 1
SET ul.request_id = NULL;

-- -----------------------------------------------------------------------------
-- 2) Idempotency constraint for usage_logs
-- -----------------------------------------------------------------------------
-- request_id 可空；MySQL 唯一索引天然允许多个 NULL，语义与 PG 一致。
CREATE UNIQUE INDEX idx_usage_logs_request_id_api_key_unique
    ON usage_logs (request_id, api_key_id);

-- -----------------------------------------------------------------------------
-- 3) Reconciliation infrastructure: billing ledger for usage charges
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS billing_usage_entries (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    usage_log_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    subscription_id BIGINT NULL,
    billing_type SMALLINT NOT NULL,
    applied BOOLEAN NOT NULL DEFAULT TRUE,
    delta_usd DECIMAL(20, 10) NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_billing_usage_entries_usage_log_id FOREIGN KEY (usage_log_id) REFERENCES usage_logs(id) ON DELETE CASCADE,
    CONSTRAINT fk_billing_usage_entries_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_billing_usage_entries_api_key_id FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE,
    CONSTRAINT fk_billing_usage_entries_subscription_id FOREIGN KEY (subscription_id) REFERENCES user_subscriptions(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX billing_usage_entries_usage_log_id_unique
    ON billing_usage_entries (usage_log_id);

CREATE INDEX idx_billing_usage_entries_user_time
    ON billing_usage_entries (user_id, created_at);

CREATE INDEX idx_billing_usage_entries_created_at
    ON billing_usage_entries (created_at);
