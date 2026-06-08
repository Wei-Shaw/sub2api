-- Add optional expiry time for redeem codes themselves.
-- `validity_days` remains the subscription duration granted after redeeming.

ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_redeem_codes_expires_at
    ON redeem_codes (expires_at);
