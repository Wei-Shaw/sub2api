package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	usageCleanupWorkerName = "usage_cleanup_worker"
)

// UsageCleanupService 负责创建与执行使用记录清理任务
type UsageCleanupService struct {
	repo        UsageCleanupRepository
	timingWheel *TimingWheelService
	dashboard   *DashboardAggregationService
	cfg         *config.Config

	running   int32
	startOnce sync.Once
	stopOnce  sync.Once

	workerCtx    context.Context
	workerCancel context.CancelFunc
REDACTED

func NewUsageCleanupService(repo UsageCleanupRepository, timingWheel *TimingWheelService, dashboard *DashboardAggregationService, cfg *config.Config) *UsageCleanupService {
	workerCtx, workerCancel := context.WithCancel(context.Background())
	return &UsageCleanupService{
		repo:         repo,
		timingWheel:  timingWheel,
		dashboard:    dashboard,
		cfg:          cfg,
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
REDACTED
REDACTED

func describeUsageCleanupFilters(filters UsageCleanupFilters) string {
	var parts []string
	parts = append(parts, "start="+filters.StartTime.UTC().Format(time.RFC3339))
	parts = append(parts, "end="+filters.EndTime.UTC().Format(time.RFC3339))
	if filters.UserID != nil {
		parts = append(parts, fmt.Sprintf("user_id=%d", *filters.UserID))
REDACTED
	if filters.APIKeyID != nil {
		parts = append(parts, fmt.Sprintf("api_key_id=%d", *filters.APIKeyID))
REDACTED
	if filters.AccountID != nil {
		parts = append(parts, fmt.Sprintf("account_id=%d", *filters.AccountID))
REDACTED
	if filters.GroupID != nil {
		parts = append(parts, fmt.Sprintf("group_id=%d", *filters.GroupID))
REDACTED
	if filters.Model != nil {
		parts = append(parts, "model="+strings.TrimSpace(*filters.Model))
REDACTED
	if filters.Stream != nil {
		parts = append(parts, fmt.Sprintf("stream=%t", *filters.Stream))
REDACTED
	if filters.BillingType != nil {
		parts = append(parts, fmt.Sprintf("billing_type=%d", *filters.BillingType))
REDACTED
	return strings.Join(parts, " ")
REDACTED

func (s *UsageCleanupService) Start() {
	if s == nil {
		return
REDACTED
	if s.cfg != nil && !s.cfg.UsageCleanup.Enabled {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] not started (disabled)")
		return
REDACTED
	if s.repo == nil || s.timingWheel == nil {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] not started (missing deps)")
		return
REDACTED

	interval := s.workerInterval()
	s.startOnce.Do(func() {
		s.timingWheel.ScheduleRecurring(usageCleanupWorkerName, interval, s.runOnce)
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] started (interval=%s max_range_days=%d batch_size=%d task_timeout=%s)", interval, s.maxRangeDays(), s.batchSize(), s.taskTimeout())
REDACTED)
REDACTED

func (s *UsageCleanupService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		if s.workerCancel != nil {
			s.workerCancel()
	REDACTED
		if s.timingWheel != nil {
			s.timingWheel.Cancel(usageCleanupWorkerName)
	REDACTED
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] stopped")
REDACTED)
REDACTED

func (s *UsageCleanupService) ListTasks(ctx context.Context, params pagination.PaginationParams) ([]UsageCleanupTask, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return nil, nil, fmt.Errorf("cleanup service not ready")
REDACTED
	return s.repo.ListTasks(ctx, params)
REDACTED

func (s *UsageCleanupService) CreateTask(ctx context.Context, filters UsageCleanupFilters, createdBy int64) (*UsageCleanupTask, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("cleanup service not ready")
REDACTED
	if s.cfg != nil && !s.cfg.UsageCleanup.Enabled {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "USAGE_CLEANUP_DISABLED", "usage cleanup is disabled")
REDACTED
	if createdBy <= 0 {
		return nil, infraerrors.BadRequest("USAGE_CLEANUP_INVALID_CREATOR", "invalid creator")
REDACTED

	logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] create_task requested: operator=%d %s", createdBy, describeUsageCleanupFilters(filters))
	sanitizeUsageCleanupFilters(&filters)
	if err := s.validateFilters(filters); err != nil {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] create_task rejected: operator=%d err=%v %s", createdBy, err, describeUsageCleanupFilters(filters))
		return nil, err
REDACTED

	task := &UsageCleanupTask{
		Status:    UsageCleanupStatusPending,
		Filters:   filters,
		CreatedBy: createdBy,
REDACTED
	if err := s.repo.CreateTask(ctx, task); err != nil {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] create_task persist failed: operator=%d err=%v %s", createdBy, err, describeUsageCleanupFilters(filters))
		return nil, fmt.Errorf("create cleanup task: %w", err)
REDACTED
	logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] create_task persisted: task=%d operator=%d status=%s deleted_rows=%d %s", task.ID, createdBy, task.Status, task.DeletedRows, describeUsageCleanupFilters(filters))
	go s.runOnce()
	return task, nil
REDACTED

func (s *UsageCleanupService) runOnce() {
	svc := s
	if svc == nil {
		return
REDACTED
	if !atomic.CompareAndSwapInt32(&svc.running, 0, 1) {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] run_once skipped: already_running=true")
		return
REDACTED
	defer atomic.StoreInt32(&svc.running, 0)

	parent := context.Background()
	if svc.workerCtx != nil {
		parent = svc.workerCtx
REDACTED
	ctx, cancel := context.WithTimeout(parent, svc.taskTimeout())
	defer cancel()

	task, err := svc.repo.ClaimNextPendingTask(ctx, int64(svc.taskTimeout().Seconds()))
	if err != nil {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] claim pending task failed: %v", err)
		return
REDACTED
	if task == nil {
		slog.Debug("[UsageCleanup] run_once done: no_task=true")
		return
REDACTED

	logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task claimed: task=%d status=%s created_by=%d deleted_rows=%d %s", task.ID, task.Status, task.CreatedBy, task.DeletedRows, describeUsageCleanupFilters(task.Filters))
	svc.executeTask(ctx, task)
REDACTED

func (s *UsageCleanupService) executeTask(ctx context.Context, task *UsageCleanupTask) {
	if task == nil {
		return
REDACTED

	batchSize := s.batchSize()
	deletedTotal := task.DeletedRows
	start := time.Now()
	logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task started: task=%d batch_size=%d deleted_rows=%d %s", task.ID, batchSize, deletedTotal, describeUsageCleanupFilters(task.Filters))
	var batchNum int

	for {
		if ctx != nil && ctx.Err() != nil {
			logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task interrupted: task=%d err=%v", task.ID, ctx.Err())
			return
	REDACTED
		canceled, err := s.isTaskCanceled(ctx, task.ID)
		if err != nil {
			s.markTaskFailed(task.ID, deletedTotal, err)
			return
	REDACTED
		if canceled {
			logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task canceled: task=%d deleted_rows=%d duration=%s", task.ID, deletedTotal, time.Since(start))
			return
	REDACTED

		batchNum++
		deleted, err := s.repo.DeleteUsageLogsBatch(ctx, task.Filters, batchSize)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// 任务被中断（例如服务停止/超时），保持 running 状态，后续通过 stale reclaim 续跑。
				logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task interrupted: task=%d err=%v", task.ID, err)
				return
		REDACTED
			s.markTaskFailed(task.ID, deletedTotal, err)
			return
	REDACTED
		deletedTotal += deleted
		if deleted > 0 {
			updateCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if err := s.repo.UpdateTaskProgress(updateCtx, task.ID, deletedTotal); err != nil {
				logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task progress update failed: task=%d deleted_rows=%d err=%v", task.ID, deletedTotal, err)
		REDACTED
			cancel()
	REDACTED
		if batchNum <= 3 || batchNum%20 == 0 || deleted < int64(batchSize) {
			logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task batch done: task=%d batch=%d deleted=%d deleted_total=%d", task.ID, batchNum, deleted, deletedTotal)
	REDACTED
		if deleted == 0 || deleted < int64(batchSize) {
			break
	REDACTED
REDACTED

	updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.repo.MarkTaskSucceeded(updateCtx, task.ID, deletedTotal); err != nil {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] update task succeeded failed: task=%d err=%v", task.ID, err)
REDACTED else {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task succeeded: task=%d deleted_rows=%d duration=%s", task.ID, deletedTotal, time.Since(start))
REDACTED

	if s.dashboard != nil {
		if err := s.dashboard.TriggerRecomputeRange(task.Filters.StartTime, task.Filters.EndTime); err != nil {
			logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] trigger dashboard recompute failed: task=%d err=%v", task.ID, err)
	REDACTED else {
			logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] trigger dashboard recompute: task=%d start=%s end=%s", task.ID, task.Filters.StartTime.UTC().Format(time.RFC3339), task.Filters.EndTime.UTC().Format(time.RFC3339))
	REDACTED
REDACTED
REDACTED

func (s *UsageCleanupService) markTaskFailed(taskID int64, deletedRows int64, err error) {
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 500 {
		msg = msg[:500]
REDACTED
	logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task failed: task=%d deleted_rows=%d err=%s", taskID, deletedRows, msg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if updateErr := s.repo.MarkTaskFailed(ctx, taskID, deletedRows, msg); updateErr != nil {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] update task failed failed: task=%d err=%v", taskID, updateErr)
REDACTED
REDACTED

func (s *UsageCleanupService) isTaskCanceled(ctx context.Context, taskID int64) (bool, error) {
	if s == nil || s.repo == nil {
		return false, fmt.Errorf("cleanup service not ready")
REDACTED
	checkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	status, err := s.repo.GetTaskStatus(checkCtx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
	REDACTED
		return false, err
REDACTED
	if status == UsageCleanupStatusCanceled {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] task cancel detected: task=%d", taskID)
REDACTED
	return status == UsageCleanupStatusCanceled, nil
REDACTED

func (s *UsageCleanupService) validateFilters(filters UsageCleanupFilters) error {
	if filters.StartTime.IsZero() || filters.EndTime.IsZero() {
		return infraerrors.BadRequest("USAGE_CLEANUP_MISSING_RANGE", "start_date and end_date are required")
REDACTED
	if filters.EndTime.Before(filters.StartTime) {
		return infraerrors.BadRequest("USAGE_CLEANUP_INVALID_RANGE", "end_date must be after start_date")
REDACTED
	maxDays := s.maxRangeDays()
	if maxDays > 0 {
		delta := filters.EndTime.Sub(filters.StartTime)
		if delta > time.Duration(maxDays)*24*time.Hour {
			return infraerrors.BadRequest("USAGE_CLEANUP_RANGE_TOO_LARGE", fmt.Sprintf("date range exceeds %d days", maxDays))
	REDACTED
REDACTED
	return nil
REDACTED

func (s *UsageCleanupService) CancelTask(ctx context.Context, taskID int64, canceledBy int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("cleanup service not ready")
REDACTED
	if s.cfg != nil && !s.cfg.UsageCleanup.Enabled {
		return infraerrors.New(http.StatusServiceUnavailable, "USAGE_CLEANUP_DISABLED", "usage cleanup is disabled")
REDACTED
	if canceledBy <= 0 {
		return infraerrors.BadRequest("USAGE_CLEANUP_INVALID_CANCELLER", "invalid canceller")
REDACTED
	status, err := s.repo.GetTaskStatus(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.New(http.StatusNotFound, "USAGE_CLEANUP_TASK_NOT_FOUND", "cleanup task not found")
	REDACTED
		return err
REDACTED
	logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] cancel_task requested: task=%d operator=%d status=%s", taskID, canceledBy, status)
	if status == UsageCleanupStatusCanceled {
		logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] cancel_task idempotent hit: task=%d operator=%d", taskID, canceledBy)
		return nil
REDACTED
	if status != UsageCleanupStatusPending && status != UsageCleanupStatusRunning {
		return infraerrors.New(http.StatusConflict, "USAGE_CLEANUP_CANCEL_CONFLICT", "cleanup task cannot be canceled in current status")
REDACTED
	ok, err := s.repo.CancelTask(ctx, taskID, canceledBy)
	if err != nil {
		return err
REDACTED
	if !ok {
		// 状态可能并发改变
		currentStatus, getErr := s.repo.GetTaskStatus(ctx, taskID)
		if getErr == nil && currentStatus == UsageCleanupStatusCanceled {
			logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] cancel_task idempotent race hit: task=%d operator=%d", taskID, canceledBy)
			return nil
	REDACTED
		return infraerrors.New(http.StatusConflict, "USAGE_CLEANUP_CANCEL_CONFLICT", "cleanup task cannot be canceled in current status")
REDACTED
	logger.LegacyPrintf("service.usage_cleanup", "[UsageCleanup] cancel_task done: task=%d operator=%d", taskID, canceledBy)
	return nil
REDACTED

func sanitizeUsageCleanupFilters(filters *UsageCleanupFilters) {
	if filters == nil {
		return
REDACTED
	if filters.UserID != nil && *filters.UserID <= 0 {
		filters.UserID = nil
REDACTED
	if filters.APIKeyID != nil && *filters.APIKeyID <= 0 {
		filters.APIKeyID = nil
REDACTED
	if filters.AccountID != nil && *filters.AccountID <= 0 {
		filters.AccountID = nil
REDACTED
	if filters.GroupID != nil && *filters.GroupID <= 0 {
		filters.GroupID = nil
REDACTED
	if filters.Model != nil {
		model := strings.TrimSpace(*filters.Model)
		if model == "" {
			filters.Model = nil
	REDACTED else {
			filters.Model = &model
	REDACTED
REDACTED
	if filters.BillingType != nil && *filters.BillingType < 0 {
		filters.BillingType = nil
REDACTED
REDACTED

func (s *UsageCleanupService) maxRangeDays() int {
	if s == nil || s.cfg == nil {
		return 31
REDACTED
	if s.cfg.UsageCleanup.MaxRangeDays > 0 {
		return s.cfg.UsageCleanup.MaxRangeDays
REDACTED
	return 31
REDACTED

func (s *UsageCleanupService) batchSize() int {
	if s == nil || s.cfg == nil {
		return 5000
REDACTED
	if s.cfg.UsageCleanup.BatchSize > 0 {
		return s.cfg.UsageCleanup.BatchSize
REDACTED
	return 5000
REDACTED

func (s *UsageCleanupService) workerInterval() time.Duration {
	if s == nil || s.cfg == nil {
		return 10 * time.Second
REDACTED
	if s.cfg.UsageCleanup.WorkerIntervalSeconds > 0 {
		return time.Duration(s.cfg.UsageCleanup.WorkerIntervalSeconds) * time.Second
REDACTED
	return 10 * time.Second
REDACTED

func (s *UsageCleanupService) taskTimeout() time.Duration {
	if s == nil || s.cfg == nil {
		return 30 * time.Minute
REDACTED
	if s.cfg.UsageCleanup.TaskTimeoutSeconds > 0 {
		return time.Duration(s.cfg.UsageCleanup.TaskTimeoutSeconds) * time.Second
REDACTED
	return 30 * time.Minute
REDACTED
