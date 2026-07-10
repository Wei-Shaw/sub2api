package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const domainOutboxColumns = `
	id, aggregate_type, aggregate_id, event_type, dedup_key, payload,
	status, attempt_count, next_attempt_at, locked_at, locked_until,
	locked_by, last_error, created_at, completed_at`

var (
	outboxURLPattern              = regexp.MustCompile(`(?i)\bhttps?://[^\s]+`)
	outboxBearerPattern           = regexp.MustCompile(`(?i)\bbearer\s+[^\s,;]+`)
	outboxCredentialHeaderPattern = regexp.MustCompile(`(?i)\b(?:authorization|proxy-authorization|x-api-key|api-key|cookie|set-cookie)\s*[:=]\s*[^\r\n]*`)
)

type domainOutboxRepository struct {
	db *sql.DB
}

func NewDomainOutboxRepository(db *sql.DB) service.DomainOutboxRepository {
	return &domainOutboxRepository{db: db}
}

func (r *domainOutboxRepository) Enqueue(ctx context.Context, input *service.DomainOutboxEvent) (*service.DomainOutboxEvent, error) {
	return enqueueDomainOutboxWith(ctx, r.db, input)
}

// enqueueDomainOutboxInTx lets repository-owned business transactions append
// an event atomically without exposing *sql.Tx through the service interface.
func enqueueDomainOutboxInTx(ctx context.Context, tx *sql.Tx, input *service.DomainOutboxEvent) (*service.DomainOutboxEvent, error) {
	return enqueueDomainOutboxWith(ctx, tx, input)
}

