package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

var _ service.VideoTaskMonitorRepository = (*usageBillingRepository)(nil)

// Keep next_attempt_at in the far future: earlier application containers must
// never claim these monitoring records for billing during a rolling release.
// Monitor scheduling uses updated_at; transient failures back off to four minutes.
const videoTaskMonitorDueSQL = `t.updated_at + CASE WHEN t.last_error='' THEN INTERVAL '30 seconds'
	ELSE INTERVAL '30 seconds' * power(2, LEAST(t.attempts,3)) END`

func (r *usageBillingRepository) ClaimDueVideoTaskMonitor(ctx context.Context, lease time.Duration) (*service.VideoTask, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrVideoTaskRepositoryUnavailable
	}
	if lease <= 0 {
		return nil, errors.New("positive video monitor lease required")
	}
	return scanVideoTask(r.db.QueryRowContext(ctx, `WITH due AS (
		SELECT t.task_id,t.user_id,t.api_key_id FROM video_tasks t
		WHERE t.state='pending' AND t.pending->>'tracking_only'='true'
		AND t.task_id NOT LIKE 'submission:%'
		AND t.last_error NOT IN ('submission_unknown','poll_exhausted','account_route_changed')
		AND (`+videoTaskMonitorDueSQL+`) <= NOW()
		AND (t.lease_until IS NULL OR t.lease_until <= NOW())
		ORDER BY t.updated_at,t.created_at LIMIT 1 FOR UPDATE SKIP LOCKED
	) UPDATE video_tasks t SET lease_token=$1,lease_until=NOW()+$2*INTERVAL '1 millisecond',
		attempts=t.attempts+1,updated_at=NOW()
	FROM due WHERE t.task_id=due.task_id AND t.user_id=due.user_id AND t.api_key_id=due.api_key_id
	RETURNING t.task_id,t.user_id,t.api_key_id,t.group_id,t.account_id,t.subscription_id,
		t.pending,t.completion,t.state,t.attempts,t.next_attempt_at,t.lease_token,t.routing_fingerprint,t.cost_snapshot,t.last_error`, uuid.NewString(), lease.Milliseconds()))
}

func (r *usageBillingRepository) SyncVideoTaskMonitorUsage(ctx context.Context, t *service.VideoTask) (bool, error) {
	if r == nil || r.db == nil {
		return false, service.ErrVideoTaskRepositoryUnavailable
	}
	if t == nil || !t.Pending.TrackingOnly {
		return false, errors.New("tracking-only task required")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE video_tasks t SET state='completed',completion='{}'::jsonb,
		completed_at=NOW(),updated_at=NOW(),last_error='',lease_token=NULL,lease_until=NULL
		WHERE t.task_id=$1 AND t.user_id=$2 AND t.api_key_id=$3 AND t.account_id=$4
		AND t.lease_token=$5 AND t.lease_until>NOW() AND t.state='pending'
		AND t.pending->>'tracking_only'='true'
		AND EXISTS (SELECT 1 FROM usage_logs l WHERE l.request_id='grok-video:'||t.task_id
			AND l.user_id=t.user_id AND l.api_key_id=t.api_key_id AND l.account_id=t.account_id)`,
		t.TaskID, t.UserID, t.APIKeyID, t.AccountID, t.LeaseToken)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *usageBillingRepository) FinishVideoTaskMonitor(ctx context.Context, t *service.VideoTask, state, code string) error {
	if r == nil || r.db == nil {
		return service.ErrVideoTaskRepositoryUnavailable
	}
	if t == nil || !t.Pending.TrackingOnly || len(code) > 64 ||
		(state != service.VideoTaskPending && state != service.VideoTaskCompleted && state != service.VideoTaskFailed) {
		return errors.New("invalid video monitor observation")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE video_tasks SET state=$6::varchar,last_error=$7,
		completion=CASE WHEN $6::varchar='completed' THEN '{}'::jsonb ELSE completion END,
		completed_at=CASE WHEN $6::varchar IN ('completed','failed') THEN NOW() ELSE completed_at END,
		updated_at=NOW(),lease_token=NULL,lease_until=NULL
		WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3 AND account_id=$4
		AND lease_token=$5 AND lease_until>NOW() AND state='pending'
		AND pending->>'tracking_only'='true'`, t.TaskID, t.UserID, t.APIKeyID, t.AccountID, t.LeaseToken, state, code)
	return videoTaskAffected(result, err)
}
