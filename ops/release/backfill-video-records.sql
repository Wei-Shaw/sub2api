-- Restore completed video history ONLY from existing settled usage rows.
-- No charges, refunds, upstream requests, active jobs or price changes.
-- Historical submission time is unavailable: created_at is the known usage time.
-- psql -v ON_ERROR_STOP=1 -v usage_log_id=<verified usage row> -f this-file.sql
BEGIN;
SET LOCAL lock_timeout = '3s';
SET LOCAL statement_timeout = '30s';
INSERT INTO video_tasks
    (task_id,user_id,api_key_id,group_id,account_id,subscription_id,
     routing_fingerprint,pending,cost_snapshot,completion,state,next_attempt_at,
     created_at,updated_at,completed_at)
SELECT DISTINCT ON (l.request_id,l.user_id,l.api_key_id)
    substring(l.request_id FROM 12),l.user_id,l.api_key_id,l.group_id,l.account_id,l.subscription_id,
    repeat('0',64),
    jsonb_build_object('tracking_only',true,'model',l.model,'original_model',COALESCE(l.requested_model,l.model),
        'video_duration_seconds',l.video_duration_seconds,'video_resolution',l.video_resolution,
        'restored_from_usage',true),
    '{}','{}','completed','9999-12-31 00:00:00+00',l.created_at,NOW(),l.created_at
FROM usage_logs l
WHERE l.id=:'usage_log_id'::bigint AND l.request_id LIKE 'grok-video:%'
    AND (l.video_count>0 OR l.request_id LIKE 'grok-video:seedance:%')
    AND length(l.request_id) BETWEEN 12 AND 523
    AND l.user_id>0 AND l.api_key_id>0 AND l.account_id>0
    AND NOT EXISTS (SELECT 1 FROM video_tasks t WHERE t.task_id=substring(l.request_id FROM 12)
        AND t.user_id=l.user_id AND t.api_key_id=l.api_key_id)
ORDER BY l.request_id,l.user_id,l.api_key_id,l.id
ON CONFLICT (task_id,user_id,api_key_id) DO NOTHING;
COMMIT;