func enqueueDomainOutboxWith(ctx context.Context, queryer sqlQueryRower, input *service.DomainOutboxEvent) (*service.DomainOutboxEvent, error) {
	payload, status, nextAttemptAt, err := validateDomainOutboxInput(input)
	if err != nil {
		return nil, err
	}

	inserted, err := scanDomainOutboxEvent(queryer.QueryRowContext(ctx, `
		INSERT INTO domain_outbox (
			aggregate_type, aggregate_id, event_type, dedup_key, payload,
			status, next_attempt_at
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		ON CONFLICT (dedup_key) DO NOTHING
		RETURNING `+domainOutboxColumns,
		input.AggregateType,
		input.AggregateID,
		input.EventType,
		input.DedupKey,
		string(payload),
		status,
		nextAttemptAt,
	))
	if err == nil {
		return inserted, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	existing, err := getDomainOutboxByDedupKeyWith(ctx, queryer, input.DedupKey)
	if err != nil {
		return nil, err
	}
	if !sameDomainOutboxEnqueue(existing, input) {
		return nil, service.ErrDomainOutboxConflict
	}
	return existing, nil
}

func (r *domainOutboxRepository) GetByID(ctx context.Context, id int64) (*service.DomainOutboxEvent, error) {
	event, err := scanDomainOutboxEvent(r.db.QueryRowContext(ctx, `
		SELECT `+domainOutboxColumns+`
		FROM domain_outbox
		WHERE id = $1
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrDomainOutboxNotFound.WithCause(err)
	}
	return event, err
}

func getDomainOutboxByDedupKeyWith(ctx context.Context, queryer sqlQueryRower, dedupKey string) (*service.DomainOutboxEvent, error) {
	return scanDomainOutboxEvent(queryer.QueryRowContext(ctx, `
		SELECT `+domainOutboxColumns+`
		FROM domain_outbox
		WHERE dedup_key = $1
	`, dedupKey))
}

func (r *domainOutboxRepository) ClaimBatch(ctx context.Context, workerID string, now time.Time, limit int, lease time.Duration) ([]*service.DomainOutboxEvent, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return nil, fmt.Errorf("outbox worker id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if lease <= 0 {
		return nil, fmt.Errorf("outbox lease must be positive")
	}
	now = now.Truncate(time.Microsecond)
	lockedUntil := now.Add(lease).Truncate(time.Microsecond)

	rows, err := r.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id
			FROM domain_outbox
			WHERE status = $1 AND next_attempt_at <= $2
			ORDER BY next_attempt_at ASC, id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE domain_outbox AS event
		SET status = $4,
			attempt_count = event.attempt_count + 1,
			locked_at = $2,
			locked_until = $5,
			locked_by = $6
		FROM candidates
		WHERE event.id = candidates.id
		RETURNING `+prefixedDomainOutboxColumns("event"),
		service.DomainOutboxStatusPending,
		now,
		limit,
		service.DomainOutboxStatusProcessing,
		lockedUntil,
		workerID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.DomainOutboxEvent, 0, limit)
	for rows.Next() {
		item, err := scanDomainOutboxEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *domainOutboxRepository) Complete(ctx context.Context, id int64, workerID string, completedAt time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE domain_outbox
		SET status = $1,
			completed_at = $2,
			locked_at = NULL,
			locked_until = NULL,
			locked_by = NULL,
			last_error = NULL
		WHERE id = $3
		  AND status = $4
		  AND locked_by = $5
		  AND locked_until > CURRENT_TIMESTAMP
	`,
		service.DomainOutboxStatusCompleted,
		completedAt.Truncate(time.Microsecond),
		id,
		service.DomainOutboxStatusProcessing,
		strings.TrimSpace(workerID),
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 1 {
		return true, nil
	}
	return false, r.classifyLeaseMiss(ctx, id, service.DomainOutboxStatusCompleted)
}

func (r *domainOutboxRepository) Retry(ctx context.Context, id int64, workerID string, nextAttemptAt time.Time, dead bool, lastError string) (bool, error) {
	status := service.DomainOutboxStatusPending
	if dead {
		status = service.DomainOutboxStatusDead
	}
	summary := sanitizeOutboxError(lastError)
	result, err := r.db.ExecContext(ctx, `
		UPDATE domain_outbox
		SET status = $1,
			next_attempt_at = $2,
			locked_at = NULL,
			locked_until = NULL,
			locked_by = NULL,
			last_error = $3
		WHERE id = $4
		  AND status = $5
		  AND locked_by = $6
		  AND locked_until > CURRENT_TIMESTAMP
	`,
		status,
		nextAttemptAt.Truncate(time.Microsecond),
		summary,
		id,
		service.DomainOutboxStatusProcessing,
		strings.TrimSpace(workerID),
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 1 {
		return true, nil
	}
	return false, r.classifyLeaseMiss(ctx, id, status)
}

func (r *domainOutboxRepository) ReapExpiredLeases(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	result, err := r.db.ExecContext(ctx, `
		WITH expired AS (
			SELECT id
			FROM domain_outbox
			WHERE status = $1 AND locked_until <= $2
			ORDER BY locked_until ASC, id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE domain_outbox AS event
		SET status = $4,
			locked_at = NULL,
			locked_until = NULL,
			locked_by = NULL
		FROM expired
		WHERE event.id = expired.id
	`,
		service.DomainOutboxStatusProcessing,
		now.Truncate(time.Microsecond),
		limit,
		service.DomainOutboxStatusPending,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *domainOutboxRepository) Counts(ctx context.Context) (service.DomainOutboxCounts, error) {
	var counts service.DomainOutboxCounts
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = $1),
			COUNT(*) FILTER (WHERE status = $2),
			COUNT(*) FILTER (WHERE status = $3),
			COUNT(*) FILTER (WHERE status = $4)
		FROM domain_outbox
	`,
		service.DomainOutboxStatusPending,
		service.DomainOutboxStatusProcessing,
		service.DomainOutboxStatusCompleted,
		service.DomainOutboxStatusDead,
	).Scan(&counts.Pending, &counts.Processing, &counts.Completed, &counts.Dead)
	return counts, err
}

func (r *domainOutboxRepository) classifyLeaseMiss(ctx context.Context, id int64, idempotentStatus string) error {
	var status string
	err := r.db.QueryRowContext(ctx, "SELECT status FROM domain_outbox WHERE id = $1", id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrDomainOutboxNotFound.WithCause(err)
	}
	if err != nil {
		return err
	}
	if status == idempotentStatus {
		return nil
	}
	return service.ErrDomainOutboxLeaseConflict
}

func scanDomainOutboxEvent(scanner sqlRowScanner) (*service.DomainOutboxEvent, error) {
	var (
		event                            service.DomainOutboxEvent
		payload                          []byte
		lockedAt, lockedUntil, completed sql.NullTime
		lockedBy, lastError              sql.NullString
	)
	if err := scanner.Scan(
		&event.ID,
		&event.AggregateType,
		&event.AggregateID,
		&event.EventType,
		&event.DedupKey,
		&payload,
		&event.Status,
		&event.AttemptCount,
		&event.NextAttemptAt,
		&lockedAt,
		&lockedUntil,
		&lockedBy,
		&lastError,
		&event.CreatedAt,
		&completed,
	); err != nil {
		return nil, err
	}
	event.Payload = append(json.RawMessage(nil), payload...)
	event.LockedAt = nullableTime(lockedAt)
	event.LockedUntil = nullableTime(lockedUntil)
	event.CompletedAt = nullableTime(completed)
	if lockedBy.Valid {
		value := lockedBy.String
		event.LockedBy = &value
	}
	if lastError.Valid {
		value := lastError.String
		event.LastError = &value
	}
	return &event, nil
}

func validateDomainOutboxInput(input *service.DomainOutboxEvent) (json.RawMessage, string, time.Time, error) {
	if input == nil {
		return nil, "", time.Time{}, fmt.Errorf("domain outbox event is required")
	}
	if strings.TrimSpace(input.AggregateType) == "" || strings.TrimSpace(input.EventType) == "" {
		return nil, "", time.Time{}, fmt.Errorf("outbox aggregate type and event type are required")
	}
	if strings.TrimSpace(input.DedupKey) == "" {
		return nil, "", time.Time{}, fmt.Errorf("outbox dedup key is required")
	}
	payload := input.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if !json.Valid(payload) {
		return nil, "", time.Time{}, fmt.Errorf("outbox payload must be valid JSON")
	}
	status := input.Status
	if status == "" {
		status = service.DomainOutboxStatusPending
	}
	if status != service.DomainOutboxStatusPending {
		return nil, "", time.Time{}, fmt.Errorf("new outbox event status must be pending")
	}
	nextAttemptAt := input.NextAttemptAt
	if nextAttemptAt.IsZero() {
		nextAttemptAt = time.Now().UTC()
	}
	return payload, status, nextAttemptAt.Truncate(time.Microsecond), nil
}

func sameDomainOutboxEnqueue(stored, input *service.DomainOutboxEvent) bool {
	if stored == nil || input == nil {
		return false
	}
	return stored.AggregateType == input.AggregateType &&
		stored.AggregateID == input.AggregateID &&
		stored.EventType == input.EventType &&
		stored.DedupKey == input.DedupKey &&
		jsonValuesEqual(stored.Payload, input.Payload)
}

func sanitizeOutboxError(input string) string {
	withoutQueries := outboxURLPattern.ReplaceAllStringFunc(input, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return "<redacted-url>"
		}
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		parsed.RawFragment = ""
		return parsed.String()
	})
	withoutCredentials := outboxCredentialHeaderPattern.ReplaceAllString(withoutQueries, "credential-header: ***")
	redacted := logredact.RedactText(
		outboxBearerPattern.ReplaceAllString(withoutCredentials, "Bearer ***"),
		"authorization",
		"api_key",
		"apikey",
		"token",
		"secret",
		"credential",
		"cookie",
		"set-cookie",
		"x-api-key",
	)
	redacted = strings.TrimSpace(redacted)
	encoded := []byte(redacted)
	if len(encoded) <= service.DomainOutboxMaxErrorSummaryBytes {
		return redacted
	}
	encoded = encoded[:service.DomainOutboxMaxErrorSummaryBytes]
	for len(encoded) > 0 && !utf8.Valid(encoded) {
		encoded = encoded[:len(encoded)-1]
	}
	return string(encoded)
}

func prefixedDomainOutboxColumns(prefix string) string {
	columns := strings.Split(domainOutboxColumns, ",")
	for index, column := range columns {
		columns[index] = prefix + "." + strings.TrimSpace(column)
	}
	return strings.Join(columns, ", ")
}
