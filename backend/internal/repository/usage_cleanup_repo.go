package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbusagecleanuptask "github.com/Wei-Shaw/sub2api/ent/usagecleanuptask"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageCleanupRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewUsageCleanupRepository(client *dbent.Client, sqlDB *sql.DB) service.UsageCleanupRepository {
	return newUsageCleanupRepositoryWithSQL(client, sqlDB)
}

func newUsageCleanupRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *usageCleanupRepository {
	return &usageCleanupRepository{client: client, sql: sqlq}
}

func (r *usageCleanupRepository) CreateTask(ctx context.Context, task *service.UsageCleanupTask) error {
	if task == nil {
		return nil
	}
	if r.client != nil {
		return r.createTaskWithEnt(ctx, task)
	}
	return r.createTaskWithSQL(ctx, task)
}

func (r *usageCleanupRepository) ListTasks(ctx context.Context, params pagination.PaginationParams) ([]service.UsageCleanupTask, *pagination.PaginationResult, error) {
	if r.client != nil {
		return r.listTasksWithEnt(ctx, params)
	}
	if IsMySQL() {
		return r.listTasksMySQL(ctx, params)
	}
	var total int64
	if err := scanSingleRow(ctx, r.sql, "SELECT COUNT(*) FROM usage_cleanup_tasks", nil, &total); err != nil {
		return nil, nil, err
	}
	if total == 0 {
		return []service.UsageCleanupTask{}, paginationResultFromTotal(0, params), nil
	}

	query := `
		SELECT id, status, filters, created_by, deleted_rows, error_message,
			canceled_by, canceled_at,
			started_at, finished_at, created_at, updated_at
		FROM usage_cleanup_tasks
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.sql.QueryContext(ctx, query, params.Limit(), params.Offset())
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

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
		}
		if err := json.Unmarshal(filtersJSON, &task.Filters); err != nil {
			return nil, nil, fmt.Errorf("parse cleanup filters: %w", err)
		}
		if errMsg.Valid {
			task.ErrorMsg = &errMsg.String
		}
		if canceledBy.Valid {
			v := canceledBy.Int64
			task.CanceledBy = &v
		}
		if canceledAt.Valid {
			task.CanceledAt = &canceledAt.Time
		}
		if startedAt.Valid {
			task.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			task.FinishedAt = &finishedAt.Time
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return tasks, paginationResultFromTotal(total, params), nil
}

func (r *usageCleanupRepository) ClaimNextPendingTask(ctx context.Context, staleRunningAfterSeconds int64) (*service.UsageCleanupTask, error) {
	if staleRunningAfterSeconds <= 0 {
		staleRunningAfterSeconds = 1800
	}
	if IsMySQL() {
		return r.claimNextPendingTaskMySQL(ctx, staleRunningAfterSeconds)
	}
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
		UPDATE usage_cleanup_tasks AS tasks
		SET status = $4,
			started_at = NOW(),
			finished_at = NULL,
			error_message = NULL,
			updated_at = NOW()
		FROM next
		WHERE tasks.id = next.id
		RETURNING tasks.id, tasks.status, tasks.filters, tasks.created_by, tasks.deleted_rows, tasks.error_message,
			tasks.started_at, tasks.finished_at, tasks.created_at, tasks.updated_at
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
		},
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
		}
		return nil, err
	}
	if err := json.Unmarshal(filtersJSON, &task.Filters); err != nil {
		return nil, fmt.Errorf("parse cleanup filters: %w", err)
	}
	if errMsg.Valid {
		task.ErrorMsg = &errMsg.String
	}
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		task.FinishedAt = &finishedAt.Time
	}
	return &task, nil
}

func (r *usageCleanupRepository) GetTaskStatus(ctx context.Context, taskID int64) (string, error) {
	if r.client != nil {
		return r.getTaskStatusWithEnt(ctx, taskID)
	}
	if IsMySQL() {
		var status string
		if err := scanSingleRow(ctx, r.sql, "SELECT status FROM usage_cleanup_tasks WHERE id = ?", []any{taskID}, &status); err != nil {
			return "", err
		}
		return status, nil
	}
	var status string
	if err := scanSingleRow(ctx, r.sql, "SELECT status FROM usage_cleanup_tasks WHERE id = $1", []any{taskID}, &status); err != nil {
		return "", err
	}
	return status, nil
}

func (r *usageCleanupRepository) UpdateTaskProgress(ctx context.Context, taskID int64, deletedRows int64) error {
	if r.client != nil {
		return r.updateTaskProgressWithEnt(ctx, taskID, deletedRows)
	}
	if IsMySQL() {
		query := `
			UPDATE usage_cleanup_tasks
			SET deleted_rows = ?,
				updated_at = NOW(3)
			WHERE id = ?
		`
		_, err := r.sql.ExecContext(ctx, query, deletedRows, taskID)
		return err
	}
	query := `
		UPDATE usage_cleanup_tasks
		SET deleted_rows = $1,
			updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.sql.ExecContext(ctx, query, deletedRows, taskID)
	return err
}

