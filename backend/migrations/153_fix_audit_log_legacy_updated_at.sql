UPDATE audit_logs
SET updated_at = created_at
WHERE session_scope = ''
  AND session_id = ''
  AND request_count = 1
  AND updated_at > created_at + INTERVAL '1 second';
