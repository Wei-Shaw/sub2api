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
}

type cleanupDeleteCall struct {
	filters UsageCleanupFilters
	limit   int
}

type cleanupMarkCall struct {
	taskID      int64
	deletedRows int64
	errMsg      string
}

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
	statusErr     error
	progressCalls []cleanupMarkCall
	updateErr     error
	cancelCalls   []int64
	cancelErr     error
	cancelResult  *bool
	markFailedErr error
}

type dashboardRepoStub struct {
	recomputeErr   error
	recomputeCalls int
}

func (s *dashboardRepoStub) AggregateRange(ctx context.Context, start, end time.Time) error {
	return nil
}

func (s *dashboardRepoStub) RecomputeRange(ctx context.Context, start, end time.Time) error {
	s.recomputeCalls++
	return s.recomputeErr
}

func (s *dashboardRepoStub) GetAggregationWatermark(ctx context.Context) (time.Time, error) {
	return time.Time{}, nil
}

func (s *dashboardRepoStub) UpdateAggregationWatermark(ctx context.Context, aggregatedAt time.Time) error {
	return nil
}

func (s *dashboardRepoStub) CleanupAggregates(ctx context.Context, hourlyCutoff, dailyCutoff time.Time) error {
	return nil
}

func (s *dashboardRepoStub) CleanupUsageLogs(ctx context.Context, cutoff time.Time) error {
	return nil
}

func (s *dashboardRepoStub) CleanupUsageBillingDedup(ctx context.Context, cutoff time.Time) error {
	return nil
}

func (s *dashboardRepoStub) EnsureUsageLogsPartitions(ctx context.Context, now time.Time) error {
	return nil
}

func (s *cleanupRepoStub) CreateTask(ctx context.Context, task *UsageCleanupTask) error {
	if task == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createErr != nil {
		return s.createErr
	}
	if task.ID == 0 {
		task.ID = int64(len(s.created) + 1)
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	clone := *task
	s.created = append(s.created, &clone)
	return nil
}

func (s *cleanupRepoStub) ListTasks(ctx context.Context, params pagination.PaginationParams) ([]UsageCleanupTask, *pagination.PaginationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listTasks, s.listResult, s.listErr
}

func (s *cleanupRepoStub) ClaimNextPendingTask(ctx context.Context, staleRunningAfterSeconds int64) (*UsageCleanupTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	if len(s.claimQueue) == 0 {
		return nil, nil
	}
	task := s.claimQueue[0]
	s.claimQueue = s.claimQueue[1:]
	if s.statusByID == nil {
		s.statusByID = map[int64]string{}
	}
	s.statusByID[task.ID] = UsageCleanupStatusRunning
	return task, nil
}

func (s *cleanupRepoStub) GetTaskStatus(ctx context.Context, taskID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statusErr != nil {
		return "", s.statusErr
	}
	if s.statusByID == nil {
		return "", sql.ErrNoRows
	}
	status, ok := s.statusByID[taskID]
	if !ok {
		return "", sql.ErrNoRows
	}
	return status, nil
}

func (s *cleanupRepoStub) UpdateTaskProgress(ctx context.Context, taskID int64, deletedRows int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progressCalls = append(s.progressCalls, cleanupMarkCall{taskID: taskID, deletedRows: deletedRows})
	if s.updateErr != nil {
		return s.updateErr
	}
	return nil
}

func (s *cleanupRepoStub) CancelTask(ctx context.Context, taskID int64, canceledBy int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelCalls = append(s.cancelCalls, taskID)
	if s.cancelErr != nil {
		return false, s.cancelErr
	}
	if s.cancelResult != nil {
		ok := *s.cancelResult
		if ok {
			if s.statusByID == nil {
				s.statusByID = map[int64]string{}
			}
			s.statusByID[taskID] = UsageCleanupStatusCanceled
		}
		return ok, nil
	}
	if s.statusByID == nil {
		s.statusByID = map[int64]string{}
	}
	status := s.statusByID[taskID]
	if status != UsageCleanupStatusPending && status != UsageCleanupStatusRunning {
		return false, nil
	}
	s.statusByID[taskID] = UsageCleanupStatusCanceled
	return true, nil
}

func (s *cleanupRepoStub) MarkTaskSucceeded(ctx context.Context, taskID int64, deletedRows int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markSucceeded = append(s.markSucceeded, cleanupMarkCall{taskID: taskID, deletedRows: deletedRows})
	if s.statusByID == nil {
		s.statusByID = map[int64]string{}
	}
	s.statusByID[taskID] = UsageCleanupStatusSucceeded
	return nil
}

func (s *cleanupRepoStub) MarkTaskFailed(ctx context.Context, taskID int64, deletedRows int64, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markFailed = append(s.markFailed, cleanupMarkCall{taskID: taskID, deletedRows: deletedRows, errMsg: errorMsg})
	if s.statusByID == nil {
		s.statusByID = map[int64]string{}
	}
	s.statusByID[taskID] = UsageCleanupStatusFailed
	if s.markFailedErr != nil {
		return s.markFailedErr
	}
	return nil
}

func (s *cleanupRepoStub) DeleteUsageLogsBatch(ctx context.Context, filters UsageCleanupFilters, limit int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteCalls = append(s.deleteCalls, cleanupDeleteCall{filters: filters, limit: limit})
	if len(s.deleteQueue) == 0 {
		return 0, nil
	}
	resp := s.deleteQueue[0]
	s.deleteQueue = s.deleteQueue[1:]
	return resp.deleted, resp.err
}

func TestUsageCleanupServiceCreateTaskSanitizeFilters(t *testing.T) {
	repo := &cleanupRepoStub{}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, MaxRangeDays: 31}}
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
	}

	task, err := svc.CreateTask(context.Background(), filters, 9)
	require.NoError(t, err)
	require.Equal(t, UsageCleanupStatusPending, task.Status)
	require.Nil(t, task.Filters.UserID)
	require.NotNil(t, task.Filters.APIKeyID)
	require.Equal(t, apiKeyID, *task.Filters.APIKeyID)
	require.NotNil(t, task.Filters.Model)
	require.Equal(t, "gpt-4", *task.Filters.Model)
	require.Nil(t, task.Filters.BillingType)
	require.Equal(t, int64(9), task.CreatedBy)
}

