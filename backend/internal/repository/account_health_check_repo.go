package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type accountBatchTestTaskRepository struct {
	db *sql.DB
}

func NewAccountBatchTestTaskRepository(db *sql.DB) service.AccountBatchTestTaskRepository {
	return &accountBatchTestTaskRepository{db: db}
}

func (r *accountBatchTestTaskRepository) Create(ctx context.Context, task *service.AccountBatchTestTask) (*service.AccountBatchTestTask, error) {
	if task.Source == "" {
		task.Source = service.AccountBatchTestSourceManual
	}
	if task.Status == "" {
		task.Status = service.AccountBatchTestStatusPending
	}
	if task.ModelID == "" {
		task.ModelID = service.DefaultAccountHealthCheckModel
	}
	if task.Concurrency <= 0 {
		task.Concurrency = service.DefaultAccountHealthCheckConcurrency
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO account_batch_test_tasks (source, status, model_id, concurrency, auto_disable, total_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, source, status, model_id, concurrency, auto_disable, total_count, completed_count,
			success_count, failed_count, deactivated_count, error_message, started_at, finished_at, created_at, updated_at
	`, task.Source, task.Status, task.ModelID, task.Concurrency, task.AutoDisable, task.TotalCount)
	return scanAccountBatchTestTask(row)
}

func (r *accountBatchTestTaskRepository) GetByID(ctx context.Context, id int64) (*service.AccountBatchTestTask, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, source, status, model_id, concurrency, auto_disable, total_count, completed_count,
			success_count, failed_count, deactivated_count, error_message, started_at, finished_at, created_at, updated_at
		FROM account_batch_test_tasks
		WHERE id = $1
	`, id)
	return scanAccountBatchTestTask(row)
}

func (r *accountBatchTestTaskRepository) List(ctx context.Context, limit, offset int) ([]*service.AccountBatchTestTask, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM account_batch_test_tasks`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, source, status, model_id, concurrency, auto_disable, total_count, completed_count,
			success_count, failed_count, deactivated_count, error_message, started_at, finished_at, created_at, updated_at
		FROM account_batch_test_tasks
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	tasks := make([]*service.AccountBatchTestTask, 0, limit)
	for rows.Next() {
		task, err := scanAccountBatchTestTask(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, task)
	}
	return tasks, total, rows.Err()
}

func (r *accountBatchTestTaskRepository) MarkRunning(ctx context.Context, id int64, startedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_batch_test_tasks
		SET status = $2, started_at = $3, updated_at = NOW()
		WHERE id = $1
	`, id, service.AccountBatchTestStatusRunning, startedAt)
	return err
}

func (r *accountBatchTestTaskRepository) IncrementProgress(ctx context.Context, id int64, status string, triggeredDisabled bool) error {
	successInc := 0
	failedInc := 0
	if status == service.AccountBatchTestResultSuccess {
		successInc = 1
	} else {
		failedInc = 1
	}
	deactivatedInc := 0
	if triggeredDisabled {
		deactivatedInc = 1
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_batch_test_tasks
		SET completed_count = completed_count + 1,
			success_count = success_count + $2,
			failed_count = failed_count + $3,
			deactivated_count = deactivated_count + $4,
			updated_at = NOW()
		WHERE id = $1
	`, id, successInc, failedInc, deactivatedInc)
	return err
}

func (r *accountBatchTestTaskRepository) Finish(ctx context.Context, id int64, status string, errMsg string, finishedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_batch_test_tasks
		SET status = $2, error_message = $3, finished_at = $4, updated_at = NOW()
		WHERE id = $1
	`, id, status, errMsg, finishedAt)
	return err
}

type accountBatchTestResultRepository struct {
	db *sql.DB
}

func NewAccountBatchTestResultRepository(db *sql.DB) service.AccountBatchTestResultRepository {
	return &accountBatchTestResultRepository{db: db}
}