func (r *usageCleanupRepository) CancelTask(ctx context.Context, taskID int64, canceledBy int64) (bool, error) {
	if r.client != nil {
		return r.cancelTaskWithEnt(ctx, taskID, canceledBy)
	}
	if IsMySQL() {
		return r.cancelTaskMySQL(ctx, taskID, canceledBy)
	}
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
	}, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *usageCleanupRepository) MarkTaskSucceeded(ctx context.Context, taskID int64, deletedRows int64) error {
	if r.client != nil {
		return r.markTaskSucceededWithEnt(ctx, taskID, deletedRows)
	}
	if IsMySQL() {
		query := `
			UPDATE usage_cleanup_tasks
			SET status = ?,
				deleted_rows = ?,
				finished_at = NOW(3),
				updated_at = NOW(3)
			WHERE id = ?
		`
		_, err := r.sql.ExecContext(ctx, query, service.UsageCleanupStatusSucceeded, deletedRows, taskID)
		return err
	}
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
}

func (r *usageCleanupRepository) MarkTaskFailed(ctx context.Context, taskID int64, deletedRows int64, errorMsg string) error {
	if r.client != nil {
		return r.markTaskFailedWithEnt(ctx, taskID, deletedRows, errorMsg)
	}
	if IsMySQL() {
		query := `
			UPDATE usage_cleanup_tasks
			SET status = ?,
				deleted_rows = ?,
				error_message = ?,
				finished_at = NOW(3),
				updated_at = NOW(3)
			WHERE id = ?
		`
		_, err := r.sql.ExecContext(ctx, query, service.UsageCleanupStatusFailed, deletedRows, errorMsg, taskID)
		return err
	}
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
}

