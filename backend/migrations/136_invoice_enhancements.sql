-- 1. Invoice profile type (general / vat_special)
ALTER TABLE invoice_profiles
    ADD COLUMN IF NOT EXISTS invoice_type VARCHAR(32) NOT NULL DEFAULT 'general';

-- 2. Invoice request: refund coupling fields
ALTER TABLE invoice_requests
    ADD COLUMN IF NOT EXISTS has_refunded_orders BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS voided_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS voided_reason TEXT;

CREATE INDEX IF NOT EXISTS invoice_requests_has_refunded_orders_idx
    ON invoice_requests(has_refunded_orders)
    WHERE has_refunded_orders = true;

-- 3. User notifications (in-app inbox)
CREATE TABLE IF NOT EXISTS user_notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL,
    body TEXT,
    link VARCHAR(512),
    metadata JSONB,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS user_notifications_user_unread_idx
    ON user_notifications(user_id, read_at, created_at DESC);

CREATE INDEX IF NOT EXISTS user_notifications_user_created_idx
    ON user_notifications(user_id, created_at DESC);
