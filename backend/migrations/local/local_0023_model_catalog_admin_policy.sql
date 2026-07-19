-- Revision-scoped operator policy for immutable model catalog publications.
-- The mutable catalog_models policy remains only the optimistic-concurrency control
-- row; historical active snapshots must read policy from catalog_model_revisions.
ALTER TABLE catalog_model_revisions
    ADD COLUMN IF NOT EXISTS operator_state VARCHAR(16) NOT NULL DEFAULT 'enabled',
    ADD COLUMN IF NOT EXISTS operator_reason TEXT,
    ADD COLUMN IF NOT EXISTS operator_version BIGINT NOT NULL DEFAULT 1;

UPDATE catalog_model_revisions cmr
SET operator_state = COALESCE(NULLIF(cm.operator_state, ''), 'enabled'),
    operator_reason = cm.operator_reason,
    operator_version = GREATEST(COALESCE(cm.operator_version, 1), 1)
FROM catalog_models cm
WHERE cm.id = cmr.model_id;

ALTER TABLE catalog_model_revisions
    DROP CONSTRAINT IF EXISTS catalog_model_revisions_operator_state_check;

ALTER TABLE catalog_model_revisions
    ADD CONSTRAINT catalog_model_revisions_operator_state_check
    CHECK (operator_state IN ('enabled', 'disabled', 'retired'));

ALTER TABLE catalog_model_revisions
    ADD CONSTRAINT catalog_model_revisions_operator_version_check
    CHECK (operator_version > 0);

CREATE INDEX IF NOT EXISTS catalog_model_revisions_operator_state_idx
    ON catalog_model_revisions (catalog_revision_id, operator_state);