func TestSanitizeUsageCleanupFiltersRequestTypePriority(t *testing.T) {
	requestType := int16(RequestTypeWSV2)
	stream := false
	model := "  gpt-5  "
	filters := UsageCleanupFilters{
		Model:       &model,
		RequestType: &requestType,
		Stream:      &stream,
	}

	sanitizeUsageCleanupFilters(&filters)

	require.NotNil(t, filters.RequestType)
	require.Equal(t, int16(RequestTypeWSV2), *filters.RequestType)
	require.Nil(t, filters.Stream)
	require.NotNil(t, filters.Model)
	require.Equal(t, "gpt-5", *filters.Model)
}

func TestSanitizeUsageCleanupFiltersInvalidRequestType(t *testing.T) {
	requestType := int16(99)
	stream := true
	filters := UsageCleanupFilters{
		RequestType: &requestType,
		Stream:      &stream,
	}

	sanitizeUsageCleanupFilters(&filters)

	require.Nil(t, filters.RequestType)
	require.NotNil(t, filters.Stream)
	require.True(t, *filters.Stream)
}

func TestDescribeUsageCleanupFiltersIncludesRequestType(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	requestType := int16(RequestTypeWSV2)
	desc := describeUsageCleanupFilters(UsageCleanupFilters{
		StartTime:   start,
		EndTime:     end,
		RequestType: &requestType,
	})

	require.Contains(t, desc, "request_type=ws_v2")
}

func TestUsageCleanupServiceCreateTaskInvalidCreator(t *testing.T) {
	repo := &cleanupRepoStub{}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	filters := UsageCleanupFilters{
		StartTime: time.Now(),
		EndTime:   time.Now().Add(24 * time.Hour),
	}
	_, err := svc.CreateTask(context.Background(), filters, 0)
	require.Error(t, err)
	require.Equal(t, "USAGE_CLEANUP_INVALID_CREATOR", infraerrors.Reason(err))
}

