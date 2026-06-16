ALTER TABLE payment_provider_instances ADD COLUMN IF NOT EXISTS allow_user_refund BOOLEAN NOT NULL DEFAULT false;
