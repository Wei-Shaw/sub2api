-- Maintain the Anthropic scheduling window cost as bounded incremental state.
--
-- Existing rows are intentionally not backfilled during startup. The first read for
-- an account/window rebuilds that single state row from usage_logs while holding the
-- row lock; later inserts update it incrementally. This keeps deployment migrations
-- independent of usage_logs volume and turns cache misses into point reads.

CREATE TABLE IF NOT EXISTS account_window_cost_state (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    window_start TIMESTAMPTZ NOT NULL,
    standard_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    initialized BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE account_window_cost_state IS
    'Incremental standard-cost total for each account current scheduling window.';
COMMENT ON COLUMN account_window_cost_state.initialized IS
    'False means the row must be rebuilt once from usage_logs before it is authoritative.';

-- INSERT is the gateway hot path. A transition table folds every batch into at most
-- one upsert per account instead of running a row-level trigger for every usage log.
CREATE OR REPLACE FUNCTION accumulate_account_window_cost_after_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO account_window_cost_state (
        account_id,
        window_start,
        standard_cost,
        initialized,
        updated_at
    )
    WITH candidates AS (
        SELECT
            inserted.account_id,
            inserted.created_at,
            inserted.total_cost,
            CASE
                WHEN accounts.session_window_start IS NOT NULL
                    AND accounts.session_window_end IS NOT NULL
                    AND statement_timestamp() < accounts.session_window_end
                THEN accounts.session_window_start
                ELSE date_trunc('hour', statement_timestamp())
            END AS window_start
        FROM inserted_usage_logs AS inserted
        JOIN accounts ON accounts.id = inserted.account_id
        WHERE accounts.deleted_at IS NULL
            AND accounts.platform = 'anthropic'
            AND accounts.type IN ('oauth', 'setup-token')
            AND accounts.extra ? 'window_cost_limit'
    ), aggregated AS (
        SELECT
            account_id,
            window_start,
            SUM(total_cost) AS standard_cost
        FROM candidates
        WHERE created_at >= window_start
        GROUP BY account_id, window_start
    )
    SELECT
        account_id,
        window_start,
        standard_cost,
        FALSE,
        NOW()
    FROM aggregated
    ORDER BY account_id
    ON CONFLICT (account_id) DO UPDATE
    SET
        window_start = EXCLUDED.window_start,
        standard_cost = CASE
            WHEN account_window_cost_state.window_start = EXCLUDED.window_start
            THEN account_window_cost_state.standard_cost + EXCLUDED.standard_cost
            ELSE EXCLUDED.standard_cost
        END,
        initialized = CASE
            WHEN account_window_cost_state.window_start = EXCLUDED.window_start
            THEN account_window_cost_state.initialized
            ELSE FALSE
        END,
        updated_at = NOW();

    RETURN NULL;
END;
$$;

-- Enabling/disabling the limit or correcting a response-derived window changes
-- which source rows belong to the state. Invalidate once and let the reader rebuild
-- under lock instead of trying to migrate a possibly partial provisional total.
CREATE OR REPLACE FUNCTION invalidate_account_window_cost_on_account_update()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.session_window_start IS DISTINCT FROM NEW.session_window_start
        OR OLD.session_window_end IS DISTINCT FROM NEW.session_window_end
        OR OLD.platform IS DISTINCT FROM NEW.platform
        OR OLD.type IS DISTINCT FROM NEW.type
        OR (OLD.extra -> 'window_cost_limit') IS DISTINCT FROM (NEW.extra -> 'window_cost_limit')
    THEN
        UPDATE account_window_cost_state
        SET initialized = FALSE,
            updated_at = NOW()
        WHERE account_id = NEW.id;
    END IF;

    RETURN NEW;
END;
$$;

-- Deletes and updates are uncommon. Marking the affected current state stale is
-- simpler and safer than trying to reverse an amount that may predate activation;
-- the next read rebuilds it once under the same row lock used by first-time reads.
CREATE OR REPLACE FUNCTION invalidate_account_window_cost_state()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        UPDATE account_window_cost_state
        SET initialized = FALSE,
            updated_at = NOW()
        WHERE account_id = OLD.account_id
            AND OLD.created_at >= window_start;
        RETURN OLD;
    END IF;

    UPDATE account_window_cost_state
    SET initialized = FALSE,
        updated_at = NOW()
    WHERE account_id = OLD.account_id
        AND OLD.created_at >= window_start;

    IF NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
        OR NEW.total_cost IS DISTINCT FROM OLD.total_cost
    THEN
        UPDATE account_window_cost_state
        SET initialized = FALSE,
            updated_at = NOW()
        WHERE account_id = NEW.account_id
            AND NEW.created_at >= window_start;
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS usage_logs_account_window_cost_insert ON usage_logs;
CREATE TRIGGER usage_logs_account_window_cost_insert
AFTER INSERT ON usage_logs
REFERENCING NEW TABLE AS inserted_usage_logs
FOR EACH STATEMENT
EXECUTE FUNCTION accumulate_account_window_cost_after_insert();

DROP TRIGGER IF EXISTS usage_logs_account_window_cost_delete ON usage_logs;
CREATE TRIGGER usage_logs_account_window_cost_delete
AFTER DELETE ON usage_logs
FOR EACH ROW
WHEN (OLD.created_at >= CURRENT_TIMESTAMP - INTERVAL '6 hours')
EXECUTE FUNCTION invalidate_account_window_cost_state();

DROP TRIGGER IF EXISTS accounts_window_cost_state_invalidate ON accounts;
CREATE TRIGGER accounts_window_cost_state_invalidate
AFTER UPDATE OF platform, type, session_window_start, session_window_end, extra ON accounts
FOR EACH ROW
WHEN (
    OLD.session_window_start IS DISTINCT FROM NEW.session_window_start
    OR OLD.session_window_end IS DISTINCT FROM NEW.session_window_end
    OR OLD.platform IS DISTINCT FROM NEW.platform
    OR OLD.type IS DISTINCT FROM NEW.type
    OR (OLD.extra -> 'window_cost_limit') IS DISTINCT FROM (NEW.extra -> 'window_cost_limit')
)
EXECUTE FUNCTION invalidate_account_window_cost_on_account_update();

DROP TRIGGER IF EXISTS usage_logs_account_window_cost_update ON usage_logs;
CREATE TRIGGER usage_logs_account_window_cost_update
AFTER UPDATE OF account_id, created_at, total_cost ON usage_logs
FOR EACH ROW
WHEN (
    (
        OLD.account_id IS DISTINCT FROM NEW.account_id
        OR OLD.created_at IS DISTINCT FROM NEW.created_at
        OR OLD.total_cost IS DISTINCT FROM NEW.total_cost
    )
    AND (
        OLD.created_at >= CURRENT_TIMESTAMP - INTERVAL '6 hours'
        OR NEW.created_at >= CURRENT_TIMESTAMP - INTERVAL '6 hours'
    )
)
EXECUTE FUNCTION invalidate_account_window_cost_state();
