package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type notificationEmailDeliveryRepository struct {
	db *sql.DB
}

func NewNotificationEmailDeliveryRepository(db *sql.DB) service.NotificationEmailDeliveryRepository {
	return &notificationEmailDeliveryRepository{db: db}
}

func (r *notificationEmailDeliveryRepository) Enqueue(ctx context.Context, delivery service.NotificationEmailDelivery) (int64, bool, error) {
	variables, err := json.Marshal(delivery.Variables)
	if err != nil {
		return 0, false, fmt.Errorf("encode notification email variables: %w", err)
	}
	rawHTMLVariables, err := json.Marshal(delivery.RawHTMLVariables)
	if err != nil {
		return 0, false, fmt.Errorf("encode notification email raw html variables: %w", err)
	}
	maxAttempts := delivery.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	exec := notificationEmailDeliveryExecutor(ctx, r.db)
	if exec == nil {
		return 0, false, fmt.Errorf("notification email SQL executor is not configured")
	}
	var id int64
	rows, err := exec.QueryContext(ctx, `
			INSERT INTO notification_email_deliveries (
			dedup_key, event, channel, recipient_email, recipient_hash, recipient_name,
			user_id, source_type, source_id, reminder_key, locale, variables,
			raw_html_variables, status, max_attempts
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'pending', $14
		)
		ON CONFLICT (dedup_key) DO NOTHING
		RETURNING id
		`, delivery.DedupKey, delivery.Event, delivery.Channel, delivery.RecipientEmail,
		delivery.RecipientHash, delivery.RecipientName, delivery.UserID, delivery.SourceType,
		delivery.SourceID, delivery.ReminderKey, delivery.Locale, variables, rawHTMLVariables,
		maxAttempts)
	if err != nil {
		return 0, false, err
	}
	if rows.Next() {
		err = rows.Scan(&id)
	} else {
		err = rows.Err()
	}
	closeErr := rows.Close()
	if err != nil {
		return 0, false, err
	}
	if closeErr != nil {
		return 0, false, closeErr
	}
	if id != 0 {
		return id, true, nil
	}
	rows, err = exec.QueryContext(ctx, `
		SELECT id FROM notification_email_deliveries WHERE dedup_key = $1
	`, delivery.DedupKey)
	if err != nil {
		return 0, false, err
	}
	if !rows.Next() {
		err = rows.Err()
		closeErr = rows.Close()
		if err != nil {
			return 0, false, err
		}
		if closeErr != nil {
			return 0, false, closeErr
		}
		return 0, false, sql.ErrNoRows
	}
	err = rows.Scan(&id)
	closeErr = rows.Close()
	if err != nil {
		return 0, false, err
	}
	if closeErr != nil {
		return 0, false, closeErr
	}
	return id, false, nil
}

func notificationEmailDeliveryExecutor(ctx context.Context, fallback sqlQueryExecutor) sqlQueryExecutor {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		if exec, ok := tx.Client().Driver().(sqlQueryExecutor); ok {
			return exec
		}
	}
	return fallback
}

