package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	appTimezone "github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
REDACTED
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)
	return db, mock
REDACTED

func setUsageCleanupRollupTestTimezone(t *testing.T) {
REDACTED
	require.NoError(t, appTimezone.Init("Asia/Shanghai"))
	t.Cleanup(func() { require.NoError(t, appTimezone.Init("Asia/Shanghai")) REDACTED)
REDACTED

func TestNewUsageCleanupRepository(t *testing.T) {
	db, _ := newSQLMock(t)
	repo := NewUsageCleanupRepository(nil, db)
	require.NotNil(t, repo)
REDACTED

func TestUsageCleanupRepositoryCreateTask(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	task := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusPending,
		Filters:   service.UsageCleanupFilters{StartTime: start, EndTime: endREDACTED,
		CreatedBy: 12,
REDACTED
	now := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("INSERT INTO usage_cleanup_tasks").
		WithArgs(task.Status, sqlmock.AnyArg(), task.CreatedBy, task.DeletedRows).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"REDACTED).AddRow(int64(1), now, now))

	err := repo.CreateTask(context.Background(), task)
REDACTED
	require.Equal(t, int64(1), task.ID)
	require.Equal(t, now, task.CreatedAt)
	require.Equal(t, now, task.UpdatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryCreateTaskNil(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	err := repo.CreateTask(context.Background(), nil)
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryCreateTaskQueryError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	task := &service.UsageCleanupTask{
		Status:    service.UsageCleanupStatusPending,
		Filters:   service.UsageCleanupFilters{StartTime: time.Now(), EndTime: time.Now().Add(time.Hour)REDACTED,
		CreatedBy: 1,
REDACTED

	mock.ExpectQuery("INSERT INTO usage_cleanup_tasks").
		WithArgs(task.Status, sqlmock.AnyArg(), task.CreatedBy, task.DeletedRows).
		WillReturnError(sql.ErrConnDone)

	err := repo.CreateTask(context.Background(), task)
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryListTasksEmpty(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_cleanup_tasks").
		WillReturnRows(sqlmock.NewRows([]string{"count"REDACTED).AddRow(int64(0)))

	tasks, result, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20REDACTED)
REDACTED
	require.Empty(t, tasks)
	require.Equal(t, int64(0), result.Total)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryListTasks(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	filters := service.UsageCleanupFilters{StartTime: start, EndTime: endREDACTED
	filtersJSON, err := json.Marshal(filters)
REDACTED

	createdAt := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	rows := sqlmock.NewRows([]string{
		"id", "status", "filters", "created_by", "deleted_rows", "error_message",
		"canceled_by", "canceled_at",
		"started_at", "finished_at", "created_at", "updated_at",
REDACTED).AddRow(
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
		WillReturnRows(sqlmock.NewRows([]string{"count"REDACTED).AddRow(int64(1)))
	mock.ExpectQuery("SELECT id, status, filters, created_by, deleted_rows, error_message").
		WithArgs(20, 0).
		WillReturnRows(rows)

	tasks, result, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20REDACTED)
REDACTED
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
REDACTED

func TestUsageCleanupRepositoryListTasksQueryError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_cleanup_tasks").
		WillReturnRows(sqlmock.NewRows([]string{"count"REDACTED).AddRow(int64(2)))
	mock.ExpectQuery("SELECT id, status, filters, created_by, deleted_rows, error_message").
		WithArgs(20, 0).
		WillReturnError(sql.ErrConnDone)

	_, _, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20REDACTED)
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryListTasksInvalidFilters(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	rows := sqlmock.NewRows([]string{
		"id", "status", "filters", "created_by", "deleted_rows", "error_message",
		"canceled_by", "canceled_at",
		"started_at", "finished_at", "created_at", "updated_at",
REDACTED).AddRow(
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
		WillReturnRows(sqlmock.NewRows([]string{"count"REDACTED).AddRow(int64(1)))
	mock.ExpectQuery("SELECT id, status, filters, created_by, deleted_rows, error_message").
		WithArgs(20, 0).
		WillReturnRows(rows)

	_, _, err := repo.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20REDACTED)
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryClaimNextPendingTaskNone(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning, int64(1800), service.UsageCleanupStatusRunning).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status", "filters", "created_by", "deleted_rows", "error_message",
			"started_at", "finished_at", "created_at", "updated_at",
	REDACTED))

	task, err := repo.ClaimNextPendingTask(context.Background(), 1800)
REDACTED
	require.Nil(t, task)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryClaimNextPendingTask(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	filters := service.UsageCleanupFilters{StartTime: start, EndTime: endREDACTED
	filtersJSON, err := json.Marshal(filters)
REDACTED

	rows := sqlmock.NewRows([]string{
		"id", "status", "filters", "created_by", "deleted_rows", "error_message",
		"started_at", "finished_at", "created_at", "updated_at",
REDACTED).AddRow(
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
REDACTED
	require.NotNil(t, task)
	require.Equal(t, int64(4), task.ID)
	require.Equal(t, service.UsageCleanupStatusRunning, task.Status)
	require.Equal(t, int64(7), task.CreatedBy)
	require.NotNil(t, task.StartedAt)
	require.Nil(t, task.ErrorMsg)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryClaimNextPendingTaskError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning, int64(1800), service.UsageCleanupStatusRunning).
		WillReturnError(sql.ErrConnDone)

	_, err := repo.ClaimNextPendingTask(context.Background(), 1800)
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryClaimNextPendingTaskInvalidFilters(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	rows := sqlmock.NewRows([]string{
		"id", "status", "filters", "created_by", "deleted_rows", "error_message",
		"started_at", "finished_at", "created_at", "updated_at",
REDACTED).AddRow(
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
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryMarkTaskSucceeded(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectExec("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusSucceeded, int64(12), int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.MarkTaskSucceeded(context.Background(), 9, 12)
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryMarkTaskFailed(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectExec("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusFailed, int64(4), "boom", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.MarkTaskFailed(context.Background(), 2, 4, "boom")
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryGetTaskStatus(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectQuery("SELECT status FROM usage_cleanup_tasks").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"status"REDACTED).AddRow(service.UsageCleanupStatusPending))

	status, err := repo.GetTaskStatus(context.Background(), 9)
REDACTED
	require.Equal(t, service.UsageCleanupStatusPending, status)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryGetTaskStatusQueryError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectQuery("SELECT status FROM usage_cleanup_tasks").
		WithArgs(int64(9)).
		WillReturnError(sql.ErrConnDone)

	_, err := repo.GetTaskStatus(context.Background(), 9)
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryUpdateTaskProgress(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectExec("UPDATE usage_cleanup_tasks").
		WithArgs(int64(123), int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateTaskProgress(context.Background(), 8, 123)
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryCancelTask(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusCanceled, int64(6), int64(9), service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning).
		WillReturnRows(sqlmock.NewRows([]string{"id"REDACTED).AddRow(int64(6)))

	ok, err := repo.CancelTask(context.Background(), 6, 9)
REDACTED
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryCancelTaskNoRows(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	mock.ExpectQuery("UPDATE usage_cleanup_tasks").
		WithArgs(service.UsageCleanupStatusCanceled, int64(6), int64(9), service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning).
		WillReturnRows(sqlmock.NewRows([]string{"id"REDACTED))

	ok, err := repo.CancelTask(context.Background(), 6, 9)
REDACTED
	require.False(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryDeleteUsageLogsBatchMissingRange(t *testing.T) {
	db, _ := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	_, err := repo.DeleteUsageLogsBatch(context.Background(), service.UsageCleanupFilters{REDACTED, 10)
REDACTED
REDACTED

func TestUsageCleanupRepositoryDeleteUsageLogsBatch(t *testing.T) {
	setUsageCleanupRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	userID := int64(3)
	model := " gpt-4 "
	filters := service.UsageCleanupFilters{
		StartTime: start,
		EndTime:   end,
		UserID:    &userID,
		Model:     &model,
REDACTED

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"REDACTED).AddRow(1))
	mock.ExpectQuery("DELETE FROM usage_logs").
		WithArgs(start, end, userID, "gpt-4", 2).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"REDACTED).AddRow(start.Add(time.Hour)).AddRow(start.Add(2 * time.Hour)))
	mock.ExpectExec(`UPDATE usage_group_rollup_state`).
		WithArgs(start.Add(time.Hour), "Asia/Shanghai").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	deleted, err := repo.DeleteUsageLogsBatch(context.Background(), filters, 2)
REDACTED
	require.Equal(t, int64(2), deleted)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryDeleteUsageLogsBatchAtomicallyInvalidatesGroupRollups(t *testing.T) {
	setUsageCleanupRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	firstDeletedAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	secondDeletedAt := firstDeletedAt.Add(time.Hour)
	filters := service.UsageCleanupFilters{StartTime: start, EndTime: endREDACTED

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"REDACTED).AddRow(1))
	mock.ExpectQuery(`(?s)DELETE FROM usage_logs.*RETURNING created_at`).
		WithArgs(start, end, 2).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"REDACTED).
			AddRow(firstDeletedAt).
			AddRow(secondDeletedAt))
	mock.ExpectExec(`UPDATE usage_group_rollup_state`).
		WithArgs(firstDeletedAt, "Asia/Shanghai").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	deleted, err := repo.DeleteUsageLogsBatch(context.Background(), filters, 2)
REDACTED
	require.Equal(t, int64(2), deleted)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryDeleteUsageLogsBatchRollsBackWhenInvalidationFails(t *testing.T) {
	setUsageCleanupRollupTestTimezone(t)
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	deletedAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	filters := service.UsageCleanupFilters{StartTime: start, EndTime: endREDACTED

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"REDACTED).AddRow(1))
	mock.ExpectQuery(`(?s)DELETE FROM usage_logs.*RETURNING created_at`).
		WithArgs(start, end, 1).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"REDACTED).AddRow(deletedAt))
	mock.ExpectExec(`UPDATE usage_group_rollup_state`).
		WithArgs(deletedAt, "Asia/Shanghai").
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	_, err := repo.DeleteUsageLogsBatch(context.Background(), filters, 1)
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

func TestUsageCleanupRepositoryDeleteUsageLogsBatchQueryError(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageCleanupRepository{sql: dbREDACTED

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	filters := service.UsageCleanupFilters{StartTime: start, EndTime: endREDACTED

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM usage_group_rollup_state.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"REDACTED).AddRow(1))
	mock.ExpectQuery("DELETE FROM usage_logs").
		WithArgs(start, end, 5).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	_, err := repo.DeleteUsageLogsBatch(context.Background(), filters, 5)
REDACTED
	require.NoError(t, mock.ExpectationsWereMet())
REDACTED

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
REDACTED)

	require.Equal(t, "created_at >= $1 AND created_at <= $2 AND user_id = $3 AND api_key_id = $4 AND account_id = $5 AND group_id = $6 AND model = $7 AND stream = $8 AND billing_type = $9", where)
	require.Equal(t, []any{start, end, userID, apiKeyID, accountID, groupID, "gpt-4", stream, billingTypeREDACTED, args)
REDACTED

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
REDACTED)

	require.Equal(t, "created_at >= $1 AND created_at <= $2 AND (request_type = $3 OR (request_type = 0 AND openai_ws_mode = TRUE))", where)
	require.Equal(t, []any{start, end, requestTypeREDACTED, args)
REDACTED

func TestBuildUsageCleanupWhereRequestTypeLegacyFallback(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(service.RequestTypeStream)

	where, args := buildUsageCleanupWhere(service.UsageCleanupFilters{
		StartTime:   start,
		EndTime:     end,
		RequestType: &requestType,
REDACTED)

	require.Equal(t, "created_at >= $1 AND created_at <= $2 AND (request_type = $3 OR (request_type = 0 AND stream = TRUE AND openai_ws_mode = FALSE))", where)
	require.Equal(t, []any{start, end, requestTypeREDACTED, args)
REDACTED

func TestBuildUsageCleanupWhereModelEmpty(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	model := "   "

	where, args := buildUsageCleanupWhere(service.UsageCleanupFilters{
		StartTime: start,
		EndTime:   end,
		Model:     &model,
REDACTED)

	require.Equal(t, "created_at >= $1 AND created_at <= $2", where)
	require.Equal(t, []any{start, endREDACTED, args)
REDACTED
