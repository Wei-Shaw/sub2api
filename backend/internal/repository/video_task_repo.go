package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

var _ service.VideoTaskRepository = (*usageBillingRepository)(nil)

const videoTaskColumns = `task_id, user_id, api_key_id, group_id, account_id, subscription_id, pending, completion, state, attempts, next_attempt_at, COALESCE(lease_token, ''), routing_fingerprint, cost_snapshot, last_error`

func scanVideoTask(row interface{ Scan(...any) error }) (*service.VideoTask, error) {
	var t service.VideoTask
	var pending, completion, cost []byte
	err := row.Scan(&t.TaskID, &t.UserID, &t.APIKeyID, &t.GroupID, &t.AccountID, &t.SubscriptionID, &pending, &completion, &t.State, &t.Attempts, &t.NextAttemptAt, &t.LeaseToken, &t.RoutingFingerprint, &cost, &t.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(pending, &t.Pending); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(cost, &t.CostSnapshot); err != nil {
		return nil, err
	}
	if len(completion) > 0 {
		if err := json.Unmarshal(completion, &t.Completion); err != nil {
			return nil, err
		}
	}
	return &t, nil
}

func (r *usageBillingRepository) CreateVideoTask(ctx context.Context, t *service.VideoTask) error {
	if r == nil || r.db == nil {
		return service.ErrVideoTaskRepositoryUnavailable
	}
	if t == nil || t.CostSnapshot == nil {
		return errors.New("video task cost snapshot required")
	}
	pending, err := json.Marshal(t.Pending)
	if err != nil {
		return err
	}
	cost, err := json.Marshal(t.CostSnapshot)
	if err != nil {
		return err
	}
	// Conflict checks make retries idempotent without replacing pinned account,
	// subscription, metadata, or the state of an already settled task.
	var id string
	err = r.db.QueryRowContext(ctx, `INSERT INTO video_tasks AS existing
		(task_id, user_id, api_key_id, group_id, account_id, subscription_id, pending, routing_fingerprint, cost_snapshot)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9::jsonb)
		ON CONFLICT (task_id, user_id, api_key_id) DO UPDATE SET task_id = existing.task_id
		WHERE existing.account_id = EXCLUDED.account_id
		  AND existing.group_id IS NOT DISTINCT FROM EXCLUDED.group_id
		  AND existing.subscription_id IS NOT DISTINCT FROM EXCLUDED.subscription_id
		  AND existing.pending = EXCLUDED.pending
		  AND existing.routing_fingerprint = EXCLUDED.routing_fingerprint
		  AND existing.cost_snapshot = EXCLUDED.cost_snapshot
		RETURNING task_id`, t.TaskID, t.UserID, t.APIKeyID, t.GroupID, t.AccountID, t.SubscriptionID, string(pending), t.RoutingFingerprint, string(cost)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrVideoTaskConflict
	}
	return err
}

func (r *usageBillingRepository) GetVideoTask(ctx context.Context, taskID string, userID, apiKeyID int64) (*service.VideoTask, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrVideoTaskRepositoryUnavailable
	}
	return scanVideoTask(r.db.QueryRowContext(ctx, `SELECT `+videoTaskColumns+` FROM video_tasks WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3`, taskID, userID, apiKeyID))
}

func (r *usageBillingRepository) ClaimDueVideoTask(ctx context.Context, lease time.Duration) (*service.VideoTask, error) {
	return r.claimVideoTask(ctx, "", 0, 0, lease)
}

func (r *usageBillingRepository) ClaimOwnedVideoTask(ctx context.Context, taskID string, userID, apiKeyID int64, lease time.Duration) (*service.VideoTask, error) {
	if taskID == "" || userID <= 0 || apiKeyID <= 0 {
		return nil, errors.New("video task owner required")
	}
	return r.claimVideoTask(ctx, taskID, userID, apiKeyID, lease)
}

