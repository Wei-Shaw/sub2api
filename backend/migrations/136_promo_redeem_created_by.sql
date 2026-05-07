-- Audit creator for marketing/admin generated promo and redeem codes.
-- Existing historical rows stay NULL because the original creator cannot be reliably reconstructed.

ALTER TABLE promo_codes
    ADD COLUMN IF NOT EXISTS created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_promo_codes_created_by
    ON promo_codes(created_by)
    WHERE created_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_redeem_codes_created_by
    ON redeem_codes(created_by)
    WHERE created_by IS NOT NULL;

COMMENT ON COLUMN promo_codes.created_by IS 'Admin/marketing user who created the promotion code; NULL for historical rows before audit tracking.';
COMMENT ON COLUMN redeem_codes.created_by IS 'Admin/marketing user who created the redeem code; NULL for historical rows before audit tracking.';
