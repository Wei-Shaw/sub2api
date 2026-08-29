-- Add optional user-facing pricing to OpenAI API key accounts.
--
-- NULL multiplier and NULL/empty pricing preserve the legacy billing path.
-- The service layer restricts these fields to OpenAI API key accounts.

ALTER TABLE IF EXISTS accounts
  ADD COLUMN IF NOT EXISTS user_billing_rate_multiplier DECIMAL(12,6);

ALTER TABLE IF EXISTS accounts
  ADD COLUMN IF NOT EXISTS user_billing_model_pricing JSONB;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'accounts_user_billing_rate_multiplier_positive'
      AND conrelid = 'accounts'::regclass
  ) THEN
    ALTER TABLE accounts
      ADD CONSTRAINT accounts_user_billing_rate_multiplier_positive
      CHECK (user_billing_rate_multiplier IS NULL OR user_billing_rate_multiplier > 0);
  END IF;
END
$$;
