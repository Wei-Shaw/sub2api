package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

type backgroundTaskRepository struct {
	db *sql.DB
}

func NewBackgroundTaskRepository(db *sql.DB) service.BackgroundTaskRepository {
	return &backgroundTaskRepository{db: db}
}

const backgroundTaskColumns = `
id, task_type, resource_type, resource_id, payload, display, run_at, status,
attempt_count, dispatch_count, max_attempts, dedupe_key, idempotency_key,
creation_request_key, dedupe_locked, claim_owner, claim_version, lease_until, first_dispatch_at,
last_dispatch_at, result_code, result, last_error_code, last_error_message,
created_by, canceled_by, canceled_at, started_at, finished_at, created_at, updated_at`

func (r *backgroundTaskRepository) Create(ctx context.Context, input *service.CreateBackgroundTaskInput) (*service.BackgroundTaskRun, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, errors.New("nil background task repository")
	}
	if err := input.Validate(); err != nil {
		return nil, false, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	// A creation request is serialized before resource dedupe. This ensures that
	// the same request key always returns one task even when concurrent callers
	// submit different resource or dedupe values.
	if input.CreationRequestKey != "" {
		if err := lockBackgroundTaskCreate(ctx, tx, "creation", input.CreationRequestKey); err != nil {
			return nil, false, err
		}
		existing, findErr := findBackgroundTaskByCreationRequestKey(ctx, tx, input.CreationRequestKey)
		if findErr == nil {
			if commitErr := tx.Commit(); commitErr != nil {
				return nil, false, commitErr
			}
			return existing, false, nil
		}
		if !errors.Is(findErr, service.ErrBackgroundTaskNotFound) {
			return nil, false, findErr
		}
	}

	// Serialize creators for one logical resource before generating its external
	// idempotency key. The partial unique index remains the final safety net.
	lockKey := fmt.Sprintf("%d:%s%s", len(input.TaskType), input.TaskType, input.DedupeKey)
	if err := lockBackgroundTaskCreate(ctx, tx, "dedupe", lockKey); err != nil {
		return nil, false, err
	}
	existing, err := findBackgroundTaskDedupeOwner(ctx, tx, input.TaskType, input.DedupeKey)
	if err == nil {
		if bindErr := bindBackgroundTaskCreationRequest(ctx, tx, input.CreationRequestKey, existing.ID); bindErr != nil {
			return nil, false, bindErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, false, commitErr
		}
		return existing, false, nil
	}
	if !errors.Is(err, service.ErrBackgroundTaskNotFound) {
		return nil, false, err
	}

	idempotencyKey := input.IdempotencyKey
	if input.GenerateIdempotencyKey {
		generated := uuid.NewString()
		idempotencyKey = &generated
	}
	q := `INSERT INTO background_task_runs (
		task_type, resource_type, resource_id, payload, display, run_at,
		status, max_attempts, dedupe_key, idempotency_key, creation_request_key, created_by
	) VALUES ($1,$2,$3,$4,$5,$6,'pending',$7,$8,$9,NULLIF($10, ''),$11)
	RETURNING ` + backgroundTaskColumns
	task, err := scanBackgroundTask(tx.QueryRowContext(
		ctx, q, input.TaskType, input.ResourceType, input.ResourceID,
		[]byte(input.Payload), []byte(input.Display), input.RunAt, input.MaxAttempts,
		input.DedupeKey, idempotencyKey, input.CreationRequestKey, input.CreatedBy,
	))
	if err != nil {
		return nil, false, err
	}
	if err := bindBackgroundTaskCreationRequest(ctx, tx, input.CreationRequestKey, task.ID); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return task, true, nil
}

func bindBackgroundTaskCreationRequest(ctx context.Context, tx *sql.Tx, key string, taskID int64) error {
	if key == "" {
		return nil
	}
	var mappedTaskID int64
	err := tx.QueryRowContext(ctx, `INSERT INTO background_task_creation_requests (request_key, task_id)
		VALUES ($1, $2)
		ON CONFLICT (request_key) DO UPDATE SET request_key = EXCLUDED.request_key
		RETURNING task_id`, key, taskID).Scan(&mappedTaskID)
	if err != nil {
		return err
	}
	if mappedTaskID != taskID {
		return service.ErrBackgroundTaskConflict
	}
	return nil
}

