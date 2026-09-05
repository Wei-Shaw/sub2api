CREATE TABLE IF NOT EXISTS web3_identities (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    address VARCHAR(42) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web3_identities_address_format_check
        CHECK (address ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_identities_address_key UNIQUE (address)
);
