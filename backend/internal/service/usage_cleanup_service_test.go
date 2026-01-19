package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type cleanupDeleteResponse struct {
	deleted int64
	err     error
REDACTED

type cleanupDeleteCall struct {
	filters UsageCleanupFilters
	limit   int
REDACTED

type cleanupMarkCall struct {
	taskID      int64
	deletedRows int64
	errMsg      string
REDACTED

type cleanupRepoStub struct {
	mu            sync.Mutex
	created       []*UsageCleanupTask
	createErr     error
	listTasks     []UsageCleanupTask
	listResult    *pagination.PaginationResult
	listErr       error
	claimQueue    []*UsageCleanupTask
	claimErr      error
	deleteQueue   []cleanupDeleteResponse
	deleteCalls   []cleanupDeleteCall
	markSucceeded []cleanupMarkCall
	markFailed    []cleanupMarkCall
	statusByID    map[int64]string
	progressCalls []cleanupMarkCall
	cancelCalls   []int64
REDACTED

func (s *cleanupRepoStub) CreateTask(ctx context.Context, task *UsageCleanupTask) error {
	if task == nil {
		return nil
REDACTED
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createErr != nil {
		return s.createErr
REDACTED
	if task.ID == 0 {
		task.ID = int64(len(s.created) + 1)
REDACTED
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
REDACTED
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
REDACTED
	clone := *task
	s.created = append(s.created, &clone)
	return nil
REDACTED

func (s *cleanupRepoStub) ListTasks(ctx context.Context, params pagination.PaginationParams) ([]UsageCleanupTask, *pagination.PaginationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listTasks, s.listResult, s.listErr
REDACTED

func (s *cleanupRepoStub) ClaimNextPendingTask(ctx context.Context, staleRunningAfterSeconds int64) (*UsageCleanupTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return nil, s.claimErr
REDACTED
	if len(s.claimQueue) == 0 {
		return nil, nil
REDACTED
	task := s.claimQueue[0]
	s.claimQueue = s.claimQueue[1:]
	if s.statusByID == nil {
		s.statusByID = map[int64]string{REDACTED
REDACTED
	s.statusByID[task.ID] = UsageCleanupStatusRunning
	return task, nil
REDACTED

func (s *cleanupRepoStub) GetTaskStatus(ctx context.Context, taskID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statusByID == nil {
		return "", sql.ErrNoRows
REDACTED
	status, ok := s.statusByID[taskID]
	if !ok {
		return "", sql.ErrNoRows
REDACTED
	return status, nil
REDACTED

func (s *cleanupRepoStub) UpdateTaskProgress(ctx context.Context, taskID int64, deletedRows int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progressCalls = append(s.progressCalls, cleanupMarkCall{taskID: taskID, deletedRows: deletedRowsREDACTED)
	return nil
REDACTED

func (s *cleanupRepoStub) CancelTask(ctx context.Context, taskID int64, canceledBy int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelCalls = append(s.cancelCalls, taskID)
	if s.statusByID == nil {
		s.statusByID = map[int64]string{REDACTED
REDACTED
	status := s.statusByID[taskID]
	if status != UsageCleanupStatusPending && status != UsageCleanupStatusRunning {
		return false, nil
REDACTED
	s.statusByID[taskID] = UsageCleanupStatusCanceled
	return true, nil
REDACTED

func (s *cleanupRepoStub) MarkTaskSucceeded(ctx context.Context, taskID int64, deletedRows int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markSucceeded = append(s.markSucceeded, cleanupMarkCall{taskID: taskID, deletedRows: deletedRowsREDACTED)
	if s.statusByID == nil {
		s.statusByID = map[int64]string{REDACTED
REDACTED
	s.statusByID[taskID] = UsageCleanupStatusSucceeded
	return nil
REDACTED

func (s *cleanupRepoStub) MarkTaskFailed(ctx context.Context, taskID int64, deletedRows int64, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markFailed = append(s.markFailed, cleanupMarkCall{taskID: taskID, deletedRows: deletedRows, errMsg: errorMsgREDACTED)
	if s.statusByID == nil {
		s.statusByID = map[int64]string{REDACTED
REDACTED
	s.statusByID[taskID] = UsageCleanupStatusFailed
	return nil
REDACTED

func (s *cleanupRepoStub) DeleteUsageLogsBatch(ctx context.Context, filters UsageCleanupFilters, limit int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteCalls = append(s.deleteCalls, cleanupDeleteCall{filters: filters, limit: limitREDACTED)
	if len(s.deleteQueue) == 0 {
		return 0, nil
REDACTED
	resp := s.deleteQueue[0]
	s.deleteQueue = s.deleteQueue[1:]
	return resp.deleted, resp.err
REDACTED

func TestUsageCleanupServiceCreateTaskSanitizeFilters(t *testing.T) {
	repo := &cleanupRepoStub{REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, MaxRangeDays: 31REDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	userID := int64(-1)
	apiKeyID := int64(10)
	model := "  gpt-4  "
	billingType := int8(-2)
	filters := UsageCleanupFilters{
		StartTime:   start,
		EndTime:     end,
		UserID:      &userID,
		APIKeyID:    &apiKeyID,
		Model:       &model,
		BillingType: &billingType,
REDACTED

	task, err := svc.CreateTask(context.Background(), filters, 9)
REDACTED
	require.Equal(t, UsageCleanupStatusPending, task.Status)
	require.Nil(t, task.Filters.UserID)
	require.NotNil(t, task.Filters.APIKeyID)
	require.Equal(t, apiKeyID, *task.Filters.APIKeyID)
	require.NotNil(t, task.Filters.Model)
	require.Equal(t, "gpt-4", *task.Filters.Model)
	require.Nil(t, task.Filters.BillingType)
	require.Equal(t, int64(9), task.CreatedBy)
REDACTED

func TestUsageCleanupServiceCreateTaskInvalidCreator(t *testing.T) {
	repo := &cleanupRepoStub{REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: trueREDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	filters := UsageCleanupFilters{
		StartTime: time.Now(),
		EndTime:   time.Now().Add(24 * time.Hour),
REDACTED
	_, err := svc.CreateTask(context.Background(), filters, 0)
REDACTED
	require.Equal(t, "USAGE_CLEANUP_INVALID_CREATOR", infraerrors.Reason(err))
REDACTED

func TestUsageCleanupServiceCreateTaskDisabled(t *testing.T) {
	repo := &cleanupRepoStub{REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: falseREDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	filters := UsageCleanupFilters{
		StartTime: time.Now(),
		EndTime:   time.Now().Add(24 * time.Hour),
REDACTED
	_, err := svc.CreateTask(context.Background(), filters, 1)
REDACTED
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
	require.Equal(t, "USAGE_CLEANUP_DISABLED", infraerrors.Reason(err))
REDACTED

func TestUsageCleanupServiceCreateTaskRangeTooLarge(t *testing.T) {
	repo := &cleanupRepoStub{REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, MaxRangeDays: 1REDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	filters := UsageCleanupFilters{StartTime: start, EndTime: endREDACTED

	_, err := svc.CreateTask(context.Background(), filters, 1)
REDACTED
	require.Equal(t, "USAGE_CLEANUP_RANGE_TOO_LARGE", infraerrors.Reason(err))
REDACTED

func TestUsageCleanupServiceCreateTaskMissingRange(t *testing.T) {
	repo := &cleanupRepoStub{REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: trueREDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	_, err := svc.CreateTask(context.Background(), UsageCleanupFilters{REDACTED, 1)
REDACTED
	require.Equal(t, "USAGE_CLEANUP_MISSING_RANGE", infraerrors.Reason(err))
REDACTED

func TestUsageCleanupServiceCreateTaskRepoError(t *testing.T) {
	repo := &cleanupRepoStub{createErr: errors.New("db down")REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: trueREDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	filters := UsageCleanupFilters{
		StartTime: time.Now(),
		EndTime:   time.Now().Add(24 * time.Hour),
REDACTED
	_, err := svc.CreateTask(context.Background(), filters, 1)
REDACTED
	require.Contains(t, err.Error(), "create cleanup task")
REDACTED

func TestUsageCleanupServiceRunOnceSuccess(t *testing.T) {
	start := time.Now()
	end := start.Add(2 * time.Hour)
	repo := &cleanupRepoStub{
		claimQueue: []*UsageCleanupTask{
			{ID: 5, Filters: UsageCleanupFilters{StartTime: start, EndTime: endREDACTEDREDACTED,
	REDACTED,
		deleteQueue: []cleanupDeleteResponse{
			{deleted: 2REDACTED,
			{deleted: 2REDACTED,
			{deleted: 1REDACTED,
	REDACTED,
REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 2, TaskTimeoutSeconds: 30REDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	svc.runOnce()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.deleteCalls, 3)
	require.Equal(t, 2, repo.deleteCalls[0].limit)
	require.True(t, repo.deleteCalls[0].filters.StartTime.Equal(start))
	require.True(t, repo.deleteCalls[0].filters.EndTime.Equal(end))
	require.Len(t, repo.markSucceeded, 1)
	require.Empty(t, repo.markFailed)
	require.Equal(t, int64(5), repo.markSucceeded[0].taskID)
	require.Equal(t, int64(5), repo.markSucceeded[0].deletedRows)
REDACTED

func TestUsageCleanupServiceRunOnceClaimError(t *testing.T) {
	repo := &cleanupRepoStub{claimErr: errors.New("claim failed")REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: trueREDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	svc.runOnce()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.markSucceeded)
	require.Empty(t, repo.markFailed)
REDACTED

func TestUsageCleanupServiceRunOnceAlreadyRunning(t *testing.T) {
	repo := &cleanupRepoStub{REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: trueREDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	svc.running = 1
	svc.runOnce()
REDACTED

func TestUsageCleanupServiceExecuteTaskFailed(t *testing.T) {
	longMsg := strings.Repeat("x", 600)
	repo := &cleanupRepoStub{
		deleteQueue: []cleanupDeleteResponse{
			{err: errors.New(longMsg)REDACTED,
	REDACTED,
REDACTED
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 3REDACTEDREDACTED
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	task := &UsageCleanupTask{
		ID: 11,
		Filters: UsageCleanupFilters{
			StartTime: time.Now(),
			EndTime:   time.Now().Add(24 * time.Hour),
	REDACTED,
REDACTED

	svc.executeTask(context.Background(), task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.markFailed, 1)
	require.Equal(t, int64(11), repo.markFailed[0].taskID)
	require.Equal(t, 500, len(repo.markFailed[0].errMsg))
REDACTED

func TestUsageCleanupServiceListTasks(t *testing.T) {
	repo := &cleanupRepoStub{
		listTasks: []UsageCleanupTask{{ID: 1REDACTED, {ID: 2REDACTEDREDACTED,
		listResult: &pagination.PaginationResult{
			Total:    2,
			Page:     1,
			PageSize: 20,
			Pages:    1,
	REDACTED,
REDACTED
	svc := NewUsageCleanupService(repo, nil, nil, &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: trueREDACTEDREDACTED)

	tasks, result, err := svc.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20REDACTED)
REDACTED
	require.Len(t, tasks, 2)
	require.Equal(t, int64(2), result.Total)
REDACTED

func TestUsageCleanupServiceListTasksNotReady(t *testing.T) {
	var nilSvc *UsageCleanupService
	_, _, err := nilSvc.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20REDACTED)
REDACTED

	svc := NewUsageCleanupService(nil, nil, nil, &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: trueREDACTEDREDACTED)
	_, _, err = svc.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20REDACTED)
REDACTED
REDACTED

func TestUsageCleanupServiceDefaultsAndLifecycle(t *testing.T) {
	var nilSvc *UsageCleanupService
	require.Equal(t, 31, nilSvc.maxRangeDays())
	require.Equal(t, 5000, nilSvc.batchSize())
	require.Equal(t, 10*time.Second, nilSvc.workerInterval())
	require.Equal(t, 30*time.Minute, nilSvc.taskTimeout())
	nilSvc.Start()
	nilSvc.Stop()

	repo := &cleanupRepoStub{REDACTED
	cfgDisabled := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: falseREDACTEDREDACTED
	svcDisabled := NewUsageCleanupService(repo, nil, nil, cfgDisabled)
	svcDisabled.Start()
	svcDisabled.Stop()

	timingWheel, err := NewTimingWheelService()
REDACTED

	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, WorkerIntervalSeconds: 5REDACTEDREDACTED
	svc := NewUsageCleanupService(repo, timingWheel, nil, cfg)
	require.Equal(t, 5*time.Second, svc.workerInterval())
	svc.Start()
	svc.Stop()

	cfgFallback := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: trueREDACTEDREDACTED
	svcFallback := NewUsageCleanupService(repo, timingWheel, nil, cfgFallback)
	require.Equal(t, 31, svcFallback.maxRangeDays())
	require.Equal(t, 5000, svcFallback.batchSize())
	require.Equal(t, 10*time.Second, svcFallback.workerInterval())

	svcMissingDeps := NewUsageCleanupService(nil, nil, nil, cfgFallback)
	svcMissingDeps.Start()
REDACTED

func TestSanitizeUsageCleanupFiltersModelEmpty(t *testing.T) {
	model := "   "
	apiKeyID := int64(-5)
	accountID := int64(-1)
	groupID := int64(-2)
	filters := UsageCleanupFilters{
		UserID:    &apiKeyID,
		APIKeyID:  &apiKeyID,
		AccountID: &accountID,
		GroupID:   &groupID,
		Model:     &model,
REDACTED

	sanitizeUsageCleanupFilters(&filters)
	require.Nil(t, filters.UserID)
	require.Nil(t, filters.APIKeyID)
	require.Nil(t, filters.AccountID)
	require.Nil(t, filters.GroupID)
	require.Nil(t, filters.Model)
REDACTED
