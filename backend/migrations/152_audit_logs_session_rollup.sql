ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS session_id VARCHAR(255) DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS session_scope VARCHAR(512) DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS request_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE audit_logs SET updated_at = created_at WHERE updated_at IS NULL;

CREATE OR REPLACE FUNCTION append_audit_turns(existing TEXT, incoming TEXT)
RETURNS TEXT AS $$
DECLARE
    existing_json JSONB;
    incoming_json JSONB;
BEGIN
    BEGIN
        existing_json := COALESCE(NULLIF(existing, ''), '[]')::JSONB;
    EXCEPTION WHEN others THEN
        existing_json := jsonb_build_array(jsonb_build_object('legacy_content', COALESCE(existing, '')));
    END;

    BEGIN
        incoming_json := COALESCE(NULLIF(incoming, ''), '[]')::JSONB;
    EXCEPTION WHEN others THEN
        incoming_json := jsonb_build_array(jsonb_build_object('legacy_content', COALESCE(incoming, '')));
    END;

    IF jsonb_typeof(existing_json) <> 'array' THEN
        existing_json := jsonb_build_array(existing_json);
    END IF;
    IF jsonb_typeof(incoming_json) <> 'array' THEN
        incoming_json := jsonb_build_array(incoming_json);
    END IF;

    RETURN (existing_json || incoming_json)::TEXT;
END;
$$ LANGUAGE plpgsql;

CREATE INDEX IF NOT EXISTS idx_audit_logs_updated_at ON audit_logs (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_session_id ON audit_logs (session_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_logs_session_scope_uniq ON audit_logs (session_scope) WHERE session_scope <> '';
