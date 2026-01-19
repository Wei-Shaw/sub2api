package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageCleanupRepository struct {
	sql sqlExecutor
REDACTED

func NewUsageCleanupRepository(sqlDB *sql.DB) service.UsageCleanupRepository {
	return &usageCleanupRepository{sql: sqlDBREDACTED
REDACTED

func (r *usageCleanupRepository) CreateTask(ctx context.Context, task *service.UsageCleanupTask) error {
	if task == nil {
		return nil
REDACTED
	filtersJSON, err := json.Marshal(task.Filters)
	if err != nil {
		return fmt.Errorf("marshal cleanup filters: %w", err)
REDACTED
	query := `
		INSERT INTO usage_cleanup_tasks (
			status,
			filters,
			created_by,
			deleted_rows
		) VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	if err := scanSingleRow(ctx, r.sql, query, []any{task.Status, filtersJSON, task.CreatedBy, task.DeletedRowsREDACTED, &task.ID, &task.CreatedAt, &task.UpdatedAt); err != nil {
		return err
REDACTED
	return nil
REDACTED

func (r *usageCleanupRepository) ListTasks(ctx context.Context, params pagination.PaginationParams) ([]service.UsageCleanupTask, *pagination.PaginationResult, error) {
	var total int64
	if err := scanSingleRow(ctx, r.sql, "SELECT COUNT(*) FROM usage_cleanup_tasks", nil, &total); err != nil {
		return nil, nil, err
REDACTED
	if total == 0 {
		return []service.UsageCleanupTask{REDACTED, paginationResultFromTotal(0, params), nil
REDACTED

	query := `
		SELECT id, status, filters, created_by, deleted_rows, error_message,
			canceled_by, canceled_at,
			started_at, finished_at, created_at, updated_at
		FROM usage_cleanup_tasks
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.sql.QueryContext(ctx, query, params.Limit(), params.Offset())
	if err != nil {
		return nil, nil, err
REDACTED
	defer func() {
		_ = rows.Close()
REDACTED()

	tasks := make([]service.UsageCleanupTask, 0)
	for rows.Next() {
		var task service.UsageCleanupTask
		var filtersJSON []byte
		var errMsg sql.NullString
		var canceledBy sql.NullInt64
		var canceledAt sql.NullTime
		var startedAt sql.NullTime
		var finishedAt sql.NullTime
		if err := rows.Scan(
			&task.ID,
			&task.Status,
			&filtersJSON,
			&task.CreatedBy,
			&task.DeletedRows,
			&errMsg,
			&canceledBy,
			&canceledAt,
			&startedAt,
			&finishedAt,
			&task.CreatedAt,
			&task.UpdatedAt,
		); err != nil {
			return nil, nil, err
	REDACTED
		if err := json.Unmarshal(filtersJSON, &task.Filters); err != nil {
			return nil, nil, fmt.Errorf("parse cleanup filters: %w", err)
	REDACTED
		if errMsg.Valid {
			task.ErrorMsg = &errMsg.String
	REDACTED
		if canceledBy.Valid {
			v := canceledBy.Int64
			task.CanceledBy = &v
	REDACTED
		if canceledAt.Valid {
			task.CanceledAt = &canceledAt.Time
	REDACTED
		if startedAt.Valid {
			task.StartedAt = &startedAt.Time
	REDACTED
		if finishedAt.Valid {
			task.FinishedAt = &finishedAt.Time
	REDACTED
		tasks = append(tasks, task)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, nil, err
REDACTED
	return tasks, paginationResultFromTotal(total, params), nil
REDACTED

func (r *usageCleanupRepository) ClaimNextPendingTask(ctx context.Context, staleRunningAfterSeconds int64) (*service.UsageCleanupTask, error) {
	if staleRunningAfterSeconds <= 0 {
		staleRunningAfterSeconds = 1800
REDACTED
	query := `
		WITH next AS (
			SELECT id
			FROM usage_cleanup_tasks
			WHERE status = $1
				OR (
					status = $2
					AND started_at IS NOT NULL
					AND started_at < NOW() - ($3 * interval '1 second')
				)
			ORDER BY created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE usage_cleanup_tasks
		SET status = $4,
			started_at = NOW(),
			finished_at = NULL,
			error_message = NULL,
			updated_at = NOW()
		FROM next
		WHERE usage_cleanup_tasks.id = next.id
		RETURNING id, status, filters, created_by, deleted_rows, error_message,
			started_at, finished_at, created_at, updated_at
	`
	var task service.UsageCleanupTask
	var filtersJSON []byte
	var errMsg sql.NullString
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	if err := scanSingleRow(
		ctx,
		r.sql,
		query,
		[]any{
			service.UsageCleanupStatusPending,
			service.UsageCleanupStatusRunning,
			staleRunningAfterSeconds,
			service.UsageCleanupStatusRunning,
	REDACTED,
		&task.ID,
		&task.Status,
		&filtersJSON,
		&task.CreatedBy,
		&task.DeletedRows,
		&errMsg,
		&startedAt,
		&finishedAt,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
	REDACTED
		return nil, err
REDACTED
	if err := json.Unmarshal(filtersJSON, &task.Filters); err != nil {
		return nil, fmt.Errorf("parse cleanup filters: %w", err)
REDACTED
	if errMsg.Valid {
		task.ErrorMsg = &errMsg.String
REDACTED
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
REDACTED
	if finishedAt.Valid {
		task.FinishedAt = &finishedAt.Time
REDACTED
	return &task, nil
REDACTED

func (r *usageCleanupRepository) GetTaskStatus(ctx context.Context, taskID int64) (string, error) {
	var status string
	if err := scanSingleRow(ctx, r.sql, "SELECT status FROM usage_cleanup_tasks WHERE id = $1", []any{taskIDREDACTED, &status); err != nil {
		return "", err
REDACTED
	return status, nil
REDACTED

func (r *usageCleanupRepository) UpdateTaskProgress(ctx context.Context, taskID int64, deletedRows int64) error {
	query := `
		UPDATE usage_cleanup_tasks
		SET deleted_rows = $1,
			updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.sql.ExecContext(ctx, query, deletedRows, taskID)
	return err
REDACTED

func (r *usageCleanupRepository) CancelTask(ctx context.Context, taskID int64, canceledBy int64) (bool, error) {
	query := `
		UPDATE usage_cleanup_tasks
		SET status = $1,
			canceled_by = $3,
			canceled_at = NOW(),
			finished_at = NOW(),
			error_message = NULL,
			updated_at = NOW()
		WHERE id = $2
			AND status IN ($4, $5)
		RETURNING id
	`
	var id int64
	err := scanSingleRow(ctx, r.sql, query, []any{
		service.UsageCleanupStatusCanceled,
		taskID,
		canceledBy,
		service.UsageCleanupStatusPending,
		service.UsageCleanupStatusRunning,
REDACTED, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
REDACTED
	if err != nil {
		return false, err
REDACTED
	return true, nil
REDACTED

func (r *usageCleanupRepository) MarkTaskSucceeded(ctx context.Context, taskID int64, deletedRows int64) error {
	query := `
		UPDATE usage_cleanup_tasks
		SET status = $1,
			deleted_rows = $2,
			finished_at = NOW(),
			updated_at = NOW()
		WHERE id = $3
	`
	_, err := r.sql.ExecContext(ctx, query, service.UsageCleanupStatusSucceeded, deletedRows, taskID)
	return err
REDACTED

func (r *usageCleanupRepository) MarkTaskFailed(ctx context.Context, taskID int64, deletedRows int64, errorMsg string) error {
	query := `
		UPDATE usage_cleanup_tasks
		SET status = $1,
			deleted_rows = $2,
			error_message = $3,
			finished_at = NOW(),
			updated_at = NOW()
		WHERE id = $4
	`
	_, err := r.sql.ExecContext(ctx, query, service.UsageCleanupStatusFailed, deletedRows, errorMsg, taskID)
	return err
REDACTED

func (r *usageCleanupRepository) DeleteUsageLogsBatch(ctx context.Context, filters service.UsageCleanupFilters, limit int) (int64, error) {
	if filters.StartTime.IsZero() || filters.EndTime.IsZero() {
		return 0, fmt.Errorf("cleanup filters missing time range")
REDACTED
	whereClause, args := buildUsageCleanupWhere(filters)
	if whereClause == "" {
		return 0, fmt.Errorf("cleanup filters missing time range")
REDACTED
	args = append(args, limit)
	query := fmt.Sprintf(`
		WITH target AS (
			SELECT id
			FROM usage_logs
			WHERE %s
			ORDER BY created_at ASC, id ASC
			LIMIT $%d
		)
		DELETE FROM usage_logs
		WHERE id IN (SELECT id FROM target)
		RETURNING id
	`, whereClause, len(args))

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
REDACTED
	defer func() {
		_ = rows.Close()
REDACTED()

	var deleted int64
	for rows.Next() {
		deleted++
REDACTED
	if err := rows.Err(); err != nil {
		return 0, err
REDACTED
	return deleted, nil
REDACTED

func buildUsageCleanupWhere(filters service.UsageCleanupFilters) (string, []any) {
	conditions := make([]string, 0, 8)
	args := make([]any, 0, 8)
	idx := 1
	if !filters.StartTime.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, filters.StartTime)
		idx++
REDACTED
	if !filters.EndTime.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, filters.EndTime)
		idx++
REDACTED
	if filters.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", idx))
		args = append(args, *filters.UserID)
		idx++
REDACTED
	if filters.APIKeyID != nil {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", idx))
		args = append(args, *filters.APIKeyID)
		idx++
REDACTED
	if filters.AccountID != nil {
		conditions = append(conditions, fmt.Sprintf("account_id = $%d", idx))
		args = append(args, *filters.AccountID)
		idx++
REDACTED
	if filters.GroupID != nil {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", idx))
		args = append(args, *filters.GroupID)
		idx++
REDACTED
	if filters.Model != nil {
		model := strings.TrimSpace(*filters.Model)
		if model != "" {
			conditions = append(conditions, fmt.Sprintf("model = $%d", idx))
			args = append(args, model)
			idx++
	REDACTED
REDACTED
	if filters.Stream != nil {
		conditions = append(conditions, fmt.Sprintf("stream = $%d", idx))
		args = append(args, *filters.Stream)
		idx++
REDACTED
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", idx))
		args = append(args, *filters.BillingType)
REDACTED
	return strings.Join(conditions, " AND "), args
REDACTED