func TestUsageCleanupServiceCreateTaskDisabled(t *testing.T) {
	repo := &cleanupRepoStub{}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: false}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	filters := UsageCleanupFilters{
		StartTime: time.Now(),
		EndTime:   time.Now().Add(24 * time.Hour),
	}
	_, err := svc.CreateTask(context.Background(), filters, 1)
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
	require.Equal(t, "USAGE_CLEANUP_DISABLED", infraerrors.Reason(err))
}

func TestUsageCleanupServiceCreateTaskRangeTooLarge(t *testing.T) {
	repo := &cleanupRepoStub{}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, MaxRangeDays: 1}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	filters := UsageCleanupFilters{StartTime: start, EndTime: end}

	_, err := svc.CreateTask(context.Background(), filters, 1)
	require.Error(t, err)
	require.Equal(t, "USAGE_CLEANUP_RANGE_TOO_LARGE", infraerrors.Reason(err))
}

func TestUsageCleanupServiceCreateTaskMissingRange(t *testing.T) {
	repo := &cleanupRepoStub{}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	_, err := svc.CreateTask(context.Background(), UsageCleanupFilters{}, 1)
	require.Error(t, err)
	require.Equal(t, "USAGE_CLEANUP_MISSING_RANGE", infraerrors.Reason(err))
}

func TestUsageCleanupServiceCreateTaskRepoError(t *testing.T) {
	repo := &cleanupRepoStub{createErr: errors.New("db down")}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	filters := UsageCleanupFilters{
		StartTime: time.Now(),
		EndTime:   time.Now().Add(24 * time.Hour),
	}
	_, err := svc.CreateTask(context.Background(), filters, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create cleanup task")
}

func TestUsageCleanupServiceRunOnceSuccess(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	repo := &cleanupRepoStub{
		claimQueue: []*UsageCleanupTask{
			{ID: 5, Filters: UsageCleanupFilters{StartTime: start, EndTime: end}},
		},
		deleteQueue: []cleanupDeleteResponse{
			{deleted: 2},
			{deleted: 2},
			{deleted: 1},
		},
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 2, TaskTimeoutSeconds: 30}}
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
	require.Equal(t, 2, repo.deleteCalls[0].limit)
	require.Equal(t, start, repo.deleteCalls[0].filters.StartTime)
	require.Equal(t, end, repo.deleteCalls[0].filters.EndTime)
}

func TestUsageCleanupServiceRunOnceClaimError(t *testing.T) {
	repo := &cleanupRepoStub{claimErr: errors.New("claim failed")}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	svc.runOnce()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.markSucceeded)
	require.Empty(t, repo.markFailed)
}

func TestUsageCleanupServiceRunOnceAlreadyRunning(t *testing.T) {
	repo := &cleanupRepoStub{}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	svc.running = 1
	svc.runOnce()
}

func TestUsageCleanupServiceExecuteTaskFailed(t *testing.T) {
	longMsg := strings.Repeat("x", 600)
	repo := &cleanupRepoStub{
		deleteQueue: []cleanupDeleteResponse{
			{err: errors.New(longMsg)},
		},
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 3}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	task := &UsageCleanupTask{
		ID: 11,
		Filters: UsageCleanupFilters{
			StartTime: time.Now(),
			EndTime:   time.Now().Add(24 * time.Hour),
		},
	}

	svc.executeTask(context.Background(), task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.markFailed, 1)
	require.Equal(t, int64(11), repo.markFailed[0].taskID)
	require.Equal(t, 500, len(repo.markFailed[0].errMsg))
}

func TestUsageCleanupServiceExecuteTaskProgressError(t *testing.T) {
	repo := &cleanupRepoStub{
		deleteQueue: []cleanupDeleteResponse{
			{deleted: 2},
			{deleted: 0},
		},
		updateErr: errors.New("update failed"),
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 2}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	task := &UsageCleanupTask{
		ID: 8,
		Filters: UsageCleanupFilters{
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(time.Hour),
		},
	}

	svc.executeTask(context.Background(), task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.markSucceeded, 1)
	require.Empty(t, repo.markFailed)
	require.Len(t, repo.progressCalls, 1)
}