func (r *usageCleanupRepository) DeleteUsageLogsBatch(ctx context.Context, filters service.UsageCleanupFilters, limit int) (int64, error) {
	if filters.StartTime.IsZero() || filters.EndTime.IsZero() {
		return 0, fmt.Errorf("cleanup filters missing time range")
	}
	if IsMySQL() {
		return r.deleteUsageLogsBatchMySQL(ctx, filters, limit)
	}
	whereClause, args := buildUsageCleanupWhere(filters)
	if whereClause == "" {
		return 0, fmt.Errorf("cleanup filters missing time range")
	}
	args = append(args, limit)
	if db, ok := r.sql.(*sql.DB); ok {
		return r.deleteUsageLogsBatchWithRollupInvalidation(ctx, db, whereClause, args)
	}
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
		RETURNING created_at
	`, whereClause, len(args))

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	var deleted int64
	for rows.Next() {
		deleted++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (r *usageCleanupRepository) deleteUsageLogsBatchWithRollupInvalidation(ctx context.Context, db *sql.DB, whereClause string, args []any) (int64, error) {
	if IsMySQL() {
		return deleteUsageLogsBatchWithRollupInvalidationMySQL(ctx, db, whereClause, args)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	rollback := func(err error) (int64, error) {
		_ = tx.Rollback()
		return 0, err
	}

	if err := lockGroupUsageRollupState(ctx, tx); err != nil {
		return rollback(err)
	}
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
		RETURNING created_at
	`, whereClause, len(args))
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return rollback(err)
	}

	var deleted int64
	var earliestDeletedAt time.Time
	for rows.Next() {
		var deletedAt time.Time
		if err := rows.Scan(&deletedAt); err != nil {
			_ = rows.Close()
			return rollback(err)
		}
		deleted++
		if earliestDeletedAt.IsZero() || deletedAt.Before(earliestDeletedAt) {
			earliestDeletedAt = deletedAt
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return rollback(err)
	}
	if err := rows.Close(); err != nil {
		return rollback(err)
	}

	if deleted > 0 {
		if err := invalidateGroupUsageRollupsAt(ctx, tx, earliestDeletedAt); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// buildUsageCleanupWhereMySQL 是 buildUsageCleanupWhere 的 MySQL 版：
// 条件一一对应，仅占位符改 ?（request_type 复用 buildRequestTypeFilterConditionMySQL）。
func buildUsageCleanupWhereMySQL(filters service.UsageCleanupFilters) (string, []any) {
	conditions := make([]string, 0, 8)
	args := make([]any, 0, 8)
	if !filters.StartTime.IsZero() {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, filters.StartTime)
	}
	if !filters.EndTime.IsZero() {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, filters.EndTime)
	}
	if filters.UserID != nil {
		conditions = append(conditions, "user_id = ?")
		args = append(args, *filters.UserID)
	}
	if filters.APIKeyID != nil {
		conditions = append(conditions, "api_key_id = ?")
		args = append(args, *filters.APIKeyID)
	}
	if filters.AccountID != nil {
		conditions = append(conditions, "account_id = ?")
		args = append(args, *filters.AccountID)
	}
	if filters.GroupID != nil {
		conditions = append(conditions, "group_id = ?")
		args = append(args, *filters.GroupID)
	}
	if filters.Model != nil {
		model := strings.TrimSpace(*filters.Model)
		if model != "" {
			conditions = append(conditions, "model = ?")
			args = append(args, model)
		}
	}
	if filters.RequestType != nil {
		condition, conditionArgs := buildRequestTypeFilterConditionMySQL(*filters.RequestType)
		conditions = append(conditions, condition)
		args = append(args, conditionArgs...)
	} else if filters.Stream != nil {
		conditions = append(conditions, "stream = ?")
		args = append(args, *filters.Stream)
	}
	if filters.BillingType != nil {
		conditions = append(conditions, "billing_type = ?")
		args = append(args, *filters.BillingType)
	}
	return strings.Join(conditions, " AND "), args
}

func buildUsageCleanupWhere(filters service.UsageCleanupFilters) (string, []any) {
	conditions := make([]string, 0, 8)
	args := make([]any, 0, 8)
	idx := 1
	if !filters.StartTime.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, filters.StartTime)
		idx++
	}
	if !filters.EndTime.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, filters.EndTime)
		idx++
	}
	if filters.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", idx))
		args = append(args, *filters.UserID)
		idx++
	}
	if filters.APIKeyID != nil {
		conditions = append(conditions, fmt.Sprintf("api_key_id = $%d", idx))
		args = append(args, *filters.APIKeyID)
		idx++
	}
	if filters.AccountID != nil {
		conditions = append(conditions, fmt.Sprintf("account_id = $%d", idx))
		args = append(args, *filters.AccountID)
		idx++
	}
	if filters.GroupID != nil {
		conditions = append(conditions, fmt.Sprintf("group_id = $%d", idx))
		args = append(args, *filters.GroupID)
		idx++
	}
	if filters.Model != nil {
		model := strings.TrimSpace(*filters.Model)
		if model != "" {
			conditions = append(conditions, fmt.Sprintf("model = $%d", idx))
			args = append(args, model)
			idx++
		}
	}
	if filters.RequestType != nil {
		condition, conditionArgs := buildRequestTypeFilterCondition(idx, *filters.RequestType)
		conditions = append(conditions, condition)
		args = append(args, conditionArgs...)
		idx += len(conditionArgs)
	} else if filters.Stream != nil {
		conditions = append(conditions, fmt.Sprintf("stream = $%d", idx))
		args = append(args, *filters.Stream)
		idx++
	}
	if filters.BillingType != nil {
		conditions = append(conditions, fmt.Sprintf("billing_type = $%d", idx))
		args = append(args, *filters.BillingType)
	}
	return strings.Join(conditions, " AND "), args
}