func lockBackgroundTaskCreate(ctx context.Context, tx *sql.Tx, namespace, key string) error {
	_, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		namespace+":"+key,
	)
	return err
}

type backgroundTaskQueryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func findBackgroundTaskDedupeOwner(ctx context.Context, queryer backgroundTaskQueryRower, taskType, dedupeKey string) (*service.BackgroundTaskRun, error) {
	q := `SELECT ` + backgroundTaskColumns + ` FROM background_task_runs
		WHERE task_type = $1 AND dedupe_key = $2
		  AND (dedupe_locked OR status IN ('pending','running','retry_wait'))
		ORDER BY id DESC LIMIT 1`
	task, err := scanBackgroundTask(queryer.QueryRowContext(ctx, q, taskType, dedupeKey))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBackgroundTaskNotFound
	}
	return task, err
}

func findBackgroundTaskByCreationRequestKey(ctx context.Context, queryer backgroundTaskQueryRower, key string) (*service.BackgroundTaskRun, error) {
	task, err := scanBackgroundTask(queryer.QueryRowContext(ctx, `SELECT `+prefixedBackgroundTaskColumns("task")+`
		FROM background_task_creation_requests AS creation
		JOIN background_task_runs AS task ON task.id = creation.task_id
		WHERE creation.request_key = $1`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBackgroundTaskNotFound
	}
	return task, err
}

func (r *backgroundTaskRepository) GetByID(ctx context.Context, id int64) (*service.BackgroundTaskRun, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil background task repository")
	}
	task, err := scanBackgroundTask(r.db.QueryRowContext(ctx, `SELECT `+backgroundTaskColumns+` FROM background_task_runs WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBackgroundTaskNotFound
	}
	return task, err
}

func (r *backgroundTaskRepository) GetByCreationRequestKey(ctx context.Context, key string) (*service.BackgroundTaskRun, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil background task repository")
	}
	if key == "" {
		return nil, service.ErrBackgroundTaskNotFound
	}
	return findBackgroundTaskByCreationRequestKey(ctx, r.db, key)
}

func (r *backgroundTaskRepository) List(ctx context.Context, filter service.BackgroundTaskListFilter) (*service.BackgroundTaskListResult, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("nil background task repository")
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, service.ErrBackgroundTaskInvalidStatus
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	where := []string{"TRUE"}
	args := make([]any, 0, 6)
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if filter.TaskType != "" {
		add("task_type = $%d", filter.TaskType)
	}
	if filter.Status != "" {
		add("status = $%d", string(filter.Status))
	}
	if filter.ResourceType != "" {
		add("resource_type = $%d", filter.ResourceType)
	}
	if filter.ResourceID != "" {
		add("resource_id = $%d", filter.ResourceID)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM background_task_runs WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, err
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	q := fmt.Sprintf(
		`SELECT %s FROM background_task_runs WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d`,
		backgroundTaskColumns, whereSQL, len(args)-1, len(args),
	)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.BackgroundTaskRun, 0, filter.PageSize)
	for rows.Next() {
		task, scanErr := scanBackgroundTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.BackgroundTaskListResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *backgroundTaskRepository) CountBacklog(ctx context.Context, now time.Time) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM background_task_runs
		WHERE (status IN ('pending','retry_wait') AND run_at <= $1)
		   OR (status = 'running' AND lease_until < $1)`, now).Scan(&count)
	return count, err
}

func (r *backgroundTaskRepository) ClaimDue(ctx context.Context, owner string, now, leaseUntil time.Time, limit int) ([]*service.BackgroundTaskRun, error) {
	if owner == "" {
		return nil, errors.New("claim owner required")
	}
	if limit <= 0 {
		return []*service.BackgroundTaskRun{}, nil
	}
	q := `WITH candidates AS (
		SELECT id FROM background_task_runs
		WHERE ((status IN ('pending','retry_wait') AND run_at <= $1)
		    OR (status = 'running' AND lease_until < $1))
		ORDER BY run_at ASC, id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT $2
	)
	UPDATE background_task_runs AS task
	SET status = 'running', claim_owner = $3, claim_version = task.claim_version + 1,
		lease_until = $4, attempt_count = task.attempt_count + 1,
		started_at = COALESCE(task.started_at, $1), updated_at = $1
	FROM candidates WHERE task.id = candidates.id
	RETURNING ` + prefixedBackgroundTaskColumns("task")
	rows, err := r.db.QueryContext(ctx, q, now, limit, owner, leaseUntil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*service.BackgroundTaskRun, 0, limit)
	for rows.Next() {
		task, scanErr := scanBackgroundTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, task)
	}
	return items, rows.Err()
}

