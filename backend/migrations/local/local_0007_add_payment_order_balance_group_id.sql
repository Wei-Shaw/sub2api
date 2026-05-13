-- Local migration for V-Claw/sub2api balance package semantics.
-- Keeps balance top-up usage-group metadata separate from subscription_group_id,
-- so balance packages do not look like subscription purchases.
ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS balance_group_id BIGINT NULL REFERENCES groups(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_payment_orders_balance_group_id ON payment_orders(balance_group_id);
