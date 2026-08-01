-- Model/pricing catalog v3.1: additive control-plane schema.
--
-- This migration intentionally does not add foreign keys to usage_logs. The
-- request path remains legacy-authoritative during shadow rollout, and the
-- large append-only table may be backfilled/validated independently later.

CREATE TABLE catalog_models (
    id BIGSERIAL PRIMARY KEY,
    canonical_key TEXT NOT NULL,
    canonical_key_normalized TEXT NOT NULL UNIQUE,
    operator_state TEXT NOT NULL DEFAULT 'enabled'
        CHECK (operator_state IN ('enabled', 'disabled', 'retired')),
    operator_reason TEXT NULL,
    replacement_model_id BIGINT NULL REFERENCES catalog_models(id) ON DELETE SET NULL,
    operator_version BIGINT NOT NULL DEFAULT 1 CHECK (operator_version > 0),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_operator_change_at TIMESTAMPTZ NULL,
    retired_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_catalog_models_operator_state
    ON catalog_models (operator_state);

CREATE TABLE catalog_sync_runs (
    id BIGSERIAL PRIMARY KEY,
    source_set TEXT NOT NULL,
    trigger TEXT NOT NULL,
    actor_user_id BIGINT NULL,
    upstream_version TEXT NULL,
    upstream_etag TEXT NULL,
    upstream_hash TEXT NULL,
    normalized_hash TEXT NULL,
    normalizer_version TEXT NOT NULL,
    status TEXT NOT NULL
        CHECK (status IN ('running', 'staged', 'validated', 'published', 'rejected', 'failed')),
    source_count INTEGER NOT NULL DEFAULT 0 CHECK (source_count >= 0),
    normalized_count INTEGER NOT NULL DEFAULT 0 CHECK (normalized_count >= 0),
    added_count INTEGER NOT NULL DEFAULT 0 CHECK (added_count >= 0),
    changed_count INTEGER NOT NULL DEFAULT 0 CHECK (changed_count >= 0),
    missing_count INTEGER NOT NULL DEFAULT 0 CHECK (missing_count >= 0),
    invalid_count INTEGER NOT NULL DEFAULT 0 CHECK (invalid_count >= 0),
    validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_catalog_sync_runs_status_started_at
    ON catalog_sync_runs (status, started_at DESC);

CREATE TABLE catalog_revisions (
    id BIGSERIAL PRIMARY KEY,
    revision BIGINT NOT NULL UNIQUE CHECK (revision > 0),
    sync_run_id BIGINT NOT NULL REFERENCES catalog_sync_runs(id),
    normalized_hash TEXT NOT NULL UNIQUE,
    normalizer_version TEXT NOT NULL,
    state TEXT NOT NULL
        CHECK (state IN ('staged', 'validated', 'published', 'rejected')),
    model_count INTEGER NOT NULL CHECK (model_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    validated_at TIMESTAMPTZ NULL,
    published_at TIMESTAMPTZ NULL
);

CREATE TABLE catalog_model_revisions (
    id BIGSERIAL PRIMARY KEY,
    catalog_revision_id BIGINT NOT NULL REFERENCES catalog_revisions(id),
    model_id BIGINT NOT NULL REFERENCES catalog_models(id),
    source_state TEXT NOT NULL
        CHECK (source_state IN ('present', 'missing', 'invalid')),
    provider TEXT NOT NULL,
    platform TEXT NOT NULL,
    mode TEXT NOT NULL,
    capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
    context_window BIGINT NULL CHECK (context_window IS NULL OR context_window >= 0),
    max_output_tokens BIGINT NULL CHECK (max_output_tokens IS NULL OR max_output_tokens >= 0),
    pricing_schema_version INTEGER NOT NULL CHECK (pricing_schema_version > 0),
    pricing_json JSONB NULL,
    pricing_valid BOOLEAN NOT NULL DEFAULT FALSE,
    pricing_source TEXT NULL,
    source_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT catalog_model_revisions_revision_model_key
        UNIQUE (catalog_revision_id, model_id),
    CHECK (pricing_valid = FALSE OR pricing_json IS NOT NULL)
);

CREATE INDEX idx_catalog_model_revisions_revision
    ON catalog_model_revisions (catalog_revision_id);
CREATE INDEX idx_catalog_model_revisions_model
    ON catalog_model_revisions (model_id);
CREATE TABLE catalog_model_aliases (
    id BIGSERIAL PRIMARY KEY,
    alias_normalized TEXT NOT NULL,
    platform_scope TEXT NOT NULL DEFAULT '*',
    model_id BIGINT NOT NULL REFERENCES catalog_models(id),
    source TEXT NOT NULL,
    state TEXT NOT NULL
        CHECK (state IN ('active', 'retired')),
    introduced_revision_id BIGINT NULL REFERENCES catalog_revisions(id) ON DELETE SET NULL,
    retired_revision_id BIGINT NULL REFERENCES catalog_revisions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (platform_scope, alias_normalized)
);

CREATE INDEX idx_catalog_model_aliases_model_id
    ON catalog_model_aliases (model_id);

CREATE TABLE catalog_publications (
    id BIGSERIAL PRIMARY KEY,
    scope TEXT NOT NULL UNIQUE,
    active_revision_id BIGINT NOT NULL REFERENCES catalog_revisions(id),
    epoch BIGINT NOT NULL CHECK (epoch > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_catalog_publications_active_revision_id
    ON catalog_publications (active_revision_id);

CREATE TABLE catalog_lifecycle_audits (
    id BIGSERIAL PRIMARY KEY,
    model_id BIGINT NULL REFERENCES catalog_models(id) ON DELETE SET NULL,
    catalog_revision_id BIGINT NULL REFERENCES catalog_revisions(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_user_id BIGINT NULL,
    reason TEXT NULL,
    before_state JSONB NULL,
    after_state JSONB NOT NULL,
    request_id TEXT NULL,
    correlation_id TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_catalog_lifecycle_audits_model_created_at
    ON catalog_lifecycle_audits (model_id, created_at DESC);
CREATE INDEX idx_catalog_lifecycle_audits_revision_created_at
    ON catalog_lifecycle_audits (catalog_revision_id, created_at DESC);

CREATE TABLE catalog_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    scope TEXT NOT NULL,
    publication_epoch BIGINT NOT NULL CHECK (publication_epoch > 0),
    catalog_revision_id BIGINT NOT NULL REFERENCES catalog_revisions(id),
    model_id BIGINT NULL REFERENCES catalog_models(id) ON DELETE SET NULL,
    payload JSONB NULL,
    dedup_key TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_catalog_outbox_scope_epoch_id
    ON catalog_outbox (scope, publication_epoch, id);
CREATE UNIQUE INDEX idx_catalog_outbox_pending_dedup_key
    ON catalog_outbox (dedup_key)
    WHERE dedup_key IS NOT NULL;

ALTER TABLE usage_logs
    ADD COLUMN catalog_epoch BIGINT NULL,
    ADD COLUMN catalog_revision_id BIGINT NULL,
    ADD COLUMN requested_model_revision_id BIGINT NULL,
    ADD COLUMN effective_model_revision_id BIGINT NULL,
    ADD COLUMN pricing_source VARCHAR(64) NULL,
    ADD COLUMN pricing_snapshot JSONB NULL;

CREATE INDEX idx_usage_logs_catalog_revision_id
    ON usage_logs (catalog_revision_id)
    WHERE catalog_revision_id IS NOT NULL;