func (r *usageBillingRepository) claimVideoTask(ctx context.Context, taskID string, userID, apiKeyID int64, lease time.Duration) (*service.VideoTask, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrVideoTaskRepositoryUnavailable
	}
	if lease <= 0 {
		return nil, errors.New("positive video lease required")
	}
	// One statement claims one row; SKIP LOCKED permits independent processes.
	// An expired lease can be reclaimed after a process crash, without a restart.
	return scanVideoTask(r.db.QueryRowContext(ctx, `WITH due AS (
		SELECT task_id, user_id, api_key_id FROM video_tasks
		WHERE state IN ('pending','ready') AND (next_attempt_at <= NOW()
		  OR ($1<>'' AND last_error='poll_exhausted' AND updated_at<=NOW()-INTERVAL '4 minutes'))
		  AND COALESCE(pending->>'tracking_only','false') <> 'true'
		  AND (lease_until IS NULL OR lease_until <= NOW())
		  AND ($1 = '' OR (task_id=$1 AND user_id=$2 AND api_key_id=$3))
		ORDER BY next_attempt_at, created_at
		LIMIT 1 FOR UPDATE SKIP LOCKED
	) UPDATE video_tasks t SET lease_token=$4, lease_until=NOW()+$5*INTERVAL '1 millisecond',
		attempts=t.attempts+1, updated_at=NOW()
	FROM due WHERE t.task_id=due.task_id AND t.user_id=due.user_id AND t.api_key_id=due.api_key_id
	RETURNING t.task_id,t.user_id,t.api_key_id,t.group_id,t.account_id,t.subscription_id,
		t.pending,t.completion,t.state,t.attempts,t.next_attempt_at,t.lease_token,t.routing_fingerprint,t.cost_snapshot,t.last_error`, taskID, userID, apiKeyID, uuid.NewString(), lease.Milliseconds()))
}

func (r *usageBillingRepository) SaveVideoTaskCompletion(ctx context.Context, t *service.VideoTask, completion *service.VideoTaskCompletion) error {
	if r == nil || r.db == nil {
		return service.ErrVideoTaskRepositoryUnavailable
	}
	if t == nil || completion == nil {
		return errors.New("video completion required")
	}
	body, err := json.Marshal(completion)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE video_tasks SET completion=$5::jsonb,state='ready',updated_at=NOW()
		WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3 AND lease_token=$4
		AND lease_until > NOW() AND state='pending'`, t.TaskID, t.UserID, t.APIKeyID, t.LeaseToken, string(body))
	return videoTaskAffected(result, err)
}

func (r *usageBillingRepository) FinishVideoTaskAttempt(ctx context.Context, t *service.VideoTask, state string, delay time.Duration, errorCode string) error {
	if r == nil || r.db == nil {
		return service.ErrVideoTaskRepositoryUnavailable
	}
	if t == nil || delay < 0 || len(errorCode) > 64 {
		return errors.New("invalid video task attempt")
	}
	if state != service.VideoTaskPending && state != service.VideoTaskReady && state != service.VideoTaskCompleted && state != service.VideoTaskFailed {
		return errors.New("invalid video task state")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE video_tasks SET state=$5,next_attempt_at=CASE WHEN $7='poll_exhausted'
		THEN '9999-12-31 00:00:00+00'::timestamptz ELSE NOW()+$6*INTERVAL '1 millisecond' END,
		last_error=$7,lease_token=NULL,lease_until=NULL,updated_at=NOW(),
		completed_at=CASE WHEN $5 IN ('completed','failed') THEN NOW() ELSE completed_at END
		WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3 AND lease_token=$4
		AND lease_until > NOW() AND state IN ('pending','ready')
		AND (state='pending' OR $5 IN ('ready','completed'))`, t.TaskID, t.UserID, t.APIKeyID, t.LeaseToken, state, delay.Milliseconds(), errorCode)
	return videoTaskAffected(result, err)
}

func videoTaskAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return service.ErrVideoTaskLeaseLost
	}
	return nil
}
