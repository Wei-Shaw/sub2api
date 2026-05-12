-- Managed users are internal customer accounts controlled by admins.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS customer_type VARCHAR(20) NOT NULL DEFAULT 'retail';

COMMENT ON COLUMN users.customer_type IS 'Customer type: retail or managed';

UPDATE users
SET customer_type = 'managed'
WHERE customer_type = 'retail'
  AND deleted_at IS NULL
  AND (
      LOWER(email) LIKE 'managed-key-%@managed.local'
      OR LOWER(notes) LIKE '%[managed-key]%'
  );

CREATE INDEX IF NOT EXISTS idx_users_customer_type
    ON users (customer_type)
    WHERE deleted_at IS NULL;

-- Internal managed keys can use dynamic IP locking and soft throttle actions.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS ip_lock_mode VARCHAR(30) NOT NULL DEFAULT 'off',
    ADD COLUMN IF NOT EXISTS limit_action VARCHAR(30) NOT NULL DEFAULT 'hard_block',
    ADD COLUMN IF NOT EXISTS rate_limit_1mo DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS usage_1mo DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS window_1mo_start TIMESTAMPTZ NULL;

COMMENT ON COLUMN api_keys.ip_lock_mode IS 'Dynamic IP lock mode: off or auto_single_ip';
COMMENT ON COLUMN api_keys.limit_action IS 'Rate limit action: hard_block or soft_throttle';
COMMENT ON COLUMN api_keys.rate_limit_1mo IS 'Rate limit in USD per 30 days (0 = unlimited)';
COMMENT ON COLUMN api_keys.usage_1mo IS 'Used amount in USD for the current 30d window';
COMMENT ON COLUMN api_keys.window_1mo_start IS 'Start time of the current 30d rate limit window';

CREATE INDEX IF NOT EXISTS idx_api_keys_ip_lock_mode
    ON api_keys (ip_lock_mode)
    WHERE deleted_at IS NULL;
