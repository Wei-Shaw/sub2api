CREATE TABLE IF NOT EXISTS balance_packages (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    label VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    amount_ledger DECIMAL(20,2) NOT NULL,
    credit_ledger DECIMAL(20,2) NOT NULL,
    bonus_ledger DECIMAL(20,2) NOT NULL DEFAULT 0,
    credit_multiplier DECIMAL(20,6) NOT NULL DEFAULT 1,
    badge VARCHAR(100) NOT NULL DEFAULT '',
    popular BOOLEAN NOT NULL DEFAULT FALSE,
    for_sale BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_balance_packages_code ON balance_packages(code);
CREATE INDEX IF NOT EXISTS idx_balance_packages_for_sale ON balance_packages(for_sale);
CREATE INDEX IF NOT EXISTS idx_balance_packages_sort_order ON balance_packages(sort_order);
