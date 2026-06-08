package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func TestNewUsageCleanupRepository(t *testing.T) {
	db, _ := newSQLMock(t)
	repo := NewUsageCleanupRepository(nil, db)
	require.NotNil(t, repo)
}

func TestUsageCleanupRepositoryCreateTask(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	task := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusPending,
		Filters:   service.UsageCleanupFilters{StartTime: start, EndTime: end},
		CreatedBy: 12,
	}
	now := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("INSERT INTO usage_cleanup_tasks").
		WithArgs(task.Status, sqlmock.AnyArg(), task.CreatedBy, task.DeletedRows).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(1), now, now))

	err := repo.CreateTask(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, int64(1), task.ID)
	require.Equal(t, now, task.CreatedAt)
	require.Equal(t, now, task.UpdatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryCreateTaskNil(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	err := repo.CreateTask(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryCreateTaskQueryError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	task := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusPending,
		Filters:   service.UsageCleanupFilters{StartTime: time.Now(), EndTime: time.Now().Add(time.Hour)},
		CreatedBy: 1,
	}

	mock.ExpectQuery("INSERT INTO usage_cleanup_tasks").
		WithArgs(task.Status, sqlmock.AnyArg(), task.CreatedBy, task.DeletedRows).
		WillReturnError(sql.ErrConnDone)

	err := repo.CreateTask(context.Background(), task)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryListTasksEmpty(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_cleanup_tasks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	tasks, result, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Empty(t, tasks)
	require.Equal(t, int64(0), result.Total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryListTasks(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	filters := service.UsageCleanupFilters{StartTime: start, EndTime: end}
	filtersJSON, err := json.Marshal(filters)
	require.NoError(t, err)

	createdAt := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	rows := sqlmock.NewRows([]string{
		"id", "status", "filters", "created_by", "deleted_rows", "error_message",
		"canceled_by", "canceled_at",
		"started_at", "finished_at", "created_at", "updated_at",
	}).AddRow(
		int64(1),
		service.UsageCleanupStatusSucceeded,
		filtersJSON,
		int64(2),
		int64(9),
		"error",
		nil,
		nil,
		start,
		end,
		createdAt,
		updatedAt,
	)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_cleanup_tasks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("SELECT id, status, filters, created_by, deleted_rows, error_message").
		WithArgs(20, 0).
		WillReturnRows(rows)

	tasks, result, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, int64(1), tasks[0].ID)
	require.Equal(t, service.UsageCleanupStatusSucceeded, tasks[0].Status)
	require.Equal(t, int64(2), tasks[0].CreatedBy)
	require.Equal(t, int64(9), tasks[0].DeletedRows)
	require.NotNil(t, tasks[0].ErrorMsg)
	require.Equal(t, "error", *tasks[0].ErrorMsg)
	require.NotNil(t, tasks[0].StartedAt)
	require.NotNil(t, tasks[0].FinishedAt)
	require.Equal(t, int64(1), result.Total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryListTasksQueryError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_cleanup_tasks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery("SELECT id, status, filters, created_by, deleted_rows, error_message").
		WithArgs(20, 0).
		WillReturnError(sql.ErrConnDone)

	_, _, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryListTasksInvalidFilters(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	rows := sqlmock.NewRows([]string{
		"id", "status", "filters", "created_by", "deleted_rows", "error_message",
		"canceled_by", "canceled_at",
		"started_at", "finished_at", "created_at", "updated_at",
	}).AddRow(
		int64(1),
		service.UsageCleanupStatusSucceeded,
		[]byte("not-json"),
		int64(2),
		int64(9),
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now().UTC(),
		time.Now().UTC(),
	)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_cleanup_tasks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("SELECT id, status, filters, created_by, deleted_rows, error_message").
		WithArgs(20, 0).
		WillReturnRows(rows)

	_, _, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryClaimNextPendingTaskNone(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning, int64(1800), service.UsageCleanupStatusRunning).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "filters", "created_by", "deleted_rows", "error_message",
			"started_at", "finished_at", "created_at", "updated_at",
		}))

	task, err := repo.ClaimNextPendingTask(context.Background(), 1800)
	require.NoError(t, err)
	require.Nil(t, task)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryClaimNextPendingTask(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	filters := service.UsageCleanupFilters{StartTime: start, EndTime: end}
	filtersJSON, err := json.Marshal(filters)
	require.NoError(t, err)

	rows := sqlmock.NewRows([]string{
		"id", "status", "filters", "created_by", "deleted_rows", "error_message",
		"started_at", "finished_at", "created_at", "updated_at",
	}).AddRow(
		int64(4),
		service.UsageCleanupStatusRunning,
		filtersJSON,
		int64(7),
		int64(0),
		nil,
		start,
		nil,
		start,
		start,
	)

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning, int64(1800), service.UsageCleanupStatusRunning).
		WillReturnRows(rows)

	task, err := repo.ClaimNextPendingTask(context.Background(), 1800)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, int64(4), task.ID)
	require.Equal(t, service.UsageCleanupStatusRunning, task.Status)
	require.Equal(t, int64(7), task.CreatedBy)
	require.NotNil(t, task.StartedAt)
	require.Nil(t, task.ErrorMsg)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryClaimNextPendingTaskError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning, int64(1800), service.UsageCleanupStatusRunning).
		WillReturnError(sql.ErrConnDone)

	_, err := repo.ClaimNextPendingTask(context.Background(), 1800)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryClaimNextPendingTaskInvalidFilters(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	rows := sqlmock.NewRows([]string{
		"id", "status", "filters", "created_by", "deleted_rows", "error_message",
		"started_at", "finished_at", "created_at", "updated_at",
	}).AddRow(
		int64(4),
		service.UsageCleanupStatusRunning,
		[]byte("invalid"),
		int64(7),
		int64(0),
		nil,
		nil,
		nil,
		time.Now().UTC(),
		time.Now().UTC(),
	)

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning, int64(1800), service.UsageCleanupStatusRunning).
		WillReturnRows(rows)

	_, err := repo.ClaimNextPendingTask(context.Background(), 1800)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryMarkTaskSucceeded(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectExec("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusSucceeded, int64(12), int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.MarkTaskSucceeded(context.Background(), 9, 12)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryMarkTaskFailed(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectExec("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusFailed, int64(4), "boom", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.MarkTaskFailed(context.Background(), 2, 4, "boom")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryGetTaskStatus(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectQuery("SELECT status FROM usage_cleanup_tasks").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(service.UsageCleanupStatusPending))

	status, err := repo.GetTaskStatus(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, service.UsageCleanupStatusPending, status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryGetTaskStatusQueryError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectQuery("SELECT status FROM usage_cleanup_tasks").
		WithArgs(int64(9)).
		WillReturnError(sql.ErrConnDone)

	_, err := repo.GetTaskStatus(context.Background(), 9)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryUpdateTaskProgress(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectExec("UPDATE usage_cleanup_tasks").
		WithArgs(int64(123), int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateTaskProgress(context.Background(), 8, 123)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryCancelTask(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusCanceled, int64(6), int64(9), service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(6)))

	ok, err := repo.CancelTask(context.Background(), 6, 9)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryCancelTaskNoRows(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusCanceled, int64(6), int64(9), service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	ok, err := repo.CancelTask(context.Background(), 6, 9)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryDeleteUsageLogsBatchMissingRange(t *testing.T) {
	db, _ := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	_, err := repo.DeleteUsageLogsBatch(context.Background(), service.UsageCleanupFilters{}, 10)
	require.Error(t, err)
}

func TestUsageCleanupRepositoryDeleteUsageLogsBatch(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	userID := int64(3)
	model := " gpt-4 "
	filters := service.UsageCleanupFilters{
		StartTime: start,
		EndTime:   end,
		UserID:    &userID,
		Model:     &model,
	}

	mock.ExpectQuery("DELETE FROM usage_logs").
		WithArgs(start, end, userID, "gpt-4", 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)).AddRow(int64(2)))

	deleted, err := repo.DeleteUsageLogsBatch(context.Background(), filters, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageCleanupRepositoryDeleteUsageLogsBatchQueryError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: db}

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	filters := service.UsageCleanupFilters{StartTime: start, EndTime: end}

	mock.ExpectQuery("DELETE FROM usage_logs").
		WithArgs(start, end, 5).
		WillReturnError(sql.ErrConnDone)

	_, err := repo.DeleteUsageLogsBatch(context.Background(), filters, 5)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildUsageCleanupWhere(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	userID := int64(1)
	apiKeyID := int64(2)
	accountID := int64(3)
	groupID := int64(4)
	model := " gpt-4 "
	stream := true
	billingType := int8(2)

	where, args := buildUsageCleanupWhere(service.UsageCleanupFilters{
		StartTime:   start,
		EndTime:     end,
		UserID:      &userID,
		APIKeyID:    &apiKeyID,
		AccountID:   &accountID,
		GroupID:     &groupID,
		Model:       &model,
		Stream:      &stream,
		BillingType: &billingType,
	})

	require.Equal(t, "created_at >= $1 AND created_at <= $2 AND user_id = $3 AND api_key_id = $4 AND account_id = $5 AND group_id = $6 AND model = $7 AND stream = $8 AND billing_type = $9", where)
	require.Equal(t, []any{start, end, userID, apiKeyID, accountID, groupID, "gpt-4", stream, billingType}, args)
}

func TestBuildUsageCleanupWhereRequestTypePriority(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(service.RequestTypeWSV2)
	stream := false

	where, args := buildUsageCleanupWhere(service.UsageCleanupFilters{
		StartTime:   start,
		EndTime:     end,
		RequestType: &requestType,
		Stream:      &stream,
	})

	require.Equal(t, "created_at >= $1 AND created_at <= $2 AND (request_type = $3 OR (request_type = 0 AND openai_ws_mode = TRUE))", where)
	require.Equal(t, []any{start, end, requestType}, args)
}

func TestBuildUsageCleanupWhereRequestTypeLegacyFallback(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(service.RequestTypeStream)

	where, args := buildUsageCleanupWhere(service.UsageCleanupFilters{
		StartTime:   start,
		EndTime:     end,
		RequestType: &requestType,
	})

	require.Equal(t, "created_at >= $1 AND created_at <= $2 AND (request_type = $3 OR (request_type = 0 AND stream = TRUE AND openai_ws_mode = FALSE))", where)
	require.Equal(t, []any{start, end, requestType}, args)
}

func TestBuildUsageCleanupWhereModelEmpty(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	model := "   "

	where, args := buildUsageCleanupWhere(service.UsageCleanupFilters{
		StartTime: start,
		EndTime:   end,
		Model:     &model,
	})

	require.Equal(t, "created_at >= $1 AND created_at <= $2", where)
	require.Equal(t, []any{start, end}, args)
}
