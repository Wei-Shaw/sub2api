-- Enterprise subscription orders.
--
-- A company owner can purchase a subscription plan (group) for the company via
-- the standard payment gateway flow, just like a personal subscription. When an
-- order carries organization_id, the fulfillment path assigns/extends an
-- organization_subscriptions row for that company instead of a personal
-- user_subscriptions row. NULL keeps the existing personal-order behavior.
ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS organization_id BIGINT;

-- Weak reference (no FK) to keep order audit history independent from company
-- lifecycle changes, mirroring the activity_id convention on this table.
CREATE INDEX IF NOT EXISTS idx_payment_orders_organization_id
    ON payment_orders(organization_id)
    WHERE organization_id IS NOT NULL;
