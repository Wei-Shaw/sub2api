CREATE TABLE IF NOT EXISTS payment_channels (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT,
    name VARCHAR(100) NOT NULL,
    platform VARCHAR(50) NOT NULL DEFAULT 'claude',
    rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    description TEXT NOT NULL DEFAULT '',
    models TEXT NOT NULL DEFAULT '',
    features TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_payment_channels_group_id ON payment_channels(group_id);
CREATE INDEX IF NOT EXISTS idx_payment_channels_enabled ON payment_channels(enabled);
