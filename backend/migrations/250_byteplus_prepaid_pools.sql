-- Durable provider funding. No reservation is released merely because its HTTP
-- request, cache entry or task deadline has expired.
CREATE TABLE IF NOT EXISTS byteplus_pack_scopes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT 'BytePlus prepaid funding',
    api_base TEXT NOT NULL,
    enforced BOOLEAN NOT NULL DEFAULT TRUE,
    safety_lock BOOLEAN NOT NULL DEFAULT FALSE,
    frozen BOOLEAN NOT NULL DEFAULT FALSE,
    frozen_reason TEXT NOT NULL DEFAULT '',
    reconcile_after TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Preserve balances from earlier private deployments of the transport gate.
ALTER TABLE byteplus_pack_scopes ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT 'BytePlus prepaid funding';
ALTER TABLE byteplus_pack_scopes ADD COLUMN IF NOT EXISTS enforced BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE byteplus_pack_scopes ADD COLUMN IF NOT EXISTS safety_lock BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE byteplus_pack_scopes ADD COLUMN IF NOT EXISTS reconcile_after TIMESTAMPTZ;
UPDATE byteplus_pack_scopes SET safety_lock=TRUE WHERE frozen_reason='provider usage exceeded the reserved ceiling';
-- Every provider key sharing a verified billing pool must be enrolled here.
-- Retaining aliases protects old keys after rotation without duplicating funds.
CREATE TABLE IF NOT EXISTS byteplus_pack_credentials (
    credential_hash TEXT PRIMARY KEY CHECK (length(credential_hash) = 64),
    scope_id TEXT NOT NULL REFERENCES byteplus_pack_scopes(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS byteplus_pack_credentials_scope_idx ON byteplus_pack_credentials(scope_id);
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='byteplus_pack_scopes' AND column_name='credential_hash') THEN
        EXECUTE 'INSERT INTO byteplus_pack_credentials(credential_hash,scope_id) SELECT credential_hash,id FROM byteplus_pack_scopes WHERE credential_hash IS NOT NULL ON CONFLICT(credential_hash) DO NOTHING';
        EXECUTE 'ALTER TABLE byteplus_pack_scopes ALTER COLUMN credential_hash DROP NOT NULL';
    END IF;
END $$;
CREATE TABLE IF NOT EXISTS byteplus_pack_bindings (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    scope_id TEXT NOT NULL REFERENCES byteplus_pack_scopes(id) ON DELETE RESTRICT
);
CREATE TABLE IF NOT EXISTS byteplus_pack_grants (
    id TEXT PRIMARY KEY,
    scope_id TEXT NOT NULL REFERENCES byteplus_pack_scopes(id) ON DELETE RESTRICT,
    model TEXT NOT NULL,
    -- One unit is 1/64 of a resource-pack token; all Ark deduction ratios are exact.
    capacity_units BIGINT NOT NULL CHECK (capacity_units >= 0),
    spent_units BIGINT NOT NULL DEFAULT 0 CHECK (spent_units >= 0),
    reserved_units BIGINT NOT NULL DEFAULT 0 CHECK (reserved_units >= 0),
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (spent_units + reserved_units <= capacity_units)
);
ALTER TABLE byteplus_pack_grants ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE byteplus_pack_grants DROP CONSTRAINT IF EXISTS byteplus_pack_grants_capacity_units_check;
ALTER TABLE byteplus_pack_grants ADD CONSTRAINT byteplus_pack_grants_capacity_units_check CHECK(capacity_units>=0);
CREATE INDEX IF NOT EXISTS byteplus_pack_grants_scope_model_idx
    ON byteplus_pack_grants(scope_id, model, expires_at);
CREATE TABLE IF NOT EXISTS byteplus_pack_reservations (
    id TEXT PRIMARY KEY,
    scope_id TEXT NOT NULL REFERENCES byteplus_pack_scopes(id) ON DELETE RESTRICT,
    model TEXT NOT NULL,
    ratio_units BIGINT NOT NULL CHECK (ratio_units > 0),
    reserved_units BIGINT NOT NULL CHECK (reserved_units > 0),
    actual_units BIGINT,
    task_id TEXT,
    state TEXT NOT NULL DEFAULT 'reserved'
        CHECK (state IN ('reserved','pending','uncertain','succeeded','failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_until TIMESTAMPTZ NOT NULL,
    UNIQUE(scope_id, task_id)
);
CREATE TABLE IF NOT EXISTS byteplus_pack_allocations (
    reservation_id TEXT NOT NULL REFERENCES byteplus_pack_reservations(id) ON DELETE RESTRICT,
    grant_id TEXT NOT NULL REFERENCES byteplus_pack_grants(id) ON DELETE RESTRICT,
    units BIGINT NOT NULL CHECK (units > 0),
    PRIMARY KEY (reservation_id, grant_id)
);

-- Keep account-level protection sticky when an enrolled key is cloned or
-- replaced, across every writer and replica, without a TTL policy cache.
CREATE OR REPLACE FUNCTION bind_byteplus_prepaid_account() RETURNS trigger AS $$
DECLARE pool TEXT;
BEGIN
    IF EXISTS(SELECT 1 FROM byteplus_pack_bindings b JOIN byteplus_pack_scopes s ON s.id=b.scope_id WHERE b.account_id=NEW.id AND s.enforced) THEN
        RETURN NEW;
    END IF;
    IF TG_OP='UPDATE' AND COALESCE(OLD.credentials->>'api_key','')<>'' THEN
        SELECT c.scope_id INTO pool FROM byteplus_pack_credentials c JOIN byteplus_pack_scopes s ON s.id=c.scope_id
        WHERE c.credential_hash=encode(sha256(convert_to(btrim(OLD.credentials->>'api_key'),'UTF8')),'hex') AND s.enforced;
    END IF;
    IF pool IS NULL AND COALESCE(NEW.credentials->>'api_key','')<>'' THEN
        SELECT c.scope_id INTO pool FROM byteplus_pack_credentials c JOIN byteplus_pack_scopes s ON s.id=c.scope_id
        WHERE c.credential_hash=encode(sha256(convert_to(btrim(NEW.credentials->>'api_key'),'UTF8')),'hex') AND s.enforced;
    END IF;
    IF pool IS NOT NULL THEN
        INSERT INTO byteplus_pack_bindings(account_id,scope_id) VALUES(NEW.id,pool)
        ON CONFLICT(account_id) DO UPDATE SET scope_id=EXCLUDED.scope_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS accounts_byteplus_prepaid_binding ON accounts;
CREATE TRIGGER accounts_byteplus_prepaid_binding AFTER INSERT OR UPDATE OF credentials ON accounts
FOR EACH ROW EXECUTE FUNCTION bind_byteplus_prepaid_account();
