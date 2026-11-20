-- Durable provider funding. No reservation is released merely because its HTTP
-- request, cache entry or task deadline has expired.
CREATE TABLE IF NOT EXISTS byteplus_pack_scopes (
    id TEXT PRIMARY KEY,
    credential_hash TEXT NOT NULL UNIQUE CHECK (length(credential_hash) = 64),
    api_base TEXT NOT NULL,
    frozen BOOLEAN NOT NULL DEFAULT FALSE,
    frozen_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS byteplus_pack_bindings (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    scope_id TEXT NOT NULL REFERENCES byteplus_pack_scopes(id) ON DELETE RESTRICT
);
CREATE TABLE IF NOT EXISTS byteplus_pack_grants (
    id TEXT PRIMARY KEY,
    scope_id TEXT NOT NULL REFERENCES byteplus_pack_scopes(id) ON DELETE RESTRICT,
    model TEXT NOT NULL,
    -- One unit is 1/64 of a resource-pack token; all Ark deduction ratios are exact.
    capacity_units BIGINT NOT NULL CHECK (capacity_units > 0),
    spent_units BIGINT NOT NULL DEFAULT 0 CHECK (spent_units >= 0),
    reserved_units BIGINT NOT NULL DEFAULT 0 CHECK (reserved_units >= 0),
    expires_at TIMESTAMPTZ NOT NULL,
    CHECK (spent_units + reserved_units <= capacity_units)
);
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
