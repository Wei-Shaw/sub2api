ALTER TABLE payment_orders
ADD COLUMN IF NOT EXISTS provider_snapshot JSONB;