func (r *backgroundTaskRepository) RenewLease(ctx context.Context, id int64, owner string, claimVersion int64, leaseUntil time.Time) error {
	return requireBackgroundTaskUpdate(r.db.ExecContext(ctx, `UPDATE background_task_runs
		SET lease_until = $4, updated_at = NOW()
		WHERE id = $1 AND status = 'running' AND claim_owner = $2 AND claim_version = $3`, id, owner, claimVersion, leaseUntil))
}

func (r *backgroundTaskRepository) BeginDispatch(ctx context.Context, id int64, owner string, claimVersion int64, now time.Time) error {
	return requireBackgroundTaskUpdate(r.db.ExecContext(ctx, `UPDATE background_task_runs
		SET first_dispatch_at = COALESCE(first_dispatch_at, $4), last_dispatch_at = $4,
			dispatch_count = dispatch_count + 1, dedupe_locked = true, updated_at = $4
		WHERE id = $1 AND status = 'running' AND claim_owner = $2
		  AND claim_version = $3 AND lease_until >= $4`, id, owner, claimVersion, now))
}

func (r *backgroundTaskRepository) ScheduleRetry(ctx context.Context, id int64, owner string, claimVersion int64, runAt time.Time, errorCode, errorMessage string) error {
	return requireBackgroundTaskUpdate(r.db.ExecContext(ctx, `UPDATE background_task_runs
		SET status = 'retry_wait', run_at = $4, claim_owner = NULL, lease_until = NULL,
			last_error_code = NULLIF($5, ''), last_error_message = NULLIF($6, ''), updated_at = NOW()
		WHERE id = $1 AND status = 'running' AND claim_owner = $2 AND claim_version = $3`,
		id, owner, claimVersion, runAt, errorCode, errorMessage))
}

func (r *backgroundTaskRepository) Finish(ctx context.Context, id int64, owner string, claimVersion int64, input service.BackgroundTaskFinishInput) error {
	switch input.Status {
	case service.BackgroundTaskStatusSucceeded, service.BackgroundTaskStatusSkipped,
		service.BackgroundTaskStatusFailed, service.BackgroundTaskStatusIndeterminate:
	default:
		return service.ErrBackgroundTaskInvalidStatus
	}
	if input.FinishedAt.IsZero() {
		input.FinishedAt = time.Now()
	}
	var result any
	if len(input.Result) > 0 {
		result = []byte(input.Result)
	}
	if result != nil && !json.Valid(input.Result) {
		return service.ErrBackgroundTaskInvalidPayload
	}
	return requireBackgroundTaskUpdate(r.db.ExecContext(ctx, `UPDATE background_task_runs
		SET status = $4, claim_owner = NULL, lease_until = NULL,
			result_code = NULLIF($5, ''), result = $6,
			last_error_code = NULLIF($7, ''), last_error_message = NULLIF($8, ''),
			dedupe_locked = CASE WHEN $9 THEN false ELSE dedupe_locked END,
			finished_at = $10, updated_at = $10
		WHERE id = $1 AND status = 'running' AND claim_owner = $2 AND claim_version = $3`,
		id, owner, claimVersion, string(input.Status), input.ResultCode, result,
		input.ErrorCode, input.ErrorMessage, input.ReleaseDedupeLock, input.FinishedAt))
}

