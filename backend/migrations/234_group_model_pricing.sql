-- Keep this expansion nullable and without a database default so the previous
-- application can continue creating groups during blue-green deployment and
-- image rollback. The trigger below preserves the historical enabled default
-- for old writers which omit the new column entirely.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS long_context_pricing_enabled BOOLEAN;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS model_pricing JSONB;

-- Existing groups must retain the pre-migration behavior (long-context pricing
-- was always enabled). This reviewed, idempotent backfill only changes NULLs.
-- sub2api-managed-update: reviewed-compatible
UPDATE groups
SET long_context_pricing_enabled = TRUE
WHERE long_context_pricing_enabled IS NULL;

-- Old binaries do not know about the new column and therefore insert NULL.
-- Normalize that legacy write at the database boundary; explicit FALSE from a
-- new binary remains untouched. This keeps mixed-version writes safe without a
-- table-rewriting NOT NULL/default alteration.
-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION sub2api_default_group_long_context_pricing_enabled()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.long_context_pricing_enabled IS NULL THEN
        NEW.long_context_pricing_enabled := TRUE;
    END IF;
    RETURN NEW;
END;
$$;

-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE TRIGGER groups_default_long_context_pricing_enabled
BEFORE INSERT OR UPDATE OF long_context_pricing_enabled ON groups
FOR EACH ROW
EXECUTE FUNCTION sub2api_default_group_long_context_pricing_enabled();
