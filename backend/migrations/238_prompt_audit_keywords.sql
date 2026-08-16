-- Persist the independent prompt-audit keyword decision without changing
-- existing AI audit events. Keep the column nullable so the previous
-- application can keep writing events during blue-green deployment.
ALTER TABLE prompt_audit_events
    ADD COLUMN IF NOT EXISTS matched_keyword VARCHAR(200);

-- Existing rows and old writers that omit the column stay compatible.
-- sub2api-managed-update: reviewed-compatible
UPDATE prompt_audit_events
SET matched_keyword = ''
WHERE matched_keyword IS NULL;

-- Old binaries omit the new column and therefore insert NULL. Normalize
-- that legacy write at the database boundary.
-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION sub2api_default_prompt_audit_matched_keyword()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.matched_keyword IS NULL THEN
        NEW.matched_keyword := '';
    END IF;
    RETURN NEW;
END;
$$;

-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE TRIGGER prompt_audit_events_default_matched_keyword
BEFORE INSERT OR UPDATE OF matched_keyword ON prompt_audit_events
FOR EACH ROW
EXECUTE FUNCTION sub2api_default_prompt_audit_matched_keyword();
