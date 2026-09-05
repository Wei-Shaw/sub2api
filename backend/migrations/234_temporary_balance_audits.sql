-- Immutable history for administrator-granted and automatically expired
-- temporary balances. The balance itself remains on users for fast billing;
-- this table is the audit trail used by the admin UI and compliance exports.
CREATE TABLE IF NOT EXISTS temporary_balance_audits (
    id BIGSERIAL PRIMARY KEY,
    -- Keep the ledger rows when an administrator attempts to delete a user;
    -- deletion is rejected while audit history exists (compliance retention).
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    actor_admin_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    operation VARCHAR(16) NOT NULL CHECK (operation IN ('grant', 'expire')),
    amount NUMERIC(20, 8) NOT NULL CHECK (amount >= 0),
    previous_balance NUMERIC(20, 8) NOT NULL CHECK (previous_balance >= 0),
    new_balance NUMERIC(20, 8) NOT NULL CHECK (new_balance >= 0),
    previous_expires_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_temporary_balance_audits_user_created
    ON temporary_balance_audits (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_temporary_balance_audits_created
    ON temporary_balance_audits (created_at DESC, id DESC);
