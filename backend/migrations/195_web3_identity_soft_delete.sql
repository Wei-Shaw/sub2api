ALTER TABLE web3_identities
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

UPDATE web3_identities wi
SET deleted_at = u.deleted_at
FROM users u
WHERE u.id = wi.user_id
  AND u.deleted_at IS NOT NULL
  AND wi.deleted_at IS NULL;

ALTER TABLE web3_identities
    DROP CONSTRAINT IF EXISTS web3_identities_address_key;

CREATE INDEX IF NOT EXISTS idx_web3_identities_deleted_at
    ON web3_identities (deleted_at);

CREATE UNIQUE INDEX IF NOT EXISTS web3_identities_active_address_key
    ON web3_identities (address)
    WHERE deleted_at IS NULL;