func TestUsageCleanupServiceExecuteTaskDeleteCanceled(t *testing.T) {
	repo := &cleanupRepoStub{
		deleteQueue: []cleanupDeleteResponse{
			{err: context.Canceled},
		},
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 2}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	task := &UsageCleanupTask{
		ID: 12,
		Filters: UsageCleanupFilters{
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(time.Hour),
		},
	}

	svc.executeTask(context.Background(), task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.markSucceeded)
	require.Empty(t, repo.markFailed)
}

func TestUsageCleanupServiceExecuteTaskContextCanceled(t *testing.T) {
	repo := &cleanupRepoStub{}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 2}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	task := &UsageCleanupTask{
		ID: 9,
		Filters: UsageCleanupFilters{
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(time.Hour),
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc.executeTask(ctx, task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.markSucceeded)
	require.Empty(t, repo.markFailed)
	require.Empty(t, repo.deleteCalls)
}

func TestUsageCleanupServiceExecuteTaskMarkFailedUpdateError(t *testing.T) {
	repo := &cleanupRepoStub{
		deleteQueue: []cleanupDeleteResponse{
			{err: errors.New("boom")},
		},
		markFailedErr: errors.New("update failed"),
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 2}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	task := &UsageCleanupTask{
		ID: 13,
		Filters: UsageCleanupFilters{
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(time.Hour),
		},
	}

	svc.executeTask(context.Background(), task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.markFailed, 1)
	require.Equal(t, int64(13), repo.markFailed[0].taskID)
}

func TestUsageCleanupServiceExecuteTaskDashboardRecomputeError(t *testing.T) {
	dashboardRepo := &dashboardRepoStub{recomputeErr: errors.New("recompute failed")}
	repo := &cleanupRepoStub{
		deleteQueue: []cleanupDeleteResponse{
			{deleted: 0},
		},
	}
	dashboard := NewDashboardAggregationService(dashboardRepo, nil, &config.Config{
		DashboardAgg: config.DashboardAggregationConfig{Enabled: true},
	})
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 2}}
	svc := NewUsageCleanupService(repo, nil, dashboard, cfg)
	task := &UsageCleanupTask{
		ID: 14,
		Filters: UsageCleanupFilters{
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(time.Hour),
		},
	}

	svc.executeTask(context.Background(), task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.markSucceeded, 1)
	require.Eventually(t, func() bool { return dashboardRepo.recomputeCalls == 1 }, time.Second, 10*time.Millisecond)
}

func TestUsageCleanupServiceExecuteTaskDashboardRecomputeSuccess(t *testing.T) {
	dashboardRepo := &dashboardRepoStub{}
	repo := &cleanupRepoStub{
		deleteQueue: []cleanupDeleteResponse{
			{deleted: 0},
		},
	}
	dashboard := NewDashboardAggregationService(dashboardRepo, nil, &config.Config{
		DashboardAgg: config.DashboardAggregationConfig{Enabled: true},
	})
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 2}}
	svc := NewUsageCleanupService(repo, nil, dashboard, cfg)
	task := &UsageCleanupTask{
		ID: 15,
		Filters: UsageCleanupFilters{
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(time.Hour),
		},
	}

	svc.executeTask(context.Background(), task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.markSucceeded, 1)
	require.Eventually(t, func() bool { return dashboardRepo.recomputeCalls == 1 }, time.Second, 10*time.Millisecond)
}

func TestUsageCleanupServiceExecuteTaskCanceled(t *testing.T) {
	repo := &cleanupRepoStub{
		statusByID: map[int64]string{
			3: UsageCleanupStatusCanceled,
		},
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, BatchSize: 2}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)
	task := &UsageCleanupTask{
		ID: 3,
		Filters: UsageCleanupFilters{
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(time.Hour),
		},
	}

	svc.executeTask(context.Background(), task)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.deleteCalls)
	require.Empty(t, repo.markSucceeded)
	require.Empty(t, repo.markFailed)
}

