-- 027_usage_billing_consistency.sql
-- Ensure usage_logs idempotency (request_id, api_key_id) and add reconciliation infrastructure.

-- -----------------------------------------------------------------------------
-- 1) Normalize legacy request_id values
-- -----------------------------------------------------------------------------
-- Both statements below are one-shot cleanups whose only purpose is to let the
-- unique index in section 2 be created. They are also the most expensive thing
-- this migration does: a full-table UPDATE and a ROW_NUMBER() window over every
-- non-null request_id in usage_logs, inside the boot transaction. On a database
-- that already carries the index -- meaning 027 has effectively already run here
-- (a dump restored without schema_migrations, a schema-only clone, hand-applied
-- DDL) -- they scan and sort the whole table, spill to temp disk, and update
-- exactly zero rows; a statement timeout then throws all of it away and the
-- server never finishes booting.
--
-- So skip them when the index is already there. On a fresh install usage_logs is
-- empty and the guard costs a single catalog lookup. Section 2 keeps its own
-- IF NOT EXISTS, so re-running the file stays idempotent either way.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class idx
        JOIN pg_namespace ns ON ns.oid = idx.relnamespace
        WHERE ns.nspname = 'public'
          AND idx.relname = 'idx_usage_logs_request_id_api_key_unique'
    ) THEN
        RETURN;
    END IF;

    -- Historically request_id may be inserted as empty string. Convert it to NULL
    -- so the upcoming unique index does not break on repeated "" values.
    UPDATE usage_logs
    SET request_id = NULL
    WHERE request_id = '';

    -- If duplicates already exist for the same (request_id, api_key_id), keep the
    -- first row and NULL-out request_id for the rest so the unique index can be
    -- created without deleting historical logs.
    WITH ranked AS (
        SELECT
            id,
            ROW_NUMBER() OVER (PARTITION BY api_key_id, request_id ORDER BY id) AS rn
        FROM usage_logs
        WHERE request_id IS NOT NULL
    )
    UPDATE usage_logs ul
    SET request_id = NULL
    FROM ranked r
    WHERE ul.id = r.id
      AND r.rn > 1;
END
$$;

-- -----------------------------------------------------------------------------
-- 2) Idempotency constraint for usage_logs
-- -----------------------------------------------------------------------------
CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_logs_request_id_api_key_unique
    ON usage_logs (request_id, api_key_id);

-- -----------------------------------------------------------------------------
-- 3) Reconciliation infrastructure: billing ledger for usage charges
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS billing_usage_entries (
    id BIGSERIAL PRIMARY KEY,
    usage_log_id BIGINT NOT NULL REFERENCES usage_logs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    subscription_id BIGINT REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    billing_type SMALLINT NOT NULL,
    applied BOOLEAN NOT NULL DEFAULT TRUE,
    delta_usd DECIMAL(20, 10) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS billing_usage_entries_usage_log_id_unique
    ON billing_usage_entries (usage_log_id);

CREATE INDEX IF NOT EXISTS idx_billing_usage_entries_user_time
    ON billing_usage_entries (user_id, created_at);

CREATE INDEX IF NOT EXISTS idx_billing_usage_entries_created_at
    ON billing_usage_entries (created_at);

