package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.VideoTaskAdminRepository = (*usageBillingRepository)(nil)

// One statement gives count and page the same MVCC snapshot, including empty or
// out-of-range pages. Only the bounded page probes usage_logs. PK joins and a
// lateral LIMIT 1 prevent multiplication, even with historical duplicate usage.
// key_hash is used solely as an internal stable submission tie-breaker and is
// deliberately excluded from the explicit final projection.
const videoTaskAdminQuery = `WITH source AS (
	SELECT 'task'::text AS kind, t.task_id, t.user_id, t.api_key_id,
		t.account_id, t.group_id,
		COALESCE(NULLIF(t.pending->>'original_model',''), t.pending->>'model','') AS model,
		t.state, t.created_at, t.updated_at, t.completed_at,
		CASE WHEN t.last_error IN ('submission_unknown','poll_exhausted','account_route_changed') THEN NULL
			WHEN t.state='pending' AND t.pending->>'tracking_only'='true' THEN ` + videoTaskMonitorDueSQL + `
			WHEN t.state IN ('pending','ready') THEN t.next_attempt_at END AS next_attempt_at,
		t.attempts,
		CASE WHEN t.state='completed' THEN 'completed'
			WHEN t.state='failed' THEN 'failed'
			WHEN t.state='ready' THEN 'billing_pending'
			WHEN t.last_error='submission_unknown' THEN 'submission_unknown'
			WHEN t.last_error IN ('poll_exhausted','account_route_changed') THEN 'review_required'
			WHEN t.last_error<>'' THEN 'query_retry'
			WHEN t.created_at < NOW()-INTERVAL '2 hours' THEN 'delayed'
			ELSE 'in_progress' END AS display_status,
		CASE WHEN t.last_error IN ('poll_failed','poll_timeout','upstream_failed','upstream_cancelled','upstream_expired',
			'completion_save_failed','billing_failed','poll_not_found','poll_exhausted','account_route_changed')
			THEN t.last_error ELSE '' END AS reason_code,
		''::text AS sort_key
	FROM video_tasks t
	WHERE t.created_at >= NOW()-$1::int*INTERVAL '1 day'
	UNION ALL
	SELECT 'submission', '', k.user_id, s.api_key_id, NULL::bigint, NULL::bigint,
		NULL::text, 'submission_unknown', s.created_at, s.created_at, NULL::timestamptz,
		NULL::timestamptz, 0, 'submission_unknown', '', s.key_hash
	FROM video_submissions s
	LEFT JOIN api_keys k ON k.id=s.api_key_id
	WHERE s.task_id='' AND s.created_at >= NOW()-$1::int*INTERVAL '1 day'
), filtered AS MATERIALIZED (
	SELECT s.*, COALESCE(u.email,'') AS user_email, COALESCE(k.name,'') AS api_key_name,
		COALESCE(a.name,'') AS account_name, COALESCE(g.name,'') AS group_name
	FROM source s
	LEFT JOIN users u ON u.id=s.user_id
	LEFT JOIN api_keys k ON k.id=s.api_key_id AND k.user_id=s.user_id
	LEFT JOIN accounts a ON a.id=s.account_id
	LEFT JOIN groups g ON g.id=s.group_id
	WHERE ($3::text='' OR s.display_status=$3)
		AND ($2::text='' OR strpos(lower(s.task_id),lower($2))>0
			OR strpos(lower(s.model),lower($2))>0
			OR strpos(lower(u.email),lower($2))>0
			OR strpos(lower(u.username),lower($2))>0
			OR strpos(s.user_id::text,$2)>0
			OR strpos(lower(a.name),lower($2))>0
			OR strpos(s.account_id::text,$2)>0)
), page AS (
	SELECT * FROM filtered
	ORDER BY created_at DESC, kind, task_id, user_id NULLS LAST, api_key_id, sort_key
	LIMIT $4 OFFSET $5
), items AS (
	SELECT p.kind, p.task_id, p.user_id, p.user_email, p.api_key_id, p.api_key_name,
		p.account_id, p.account_name, p.group_id, p.group_name, p.model, p.state,
		p.display_status, p.reason_code, p.created_at, p.updated_at, p.completed_at,
		p.next_attempt_at, p.attempts,
		CASE WHEN jsonb_typeof(t.cost_snapshot #> '{cost,ActualCost}')='number'
			THEN (t.cost_snapshot #>> '{cost,ActualCost}')::numeric END AS expected_cost,
		ul.actual_cost AS charged_cost, ul.id AS usage_log_id,
		CASE WHEN ul.id IS NOT NULL AND p.state='failed' THEN 'review'
			WHEN ul.id IS NOT NULL THEN 'charged'
			WHEN p.display_status='submission_unknown' OR p.state='completed' THEN 'review'
			WHEN p.state='ready' THEN 'pending' ELSE 'not_charged' END AS billing_status,
		CASE WHEN jsonb_typeof(t.pending->'video_duration_seconds')='number' THEN
			CASE WHEN (t.pending->>'video_duration_seconds')::numeric BETWEEN 1 AND 2147483647
				THEN (t.pending->>'video_duration_seconds')::numeric::int END
			END AS video_duration_seconds,
		COALESCE(t.pending->>'video_resolution','') AS video_resolution,
		p.sort_key
	FROM page p
	LEFT JOIN video_tasks t ON p.kind='task' AND t.task_id=p.task_id
		AND t.user_id=p.user_id AND t.api_key_id=p.api_key_id
	LEFT JOIN LATERAL (
		SELECT l.id, l.actual_cost FROM usage_logs l
		WHERE p.kind='task' AND l.request_id='grok-video:' || p.task_id
			AND l.api_key_id=p.api_key_id AND l.user_id=p.user_id AND l.account_id=p.account_id
		ORDER BY l.id LIMIT 1
	) ul ON TRUE
)
SELECT (SELECT COUNT(*) FROM filtered),
	COALESCE(jsonb_agg(to_jsonb(items)-'sort_key'
		ORDER BY created_at DESC, kind, task_id, user_id NULLS LAST, api_key_id, sort_key),'[]'::jsonb)
FROM items`

func (r *usageBillingRepository) ListVideoTasks(ctx context.Context, f service.VideoTaskAdminFilter) ([]service.VideoTaskAdminItem, int64, error) {
	if err := f.Validate(); err != nil {
		return nil, 0, err
	}
	if r == nil || r.db == nil {
		return nil, 0, service.ErrVideoTaskRepositoryUnavailable
	}
	var total int64
	var payload []byte
	err := r.db.QueryRowContext(ctx, videoTaskAdminQuery,
		f.Days, f.Search, f.Status, f.PageSize, int64(f.Page-1)*int64(f.PageSize)).Scan(&total, &payload)
	if err != nil {
		return nil, 0, fmt.Errorf("list admin video tasks: %w", err)
	}
	items := make([]service.VideoTaskAdminItem, 0)
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, 0, fmt.Errorf("decode admin video tasks: %w", err)
	}
	return items, total, nil
}