func TestUsageCleanupServiceCancelTaskSuccess(t *testing.T) {
	repo := &cleanupRepoStub{
		statusByID: map[int64]string{
			5: UsageCleanupStatusPending,
		},
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	err := svc.CancelTask(context.Background(), 5, 9)
	require.NoError(t, err)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Equal(t, UsageCleanupStatusCanceled, repo.statusByID[5])
	require.Len(t, repo.cancelCalls, 1)
}

func TestUsageCleanupServiceCancelTaskDisabled(t *testing.T) {
	repo := &cleanupRepoStub{}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: false}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	err := svc.CancelTask(context.Background(), 1, 2)
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
	require.Equal(t, "USAGE_CLEANUP_DISABLED", infraerrors.Reason(err))
}

func TestUsageCleanupServiceCancelTaskNotFound(t *testing.T) {
	repo := &cleanupRepoStub{}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	err := svc.CancelTask(context.Background(), 999, 1)
	require.Error(t, err)
	require.Equal(t, http.StatusNotFound, infraerrors.Code(err))
	require.Equal(t, "USAGE_CLEANUP_TASK_NOT_FOUND", infraerrors.Reason(err))
}

func TestUsageCleanupServiceCancelTaskStatusError(t *testing.T) {
	repo := &cleanupRepoStub{statusErr: errors.New("status broken")}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	err := svc.CancelTask(context.Background(), 7, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "status broken")
}

func TestUsageCleanupServiceCancelTaskConflict(t *testing.T) {
	repo := &cleanupRepoStub{
		statusByID: map[int64]string{
			7: UsageCleanupStatusSucceeded,
		},
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	err := svc.CancelTask(context.Background(), 7, 1)
	require.Error(t, err)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
	require.Equal(t, "USAGE_CLEANUP_CANCEL_CONFLICT", infraerrors.Reason(err))
}

func TestUsageCleanupServiceCancelTaskAlreadyCanceledIsIdempotent(t *testing.T) {
	repo := &cleanupRepoStub{
		statusByID: map[int64]string{
			7: UsageCleanupStatusCanceled,
		},
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	err := svc.CancelTask(context.Background(), 7, 1)
	require.NoError(t, err)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.cancelCalls, "already canceled should return success without extra cancel write")
}

func TestUsageCleanupServiceCancelTaskRepoConflict(t *testing.T) {
	shouldCancel := false
	repo := &cleanupRepoStub{
		statusByID: map[int64]string{
			7: UsageCleanupStatusPending,
		},
		cancelResult: &shouldCancel,
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	err := svc.CancelTask(context.Background(), 7, 1)
	require.Error(t, err)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
	require.Equal(t, "USAGE_CLEANUP_CANCEL_CONFLICT", infraerrors.Reason(err))
}

func TestUsageCleanupServiceCancelTaskRepoError(t *testing.T) {
	repo := &cleanupRepoStub{
		statusByID: map[int64]string{
			7: UsageCleanupStatusPending,
		},
		cancelErr: errors.New("cancel failed"),
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	err := svc.CancelTask(context.Background(), 7, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cancel failed")
}

func TestUsageCleanupServiceCancelTaskInvalidCanceller(t *testing.T) {
	repo := &cleanupRepoStub{
		statusByID: map[int64]string{
			7: UsageCleanupStatusRunning,
		},
	}
	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svc := NewUsageCleanupService(repo, nil, nil, cfg)

	err := svc.CancelTask(context.Background(), 7, 0)
	require.Error(t, err)
	require.Equal(t, "USAGE_CLEANUP_INVALID_CANCELLER", infraerrors.Reason(err))
}

func TestUsageCleanupServiceListTasks(t *testing.T) {
	repo := &cleanupRepoStub{
		listTasks: []UsageCleanupTask{{ID: 1}, {ID: 2}},
		listResult: &pagination.PaginationResult{
			Total:    2,
			Page:     1,
			PageSize: 20,
			Pages:    1,
		},
	}
	svc := NewUsageCleanupService(repo, nil, nil, &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}})

	tasks, result, err := svc.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, int64(2), result.Total)
}

func TestUsageCleanupServiceListTasksNotReady(t *testing.T) {
	var nilSvc *UsageCleanupService
	_, _, err := nilSvc.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20})
	require.Error(t, err)

	svc := NewUsageCleanupService(nil, nil, nil, &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}})
	_, _, err = svc.ListTasks(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20})
	require.Error(t, err)
}