func (r *notificationEmailDeliveryRepository) Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]service.NotificationEmailDelivery, error) {
	if limit <= 0 {
		return []service.NotificationEmailDelivery{}, nil
	}
	leaseSeconds := int64(lease / time.Second)
	if leaseSeconds < 1 {
		leaseSeconds = 1
	}
	rows, err := r.db.QueryContext(ctx, `
			WITH exhausted AS (
				UPDATE notification_email_deliveries
				SET status = 'failed', updated_at = NOW(), lease_owner = NULL,
					lease_expires_at = NULL, last_error_category = 'internal',
					last_error = 'delivery lease expired after maximum attempts'
				WHERE status = 'processing' AND lease_expires_at < NOW()
					AND attempt_count >= max_attempts
			), candidates AS (
				SELECT id
			FROM notification_email_deliveries
			WHERE (
				status IN ('pending', 'retry_wait')
				AND next_attempt_at <= NOW()
			) OR (
					status = 'processing'
					AND lease_expires_at < NOW()
					AND attempt_count < max_attempts
			)
			ORDER BY next_attempt_at ASC, id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notification_email_deliveries AS d
		SET status = 'processing',
			attempt_count = attempt_count + 1,
			lease_owner = $1,
			lease_expires_at = NOW() + ($3 * INTERVAL '1 second'),
			updated_at = NOW()
		FROM candidates AS c
		WHERE d.id = c.id
		RETURNING d.id, d.dedup_key, d.event, d.channel, d.recipient_email,
			d.recipient_hash, d.recipient_name, d.user_id, d.source_type, d.source_id,
			d.reminder_key, d.locale, d.variables, d.raw_html_variables, d.status,
			d.attempt_count, d.max_attempts, d.next_attempt_at, d.last_error_category,
			d.last_error, d.created_at, d.updated_at, d.sent_at
	`, workerID, limit, leaseSeconds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	deliveries := make([]service.NotificationEmailDelivery, 0, limit)
	for rows.Next() {
		delivery, scanErr := scanNotificationEmailDelivery(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return deliveries, nil
}

func (r *notificationEmailDeliveryRepository) MarkSent(ctx context.Context, id int64, workerID string) error {
	return r.finishClaim(ctx, id, workerID, `
		UPDATE notification_email_deliveries
		SET status = 'sent', sent_at = NOW(), updated_at = NOW(),
			lease_owner = NULL, lease_expires_at = NULL,
			last_error_category = NULL, last_error = NULL
		WHERE id = $1 AND status = 'processing' AND lease_owner = $2
	`)
}

func (r *notificationEmailDeliveryRepository) MarkSuppressed(ctx context.Context, id int64, workerID, category, detail string) error {
	return r.finishClaim(ctx, id, workerID, `
		UPDATE notification_email_deliveries
		SET status = 'suppressed', updated_at = NOW(),
			lease_owner = NULL, lease_expires_at = NULL,
			last_error_category = $3, last_error = $4
		WHERE id = $1 AND status = 'processing' AND lease_owner = $2
	`, category, detail)
}

func (r *notificationEmailDeliveryRepository) MarkFailed(ctx context.Context, id int64, workerID, category, detail string, retryAt *time.Time) error {
	status := service.NotificationEmailDeliveryStatusFailed
	nextAttempt := time.Now().UTC()
	if retryAt != nil {
		status = service.NotificationEmailDeliveryStatusRetryWait
		nextAttempt = retryAt.UTC()
	}
	return r.finishClaim(ctx, id, workerID, `
		UPDATE notification_email_deliveries
		SET status = $3, next_attempt_at = $4, updated_at = NOW(),
			lease_owner = NULL, lease_expires_at = NULL,
			last_error_category = $5, last_error = $6
		WHERE id = $1 AND status = 'processing' AND lease_owner = $2
	`, status, nextAttempt, category, detail)
}

func (r *notificationEmailDeliveryRepository) finishClaim(ctx context.Context, id int64, workerID, query string, args ...any) error {
	queryArgs := []any{id, workerID}
	queryArgs = append(queryArgs, args...)
	result, err := r.db.ExecContext(ctx, query, queryArgs...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("notification email claim %d is no longer owned by %s", id, workerID)
	}
	return nil
}

func (r *notificationEmailDeliveryRepository) List(ctx context.Context, filter service.NotificationEmailDeliveryListFilter) (service.NotificationEmailDeliveryListResult, error) {
	where, args := notificationEmailDeliveryListWhere(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_email_deliveries"+where, args...).Scan(&total); err != nil {
		return service.NotificationEmailDeliveryListResult{}, err
	}
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(append([]any{}, args...), filter.PageSize, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, dedup_key, event, channel, recipient_email, recipient_hash,
			recipient_name, user_id, source_type, source_id, reminder_key, locale,
			variables, raw_html_variables, status, attempt_count, max_attempts,
			next_attempt_at, last_error_category, last_error, created_at, updated_at, sent_at
		FROM notification_email_deliveries`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)+1)+` OFFSET $`+fmt.Sprint(len(args)+2), queryArgs...)
	if err != nil {
		return service.NotificationEmailDeliveryListResult{}, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.NotificationEmailDelivery, 0, filter.PageSize)
	for rows.Next() {
		item, scanErr := scanNotificationEmailDelivery(rows)
		if scanErr != nil {
			return service.NotificationEmailDeliveryListResult{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return service.NotificationEmailDeliveryListResult{}, err
	}
	return service.NotificationEmailDeliveryListResult{Items: items, Total: total}, nil
}

func notificationEmailDeliveryListWhere(filter service.NotificationEmailDeliveryListFilter) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	add := func(column, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		args = append(args, value)
		clauses = append(clauses, column+" = $"+fmt.Sprint(len(args)))
	}
	add("event", filter.Event)
	add("status", filter.Status)
	add("source_type", filter.SourceType)
	add("source_id", filter.SourceID)
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (r *notificationEmailDeliveryRepository) Retry(ctx context.Context, id int64) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE notification_email_deliveries
		SET status = 'pending', attempt_count = 0, next_attempt_at = NOW(),
			lease_owner = NULL, lease_expires_at = NULL, updated_at = NOW(),
			last_error_category = NULL, last_error = NULL
		WHERE id = $1
		  AND status IN ('failed', 'retry_wait')
			  AND COALESCE(last_error_category, '') IN ('transport', 'internal', 'configuration', 'template')
	`, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (r *notificationEmailDeliveryRepository) Stats(ctx context.Context) (service.NotificationEmailDeliveryStats, error) {
	var (
		stats     service.NotificationEmailDeliveryStats
		oldest    sql.NullTime
		lastError sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FILTER (WHERE status IN ('pending', 'processing', 'retry_wait')),
				MIN(created_at) FILTER (WHERE status IN ('pending', 'processing', 'retry_wait')),
				COALESCE(MAX(attempt_count) FILTER (WHERE status IN ('pending', 'processing', 'retry_wait')), 0),
				(SELECT last_error FROM notification_email_deliveries
				 WHERE status IN ('pending', 'processing', 'retry_wait') AND last_error IS NOT NULL
				 ORDER BY updated_at DESC, id DESC LIMIT 1)
		FROM notification_email_deliveries
	`).Scan(&stats.Pending, &oldest, &stats.MaxAttempts, &lastError)
	if err != nil {
		return stats, err
	}
	if oldest.Valid {
		value := oldest.Time
		stats.OldestCreatedAt = &value
	}
	if lastError.Valid {
		stats.LastError = lastError.String
	}
	return stats, nil
}

func (r *notificationEmailDeliveryRepository) DeleteTerminalBefore(ctx context.Context, successfulBefore, failedBefore time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	result, err := r.db.ExecContext(ctx, `
		WITH candidates AS (
			SELECT id
			FROM notification_email_deliveries
			WHERE (status IN ('sent', 'suppressed') AND updated_at < $1)
			   OR (status = 'failed' AND updated_at < $2)
			ORDER BY id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM notification_email_deliveries AS d
		USING candidates AS c
		WHERE d.id = c.id
	`, successfulBefore.UTC(), failedBefore.UTC(), limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

type notificationEmailDeliveryScanner interface {
	Scan(dest ...any) error
}

func scanNotificationEmailDelivery(scanner notificationEmailDeliveryScanner) (service.NotificationEmailDelivery, error) {
	var (
		delivery         service.NotificationEmailDelivery
		variables        []byte
		rawHTMLVariables []byte
		lastCategory     sql.NullString
		lastError        sql.NullString
		sentAt           sql.NullTime
	)
	err := scanner.Scan(
		&delivery.ID, &delivery.DedupKey, &delivery.Event, &delivery.Channel,
		&delivery.RecipientEmail, &delivery.RecipientHash, &delivery.RecipientName,
		&delivery.UserID, &delivery.SourceType, &delivery.SourceID, &delivery.ReminderKey,
		&delivery.Locale, &variables, &rawHTMLVariables, &delivery.Status,
		&delivery.AttemptCount, &delivery.MaxAttempts, &delivery.NextAttemptAt,
		&lastCategory, &lastError, &delivery.CreatedAt, &delivery.UpdatedAt, &sentAt,
	)
	if err != nil {
		return delivery, err
	}
	if err := json.Unmarshal(variables, &delivery.Variables); err != nil {
		return delivery, fmt.Errorf("decode notification email variables: %w", err)
	}
	if err := json.Unmarshal(rawHTMLVariables, &delivery.RawHTMLVariables); err != nil {
		return delivery, fmt.Errorf("decode notification email raw html variables: %w", err)
	}
	if lastCategory.Valid {
		delivery.LastErrorCategory = lastCategory.String
	}
	if lastError.Valid {
		delivery.LastError = lastError.String
	}
	if sentAt.Valid {
		value := sentAt.Time
		delivery.SentAt = &value
	}
	return delivery, nil
}
