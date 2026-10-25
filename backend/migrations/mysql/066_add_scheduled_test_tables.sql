-- MySQL 方言版本（翻译自 ../066_add_scheduled_test_tables.sql）
-- Scheduled account test plans and results
--
-- 说明：TEXT 列的 NOT NULL DEFAULT '' 使用表达式默认值（8.0.13+）；
--       PG 版部分索引 WHERE enabled = true（非唯一）→ 去掉 WHERE。

CREATE TABLE IF NOT EXISTS scheduled_test_plans (
    id              BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    account_id      BIGINT NOT NULL,
    model_id        VARCHAR(100) NOT NULL DEFAULT '',
    cron_expression VARCHAR(100) NOT NULL DEFAULT '*/30 * * * *',
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    max_results     INT NOT NULL DEFAULT 50,
    last_run_at     DATETIME(3) NULL,
    next_run_at     DATETIME(3) NULL,
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_scheduled_test_plans_account_id FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_stp_account_id ON scheduled_test_plans(account_id);
CREATE INDEX idx_stp_enabled_next_run ON scheduled_test_plans(enabled, next_run_at);

CREATE TABLE IF NOT EXISTS scheduled_test_results (
    id            BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    plan_id       BIGINT NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'success',
    response_text TEXT NOT NULL DEFAULT (''),
    error_message TEXT NOT NULL DEFAULT (''),
    latency_ms    BIGINT NOT NULL DEFAULT 0,
    started_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    finished_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_scheduled_test_results_plan_id FOREIGN KEY (plan_id) REFERENCES scheduled_test_plans(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_str_plan_created ON scheduled_test_results(plan_id, created_at DESC);
