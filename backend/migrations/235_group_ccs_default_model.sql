ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS ccs_default_model VARCHAR(200) NOT NULL DEFAULT '';

COMMENT ON COLUMN groups.ccs_default_model IS
    'Model ID used by the CCS one-click import for this group; an empty value uses the platform default';
