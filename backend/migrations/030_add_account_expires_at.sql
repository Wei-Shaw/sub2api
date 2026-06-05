-- Add expires_at for account expiration configuration
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS expires_at timestamptz;
-- Document expires_at meaning
COMMENT ON COLUMN accounts.expires_at IS 'Account expiration time (NULL means no expiration).';
-- Add auto_pause_on_expired for account expiration scheduling control
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS auto_pause_on_expired boolean NOT NULL DEFAULT true;
-- Document auto_pause_on_expired meaning
COMMENT ON COLUMN accounts.auto_pause_on_expired IS 'Auto pause scheduling when account expires.';
-- Ensure existing accounts are enabled by default
UPDATE accounts SET auto_pause_on_expired = true;
