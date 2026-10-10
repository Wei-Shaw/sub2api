-- Retention may delete the usage row without deleting the native settlement
-- receipt. settled_at and billing deduplication continue to prevent recharging.
ALTER TABLE muse_turns DROP CONSTRAINT IF EXISTS muse_turns_usage_log_id_fkey;
ALTER TABLE muse_turns ADD CONSTRAINT muse_turns_usage_log_id_fkey
    FOREIGN KEY (usage_log_id) REFERENCES usage_logs(id) ON DELETE SET NULL;

-- Durable balance reservations survive handler completion, Redis TTLs and
-- replica restarts. The existing native turn remains the settlement receipt.
ALTER TABLE muse_turns ADD COLUMN IF NOT EXISTS balance_hold NUMERIC(20,10) NOT NULL DEFAULT 0 CHECK (balance_hold >= 0);
UPDATE muse_turns SET balance_hold = COALESCE((billing_command->'command'->>'BalanceCost')::numeric, 0)
    WHERE billing_command IS NOT NULL AND settled_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_muse_turns_balance_hold ON muse_turns(owner_user_id)
    WHERE settled_at IS NULL AND balance_hold > 0 AND state NOT IN ('failed','cancelled','rejected');
