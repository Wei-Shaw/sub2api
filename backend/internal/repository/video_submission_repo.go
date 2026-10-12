package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageBillingRepository) ClaimVideoSubmission(ctx context.Context, keyID int64, keyHash, payloadHash string) (bool, string, error) {
	var inserted string
	err := r.db.QueryRowContext(ctx, `INSERT INTO video_submissions (api_key_id,key_hash,payload_hash)
        VALUES ($1,$2,$3) ON CONFLICT DO NOTHING RETURNING key_hash`, keyID, keyHash, payloadHash).Scan(&inserted)
	if err == nil {
		return true, "", nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, "", err
	}
	var fingerprint, taskID string
	err = r.db.QueryRowContext(ctx, `SELECT payload_hash,task_id FROM video_submissions WHERE api_key_id=$1 AND key_hash=$2`, keyID, keyHash).Scan(&fingerprint, &taskID)
	if err != nil {
		return false, "", err
	}
	if fingerprint != payloadHash {
		return false, "", service.ErrVideoSubmissionConflict
	}
	return false, taskID, nil
}

func (r *usageBillingRepository) CompleteVideoSubmission(ctx context.Context, keyID int64, keyHash, taskID string) error {
	if taskID == "" {
		return errors.New("missing accepted video task id")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE video_submissions SET task_id=$3 WHERE api_key_id=$1 AND key_hash=$2 AND (task_id='' OR task_id=$3)`, keyID, keyHash, taskID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("video submission claim missing or conflicting")
	}
	return nil
}
