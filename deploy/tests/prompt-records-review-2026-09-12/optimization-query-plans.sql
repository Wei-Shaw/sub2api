\set ON_ERROR_STOP on
\timing on
-- Run only in the disposable prompt_record_optimization_test database.
INSERT INTO prompt_records (request_id, user_id, api_key_id, model, stage, prompt_hash, prompt_text, request_body, created_at)
SELECT 'synthetic-' || n, n % 100 + 1, n % 1000 + 1,
       CASE WHEN n % 20 = 0 THEN 'synthetic-target-model' ELSE 'synthetic-common-model' END,
       'http', md5(n::text) || md5(n::text), repeat('synthetic text ', 8), '{"input":"synthetic text"}',
       TIMESTAMPTZ '2026-09-12 12:00:00+00' - n * INTERVAL '1 second'
FROM generate_series(1, 200000) AS n;
VACUUM ANALYZE prompt_records;
EXPLAIN (ANALYZE, BUFFERS) SELECT COUNT(*) FROM prompt_records
WHERE (expires_at IS NULL OR expires_at > NOW()) AND model ILIKE '%target%';
SELECT created_at AS cursor_created_at, id AS cursor_id FROM prompt_records
ORDER BY created_at DESC, id DESC OFFSET 99999 LIMIT 1 \gset
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, request_id, turn_no, stage, COALESCE(user_id,0), username_snapshot,
user_email_snapshot, COALESCE(api_key_id,0), api_key_name_snapshot, group_id, group_name, provider, endpoint,
protocol, model, prompt_hash, prompt_length, message_count, risk_status, risk_checked_at, created_at, expires_at
FROM prompt_records WHERE (expires_at IS NULL OR expires_at > NOW())
AND (created_at,id) < (:'cursor_created_at'::timestamptz, :cursor_id)
ORDER BY created_at DESC, id DESC LIMIT 21;