func (r *usageCleanupRepository) createTaskWithEnt(ctx context.Context, task *service.UsageCleanupTask) error {
	client := clientFromContext(ctx, r.client)
	filtersJSON, err := json.Marshal(task.Filters)
	if err != nil {
		return fmt.Errorf("marshal cleanup filters: %w", err)
	}
	created, err := client.UsageCleanupTask.
		Create().
		SetStatus(task.Status).
		SetFilters(json.RawMessage(filtersJSON)).
		SetCreatedBy(task.CreatedBy).
		SetDeletedRows(task.DeletedRows).
		Save(ctx)
	if err != nil {
		return err
	}
	task.ID = created.ID
	task.CreatedAt = created.CreatedAt
	task.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *usageCleanupRepository) createTaskWithSQL(ctx context.Context, task *service.UsageCleanupTask) error {
	if IsMySQL() {
		return r.createTaskWithSQLMySQL(ctx, task)
	}
	filtersJSON, err := json.Marshal(task.Filters)
	if err != nil {
		return fmt.Errorf("marshal cleanup filters: %w", err)
	}
	query := `
		INSERT INTO usage_cleanup_tasks (
			status,
			filters,
			created_by,
			deleted_rows
		) VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`
	if err := scanSingleRow(ctx, r.sql, query, []any{task.Status, filtersJSON, task.CreatedBy, task.DeletedRows}, &task.ID, &task.CreatedAt, &task.UpdatedAt); err != nil {
		return err
	}
	return nil
}

