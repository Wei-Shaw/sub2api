ALTER TABLE prompt_audit_events
    ADD COLUMN IF NOT EXISTS audit_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_prompt_audit_events_metadata_json'
    ) THEN
        ALTER TABLE prompt_audit_events
            ADD CONSTRAINT chk_prompt_audit_events_metadata_json
            CHECK (jsonb_typeof(audit_metadata) = 'object');
    END IF;
END $$;
