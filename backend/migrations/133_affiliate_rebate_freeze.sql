-- 1) Add frozen quota column to user_affiliates for rebate freeze period.
ALTER TABLE user_affiliates
    ADD COLUMN IF NOT EXISTS aff_frozen_quota DECIMAL(20,8) NOT NULL DEFAULT 0;

COMMENT ON COLUMN user_affiliates.aff_frozen_quota IS 'Rebate quota currently frozen (pending thaw after freeze period)';

-- 2) Add frozen_until column to user_affiliate_ledger for per-entry freeze tracking.
-- NULL = no freeze (or already thawed); non-NULL = frozen until this timestamp.
ALTER TABLE user_affiliate_ledger
    ADD COLUMN IF NOT EXISTS frozen_until TIMESTAMPTZ NULL;

COMMENT ON COLUMN user_affiliate_ledger.frozen_until IS 'Rebate frozen until this time; NULL means already thawed or never frozen';

-- 3) Partial index for efficient thaw queries (only rows still frozen).
CREATE INDEX IF NOT EXISTS idx_ual_frozen_thaw
    ON user_affiliate_ledger (user_id, frozen_until)
    WHERE frozen_until IS NOT NULL;
