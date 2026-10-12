package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.VideoTaskRecordRepository = (*usageBillingRepository)(nil)

func (r *usageBillingRepository) BeginVideoTaskRecord(ctx context.Context, t *service.VideoTask) error {
	if r == nil || r.db == nil {
		return service.ErrVideoTaskRepositoryUnavailable
	}
	pending, err := json.Marshal(t.Pending)
	if err != nil {
		return err
	}
	// A finite far-future due time also protects rolling deployments: OLD
	// workers must not claim records whose billing remains on the legacy path.
	_, err = r.db.ExecContext(ctx, `INSERT INTO video_tasks
		(task_id,user_id,api_key_id,group_id,account_id,subscription_id,pending,
		 routing_fingerprint,cost_snapshot,next_attempt_at,last_error)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,'{}','9999-12-31 00:00:00+00','submission_unknown')`,
		t.TaskID, t.UserID, t.APIKeyID, t.GroupID, t.AccountID, t.SubscriptionID, string(pending), t.RoutingFingerprint)
	return err
}

func (r *usageBillingRepository) AcceptVideoTaskRecord(ctx context.Context, t *service.VideoTask, taskID, state string) error {
	if r == nil || r.db == nil {
		return service.ErrVideoTaskRepositoryUnavailable
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// OpenAI's existing durable callback may already have stored the real task.
	// Lock/verify identity and discard only our own temporary submission record.
	var accountID int64
	var groupID sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT account_id,group_id FROM video_tasks
		WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3 FOR UPDATE`, taskID, t.UserID, t.APIKeyID).Scan(&accountID, &groupID)
	if err == nil {
		if accountID != t.AccountID || groupID.Valid != (t.GroupID != nil) || (groupID.Valid && groupID.Int64 != *t.GroupID) {
			return service.ErrVideoTaskConflict
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM video_tasks WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3
			AND account_id=$4 AND pending->>'tracking_only'='true'`, t.TaskID, t.UserID, t.APIKeyID, t.AccountID)
	} else if errors.Is(err, sql.ErrNoRows) {
		var pending []byte
		pending, err = json.Marshal(t.Pending)
		if err != nil {
			return err
		}
		var res sql.Result
		res, err = tx.ExecContext(ctx, `UPDATE video_tasks SET task_id=$5,state=$6::varchar,pending=$7::jsonb,
			last_error='',updated_at=NOW(),
			completion=CASE WHEN $6::varchar='completed' THEN '{}'::jsonb ELSE completion END,
			completed_at=CASE WHEN $6::varchar IN ('completed','failed') THEN NOW() ELSE completed_at END
			WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3 AND account_id=$4
			AND pending->>'tracking_only'='true'`, t.TaskID, t.UserID, t.APIKeyID, t.AccountID, taskID, state, string(pending))
		if err = videoTaskAffected(res, err); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *usageBillingRepository) ObserveVideoTaskRecord(ctx context.Context, taskID string, userID, keyID, accountID int64, state string) error {
	if r == nil || r.db == nil {
		return service.ErrVideoTaskRepositoryUnavailable
	}
	// Observations cannot resurrect terminal tasks, change owners or touch
	// worker-managed billing. Repeated status/content requests are harmless.
	_, err := r.db.ExecContext(ctx, `UPDATE video_tasks SET state=$5::varchar,updated_at=NOW(),last_error='',
		completion=CASE WHEN $5::varchar='completed' THEN '{}'::jsonb ELSE completion END,
		completed_at=CASE WHEN $5::varchar IN ('completed','failed') THEN NOW() ELSE completed_at END
		WHERE task_id=$1 AND user_id=$2 AND api_key_id=$3 AND account_id=$4
		AND pending->>'tracking_only'='true' AND state='pending'`, taskID, userID, keyID, accountID, state)
	return err
}