func (r *usageCleanupRepository) listTasksWithEnt(ctx context.Context, params pagination.PaginationParams) ([]service.UsageCleanupTask, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	query := client.UsageCleanupTask.Query()
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	if total == 0 {
		return []service.UsageCleanupTask{}, paginationResultFromTotal(0, params), nil
	}
	rows, err := query.
		Order(dbent.Desc(dbusagecleanuptask.FieldCreatedAt), dbent.Desc(dbusagecleanuptask.FieldID)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	tasks := make([]service.UsageCleanupTask, 0, len(rows))
	for _, row := range rows {
		task, err := usageCleanupTaskFromEnt(row)
		if err != nil {
			return nil, nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, paginationResultFromTotal(int64(total), params), nil
}

func (r *usageCleanupRepository) getTaskStatusWithEnt(ctx context.Context, taskID int64) (string, error) {
	client := clientFromContext(ctx, r.client)
	task, err := client.UsageCleanupTask.Query().
		Where(dbusagecleanuptask.IDEQ(taskID)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return "", sql.ErrNoRows
		}
		return "", err
	}
	return task.Status, nil
}

func (r *usageCleanupRepository) updateTaskProgressWithEnt(ctx context.Context, taskID int64, deletedRows int64) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	_, err := client.UsageCleanupTask.Update().
		Where(dbusagecleanuptask.IDEQ(taskID)).
		SetDeletedRows(deletedRows).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *usageCleanupRepository) cancelTaskWithEnt(ctx context.Context, taskID int64, canceledBy int64) (bool, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	affected, err := client.UsageCleanupTask.Update().
		Where(
			dbusagecleanuptask.IDEQ(taskID),
			dbusagecleanuptask.StatusIn(service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning),
		).
		SetStatus(service.UsageCleanupStatusCanceled).
		SetCanceledBy(canceledBy).
		SetCanceledAt(now).
		SetFinishedAt(now).
		ClearErrorMessage().
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *usageCleanupRepository) markTaskSucceededWithEnt(ctx context.Context, taskID int64, deletedRows int64) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	_, err := client.UsageCleanupTask.Update().
		Where(dbusagecleanuptask.IDEQ(taskID)).
		SetStatus(service.UsageCleanupStatusSucceeded).
		SetDeletedRows(deletedRows).
		SetFinishedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *usageCleanupRepository) markTaskFailedWithEnt(ctx context.Context, taskID int64, deletedRows int64, errorMsg string) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	_, err := client.UsageCleanupTask.Update().
		Where(dbusagecleanuptask.IDEQ(taskID)).
		SetStatus(service.UsageCleanupStatusFailed).
		SetDeletedRows(deletedRows).
		SetErrorMessage(errorMsg).
		SetFinishedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func usageCleanupTaskFromEnt(row *dbent.UsageCleanupTask) (service.UsageCleanupTask, error) {
	task := service.UsageCleanupTask{
		ID:          row.ID,
		Status:      row.Status,
		CreatedBy:   row.CreatedBy,
		DeletedRows: row.DeletedRows,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if len(row.Filters) > 0 {
		if err := json.Unmarshal(row.Filters, &task.Filters); err != nil {
			return service.UsageCleanupTask{}, fmt.Errorf("parse cleanup filters: %w", err)
		}
	}
	if row.ErrorMessage != nil {
		task.ErrorMsg = row.ErrorMessage
	}
	if row.CanceledBy != nil {
		task.CanceledBy = row.CanceledBy
	}
	if row.CanceledAt != nil {
		task.CanceledAt = row.CanceledAt
	}
	if row.StartedAt != nil {
		task.StartedAt = row.StartedAt
	}
	if row.FinishedAt != nil {
		task.FinishedAt = row.FinishedAt
	}
	return task, nil
}

// ---------------------------------------------------------------------------
// MySQL 方言实现（见 DIALECT_MYSQL_GUIDE.md）。ent 路径（*WithEnt）由 ent 处理方言，无需分支。
// ---------------------------------------------------------------------------

// usageCleanupTaskRowColumns 是 usage_cleanup_tasks 行扫描的列清单（PG/MySQL 共用）。
const usageCleanupTaskRowColumns = `id, status, filters, created_by, deleted_rows, error_message,
			canceled_by, canceled_at, started_at, finished_at, created_at, updated_at`

// scanUsageCleanupTaskRowMySQL 扫描一行 usage_cleanup_tasks 到 service.UsageCleanupTask。
func scanUsageCleanupTaskRowMySQL(scan func(dest ...any) error) (*service.UsageCleanupTask, error) {
	var task service.UsageCleanupTask
	var filtersJSON []byte
	var errMsg sql.NullString
	var canceledBy sql.NullInt64
	var canceledAt sql.NullTime
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	if err := scan(
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
		return nil, err
	}
	if err := json.Unmarshal(filtersJSON, &task.Filters); err != nil {
		return nil, fmt.Errorf("parse cleanup filters: %w", err)
	}
	if errMsg.Valid {
		task.ErrorMsg = &errMsg.String
	}
	if canceledBy.Valid {
		v := canceledBy.Int64
		task.CanceledBy = &v
	}
	if canceledAt.Valid {
		task.CanceledAt = &canceledAt.Time
	}
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		task.FinishedAt = &finishedAt.Time
	}
	return &task, nil
}

