-- Batch account health checks with opt-in maintenance pool.

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS health_check_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS health_check_protected BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS health_check_fail_streak INT NOT NULL DEFAULT 0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_health_check_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_health_check_status VARCHAR(20);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_health_check_error TEXT;

CREATE INDEX IF NOT EXISTS idx_accounts_health_check_pool
    ON accounts(platform, status, schedulable, health_check_enabled, health_check_protected)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS account_batch_test_tasks (
    id                BIGSERIAL PRIMARY KEY,
    source            VARCHAR(20) NOT NULL DEFAULT 'manual',
    status            VARCHAR(20) NOT NULL DEFAULT 'pending',
    model_id          VARCHAR(100) NOT NULL DEFAULT 'gpt-5.4-mini',
    concurrency       INT NOT NULL DEFAULT 5,
    auto_disable      BOOLEAN NOT NULL DEFAULT false,
    total_count       INT NOT NULL DEFAULT 0,
    completed_count   INT NOT NULL DEFAULT 0,
    success_count     INT NOT NULL DEFAULT 0,
    failed_count      INT NOT NULL DEFAULT 0,
    deactivated_count INT NOT NULL DEFAULT 0,
    error_message     TEXT NOT NULL DEFAULT '',
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_batch_test_tasks_created
    ON account_batch_test_tasks(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_account_batch_test_tasks_status
    ON account_batch_test_tasks(status, created_at DESC);

CREATE TABLE IF NOT EXISTS account_batch_test_results (
    id                  BIGSERIAL PRIMARY KEY,
    task_id             BIGINT NOT NULL REFERENCES account_batch_test_tasks(id) ON DELETE CASCADE,
    account_id          BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    account_name        VARCHAR(100) NOT NULL DEFAULT '',
    platform            VARCHAR(50) NOT NULL DEFAULT '',
    account_type        VARCHAR(20) NOT NULL DEFAULT '',
    status              VARCHAR(20) NOT NULL DEFAULT 'failed',
    response_text       TEXT NOT NULL DEFAULT '',
    error_message       TEXT NOT NULL DEFAULT '',
    latency_ms          BIGINT NOT NULL DEFAULT 0,
    fail_streak         INT NOT NULL DEFAULT 0,
    triggered_disabled  BOOLEAN NOT NULL DEFAULT false,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_batch_test_results_task
    ON account_batch_test_results(task_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_account_batch_test_results_account
    ON account_batch_test_results(account_id, created_at DESC);
