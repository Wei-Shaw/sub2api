-- MySQL 方言版本（翻译自 ../133_affiliate_rebate_freeze.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN；TIMESTAMPTZ NULL → DATETIME(3) NULL。
--   - 部分索引 WHERE frozen_until IS NOT NULL（非唯一）→ 去掉 WHERE（普通索引，
--     MySQL 唯一/普通索引均不约束 NULL，此处为普通索引仅性能差异）。
--   - COMMENT ON COLUMN 整句跳过。

-- 1) Add frozen quota column to user_affiliates for rebate freeze period.
ALTER TABLE user_affiliates
    ADD COLUMN aff_frozen_quota DECIMAL(20,8) NOT NULL DEFAULT 0;

-- (comment skipped)

-- 2) Add frozen_until column to user_affiliate_ledger for per-entry freeze tracking.
-- NULL = no freeze (or already thawed); non-NULL = frozen until this timestamp.
ALTER TABLE user_affiliate_ledger
    ADD COLUMN frozen_until DATETIME(3) NULL;

-- (comment skipped)

-- 3) Partial index for efficient thaw queries (only rows still frozen).
CREATE INDEX idx_ual_frozen_thaw
    ON user_affiliate_ledger (user_id, frozen_until);