func TestUsageCleanupServiceDefaultsAndLifecycle(t *testing.T) {
	var nilSvc *UsageCleanupService
	require.Equal(t, 31, nilSvc.maxRangeDays())
	require.Equal(t, 5000, nilSvc.batchSize())
	require.Equal(t, 10*time.Second, nilSvc.workerInterval())
	require.Equal(t, 30*time.Minute, nilSvc.taskTimeout())
	nilSvc.Start()
	nilSvc.Stop()

	repo := &cleanupRepoStub{}
	cfgDisabled := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: false}}
	svcDisabled := NewUsageCleanupService(repo, nil, nil, cfgDisabled)
	svcDisabled.Start()
	svcDisabled.Stop()

	timingWheel, err := NewTimingWheelService()
	require.NoError(t, err)

	cfg := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true, WorkerIntervalSeconds: 5}}
	svc := NewUsageCleanupService(repo, timingWheel, nil, cfg)
	require.Equal(t, 5*time.Second, svc.workerInterval())
	svc.Start()
	svc.Stop()

	cfgFallback := &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}}
	svcFallback := NewUsageCleanupService(repo, timingWheel, nil, cfgFallback)
	require.Equal(t, 31, svcFallback.maxRangeDays())
	require.Equal(t, 5000, svcFallback.batchSize())
	require.Equal(t, 10*time.Second, svcFallback.workerInterval())

	svcMissingDeps := NewUsageCleanupService(nil, nil, nil, cfgFallback)
	svcMissingDeps.Start()
}

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
	}

	sanitizeUsageCleanupFilters(&filters)
	require.Nil(t, filters.UserID)
	require.Nil(t, filters.APIKeyID)
	require.Nil(t, filters.AccountID)
	require.Nil(t, filters.GroupID)
	require.Nil(t, filters.Model)
}

func TestDescribeUsageCleanupFiltersAllFields(t *testing.T) {
	start := time.Date(2024, 2, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	userID := int64(1)
	apiKeyID := int64(2)
	accountID := int64(3)
	groupID := int64(4)
	model := " gpt-4 "
	stream := true
	billingType := int8(2)
	filters := UsageCleanupFilters{
		StartTime:   start,
		EndTime:     end,
		UserID:      &userID,
		APIKeyID:    &apiKeyID,
		AccountID:   &accountID,
		GroupID:     &groupID,
		Model:       &model,
		Stream:      &stream,
		BillingType: &billingType,
	}

	desc := describeUsageCleanupFilters(filters)
	require.Equal(t, "start=2024-02-01T10:00:00Z end=2024-02-01T12:00:00Z user_id=1 api_key_id=2 account_id=3 group_id=4 model=gpt-4 stream=true billing_type=2", desc)
}

func TestUsageCleanupServiceIsTaskCanceledNotFound(t *testing.T) {
	repo := &cleanupRepoStub{}
	svc := NewUsageCleanupService(repo, nil, nil, &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}})

	canceled, err := svc.isTaskCanceled(context.Background(), 9)
	require.NoError(t, err)
	require.False(t, canceled)
}

func TestUsageCleanupServiceIsTaskCanceledError(t *testing.T) {
	repo := &cleanupRepoStub{statusErr: errors.New("status err")}
	svc := NewUsageCleanupService(repo, nil, nil, &config.Config{UsageCleanup: config.UsageCleanupConfig{Enabled: true}})

	_, err := svc.isTaskCanceled(context.Background(), 9)
	require.Error(t, err)
	require.Contains(t, err.Error(), "status err")
}