// listTasksMySQL 是 ListTasks 纯 SQL 路径的 MySQL 实现（$n→?）。
func (r *usageCleanupRepository) listTasksMySQL(ctx context.Context, params pagination.PaginationParams) ([]service.UsageCleanupTask, *pagination.PaginationResult, error) {
	var total int64
	if err := scanSingleRow(ctx, r.sql, "SELECT COUNT(*) FROM usage_cleanup_tasks", nil, &total); err != nil {
		return nil, nil, err
	}
	if total == 0 {
		return []service.UsageCleanupTask{}, paginationResultFromTotal(0, params), nil
	}

	query := `
		SELECT ` + usageCleanupTaskRowColumns + `
		FROM usage_cleanup_tasks
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.sql.QueryContext(ctx, query, params.Limit(), params.Offset())
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	tasks := make([]service.UsageCleanupTask, 0)
	for rows.Next() {
		task, err := scanUsageCleanupTaskRowMySQL(rows.Scan)
		if err != nil {
			return nil, nil, err
		}
		tasks = append(tasks, *task)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return tasks, paginationResultFromTotal(total, params), nil
}

// claimNextPendingTaskMySQL 是 ClaimNextPendingTask 的 MySQL 实现。
// PG 版为可写 CTE（next FOR UPDATE SKIP LOCKED + UPDATE..RETURNING NEW 值）；
// MySQL 无可写 CTE，拆为同一 sqlAtomicScope（同连接同事务）三步：
//  1. SELECT ... FOR UPDATE SKIP LOCKED 取唯一目标行；
//  2. UPDATE 认领；
//  3. 回读该行（NEW 值），与 PG UPDATE..RETURNING 语义一致。
func (r *usageCleanupRepository) claimNextPendingTaskMySQL(ctx context.Context, staleRunningAfterSeconds int64) (*service.UsageCleanupTask, error) {
	scope, err := beginSQLAtomicScope(ctx, r.client, r.sql)
	if err != nil {
		return nil, err
	}
	defer scope.rollback()

	rows, err := scope.query(ctx, `
		SELECT id
		FROM usage_cleanup_tasks
		WHERE status = ?
			OR (
				status = ?
				AND started_at IS NOT NULL
				AND started_at < NOW(3) - INTERVAL ? SECOND
			)
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning, staleRunningAfterSeconds)
	if err != nil {
		return nil, err
	}
	var claimID int64
	hasCandidate := false
	if rows.Next() {
		if err := rows.Scan(&claimID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		hasCandidate = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if !hasCandidate {
		// 无待认领任务，与 PG 的 ErrNoRows → (nil, nil) 语义一致。
		return nil, nil
	}

	if _, err := scope.exec(ctx, `
		UPDATE usage_cleanup_tasks
		SET status = ?,
			started_at = NOW(3),
			finished_at = NULL,
			error_message = NULL,
			updated_at = NOW(3)
		WHERE id = ?
	`, service.UsageCleanupStatusRunning, claimID); err != nil {
		return nil, err
	}

	rows, err = scope.query(ctx, `
		SELECT `+usageCleanupTaskRowColumns+`
		FROM usage_cleanup_tasks
		WHERE id = ?
	`, claimID)
	if err != nil {
		return nil, err
	}
	var task *service.UsageCleanupTask
	if rows.Next() {
		task, err = scanUsageCleanupTaskRowMySQL(rows.Scan)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if task == nil {
		// FOR UPDATE 已锁定该行，正常不会发生；保守按无任务处理。
		return nil, nil
	}
	if err := scope.commit(); err != nil {
		return nil, err
	}
	return task, nil
}

// cancelTaskMySQL 是 CancelTask 纯 SQL 路径的 MySQL 实现：
// PG 版 UPDATE..RETURNING id → UPDATE + RowsAffected 判定（1=取消成功，0=状态不符）。
func (r *usageCleanupRepository) cancelTaskMySQL(ctx context.Context, taskID int64, canceledBy int64) (bool, error) {
	query := `
		UPDATE usage_cleanup_tasks
		SET status = ?,
			canceled_by = ?,
			canceled_at = NOW(3),
			finished_at = NOW(3),
			error_message = NULL,
			updated_at = NOW(3)
		WHERE id = ?
			AND status IN (?, ?)
	`
	result, err := r.sql.ExecContext(ctx, query,
		service.UsageCleanupStatusCanceled,
		canceledBy,
		taskID,
		service.UsageCleanupStatusPending,
		service.UsageCleanupStatusRunning,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// deleteUsageLogsBatchMySQL 是 DeleteUsageLogsBatch 的 MySQL 实现：
// PG 版为 target CTE + DELETE..IN..RETURNING，MySQL 单表 DELETE 原生支持 ORDER BY/LIMIT，
// 一条语句等价（RowsAffected 对应 RETURNING 行数）；非 *sql.DB 的
// 事务路径走 deleteUsageLogsBatchWithRollupInvalidationMySQL（先取后删 + 汇总水位失效）。
func (r *usageCleanupRepository) deleteUsageLogsBatchMySQL(ctx context.Context, filters service.UsageCleanupFilters, limit int) (int64, error) {
	whereClause, args := buildUsageCleanupWhereMySQL(filters)
	if whereClause == "" {
		return 0, fmt.Errorf("cleanup filters missing time range")
	}
	args = append(args, limit)
	if db, ok := r.sql.(*sql.DB); ok {
		return deleteUsageLogsBatchWithRollupInvalidationMySQL(ctx, db, whereClause, args)
	}
	query := fmt.Sprintf(`
		DELETE FROM usage_logs
		WHERE %s
		ORDER BY created_at ASC, id ASC
		LIMIT ?
	`, whereClause)
	res, err := r.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// deleteUsageLogsBatchWithRollupInvalidationMySQL 是 deleteUsageLogsBatchWithRollupInvalidation
// 的 MySQL 实现。PG 版 DELETE..RETURNING created_at；MySQL 拆为同事务两步：
// 先 SELECT id, created_at ... FOR UPDATE 取目标行（先取后删），再按主键删除，
// 用删除行集合还原 RETURNING 的行数与最早 created_at；汇总水位失效与 PG 一致。
func deleteUsageLogsBatchWithRollupInvalidationMySQL(ctx context.Context, db *sql.DB, whereClause string, args []any) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	rollback := func(err error) (int64, error) {
		_ = tx.Rollback()
		return 0, err
	}

	if err := lockGroupUsageRollupState(ctx, tx); err != nil {
		return rollback(err)
	}
	selectQuery := fmt.Sprintf(`
		SELECT id, created_at
		FROM usage_logs
		WHERE %s
		ORDER BY created_at ASC, id ASC
		LIMIT ?
		FOR UPDATE
	`, whereClause)
	rows, err := tx.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return rollback(err)
	}

	var deleted int64
	var earliestDeletedAt time.Time
	idArgs := make([]any, 0, len(args))
	for rows.Next() {
		var id int64
		var deletedAt time.Time
		if err := rows.Scan(&id, &deletedAt); err != nil {
			_ = rows.Close()
			return rollback(err)
		}
		idArgs = append(idArgs, id)
		deleted++
		if earliestDeletedAt.IsZero() || deletedAt.Before(earliestDeletedAt) {
			earliestDeletedAt = deletedAt
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return rollback(err)
	}
	if err := rows.Close(); err != nil {
		return rollback(err)
	}

	if deleted > 0 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM usage_logs WHERE id IN ("+Placeholders(len(idArgs))+")", idArgs...); err != nil {
			return rollback(err)
		}
		if err := invalidateGroupUsageRollupsAt(ctx, tx, earliestDeletedAt); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// createTaskWithSQLMySQL 是 createTaskWithSQL 的 MySQL 实现：
// PG 版 INSERT..RETURNING id, created_at, updated_at → INSERT + LastInsertId，
// 再回查 DB 默认生成的 created_at/updated_at（对应规范 RETURNING 改写第 2 类）。
func (r *usageCleanupRepository) createTaskWithSQLMySQL(ctx context.Context, task *service.UsageCleanupTask) error {
	filtersJSON, err := json.Marshal(task.Filters)
	if err != nil {
		return fmt.Errorf("marshal cleanup filters: %w", err)
	}
	result, err := r.sql.ExecContext(ctx, `
		INSERT INTO usage_cleanup_tasks (
			status,
			filters,
			created_by,
			deleted_rows
		) VALUES (?, ?, ?, ?)
	`, task.Status, filtersJSON, task.CreatedBy, task.DeletedRows)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	task.ID = id
	return scanSingleRow(ctx, r.sql, `
		SELECT created_at, updated_at
		FROM usage_cleanup_tasks
		WHERE id = ?
	`, []any{id}, &task.CreatedAt, &task.UpdatedAt)
}
