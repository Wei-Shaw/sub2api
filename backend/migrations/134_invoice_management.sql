CREATE TABLE IF NOT EXISTS invoice_profiles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    tax_number VARCHAR(64) NOT NULL,
    email VARCHAR(255) NOT NULL,
    address TEXT,
    phone VARCHAR(64),
    bank_name VARCHAR(255),
    bank_account VARCHAR(128),
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS invoice_profiles_user_id_idx ON invoice_profiles(user_id);
CREATE INDEX IF NOT EXISTS invoice_profiles_user_default_idx ON invoice_profiles(user_id, is_default);

CREATE TABLE IF NOT EXISTS invoice_requests (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id BIGINT REFERENCES invoice_profiles(id) ON DELETE SET NULL,
    serial_no VARCHAR(64) NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    profile_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    total_amount DECIMAL(20, 2) NOT NULL DEFAULT 0,
    reject_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS invoice_requests_user_id_idx ON invoice_requests(user_id);
CREATE INDEX IF NOT EXISTS invoice_requests_status_idx ON invoice_requests(status);
CREATE INDEX IF NOT EXISTS invoice_requests_created_at_idx ON invoice_requests(created_at);

CREATE TABLE IF NOT EXISTS invoice_request_orders (
    invoice_request_id BIGINT NOT NULL REFERENCES invoice_requests(id) ON DELETE CASCADE,
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE RESTRICT,
    PRIMARY KEY (invoice_request_id, payment_order_id)
);

CREATE INDEX IF NOT EXISTS invoice_request_orders_payment_order_id_idx ON invoice_request_orders(payment_order_id);
