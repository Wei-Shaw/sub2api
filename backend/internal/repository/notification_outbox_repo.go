package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type notificationOutboxRepository struct{ db *sql.DB }

func NewNotificationOutboxRepository(db *sql.DB) service.NotificationOutboxRepository {
	return &notificationOutboxRepository{db: db}
}

func (r *notificationOutboxRepository) Claim(ctx context.Context, workerID string, limit, maxAttempts int, lease time.Duration) ([]service.NotificationOutboxMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id FROM notification_outbox
			WHERE attempt_count < $2 AND next_attempt_at <= NOW()
			  AND (status IN ('pending','failed') OR (status='processing' AND claimed_at < NOW()-($3*INTERVAL '1 second')))
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE notification_outbox o
		SET status='processing',claimed_at=NOW(),claimed_by_worker_id=$4,attempt_count=o.attempt_count+1,last_error=NULL,updated_at=NOW()
		FROM candidates c WHERE o.id=c.id
		RETURNING o.id,o.event,o.recipient,o.locale,o.variables,o.attempt_count`, limit, maxAttempts, int64(lease.Seconds()), workerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.NotificationOutboxMessage, 0)
	for rows.Next() {
		var message service.NotificationOutboxMessage
		var raw []byte
		if err := rows.Scan(&message.ID, &message.Event, &message.Recipient, &message.Locale, &raw, &message.AttemptCount); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &message.Variables); err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	return out, rows.Err()
}

func (r *notificationOutboxRepository) MarkDelivered(ctx context.Context, id int64, workerID string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE notification_outbox SET status='delivered',delivered_at=NOW(),claimed_at=NULL,claimed_by_worker_id=NULL,updated_at=NOW() WHERE id=$1 AND status='processing' AND claimed_by_worker_id=$2`, id, workerID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *notificationOutboxRepository) MarkRetry(ctx context.Context, id int64, workerID string, nextAttempt time.Time, lastError string, terminal bool) error {
	if terminal {
		res, err := r.db.ExecContext(ctx, `UPDATE notification_outbox SET status='failed',next_attempt_at='infinity',claimed_at=NULL,claimed_by_worker_id=NULL,last_error=$3,updated_at=NOW() WHERE id=$1 AND status='processing' AND claimed_by_worker_id=$2`, id, workerID, lastError)
		return requireOneOutboxRow(res, err)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE notification_outbox SET status='failed',next_attempt_at=$3,claimed_at=NULL,claimed_by_worker_id=NULL,last_error=$4,updated_at=NOW() WHERE id=$1 AND status='processing' AND claimed_by_worker_id=$2`, id, workerID, nextAttempt, lastError)
	return requireOneOutboxRow(res, err)
}

func requireOneOutboxRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *notificationOutboxRepository) Stats(ctx context.Context, maxAttempts int) (service.NotificationOutboxStats, error) {
	var out service.NotificationOutboxStats
	err := r.db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE status <> 'delivered' AND attempt_count < $1),
		       min(created_at) FILTER (WHERE status <> 'delivered' AND attempt_count < $1),
		       count(*) FILTER (WHERE status='failed' AND attempt_count >= $1)
		FROM notification_outbox`, maxAttempts).Scan(&out.Pending, &out.OldestCreatedAt, &out.Failed)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	return out, err
}
