-- Preserve purchased plan display metadata independently of mutable plan settings.
-- Existing orders remain NULL because their historical name/currency is unknown.
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS subscription_plan_snapshot JSONB;