func (r *accountBatchTestResultRepository) Create(ctx context.Context, result *service.AccountBatchTestResult) (*service.AccountBatchTestResult, error) {
	if result.Status == "" {
		result.Status = service.AccountBatchTestResultFailed
	}
	if result.StartedAt.IsZero() {
		result.StartedAt = time.Now()
	}
	if result.FinishedAt.IsZero() {
		result.FinishedAt = result.StartedAt
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO account_batch_test_results (
			task_id, account_id, account_name, platform, account_type, status, response_text, error_message,
			latency_ms, fail_streak, triggered_disabled, started_at, finished_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
		RETURNING id, task_id, account_id, account_name, platform, account_type, status, response_text, error_message,
			latency_ms, fail_streak, triggered_disabled, started_at, finished_at, created_at
	`, result.TaskID, result.AccountID, result.AccountName, result.Platform, result.AccountType, result.Status,
		result.ResponseText, result.ErrorMessage, result.LatencyMs, result.FailStreak, result.TriggeredDisabled,
		result.StartedAt, result.FinishedAt)
	return scanAccountBatchTestResult(row)
}

func (r *accountBatchTestResultRepository) ListByTaskID(ctx context.Context, taskID int64, limit, offset int) ([]*service.AccountBatchTestResult, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM account_batch_test_results WHERE task_id = $1`, taskID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, account_id, account_name, platform, account_type, status, response_text, error_message,
			latency_ms, fail_streak, triggered_disabled, started_at, finished_at, created_at
		FROM account_batch_test_results
		WHERE task_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, taskID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	results := make([]*service.AccountBatchTestResult, 0, limit)
	for rows.Next() {
		result, err := scanAccountBatchTestResult(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, result)
	}
	return results, total, rows.Err()
}

type accountHealthCheckRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewAccountHealthCheckRepository(client *dbent.Client, db *sql.DB) service.AccountHealthCheckRepository {
	return &accountHealthCheckRepository{client: client, db: db}
}

func (r *accountHealthCheckRepository) GetAccountsByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return []*service.Account{}, nil
	}
	entAccounts, err := r.client.Account.Query().
		Where(dbaccount.IDIn(uniqueIDs...), dbaccount.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*service.Account, len(entAccounts))
	for _, entAccount := range entAccounts {
		account := accountEntityToService(entAccount)
		if account != nil {
			byID[account.ID] = account
		}
	}
	out := make([]*service.Account, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if account, ok := byID[id]; ok {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r *accountHealthCheckRepository) ListScheduledCandidates(ctx context.Context) ([]*service.Account, error) {
	entAccounts, err := r.client.Account.Query().
		Where(
			dbaccount.DeletedAtIsNil(),
			dbaccount.PlatformEQ(service.PlatformOpenAI),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			dbaccount.HealthCheckEnabledEQ(true),
			dbaccount.HealthCheckProtectedEQ(false),
		).
		Order(dbent.Asc(dbaccount.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.Account, 0, len(entAccounts))
	for _, entAccount := range entAccounts {
		if account := accountEntityToService(entAccount); account != nil {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r *accountHealthCheckRepository) UpdateSettings(ctx context.Context, id int64, settings service.AccountHealthCheckSettings) (*service.Account, error) {
	builder := r.client.Account.UpdateOneID(id)
	if settings.Enabled != nil {
		builder.SetHealthCheckEnabled(*settings.Enabled)
	}
	if settings.Protected != nil {
		builder.SetHealthCheckProtected(*settings.Protected)
	}
	entAccount, err := builder.Save(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}
	return accountEntityToService(entAccount), nil
}

func (r *accountHealthCheckRepository) BulkUpdateSettings(ctx context.Context, ids []int64, settings service.AccountHealthCheckSettings) (int64, error) {
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return 0, nil
	}
	clauses := []string{"updated_at = NOW()"}
	args := []any{pq.Array(uniqueIDs)}
	if settings.Enabled != nil {
		args = append(args, *settings.Enabled)
		clauses = append(clauses, "health_check_enabled = $"+itoa(len(args)))
	}
	if settings.Protected != nil {
		args = append(args, *settings.Protected)
		clauses = append(clauses, "health_check_protected = $"+itoa(len(args)))
	}
	query := `
		UPDATE accounts
		SET ` + strings.Join(clauses, ", ") + `
		WHERE deleted_at IS NULL AND id = ANY($1)
	`
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (r *accountHealthCheckRepository) RecordResult(ctx context.Context, accountID int64, status string, errorMessage string, failStreak int, disable bool) (*service.Account, error) {
	if status == service.AccountBatchTestResultSuccess {
		failStreak = 0
		errorMessage = ""
	}
	builder := r.client.Account.UpdateOneID(accountID).
		SetHealthCheckFailStreak(failStreak).
		SetLastHealthCheckAt(time.Now()).
		SetLastHealthCheckStatus(status)
	if errorMessage == "" {
		builder.ClearLastHealthCheckError()
	} else {
		builder.SetLastHealthCheckError(errorMessage)
	}
	if disable {
		builder.SetSchedulable(false)
	}
	entAccount, err := builder.Save(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}
	if disable {
		if err := enqueueSchedulerOutbox(ctx, r.db, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
			logger.LegacyPrintf("repository.account_health_check", "[SchedulerOutbox] enqueue health disable failed: account=%d err=%v", accountID, err)
		}
	}
	return accountEntityToService(entAccount), nil
}

func scanAccountBatchTestTask(row scannable) (*service.AccountBatchTestTask, error) {
	task := &service.AccountBatchTestTask{}
	if err := row.Scan(
		&task.ID, &task.Source, &task.Status, &task.ModelID, &task.Concurrency, &task.AutoDisable,
		&task.TotalCount, &task.CompletedCount, &task.SuccessCount, &task.FailedCount, &task.DeactivatedCount,
		&task.ErrorMessage, &task.StartedAt, &task.FinishedAt, &task.CreatedAt, &task.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return task, nil
}

func scanAccountBatchTestResult(row scannable) (*service.AccountBatchTestResult, error) {
	result := &service.AccountBatchTestResult{}
	if err := row.Scan(
		&result.ID, &result.TaskID, &result.AccountID, &result.AccountName, &result.Platform, &result.AccountType,
		&result.Status, &result.ResponseText, &result.ErrorMessage, &result.LatencyMs, &result.FailStreak,
		&result.TriggeredDisabled, &result.StartedAt, &result.FinishedAt, &result.CreatedAt,
	); err != nil {
		return nil, err
	}
	return result, nil
}
