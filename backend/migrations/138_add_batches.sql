-- 138_add_batches.sql
-- 新增批次管理功能：batches 表 + accounts.batch_id 列

CREATE TABLE IF NOT EXISTS batches (
    id            BIGSERIAL PRIMARY KEY,
    name          VARCHAR(100) NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    source        VARCHAR(50) NOT NULL DEFAULT 'manual',
    account_count INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS batch_id BIGINT REFERENCES batches(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_accounts_batch_id ON accounts(batch_id);