func (r *backgroundTaskRepository) Cancel(ctx context.Context, id, canceledBy int64, now time.Time) (*service.BackgroundTaskRun, error) {
	q := `UPDATE background_task_runs SET status = 'canceled', canceled_by = $2,
		canceled_at = $3, finished_at = $3, claim_owner = NULL, lease_until = NULL,
		updated_at = $3
	WHERE id = $1 AND status IN ('pending','running','retry_wait') AND first_dispatch_at IS NULL
	RETURNING ` + backgroundTaskColumns
	task, err := scanBackgroundTask(r.db.QueryRowContext(ctx, q, id, canceledBy, now))
	if err == nil {
		return task, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, getErr := r.GetByID(ctx, id); errors.Is(getErr, service.ErrBackgroundTaskNotFound) {
		return nil, service.ErrBackgroundTaskNotFound
	} else if getErr != nil {
		return nil, getErr
	}
	return nil, service.ErrBackgroundTaskCannotCancel
}

func (r *backgroundTaskRepository) RequeueIndeterminate(ctx context.Context, id int64, now time.Time) (*service.BackgroundTaskRun, error) {
	q := `UPDATE background_task_runs SET status = 'pending', run_at = $2,
		max_attempts = GREATEST(max_attempts, dispatch_count + 1),
		claim_owner = NULL, lease_until = NULL, finished_at = NULL, updated_at = $2
	WHERE id = $1 AND status = 'indeterminate' AND dedupe_locked = true
	  AND idempotency_key IS NOT NULL
	RETURNING ` + backgroundTaskColumns
	task, err := scanBackgroundTask(r.db.QueryRowContext(ctx, q, id, now))
	if err == nil {
		return task, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, getErr := r.GetByID(ctx, id); errors.Is(getErr, service.ErrBackgroundTaskNotFound) {
		return nil, service.ErrBackgroundTaskNotFound
	} else if getErr != nil {
		return nil, getErr
	}
	return nil, service.ErrBackgroundTaskCannotRetry
}

type backgroundTaskScanner interface {
	Scan(dest ...any) error
}

func scanBackgroundTask(scanner backgroundTaskScanner) (*service.BackgroundTaskRun, error) {
	var task service.BackgroundTaskRun
	var payload, display, result []byte
	var status string
	var idempotencyKey, creationRequestKey, claimOwner, resultCode, errorCode, errorMessage sql.NullString
	var leaseUntil, firstDispatchAt, lastDispatchAt, canceledAt, startedAt, finishedAt sql.NullTime
	var canceledBy sql.NullInt64
	err := scanner.Scan(
		&task.ID, &task.TaskType, &task.ResourceType, &task.ResourceID, &payload, &display,
		&task.RunAt, &status, &task.AttemptCount, &task.DispatchCount, &task.MaxAttempts,
		&task.DedupeKey, &idempotencyKey, &creationRequestKey, &task.DedupeLocked, &claimOwner,
		&task.ClaimVersion, &leaseUntil, &firstDispatchAt, &lastDispatchAt,
		&resultCode, &result, &errorCode, &errorMessage, &task.CreatedBy, &canceledBy,
		&canceledAt, &startedAt, &finishedAt, &task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	task.Status = service.BackgroundTaskStatus(status)
	task.Payload = cloneRawJSON(payload)
	task.Display = cloneRawJSON(display)
	task.Result = cloneRawJSON(result)
	task.IdempotencyKey = nullStringPointer(idempotencyKey)
	task.CreationRequestKey = nullStringPointer(creationRequestKey)
	task.ClaimOwner = nullStringPointer(claimOwner)
	task.ResultCode = nullStringPointer(resultCode)
	task.LastErrorCode = nullStringPointer(errorCode)
	task.LastErrorMessage = nullStringPointer(errorMessage)
	task.LeaseUntil = nullTimePointer(leaseUntil)
	task.FirstDispatchAt = nullTimePointer(firstDispatchAt)
	task.LastDispatchAt = nullTimePointer(lastDispatchAt)
	task.CanceledAt = nullTimePointer(canceledAt)
	task.StartedAt = nullTimePointer(startedAt)
	task.FinishedAt = nullTimePointer(finishedAt)
	if canceledBy.Valid {
		value := canceledBy.Int64
		task.CanceledBy = &value
	}
	return &task, nil
}

func prefixedBackgroundTaskColumns(prefix string) string {
	parts := strings.Split(backgroundTaskColumns, ",")
	for i, part := range parts {
		parts[i] = prefix + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func requireBackgroundTaskUpdate(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return service.ErrBackgroundTaskLeaseLost
	}
	return nil
}

func cloneRawJSON(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	out := value.String
	return &out
}

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	out := value.Time
	return &out
}
