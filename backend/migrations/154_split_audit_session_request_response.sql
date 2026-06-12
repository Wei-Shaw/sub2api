CREATE OR REPLACE FUNCTION audit_session_request_content(existing TEXT)
RETURNS TEXT AS $$
DECLARE
    item JSONB;
    out JSONB := '[]'::JSONB;
BEGIN
    FOR item IN SELECT * FROM jsonb_array_elements(COALESCE(NULLIF(existing, ''), '[]')::JSONB)
    LOOP
        out := out || jsonb_build_array(jsonb_build_object(
            'request_id', item->>'request_id',
            'endpoint', item->>'endpoint',
            'method', item->>'method',
            'model', item->>'model',
            'status_code', COALESCE((item->>'status_code')::INT, 0),
            'content', COALESCE(item->>'request_body', item->>'content', ''),
            'truncated', COALESCE((item->>'request_truncated')::BOOLEAN, (item->>'truncated')::BOOLEAN, false),
            'duration_ms', COALESCE((item->>'duration_ms')::INT, 0),
            'created_at', COALESCE(item->>'created_at', '')
        ));
    END LOOP;
    RETURN out::TEXT;
EXCEPTION WHEN others THEN
    RETURN existing;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION audit_session_response_content(existing TEXT)
RETURNS TEXT AS $$
DECLARE
    item JSONB;
    out JSONB := '[]'::JSONB;
BEGIN
    FOR item IN SELECT * FROM jsonb_array_elements(COALESCE(NULLIF(existing, ''), '[]')::JSONB)
    LOOP
        out := out || jsonb_build_array(jsonb_build_object(
            'request_id', item->>'request_id',
            'endpoint', item->>'endpoint',
            'method', item->>'method',
            'model', item->>'model',
            'status_code', COALESCE((item->>'status_code')::INT, 0),
            'content', COALESCE(item->>'response_body', item->>'content', ''),
            'truncated', COALESCE((item->>'response_truncated')::BOOLEAN, (item->>'truncated')::BOOLEAN, false),
            'duration_ms', COALESCE((item->>'duration_ms')::INT, 0),
            'created_at', COALESCE(item->>'created_at', '')
        ));
    END LOOP;
    RETURN out::TEXT;
EXCEPTION WHEN others THEN
    RETURN existing;
END;
$$ LANGUAGE plpgsql;

UPDATE audit_logs
SET
    request_body = audit_session_request_content(request_body),
    response_body = audit_session_response_content(response_body)
WHERE session_scope <> ''
  AND request_body = response_body
  AND request_body LIKE '%"request_body"%'
  AND response_body LIKE '%"response_body"%';

DROP FUNCTION IF EXISTS audit_session_request_content(TEXT);
DROP FUNCTION IF EXISTS audit_session_response_content(TEXT);
