-- Reliability core foundations. This migration is additive only: it does not
-- charge balances, release reservations, or perform any network/asset work.

ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS api_key_id BIGINT,
    ADD COLUMN IF NOT EXISTS creation_key VARCHAR(64),
    ADD COLUMN IF NOT EXISTS creation_fingerprint VARCHAR(64),
    ADD COLUMN IF NOT EXISTS reservation_id BIGINT,
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS dispatch_state VARCHAR(24) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS settlement_status VARCHAR(24) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS archive_status VARCHAR(24) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS capture_status VARCHAR(24) NOT NULL DEFAULT 'pending';

CREATE TABLE IF NOT EXISTS billing_reservations (
    id BIGSERIAL PRIMARY KEY,
    reservation_key VARCHAR(128) NOT NULL,
    source_type VARCHAR(32) NOT NULL,
    source_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    api_key_id BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    reserved_amount_usd NUMERIC(20,10) NOT NULL,
    settled_amount_usd NUMERIC(20,10) NOT NULL DEFAULT 0,
    status VARCHAR(24) NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    settled_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS billing_transactions (
    id BIGSERIAL PRIMARY KEY,
    transaction_key VARCHAR(160) NOT NULL,
    source_type VARCHAR(32) NOT NULL,
    source_id BIGINT NOT NULL,
    transaction_kind VARCHAR(24) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    api_key_id BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    subscription_id BIGINT REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    reservation_id BIGINT REFERENCES billing_reservations(id) ON DELETE SET NULL,
    amount_original NUMERIC(20,10) NOT NULL,
    currency_original VARCHAR(3) NOT NULL,
    amount_usd NUMERIC(20,10) NOT NULL,
    exchange_rate NUMERIC(20,10) NOT NULL,
    exchange_rate_as_of TIMESTAMPTZ NOT NULL,
    pricing_source VARCHAR(64) NOT NULL,
    pricing_version VARCHAR(64) NOT NULL,
    balance_before NUMERIC(20,10) NOT NULL,
    balance_after NUMERIC(20,10) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS domain_outbox (
    id BIGSERIAL PRIMARY KEY,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id BIGINT NOT NULL,
    event_type VARCHAR(96) NOT NULL,
    dedup_key VARCHAR(192) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(24) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    locked_until TIMESTAMPTZ,
    locked_by VARCHAR(128),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'video_tasks_api_key_id_fkey'
          AND conrelid = 'video_tasks'::regclass
    ) THEN
        ALTER TABLE video_tasks
            ADD CONSTRAINT video_tasks_api_key_id_fkey
            FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'video_tasks_reservation_id_fkey'
          AND conrelid = 'video_tasks'::regclass
    ) THEN
        ALTER TABLE video_tasks
            ADD CONSTRAINT video_tasks_reservation_id_fkey
            FOREIGN KEY (reservation_id) REFERENCES billing_reservations(id) ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'video_tasks_version_check'
          AND conrelid = 'video_tasks'::regclass
    ) THEN
        ALTER TABLE video_tasks
            ADD CONSTRAINT video_tasks_version_check CHECK (version > 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'video_tasks_dispatch_state_check'
          AND conrelid = 'video_tasks'::regclass
    ) THEN
        ALTER TABLE video_tasks
            ADD CONSTRAINT video_tasks_dispatch_state_check
            CHECK (dispatch_state IN ('pending', 'dispatching', 'accepted', 'unknown', 'not_required'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'video_tasks_settlement_status_check'
          AND conrelid = 'video_tasks'::regclass
    ) THEN
        ALTER TABLE video_tasks
            ADD CONSTRAINT video_tasks_settlement_status_check
            CHECK (settlement_status IN ('pending', 'settled', 'released', 'not_required', 'error'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'video_tasks_archive_status_check'
          AND conrelid = 'video_tasks'::regclass
    ) THEN
        ALTER TABLE video_tasks
            ADD CONSTRAINT video_tasks_archive_status_check
            CHECK (archive_status IN ('pending', 'succeeded', 'failed', 'not_required'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'video_tasks_capture_status_check'
          AND conrelid = 'video_tasks'::regclass
    ) THEN
        ALTER TABLE video_tasks
            ADD CONSTRAINT video_tasks_capture_status_check
            CHECK (capture_status IN ('pending', 'succeeded', 'failed', 'not_required'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'billing_reservations_amounts_check'
          AND conrelid = 'billing_reservations'::regclass
    ) THEN
        ALTER TABLE billing_reservations
            ADD CONSTRAINT billing_reservations_amounts_check
            CHECK (reserved_amount_usd >= 0 AND settled_amount_usd >= 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'billing_reservations_status_check'
          AND conrelid = 'billing_reservations'::regclass
    ) THEN
        ALTER TABLE billing_reservations
            ADD CONSTRAINT billing_reservations_status_check
            CHECK (status IN ('active', 'settled', 'released', 'expired', 'review_required'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'billing_transactions_source_type_check'
          AND conrelid = 'billing_transactions'::regclass
    ) THEN
        ALTER TABLE billing_transactions
            ADD CONSTRAINT billing_transactions_source_type_check
            CHECK (source_type IN ('gateway_request', 'video_task', 'payment', 'refund'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'billing_transactions_kind_check'
          AND conrelid = 'billing_transactions'::regclass
    ) THEN
        ALTER TABLE billing_transactions
            ADD CONSTRAINT billing_transactions_kind_check
            CHECK (transaction_kind IN ('charge', 'release', 'refund', 'adjustment'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'billing_transactions_exchange_rate_check'
          AND conrelid = 'billing_transactions'::regclass
    ) THEN
        ALTER TABLE billing_transactions
            ADD CONSTRAINT billing_transactions_exchange_rate_check CHECK (exchange_rate > 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'domain_outbox_status_check'
          AND conrelid = 'domain_outbox'::regclass
    ) THEN
        ALTER TABLE domain_outbox
            ADD CONSTRAINT domain_outbox_status_check
            CHECK (status IN ('pending', 'processing', 'completed', 'dead'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'domain_outbox_attempt_count_check'
          AND conrelid = 'domain_outbox'::regclass
    ) THEN
        ALTER TABLE domain_outbox
            ADD CONSTRAINT domain_outbox_attempt_count_check CHECK (attempt_count >= 0);
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_tasks_creation_key
    ON video_tasks (creation_key)
    WHERE creation_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_video_tasks_api_key_created_at
    ON video_tasks (api_key_id, created_at DESC)
    WHERE api_key_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_video_tasks_reservation_id
    ON video_tasks (reservation_id)
    WHERE reservation_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_billing_reservations_reservation_key
    ON billing_reservations (reservation_key);
CREATE INDEX IF NOT EXISTS idx_billing_reservations_user_status_expires
    ON billing_reservations (user_id, status, expires_at);
CREATE INDEX IF NOT EXISTS idx_billing_reservations_source
    ON billing_reservations (source_type, source_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_billing_transactions_transaction_key
    ON billing_transactions (transaction_key);
CREATE INDEX IF NOT EXISTS idx_billing_transactions_source_created_at
    ON billing_transactions (source_type, source_id, created_at);
CREATE INDEX IF NOT EXISTS idx_billing_transactions_user_created_at
    ON billing_transactions (user_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uq_domain_outbox_dedup_key
    ON domain_outbox (dedup_key);
CREATE INDEX IF NOT EXISTS idx_domain_outbox_claim
    ON domain_outbox (status, next_attempt_at, id)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_domain_outbox_processing_lease
    ON domain_outbox (locked_until, id)
    WHERE status = 'processing';
CREATE INDEX IF NOT EXISTS idx_domain_outbox_aggregate
    ON domain_outbox (aggregate_type, aggregate_id, created_at);

-- Deterministic historical projections only. No financial or external side
-- effects are allowed during migration.
UPDATE video_tasks
SET settlement_status = 'settled'
WHERE status = 'succeeded'
  AND balance_charged_at IS NOT NULL
  AND settlement_status = 'pending';

UPDATE video_tasks
SET settlement_status = 'not_required'
WHERE status IN ('failed', 'cancelled')
  AND settlement_status = 'pending';

UPDATE video_tasks
SET archive_status = 'succeeded'
WHERE COALESCE(local_asset_path, '') <> ''
  AND archive_status = 'pending';
