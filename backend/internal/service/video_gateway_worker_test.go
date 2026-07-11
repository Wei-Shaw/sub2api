package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestVideoGatewayWorkerDisabledIsOperatorVisible(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	cfg := &config.Config{
		VideoGateway:    config.VideoGatewayConfig{WorkerEnabled: false},
		ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: false},
	}
	service := NewVideoGatewayService(newMemoryVideoGatewayRepo(), noopVideoKeyEncryptor{}, cfg)
	worker := NewVideoGatewayWorker(service, cfg)
	worker.Start()
	worker.Stop()

	logs := output.String()
	if !strings.Contains(logs, "video_gateway_worker_disabled") {
		t.Fatalf("disabled worker log missing stable event name: %s", logs)
	}
	if !strings.Contains(logs, "queued video tasks will not progress") {
		t.Fatalf("disabled worker log missing operator consequence: %s", logs)
	}
}

type noopVideoKeyEncryptor struct{}

type recordingWorkerVideoPricing struct {
	actualCalls int
	amountUSD   Money
	snapshot    PricingSnapshot
	actualErr   error
}

type recordingWorkerFinalizationRepository struct {
	mu      sync.Mutex
	calls   []VideoTaskFinalizationInput
	results []VideoTaskFinalizationResult
	errs    []error
}

// billableFakeAdapter is an explicitly offline adapter used only to prove the
// billable reliability path. It owns no HTTP client or transport and therefore
// cannot open a socket.
type billableFakeAdapter struct {
	pollCalls    int
	networkCalls int
	result       *VideoAdapterResult
}

func (a *billableFakeAdapter) Provider() string { return VideoProviderSeedance }
func (a *billableFakeAdapter) CreateTask(_ context.Context, _ *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{UpstreamTaskID: fmt.Sprintf("billable-fake-%d", task.ID), Status: VideoStatusSubmitted}, nil
}
func (a *billableFakeAdapter) PollTask(context.Context, *VideoProviderAccount, *VideoTask) (*VideoAdapterResult, error) {
	a.pollCalls++
	if a.result == nil {
		return &VideoAdapterResult{Status: VideoStatusSucceeded, ResultURL: "local://billable-fake/result.mp4"}, nil
	}
	result := *a.result
	return &result, nil
}
func (a *billableFakeAdapter) CancelTask(context.Context, *VideoProviderAccount, *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{Status: VideoStatusCancelled}, nil
}
func (a *billableFakeAdapter) NormalizeStatus(status string) string {
	return normalizeVideoStatus(status)
}
func (a *billableFakeAdapter) BuildCreatePayload(_ *VideoProviderAccount, task *VideoTask) map[string]any {
	return map[string]any{"model": task.Model, "prompt": task.Prompt}
}

func (r *recordingWorkerFinalizationRepository) FinalizeVideoTask(_ context.Context, input VideoTaskFinalizationInput) (VideoTaskFinalizationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, input)
	index := len(r.calls) - 1
	var result VideoTaskFinalizationResult
	if index < len(r.results) {
		result = r.results[index]
	}
	if index < len(r.errs) {
		return result, r.errs[index]
	}
	return result, nil
}

func (r *recordingWorkerFinalizationRepository) recordedCalls() []VideoTaskFinalizationInput {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]VideoTaskFinalizationInput(nil), r.calls...)
}

func (p *recordingWorkerVideoPricing) EstimatePrice(context.Context, *VideoTask) (Money, PricingSnapshot, error) {
	return MustUSD("0"), PricingSnapshot{}, nil
}

func (p *recordingWorkerVideoPricing) ActualPrice(context.Context, *VideoTask) (Money, PricingSnapshot, error) {
	p.actualCalls++
	if p.actualErr != nil {
		return Money{}, PricingSnapshot{}, p.actualErr
	}
	return p.amountUSD, p.snapshot, nil
}

func (noopVideoKeyEncryptor) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (noopVideoKeyEncryptor) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "enc:"), nil
}

type memoryVideoGatewayRepo struct {
	dispatchMu      sync.Mutex
	nextProviderID  int64
	nextTaskID      int64
	nextEventID     int64
	providers       map[int64]*VideoProviderAccount
	tasks           map[int64]*VideoTask
	events          []*VideoTaskEvent
	usage           []*VideoTask
	balanceClaims   map[int64]time.Time
	dailyTrials     map[string]struct{}
	acceptErr       error
	acceptConflict  bool
	unknownErr      error
	unknownConflict bool
	pollCASErr      error
	pollCASCalls    int
	updateTaskCalls int
	addEventCalls   int
	listEventsErr   error
}

func newMemoryVideoGatewayRepo() *memoryVideoGatewayRepo {
	return &memoryVideoGatewayRepo{
		nextProviderID: 1,
		nextTaskID:     1,
		nextEventID:    1,
		providers:      make(map[int64]*VideoProviderAccount),
		tasks:          make(map[int64]*VideoTask),
		balanceClaims:  make(map[int64]time.Time),
		dailyTrials:    make(map[string]struct{}),
	}
}

func (r *memoryVideoGatewayRepo) seedMockProvider() int64 {
	id := r.nextProviderID
	r.nextProviderID++
	now := time.Now().UTC()
	r.providers[id] = &VideoProviderAccount{
		ID:                 id,
		Provider:           VideoProviderMock,
		DisplayName:        "Mock Provider",
		Enabled:            true,
		BaseURL:            "mock://video-gateway",
		DefaultModel:       "mock-video-v1",
		RateLimitPerMinute: 60,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return id
}

func (r *memoryVideoGatewayRepo) CreateProviderAccount(_ context.Context, account *VideoProviderAccount) error {
	now := time.Now().UTC()
	account.ID = r.nextProviderID
	r.nextProviderID++
	account.CreatedAt = now
	account.UpdatedAt = now
	r.providers[account.ID] = cloneVideoProvider(account)
	return nil
}

func (r *memoryVideoGatewayRepo) GetProviderAccount(_ context.Context, id int64) (*VideoProviderAccount, error) {
	account, ok := r.providers[id]
	if !ok {
		return nil, ErrVideoProviderNotFound
	}
	return cloneVideoProvider(account), nil
}

func (r *memoryVideoGatewayRepo) ListProviderAccounts(_ context.Context) ([]*VideoProviderAccount, error) {
	out := make([]*VideoProviderAccount, 0, len(r.providers))
	for _, account := range r.providers {
		out = append(out, cloneVideoProvider(account))
	}
	return out, nil
}

func (r *memoryVideoGatewayRepo) UpdateProviderAccount(_ context.Context, account *VideoProviderAccount) error {
	account.UpdatedAt = time.Now().UTC()
	r.providers[account.ID] = cloneVideoProvider(account)
	return nil
}

func (r *memoryVideoGatewayRepo) CreateTask(_ context.Context, task *VideoTask) error {
	now := time.Now().UTC()
	task.ID = r.nextTaskID
	r.nextTaskID++
	task.CreatedAt = now
	task.UpdatedAt = now
	r.tasks[task.ID] = cloneVideoTask(task)
	return nil
}

func (r *memoryVideoGatewayRepo) CreateWithReservation(context.Context, VideoTaskCreationInput) (*VideoTaskCreationResult, error) {
	return nil, errors.New("atomic video task creation is not configured in memory repository")
}

func (r *memoryVideoGatewayRepo) ReplayExisting(context.Context, VideoTaskCreationReplayInput) (*VideoTaskCreationResult, bool, error) {
	return nil, false, nil
}

func (r *memoryVideoGatewayRepo) CreateDailyTrialTask(ctx context.Context, task *VideoTask, provider string, createdBy int64, trialDate time.Time) (bool, error) {
	key := fmt.Sprintf("%s:%d:%s", provider, createdBy, trialDate.Format("2006-01-02"))
	if _, ok := r.dailyTrials[key]; ok {
		return false, nil
	}
	r.dailyTrials[key] = struct{}{}
	if err := r.CreateTask(ctx, task); err != nil {
		delete(r.dailyTrials, key)
		return false, err
	}
	return true, nil
}

func (r *memoryVideoGatewayRepo) GetTask(_ context.Context, id int64) (*VideoTask, error) {
	task, ok := r.tasks[id]
	if !ok {
		return nil, ErrVideoTaskNotFound
	}
	return cloneVideoTask(task), nil
}

func (r *memoryVideoGatewayRepo) ListTasks(_ context.Context, params VideoTaskListParams) ([]*VideoTask, int64, error) {
	out := make([]*VideoTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if params.Status != "" && task.Status != params.Status {
			continue
		}
		if params.Provider != "" && task.Provider != params.Provider {
			continue
		}
		if !params.IsAdmin && task.CreatedBy != params.CreatedBy {
			continue
		}
		out = append(out, cloneVideoTask(task))
	}
	return out, int64(len(out)), nil
}

func (r *memoryVideoGatewayRepo) ListDramaTasks(ctx context.Context, params VideoTaskListParams, filters map[string]string) ([]*VideoTask, int64, error) {
	matched := make([]*VideoTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if params.Status != "" && task.Status != params.Status {
			continue
		}
		if params.Provider != "" && task.Provider != params.Provider {
			continue
		}
		if !params.IsAdmin && task.CreatedBy != params.CreatedBy {
			continue
		}
		events, err := r.ListTaskEvents(ctx, task.ID, 200)
		if err != nil {
			return nil, 0, err
		}
		if !hasDramaContextEvent(events) {
			continue
		}
		contract := dramaTaskContract(task, events)
		if !dramaTaskMatchesFilters(contract, filters) {
			continue
		}
		matched = append(matched, cloneVideoTask(task))
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID > matched[j].ID
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})
	total := int64(len(matched))
	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(matched) {
		return []*VideoTask{}, total, nil
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	return matched[start:end], total, nil
}

func hasDramaContextEvent(events []*VideoTaskEvent) bool {
	for _, event := range events {
		if event != nil && event.EventType == "drama_context" {
			return true
		}
	}
	return false
}

func (r *memoryVideoGatewayRepo) ListRunnableTasks(_ context.Context, limit int) ([]*VideoTask, error) {
	out := make([]*VideoTask, 0, limit)
	for _, task := range r.tasks {
		if IsTerminalVideoStatus(task.Status) || task.DispatchState == "unknown" {
			continue
		}
		out = append(out, cloneVideoTask(task))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *memoryVideoGatewayRepo) MarkDispatchingCAS(_ context.Context, task *VideoTask, event *VideoTaskEvent) (bool, error) {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	stored, ok := r.tasks[task.ID]
	if !ok || stored.Status != VideoStatusQueued || stored.DispatchState != "pending" || stored.Version != task.Version {
		return false, nil
	}
	updated := cloneVideoTask(stored)
	updated.Status = VideoStatusSubmitted
	updated.DispatchState = "dispatching"
	updated.Version++
	updated.UpdatedAt = time.Now().UTC()
	r.tasks[task.ID] = updated
	*task = *cloneVideoTask(updated)
	r.appendDispatchEvent(event)
	return true, nil
}

func (r *memoryVideoGatewayRepo) MarkDispatchAcceptedCAS(_ context.Context, task *VideoTask, event *VideoTaskEvent) (bool, error) {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	if r.acceptErr != nil {
		return false, r.acceptErr
	}
	if r.acceptConflict {
		return false, nil
	}
	stored, ok := r.tasks[task.ID]
	if !ok || stored.DispatchState != "dispatching" || stored.Version != task.Version {
		return false, nil
	}
	updated := cloneVideoTask(stored)
	updated.Status = task.Status
	updated.UpstreamTaskID = task.UpstreamTaskID
	updated.DispatchState = "accepted"
	updated.Version++
	updated.UpdatedAt = time.Now().UTC()
	r.tasks[task.ID] = updated
	*task = *cloneVideoTask(updated)
	r.appendDispatchEvent(event)
	return true, nil
}

func (r *memoryVideoGatewayRepo) MarkDispatchUnknownCAS(_ context.Context, task *VideoTask, event *VideoTaskEvent) (bool, error) {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	if r.unknownErr != nil {
		return false, r.unknownErr
	}
	if r.unknownConflict {
		return false, nil
	}
	stored, ok := r.tasks[task.ID]
	if !ok || stored.DispatchState != "dispatching" || stored.Version != task.Version {
		return false, nil
	}
	updated := cloneVideoTask(stored)
	updated.Status = VideoStatusSubmitted
	updated.DispatchState = "unknown"
	updated.ErrorMessage = task.ErrorMessage
	updated.Version++
	updated.UpdatedAt = time.Now().UTC()
	r.tasks[task.ID] = updated
	*task = *cloneVideoTask(updated)
	r.appendDispatchEvent(event)
	return true, nil
}

func (r *memoryVideoGatewayRepo) UpdatePolledTaskCAS(_ context.Context, expectedVersion int64, candidate *VideoTask, event *VideoTaskEvent) (VideoTaskPollUpdateResult, error) {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	r.pollCASCalls++
	if r.pollCASErr != nil {
		return VideoTaskPollUpdateResult{}, r.pollCASErr
	}
	stored, ok := r.tasks[candidate.ID]
	if !ok {
		return VideoTaskPollUpdateResult{}, ErrVideoTaskNotFound
	}
	if stored.Version != expectedVersion || IsTerminalVideoStatus(stored.Status) {
		return videoTaskPollUpdateResultForTest(stored, false), nil
	}
	if event == nil || event.VideoTaskID != candidate.ID {
		return VideoTaskPollUpdateResult{}, fmt.Errorf("matching poll event is required")
	}
	updated := cloneVideoTask(candidate)
	updated.Version = stored.Version + 1
	updated.WorkerClaimedAt = nil
	updated.WorkerClaimedUntil = nil
	updated.UpdatedAt = time.Now().UTC()
	r.tasks[candidate.ID] = updated
	r.appendDispatchEvent(event)
	return videoTaskPollUpdateResultForTest(updated, true), nil
}

func videoTaskPollUpdateResultForTest(task *VideoTask, applied bool) VideoTaskPollUpdateResult {
	return VideoTaskPollUpdateResult{
		Applied:            applied,
		Status:             task.Status,
		Version:            task.Version,
		ResultURL:          task.ResultURL,
		ErrorMessage:       task.ErrorMessage,
		PollCount:          task.PollCount,
		UsageTotalTokens:   task.UsageTotalTokens,
		ActualResolution:   task.ActualResolution,
		ActualDuration:     task.ActualDuration,
		LastFrameURL:       task.LastFrameURL,
		SettlementStatus:   task.SettlementStatus,
		BalanceChargedAt:   task.BalanceChargedAt,
		WorkerClaimedAt:    task.WorkerClaimedAt,
		WorkerClaimedUntil: task.WorkerClaimedUntil,
		ArchiveStatus:      task.ArchiveStatus,
		CaptureStatus:      task.CaptureStatus,
		CompletedAt:        task.CompletedAt,
	}
}

func (r *memoryVideoGatewayRepo) ListDispatchUnknownTasks(_ context.Context, limit int) ([]*VideoTask, error) {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	out := make([]*VideoTask, 0, limit)
	for _, task := range r.tasks {
		if task.DispatchState != "unknown" {
			continue
		}
		out = append(out, cloneVideoTask(task))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *memoryVideoGatewayRepo) appendDispatchEvent(event *VideoTaskEvent) {
	if event == nil {
		return
	}
	event.ID = r.nextEventID
	r.nextEventID++
	event.CreatedAt = time.Now().UTC()
	r.events = append(r.events, cloneVideoEvent(event))
}

func (r *memoryVideoGatewayRepo) ListUnchargedSucceededVideoTasks(_ context.Context, limit int) ([]*VideoTask, error) {
	out := make([]*VideoTask, 0, limit)
	for _, task := range r.tasks {
		if task.Status != VideoStatusSucceeded {
			continue
		}
		if task.CostEstimate <= 0 && (task.UsageTotalTokens == nil || *task.UsageTotalTokens <= 0) {
			continue
		}
		if _, ok := r.balanceClaims[task.ID]; ok {
			continue
		}
		out = append(out, cloneVideoTask(task))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *memoryVideoGatewayRepo) ClaimTaskForSubmit(_ context.Context, taskID int64) (bool, error) {
	task, ok := r.tasks[taskID]
	if !ok || task.Status != VideoStatusQueued {
		return false, nil
	}
	updated := cloneVideoTask(task)
	updated.Status = VideoStatusSubmitted
	updated.UpdatedAt = time.Now().UTC()
	r.tasks[taskID] = updated
	return true, nil
}

func (r *memoryVideoGatewayRepo) UpdateTask(_ context.Context, task *VideoTask) error {
	r.updateTaskCalls++
	task.UpdatedAt = time.Now().UTC()
	r.tasks[task.ID] = cloneVideoTask(task)
	return nil
}

func (r *memoryVideoGatewayRepo) SetTaskLocalAsset(_ context.Context, taskID int64, path string, savedAt time.Time) error {
	task, ok := r.tasks[taskID]
	if !ok {
		return ErrVideoTaskNotFound
	}
	task.LocalAssetPath = path
	t := savedAt
	task.LocalAssetSavedAt = &t
	return nil
}

func (r *memoryVideoGatewayRepo) ClearTaskLocalAsset(_ context.Context, taskID int64) error {
	task, ok := r.tasks[taskID]
	if !ok {
		return nil
	}
	task.LocalAssetPath = ""
	task.LocalAssetSavedAt = nil
	return nil
}

func (r *memoryVideoGatewayRepo) ListExpiredLocalAssets(_ context.Context, olderThan time.Time, limit int) ([]*VideoTask, error) {
	out := make([]*VideoTask, 0)
	for _, task := range r.tasks {
		if task.LocalAssetPath == "" || task.LocalAssetSavedAt == nil {
			continue
		}
		if task.LocalAssetSavedAt.Before(olderThan) {
			out = append(out, task)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (r *memoryVideoGatewayRepo) AddTaskEvent(_ context.Context, event *VideoTaskEvent) error {
	r.addEventCalls++
	event.ID = r.nextEventID
	r.nextEventID++
	event.CreatedAt = time.Now().UTC()
	r.events = append(r.events, cloneVideoEvent(event))
	return nil
}

func (r *memoryVideoGatewayRepo) ListTaskEvents(_ context.Context, taskID int64, _ int) ([]*VideoTaskEvent, error) {
	if r.listEventsErr != nil {
		return nil, r.listEventsErr
	}
	out := make([]*VideoTaskEvent, 0)
	for _, event := range r.events {
		if event.VideoTaskID == taskID {
			out = append(out, cloneVideoEvent(event))
		}
	}
	return out, nil
}

func (r *memoryVideoGatewayRepo) InsertUsageLog(_ context.Context, task *VideoTask) error {
	r.usage = append(r.usage, cloneVideoTask(task))
	return nil
}

func (r *memoryVideoGatewayRepo) ClaimVideoBalanceCharge(_ context.Context, taskID int64) (time.Time, bool, error) {
	if _, ok := r.balanceClaims[taskID]; ok {
		return time.Time{}, false, nil
	}
	task, ok := r.tasks[taskID]
	if !ok {
		return time.Time{}, false, nil
	}
	claimedAt := time.Now().UTC()
	r.balanceClaims[taskID] = claimedAt
	updated := cloneVideoTask(task)
	r.tasks[taskID] = updated
	return claimedAt, true, nil
}

func (r *memoryVideoGatewayRepo) ClearVideoBalanceChargeIfClaimedAt(_ context.Context, taskID int64, claimedAt time.Time) (bool, error) {
	existing, ok := r.balanceClaims[taskID]
	if !ok || !existing.Equal(claimedAt) {
		return false, nil
	}
	delete(r.balanceClaims, taskID)
	return true, nil
}

func (r *memoryVideoGatewayRepo) CountTasksSince(_ context.Context, _ time.Time) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (r *memoryVideoGatewayRepo) CountProviderTasksSince(_ context.Context, _ time.Time) (map[string]map[string]int64, error) {
	return map[string]map[string]int64{}, nil
}

func (r *memoryVideoGatewayRepo) ProviderAccountTaskStatsSince(_ context.Context, since time.Time) (map[int64]VideoProviderRuntimeStats, error) {
	out := map[int64]VideoProviderRuntimeStats{}
	for _, task := range r.tasks {
		item := out[task.ProviderAccountID]
		if !task.CreatedAt.Before(since) {
			item.TodayTasks++
			if task.Status == VideoStatusFailed {
				item.TodayFailures++
			}
		}
		switch task.Status {
		case VideoStatusQueued, VideoStatusSubmitted, VideoStatusRunning:
			item.CurrentInflight++
		}
		if task.Status == VideoStatusFailed && task.ErrorMessage != "" {
			if item.LastErrorAt == nil || task.UpdatedAt.After(*item.LastErrorAt) {
				updatedAt := task.UpdatedAt
				item.LastError = task.ErrorMessage
				item.LastErrorAt = &updatedAt
			}
		}
		out[task.ProviderAccountID] = item
	}
	return out, nil
}

func (r *memoryVideoGatewayRepo) ListRecentTasksByStatus(_ context.Context, _ string, _ int) ([]*VideoTask, error) {
	return []*VideoTask{}, nil
}

func (r *memoryVideoGatewayRepo) UsageSummarySince(_ context.Context, _ time.Time) ([]VideoUsageSummary, error) {
	return []VideoUsageSummary{}, nil
}

type recordingDispatchAdapter struct {
	mu          sync.Mutex
	createCalls int
	pollCalls   int
	result      *VideoAdapterResult
	err         error
	pollErr     error
}

func (a *recordingDispatchAdapter) Provider() string { return VideoProviderMock }

func (a *recordingDispatchAdapter) CreateTask(context.Context, *VideoProviderAccount, *VideoTask) (*VideoAdapterResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.createCalls++
	if a.result == nil {
		return nil, a.err
	}
	return cloneVideoAdapterResult(a.result), a.err
}

func (a *recordingDispatchAdapter) PollTask(context.Context, *VideoProviderAccount, *VideoTask) (*VideoAdapterResult, error) {
	a.mu.Lock()
	a.pollCalls++
	a.mu.Unlock()
	if a.pollErr != nil {
		return nil, a.pollErr
	}
	return &VideoAdapterResult{Status: VideoStatusRunning}, nil
}

func (a *recordingDispatchAdapter) CancelTask(context.Context, *VideoProviderAccount, *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{Status: VideoStatusCancelled}, nil
}

func (a *recordingDispatchAdapter) NormalizeStatus(status string) string {
	return normalizeVideoStatus(status)
}

func (a *recordingDispatchAdapter) BuildCreatePayload(*VideoProviderAccount, *VideoTask) map[string]any {
	return map[string]any{}
}

func (a *recordingDispatchAdapter) calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.createCalls
}

func (a *recordingDispatchAdapter) polls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pollCalls
}

func seedPendingDispatchTask(repo *memoryVideoGatewayRepo, providerID, taskID int64) *VideoTask {
	task := &VideoTask{
		ID:                taskID,
		ProviderAccountID: providerID,
		Provider:          VideoProviderMock,
		Model:             "fake-video",
		Status:            VideoStatusQueued,
		Version:           1,
		DispatchState:     "pending",
		CreatedAt:         time.Now().UTC(),
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	return task
}

func TestVideoGatewayWorkerDispatchClaimsQueuedPendingOnceAndAccepts(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := seedPendingDispatchTask(repo, providerID, 9101)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	adapter := &recordingDispatchAdapter{result: &VideoAdapterResult{UpstreamTaskID: "upstream-9101", Status: VideoStatusSubmitted}}
	account := repo.providers[providerID]

	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		copy := cloneVideoTask(task)
		go func() {
			<-start
			errs <- svc.submitTask(context.Background(), adapter, account, copy)
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("submit task: %v", err)
		}
	}

	stored := repo.tasks[task.ID]
	if adapter.calls() != 1 {
		t.Fatalf("provider create calls = %d, want 1", adapter.calls())
	}
	if stored.Status != VideoStatusSubmitted || stored.DispatchState != "accepted" || stored.UpstreamTaskID != "upstream-9101" || stored.Version != 3 {
		t.Fatalf("accepted dispatch = status=%q state=%q upstream=%q version=%d", stored.Status, stored.DispatchState, stored.UpstreamTaskID, stored.Version)
	}
}

func TestVideoGatewayWorkerDispatchTimeoutOrEOFMovesUnknownWithoutRecreate(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "eof", err: io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMemoryVideoGatewayRepo()
			providerID := repo.seedMockProvider()
			task := seedPendingDispatchTask(repo, providerID, 9201)
			svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
			adapter := &recordingDispatchAdapter{err: &VideoDispatchTransportError{
				Err:                    tc.err,
				RequestMayHaveBeenSent: true,
			}}

			_ = svc.submitTask(context.Background(), adapter, repo.providers[providerID], task)
			stored := repo.tasks[task.ID]
			if stored.Status != VideoStatusSubmitted || stored.DispatchState != "unknown" || stored.Version != 3 {
				t.Fatalf("ambiguous dispatch = status=%q state=%q version=%d", stored.Status, stored.DispatchState, stored.Version)
			}
			_ = svc.submitTask(context.Background(), adapter, repo.providers[providerID], cloneVideoTask(stored))
			if adapter.calls() != 1 {
				t.Fatalf("provider create calls after unknown replay = %d, want 1", adapter.calls())
			}
		})
	}
}

func TestVideoGatewayWorkerDispatchSaveFailureMovesUnknown(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := seedPendingDispatchTask(repo, providerID, 9301)
	repo.acceptErr = errors.New("persist upstream id failed")
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	adapter := &recordingDispatchAdapter{result: &VideoAdapterResult{UpstreamTaskID: "upstream-9301", Status: VideoStatusSubmitted}}

	_ = svc.submitTask(context.Background(), adapter, repo.providers[providerID], task)
	stored := repo.tasks[task.ID]
	if adapter.calls() != 1 || stored.DispatchState != "unknown" || stored.Version != 3 || stored.UpstreamTaskID != "" {
		t.Fatalf("save failure dispatch = calls=%d state=%q version=%d upstream=%q", adapter.calls(), stored.DispatchState, stored.Version, stored.UpstreamTaskID)
	}
	var unknownEvent *VideoTaskEvent
	for _, event := range repo.events {
		if event != nil && event.EventType == "dispatch_unknown" {
			unknownEvent = event
			break
		}
	}
	if unknownEvent == nil {
		t.Fatal("expected dispatch_unknown event after accept persist failure")
	}
	if got, _ := unknownEvent.Payload["known_upstream_task_id"].(string); got != "upstream-9301" {
		t.Fatalf("dispatch_unknown payload known_upstream_task_id = %#v, want upstream-9301", unknownEvent.Payload["known_upstream_task_id"])
	}
}

func TestVideoGatewayWorkerDispatchCASConflictDoesNotOverwriteOtherWorker(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := seedPendingDispatchTask(repo, providerID, 9401)
	other := cloneVideoTask(task)
	other.Status = VideoStatusSubmitted
	other.DispatchState = "accepted"
	other.UpstreamTaskID = "accepted-by-other"
	other.Version = 2
	repo.tasks[task.ID] = other
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	adapter := &recordingDispatchAdapter{result: &VideoAdapterResult{UpstreamTaskID: "must-not-send", Status: VideoStatusSubmitted}}

	if err := svc.submitTask(context.Background(), adapter, repo.providers[providerID], task); err != nil {
		t.Fatalf("stale submit: %v", err)
	}
	stored := repo.tasks[task.ID]
	if adapter.calls() != 0 || stored.UpstreamTaskID != "accepted-by-other" || stored.Version != 2 {
		t.Fatalf("CAS conflict overwrote task: calls=%d upstream=%q version=%d", adapter.calls(), stored.UpstreamTaskID, stored.Version)
	}
}

func TestVideoGatewayWorkerDispatchCASFalseOrErrorDoesNotPolluteCaller(t *testing.T) {
	t.Run("accepted conflict keeps dispatching caller", func(t *testing.T) {
		repo := newMemoryVideoGatewayRepo()
		providerID := repo.seedMockProvider()
		task := seedPendingDispatchTask(repo, providerID, 9451)
		repo.acceptConflict = true
		svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
		adapter := &recordingDispatchAdapter{result: &VideoAdapterResult{UpstreamTaskID: "not-committed", Status: VideoStatusSubmitted}}

		if err := svc.submitTask(context.Background(), adapter, repo.providers[providerID], task); err != nil {
			t.Fatalf("submit with accepted conflict: %v", err)
		}
		if task.Status != VideoStatusSubmitted || task.DispatchState != VideoDispatchStateDispatching || task.Version != 2 || task.UpstreamTaskID != "" {
			t.Fatalf("accepted conflict polluted caller: %#v", task)
		}
	})

	for _, tc := range []struct {
		name      string
		configure func(*memoryVideoGatewayRepo)
	}{
		{
			name: "unknown conflict keeps dispatching caller",
			configure: func(repo *memoryVideoGatewayRepo) {
				repo.unknownConflict = true
			},
		},
		{
			name: "unknown error keeps dispatching caller",
			configure: func(repo *memoryVideoGatewayRepo) {
				repo.unknownErr = errors.New("unknown CAS unavailable")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMemoryVideoGatewayRepo()
			providerID := repo.seedMockProvider()
			task := seedPendingDispatchTask(repo, providerID, 9461)
			tc.configure(repo)
			svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
			adapter := &recordingDispatchAdapter{err: &VideoDispatchTransportError{
				Err:                    errors.New("post-write timeout"),
				RequestMayHaveBeenSent: true,
			}}

			err := svc.submitTask(context.Background(), adapter, repo.providers[providerID], task)
			if !errors.Is(err, ErrVideoDispatchUnknown) {
				t.Fatalf("ambiguous submit error=%v", err)
			}
			if task.Status != VideoStatusSubmitted || task.DispatchState != VideoDispatchStateDispatching || task.Version != 2 || task.ErrorMessage != "" {
				t.Fatalf("unknown CAS failure polluted caller: %#v", task)
			}
		})
	}
}

func TestVideoGatewayWorkerDispatchWrappedSeedanceHTTPErrorIsUnknownAndSafe(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := seedPendingDispatchTask(repo, providerID, 9501)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	adapter := &recordingDispatchAdapter{err: &VideoDispatchTransportError{
		Err:                    infraerrorsUnavailable("SEEDANCE_CREATE_HTTP_ERROR", "POST https://secret.example/path?token=must-not-leak failed"),
		RequestMayHaveBeenSent: true,
	}}

	err := svc.submitTask(context.Background(), adapter, repo.providers[providerID], task)
	if err == nil || !errors.Is(err, ErrVideoDispatchUnknown) {
		t.Fatalf("wrapped network error = %v, want dispatch unknown", err)
	}
	if strings.Contains(err.Error(), "must-not-leak") || strings.Contains(err.Error(), "secret.example") {
		t.Fatalf("dispatch unknown error leaked provider detail: %v", err)
	}
	stored := repo.tasks[task.ID]
	if stored.DispatchState != VideoDispatchStateUnknown || adapter.calls() != 1 {
		t.Fatalf("wrapped error state=%q calls=%d", stored.DispatchState, adapter.calls())
	}
}

func TestVideoGatewayWorkerDispatchPreSendTransportFailureFailsOnce(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := seedPendingDispatchTask(repo, providerID, 9551)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	const secretTransportDetail = "dial https://user:password@dispatch.internal/create?token=must-not-persist failed"
	adapter := &recordingDispatchAdapter{err: &VideoDispatchTransportError{
		Err:                    infraerrorsUnavailable("SEEDANCE_CREATE_HTTP_ERROR", secretTransportDetail),
		RequestMayHaveBeenSent: false,
	}}
	svc.adapters[VideoProviderMock] = adapter

	if err := svc.processTask(context.Background(), task, time.Hour); err != nil {
		t.Fatalf("process explicit transport failure: %v", err)
	}
	stored := repo.tasks[task.ID]
	if stored.Status != VideoStatusFailed || stored.DispatchState == VideoDispatchStateUnknown {
		t.Fatalf("pre-send failure state=%q dispatch=%q", stored.Status, stored.DispatchState)
	}
	for _, forbidden := range []string{"user:password", "dispatch.internal", "token=", "must-not-persist", secretTransportDetail} {
		if strings.Contains(stored.ErrorMessage, forbidden) {
			t.Fatalf("persisted pre-send error leaked %q: %s", forbidden, stored.ErrorMessage)
		}
	}
	if err := svc.processTask(context.Background(), task, time.Hour); err != nil {
		t.Fatalf("reprocess terminal failure: %v", err)
	}
	if adapter.calls() != 1 {
		t.Fatalf("pre-send failure create calls=%d, want 1", adapter.calls())
	}
}

func TestVideoGatewayWorkerDispatchingWithoutUpstreamBecomesUnknownWithoutPoll(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{
		ID:                9601,
		ProviderAccountID: providerID,
		Provider:          VideoProviderMock,
		Status:            VideoStatusSubmitted,
		DispatchState:     VideoDispatchStateDispatching,
		Version:           2,
		CreatedAt:         time.Now().UTC(),
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	adapter := &recordingDispatchAdapter{}
	svc.adapters[VideoProviderMock] = adapter

	err := svc.processTask(context.Background(), task, time.Hour)
	if err == nil || !errors.Is(err, ErrVideoDispatchUnknown) {
		t.Fatalf("crash recovery error = %v, want dispatch unknown", err)
	}
	stored := repo.tasks[task.ID]
	if stored.DispatchState != VideoDispatchStateUnknown || adapter.calls() != 0 || adapter.polls() != 0 {
		t.Fatalf("crash recovery state=%q creates=%d polls=%d", stored.DispatchState, adapter.calls(), adapter.polls())
	}
}

func TestVideoGatewayMockWorkerSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	success, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "render a quiet enterprise dashboard demo",
		CreatedBy:         7,
	})
	if err != nil {
		t.Fatalf("create success task: %v", err)
	}
	for range 3 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process success task: %v", err)
		}
	}
	success, _, err = svc.GetTask(ctx, success.ID, 7, false)
	if err != nil {
		t.Fatalf("get success task: %v", err)
	}
	if success.Status != VideoStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", success.Status)
	}
	if !strings.Contains(success.ResultURL, "/api/v1/video/mock-assets/") {
		t.Fatalf("expected mock result url, got %q", success.ResultURL)
	}

	failed, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "mock:fail validation path",
		CreatedBy:         7,
	})
	if err != nil {
		t.Fatalf("create failure task: %v", err)
	}
	for range 3 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process failure task: %v", err)
		}
	}
	failed, events, err := svc.GetTask(ctx, failed.ID, 7, false)
	if err != nil {
		t.Fatalf("get failure task: %v", err)
	}
	if failed.Status != VideoStatusFailed {
		t.Fatalf("expected failed, got %s", failed.Status)
	}
	if failed.ErrorMessage == "" {
		t.Fatal("expected failure error message")
	}
	if len(events) < 4 {
		t.Fatalf("expected queued/submitted/running/failed events, got %d", len(events))
	}
	if len(repo.usage) != 2 {
		t.Fatalf("expected usage logs for terminal tasks, got %d", len(repo.usage))
	}
}

func TestVideoGatewayWorkerFlagOnTerminalCallsOnlyFinalizer(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.nextProviderID
	repo.nextProviderID++
	repo.providers[providerID] = &VideoProviderAccount{
		ID:          providerID,
		Provider:    VideoProviderSeedance,
		DisplayName: "offline finalization provider",
		Enabled:     true,
	}
	tokens := int64(1234)
	duration := 12
	task := &VideoTask{
		ID:                9101,
		ProviderAccountID: providerID,
		Provider:          VideoProviderSeedance,
		Model:             "offline-model",
		TaskType:          VideoTaskTypeTextToVideo,
		Status:            VideoStatusSubmitted,
		DispatchState:     VideoDispatchStateAccepted,
		Version:           7,
		CreatedBy:         81,
		CreatedAt:         time.Now().UTC(),
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	original, err := NewMoney("9", Currency("CNY"))
	if err != nil {
		t.Fatalf("original money: %v", err)
	}
	pricing := &recordingWorkerVideoPricing{
		amountUSD: MustUSD("1.25"),
		snapshot: PricingSnapshot{
			AmountOriginal: original,
			ExchangeRate:   "7.2000000000",
			PricingSource:  PricingSourceProviderUsage,
			PricingVersion: "worker-finalization-v1",
		},
	}
	cfg := &config.Config{
		VideoGateway:    config.VideoGatewayConfig{WorkerEnabled: true},
		ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true},
	}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.SetVideoTaskPricing(pricing)
	svc.adapters[VideoProviderSeedance] = &billingSeedanceAdapter{result: &VideoAdapterResult{
		Status:           VideoStatusSucceeded,
		ResultURL:        "https://provider.invalid/result.mp4",
		UsageTotalTokens: &tokens,
		ActualDuration:   &duration,
		ActualResolution: "1080p",
		Payload:          map[string]any{"request_id": "offline-worker"},
	}}
	finalizationRepo := &recordingWorkerFinalizationRepository{
		results: []VideoTaskFinalizationResult{{Applied: true, Status: VideoStatusSucceeded, Version: 8}},
	}
	worker := NewVideoGatewayWorker(svc, cfg, NewVideoTaskFinalizer(finalizationRepo))

	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	calls := finalizationRepo.recordedCalls()
	if len(calls) != 1 {
		t.Fatalf("finalization calls = %d, want 1", len(calls))
	}
	if calls[0].TaskID != task.ID || calls[0].ExpectedVersion != 7 || calls[0].TerminalStatus != VideoStatusSucceeded {
		t.Fatalf("finalization input = %#v", calls[0])
	}
	if calls[0].ActualCostUSD.String() != "1.2500000000" || calls[0].ActualTokens == nil || *calls[0].ActualTokens != tokens {
		t.Fatalf("finalization financial/usage input = %#v", calls[0])
	}
	stored := repo.tasks[task.ID]
	if stored.Status != VideoStatusSubmitted || stored.Version != 7 {
		t.Fatalf("legacy task path mutated storage: %#v", stored)
	}
	if len(repo.events) != 0 || len(repo.usage) != 0 || len(repo.balanceClaims) != 0 {
		t.Fatalf("legacy side effects events/usage/claims = %d/%d/%d", len(repo.events), len(repo.usage), len(repo.balanceClaims))
	}
}

func TestVideoGatewayWorkerBillableFakeAdapterUsesFixedPriceWithoutNetwork(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.nextProviderID
	repo.nextProviderID++
	repo.providers[providerID] = &VideoProviderAccount{ID: providerID, Provider: VideoProviderSeedance, DisplayName: "billable fake", Enabled: true}
	reservationID := int64(4401)
	task := &VideoTask{ID: 4400, ProviderAccountID: providerID, Provider: VideoProviderSeedance, Model: "billable-fake-v1", TaskType: VideoTaskTypeTextToVideo, Status: VideoStatusRunning, DispatchState: VideoDispatchStateAccepted, SettlementStatus: VideoSettlementStatusPending, ReservationID: &reservationID, Version: 4, CreatedBy: 44, CreatedAt: time.Now().UTC()}
	repo.tasks[task.ID] = cloneVideoTask(task)
	original, err := NewMoney("9.0000000000", Currency("CNY"))
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	pricing := &recordingWorkerVideoPricing{amountUSD: MustUSD("1.2500000000"), snapshot: PricingSnapshot{AmountOriginal: original, ExchangeRate: "7.2000000000", PricingSource: "billable_fake", PricingVersion: "fixed-v1"}}
	adapter := &billableFakeAdapter{result: &VideoAdapterResult{Status: VideoStatusSucceeded, ResultURL: "local://billable-fake/result.mp4"}}
	finalizationRepo := &recordingWorkerFinalizationRepository{results: []VideoTaskFinalizationResult{{Applied: true, Status: VideoStatusSucceeded, Version: 5, SettlementStatus: BillingReservationStatusSettled, ArchiveStatus: VideoSideEffectStatusPending, CaptureStatus: VideoSideEffectStatusPending}}}
	cfg := &config.Config{VideoGateway: config.VideoGatewayConfig{WorkerEnabled: true}, ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true}}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.SetVideoTaskPricing(pricing)
	svc.adapters[VideoProviderSeedance] = adapter

	if err := NewVideoGatewayWorker(svc, cfg, NewVideoTaskFinalizer(finalizationRepo)).ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	calls := finalizationRepo.recordedCalls()
	if len(calls) != 1 {
		t.Fatalf("finalization calls = %d, want 1", len(calls))
	}
	if calls[0].ActualCostUSD.String() != "1.2500000000" || calls[0].PricingSnapshot.PricingSource != "billable_fake" || calls[0].PricingSnapshot.PricingVersion != "fixed-v1" {
		t.Fatalf("fixed billable price was not preserved: %#v", calls[0])
	}
	if adapter.pollCalls != 1 || adapter.networkCalls != 0 {
		t.Fatalf("billable fake poll/network calls = %d/%d, want 1/0", adapter.pollCalls, adapter.networkCalls)
	}
	if pricing.actualCalls != 1 {
		t.Fatalf("fixed pricing calls = %d, want 1", pricing.actualCalls)
	}
}

func TestVideoGatewayWorkerFlagOnNonterminalPollUsesAtomicCASAndAuthoritativeState(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	claimAt := time.Now().UTC()
	claimUntil := claimAt.Add(time.Minute)
	task := &VideoTask{
		ID:                 9109,
		ProviderAccountID:  providerID,
		Provider:           VideoProviderMock,
		Model:              "mock-video-v1",
		Status:             VideoStatusRunning,
		Version:            5,
		PollCount:          2,
		WorkerClaimedAt:    &claimAt,
		WorkerClaimedUntil: &claimUntil,
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	tokens := int64(321)
	duration := 8
	adapter := &fakeVideoAdapter{pollsUntilSucceed: 1, pollResult: &VideoAdapterResult{
		Status:           VideoStatusRunning,
		ResultURL:        "mock://poll-authoritative",
		UsageTotalTokens: &tokens,
		ActualResolution: "1080p",
		ActualDuration:   &duration,
		LastFrameURL:     "mock://poll-last-frame",
		Payload:          map[string]any{"request_id": "poll-cas"},
	}}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, &config.Config{ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true}})

	if err := svc.pollTaskWithFinalizer(ctx, adapter, repo.providers[providerID], task, nil); err != nil {
		t.Fatalf("pollTaskWithFinalizer: %v", err)
	}
	if repo.pollCASCalls != 1 || repo.updateTaskCalls != 0 || repo.addEventCalls != 0 {
		t.Fatalf("poll/legacy calls = %d/%d/%d, want 1/0/0", repo.pollCASCalls, repo.updateTaskCalls, repo.addEventCalls)
	}
	if task.Status != VideoStatusRunning || task.Version != 6 || task.PollCount != 3 {
		t.Fatalf("authoritative poll state = %q/%d/%d", task.Status, task.Version, task.PollCount)
	}
	if task.ResultURL != "mock://poll-authoritative" || task.UsageTotalTokens == nil || *task.UsageTotalTokens != tokens {
		t.Fatalf("authoritative poll details = %#v", task)
	}
	if task.WorkerClaimedAt != nil || task.WorkerClaimedUntil != nil {
		t.Fatalf("poll commit did not clear worker claim: %#v/%#v", task.WorkerClaimedAt, task.WorkerClaimedUntil)
	}
	if len(repo.events) != 1 || repo.events[0].EventType != VideoStatusRunning {
		t.Fatalf("poll events = %#v", repo.events)
	}
}

func TestVideoGatewayWorkerFlagOnPollPersistenceFailureRetriesWithoutTerminalization(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	reservationID := int64(77)
	task := &VideoTask{
		ID:                9112,
		ProviderAccountID: providerID,
		Provider:          VideoProviderMock,
		Model:             "mock-video-v1",
		Status:            VideoStatusRunning,
		DispatchState:     VideoDispatchStateNotRequired,
		Version:           5,
		PollCount:         2,
		SettlementStatus:  VideoSettlementStatusPending,
		ReservationID:     &reservationID,
		CreatedBy:         86,
		CreatedAt:         time.Now().UTC(),
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	repo.pollCASErr = errors.New("injected poll persistence failure")
	cfg := &config.Config{
		VideoGateway:    config.VideoGatewayConfig{WorkerEnabled: true},
		ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true},
	}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.adapters[VideoProviderMock] = &fakeVideoAdapter{}
	finalizationRepo := &recordingWorkerFinalizationRepository{}
	worker := NewVideoGatewayWorker(svc, cfg, NewVideoTaskFinalizer(finalizationRepo))

	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if repo.pollCASCalls != 1 {
		t.Fatalf("poll CAS calls = %d, want 1", repo.pollCASCalls)
	}
	if calls := finalizationRepo.recordedCalls(); len(calls) != 0 {
		t.Fatalf("poll persistence failure produced terminal finalization calls: %#v", calls)
	}
	stored := repo.tasks[task.ID]
	if stored.Status != VideoStatusRunning || stored.Version != 5 || stored.PollCount != 2 {
		t.Fatalf("poll persistence failure mutated task: %#v", stored)
	}
	if stored.SettlementStatus != VideoSettlementStatusPending || stored.ReservationID == nil || *stored.ReservationID != reservationID {
		t.Fatalf("poll persistence failure mutated reservation: %#v", stored)
	}
	if repo.updateTaskCalls != 0 || repo.addEventCalls != 0 || len(repo.events) != 0 || len(repo.usage) != 0 || len(repo.balanceClaims) != 0 {
		t.Fatalf("poll persistence failure produced legacy side effects: updates=%d add_events=%d events=%d usage=%d balance_claims=%d", repo.updateTaskCalls, repo.addEventCalls, len(repo.events), len(repo.usage), len(repo.balanceClaims))
	}

	repo.pollCASErr = nil
	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("retry ProcessOnce: %v", err)
	}
	if repo.pollCASCalls != 2 {
		t.Fatalf("poll CAS calls after retry = %d, want 2", repo.pollCASCalls)
	}
	if calls := finalizationRepo.recordedCalls(); len(calls) != 0 {
		t.Fatalf("poll persistence retry produced terminal finalization calls: %#v", calls)
	}
	stored = repo.tasks[task.ID]
	if stored.Status != VideoStatusRunning || stored.Version != 6 || stored.PollCount != 3 {
		t.Fatalf("poll persistence retry state = %#v", stored)
	}
	if stored.SettlementStatus != VideoSettlementStatusPending || stored.ReservationID == nil || *stored.ReservationID != reservationID {
		t.Fatalf("poll persistence retry mutated reservation: %#v", stored)
	}
	if repo.updateTaskCalls != 0 || repo.addEventCalls != 0 || len(repo.events) != 1 || len(repo.usage) != 0 || len(repo.balanceClaims) != 0 {
		t.Fatalf("poll persistence retry side effects: updates=%d add_events=%d events=%d usage=%d balance_claims=%d", repo.updateTaskCalls, repo.addEventCalls, len(repo.events), len(repo.usage), len(repo.balanceClaims))
	}
}

func TestVideoGatewayWorkerFlagOnProviderPollErrorStillTerminalizes(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{
		ID:                9113,
		ProviderAccountID: providerID,
		Provider:          VideoProviderMock,
		Model:             "mock-video-v1",
		Status:            VideoStatusRunning,
		DispatchState:     VideoDispatchStateNotRequired,
		Version:           5,
		PollCount:         2,
		CreatedBy:         87,
		CreatedAt:         time.Now().UTC(),
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	cfg := &config.Config{
		VideoGateway:    config.VideoGatewayConfig{WorkerEnabled: true},
		ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true},
	}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.adapters[VideoProviderMock] = &recordingDispatchAdapter{pollErr: errors.New("injected provider poll failure")}
	finalizationRepo := &recordingWorkerFinalizationRepository{
		results: []VideoTaskFinalizationResult{{Applied: true, Status: VideoStatusFailed, Version: 6}},
	}
	worker := NewVideoGatewayWorker(svc, cfg, NewVideoTaskFinalizer(finalizationRepo))

	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	calls := finalizationRepo.recordedCalls()
	if len(calls) != 1 {
		t.Fatalf("provider poll failure finalization calls = %d, want 1", len(calls))
	}
	if calls[0].TerminalStatus != VideoStatusFailed || calls[0].ExpectedVersion != 5 || !strings.Contains(calls[0].ProviderErrorMessage, "injected provider poll failure") {
		t.Fatalf("provider poll failure finalization input = %#v", calls[0])
	}
	if repo.pollCASCalls != 0 || repo.updateTaskCalls != 0 || repo.addEventCalls != 0 || len(repo.events) != 0 || len(repo.usage) != 0 || len(repo.balanceClaims) != 0 {
		t.Fatalf("provider poll failure used non-finalizer writes: poll_cas=%d updates=%d add_events=%d events=%d usage=%d balance_claims=%d", repo.pollCASCalls, repo.updateTaskCalls, repo.addEventCalls, len(repo.events), len(repo.usage), len(repo.balanceClaims))
	}
}

func TestVideoGatewayWorkerFlagOffNonterminalPollKeepsLegacyWrites(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{ID: 9111, ProviderAccountID: providerID, Provider: VideoProviderMock, Model: "mock-video-v1", Status: VideoStatusRunning, Version: 4}
	repo.tasks[task.ID] = cloneVideoTask(task)
	adapter := &fakeVideoAdapter{pollsUntilSucceed: 1, pollResult: &VideoAdapterResult{Status: VideoStatusRunning}}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, &config.Config{})

	if err := svc.pollTaskWithFinalizer(ctx, adapter, repo.providers[providerID], task, nil); err != nil {
		t.Fatalf("pollTaskWithFinalizer: %v", err)
	}
	if repo.pollCASCalls != 0 || repo.updateTaskCalls != 1 || repo.addEventCalls != 1 {
		t.Fatalf("poll/legacy calls = %d/%d/%d, want 0/1/1", repo.pollCASCalls, repo.updateTaskCalls, repo.addEventCalls)
	}
}

func TestVideoGatewayWorkerIdempotentFinalizationUsesAuthoritativeTaskState(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	claimAt := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)
	claimUntil := claimAt.Add(time.Minute)
	task := &VideoTask{
		ID:                 9110,
		ProviderAccountID:  providerID,
		Provider:           VideoProviderMock,
		Model:              "mock-video-v1",
		Status:             VideoStatusRunning,
		Version:            12,
		CreatedBy:          86,
		CreatedAt:          time.Now().UTC(),
		SettlementStatus:   VideoSettlementStatusPending,
		ArchiveStatus:      VideoSideEffectStatusPending,
		CaptureStatus:      VideoSideEffectStatusPending,
		WorkerClaimedAt:    &claimAt,
		WorkerClaimedUntil: &claimUntil,
	}
	completedAt := time.Date(2026, 7, 10, 2, 0, 0, 0, time.UTC)
	chargedAt := completedAt.Add(-time.Second)
	authoritativeTokens := int64(555)
	authoritativeDuration := 44
	finalizationRepo := &recordingWorkerFinalizationRepository{results: []VideoTaskFinalizationResult{{
		Idempotent:         true,
		Status:             VideoStatusSucceeded,
		Version:            13,
		SettlementStatus:   VideoSettlementStatusNotNeeded,
		BalanceChargedAt:   &chargedAt,
		WorkerClaimedAt:    nil,
		WorkerClaimedUntil: nil,
		ArchiveStatus:      VideoSideEffectStatusNotNeeded,
		CaptureStatus:      "succeeded",
		CompletedAt:        &completedAt,
		ResultURL:          "mock://db-authoritative",
		ErrorMessage:       "db-authoritative-message",
		PollCount:          44,
		UsageTotalTokens:   &authoritativeTokens,
		ActualResolution:   "4k",
		ActualDuration:     &authoritativeDuration,
		LastFrameURL:       "mock://db-last-frame",
	}}}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, &config.Config{ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true}})
	adapter := &fakeVideoAdapter{pollsUntilSucceed: 1, pollResult: &VideoAdapterResult{Status: VideoStatusSucceeded, ResultURL: "mock://authoritative"}}

	if err := svc.pollTaskWithFinalizer(ctx, adapter, repo.providers[providerID], task, NewVideoTaskFinalizer(finalizationRepo)); err != nil {
		t.Fatalf("pollTaskWithFinalizer: %v", err)
	}
	if task.Status != VideoStatusSucceeded || task.Version != 13 || task.SettlementStatus != VideoSettlementStatusNotNeeded {
		t.Fatalf("authoritative status/version/settlement = %q/%d/%q", task.Status, task.Version, task.SettlementStatus)
	}
	if task.BalanceChargedAt == nil || !task.BalanceChargedAt.Equal(chargedAt) {
		t.Fatalf("balance charged at = %#v", task.BalanceChargedAt)
	}
	if task.WorkerClaimedAt != nil || task.WorkerClaimedUntil != nil {
		t.Fatalf("worker claim was not cleared: %#v/%#v", task.WorkerClaimedAt, task.WorkerClaimedUntil)
	}
	if task.ArchiveStatus != VideoSideEffectStatusNotNeeded || task.CaptureStatus != "succeeded" {
		t.Fatalf("authoritative side effects = %q/%q", task.ArchiveStatus, task.CaptureStatus)
	}
	if task.CompletedAt == nil || !task.CompletedAt.Equal(completedAt) {
		t.Fatalf("completed at = %#v", task.CompletedAt)
	}
	if task.ResultURL != "mock://db-authoritative" || task.ErrorMessage != "db-authoritative-message" || task.PollCount != 44 {
		t.Fatalf("authoritative provider fields = %#v", task)
	}
	if task.UsageTotalTokens == nil || *task.UsageTotalTokens != authoritativeTokens || task.ActualResolution != "4k" || task.ActualDuration == nil || *task.ActualDuration != authoritativeDuration || task.LastFrameURL != "mock://db-last-frame" {
		t.Fatalf("authoritative usage fields = %#v", task)
	}
}

func TestVideoGatewayWorkerFlagOnFinalizationFailureRetriesWithoutTerminalPollution(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{
		ID:                9102,
		ProviderAccountID: providerID,
		Provider:          VideoProviderMock,
		Model:             "mock-video-v1",
		TaskType:          VideoTaskTypeTextToVideo,
		Status:            VideoStatusRunning,
		DispatchState:     VideoDispatchStateNotRequired,
		Version:           5,
		CreatedBy:         82,
		CreatedAt:         time.Now().UTC(),
	}
	repo.tasks[task.ID] = cloneVideoTask(task)
	cfg := &config.Config{
		VideoGateway:    config.VideoGatewayConfig{WorkerEnabled: true},
		ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true},
	}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.adapters[VideoProviderMock] = &fakeVideoAdapter{
		pollsUntilSucceed: 1,
		pollResult:        &VideoAdapterResult{Status: VideoStatusSucceeded, ResultURL: "mock://result"},
	}
	finalizationRepo := &recordingWorkerFinalizationRepository{
		results: []VideoTaskFinalizationResult{{}, {Applied: true, Status: VideoStatusSucceeded, Version: 6}},
		errs:    []error{errors.New("injected finalization failure"), nil},
	}
	worker := NewVideoGatewayWorker(svc, cfg, NewVideoTaskFinalizer(finalizationRepo))

	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("first ProcessOnce: %v", err)
	}
	if stored := repo.tasks[task.ID]; stored.Status != VideoStatusRunning || stored.Version != 5 {
		t.Fatalf("failed finalization polluted terminal state: %#v", stored)
	}
	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("retry ProcessOnce: %v", err)
	}
	calls := finalizationRepo.recordedCalls()
	if len(calls) != 2 || calls[0].ExpectedVersion != 5 || calls[1].ExpectedVersion != 5 {
		t.Fatalf("retry finalization calls = %#v", calls)
	}
	if len(repo.events) != 0 || len(repo.usage) != 0 || len(repo.balanceClaims) != 0 {
		t.Fatalf("failed/retried finalization used legacy side effects")
	}
}

func TestVideoGatewayWorkerFlagOnPricingFailureDoesNotRewriteProviderSuccessAsFailed(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.nextProviderID
	repo.nextProviderID++
	repo.providers[providerID] = &VideoProviderAccount{ID: providerID, Provider: VideoProviderSeedance, DisplayName: "offline pricing failure", Enabled: true}
	task := &VideoTask{ID: 9105, ProviderAccountID: providerID, Provider: VideoProviderSeedance, Model: "offline-model", Status: VideoStatusRunning, DispatchState: VideoDispatchStateAccepted, Version: 9, CreatedBy: 85, CreatedAt: time.Now().UTC()}
	repo.tasks[task.ID] = cloneVideoTask(task)
	cfg := &config.Config{VideoGateway: config.VideoGatewayConfig{WorkerEnabled: true}, ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true}}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.SetVideoTaskPricing(&recordingWorkerVideoPricing{actualErr: errors.New("pricing snapshot unavailable")})
	svc.adapters[VideoProviderSeedance] = &billingSeedanceAdapter{result: &VideoAdapterResult{Status: VideoStatusSucceeded, ResultURL: "https://provider.invalid/result.mp4"}}
	finalizationRepo := &recordingWorkerFinalizationRepository{}
	worker := NewVideoGatewayWorker(svc, cfg, NewVideoTaskFinalizer(finalizationRepo))

	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if len(finalizationRepo.recordedCalls()) != 0 {
		t.Fatalf("pricing failure produced terminal finalization calls: %#v", finalizationRepo.recordedCalls())
	}
	if stored := repo.tasks[task.ID]; stored.Status != VideoStatusRunning || stored.Version != 9 {
		t.Fatalf("pricing failure rewrote provider success: %#v", stored)
	}
	if len(repo.events) != 0 || len(repo.usage) != 0 || len(repo.balanceClaims) != 0 {
		t.Fatal("pricing failure produced legacy terminal side effects")
	}
}

func TestVideoGatewayWorkerFlagOnFailedTerminalUsesFinalizerWithoutCharge(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{ID: 9103, ProviderAccountID: providerID, Provider: VideoProviderMock, Model: "mock-video-v1", Status: VideoStatusRunning, DispatchState: VideoDispatchStateNotRequired, Version: 3, CreatedBy: 83, CreatedAt: time.Now().UTC()}
	repo.tasks[task.ID] = cloneVideoTask(task)
	cfg := &config.Config{VideoGateway: config.VideoGatewayConfig{WorkerEnabled: true}, ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true}}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.adapters[VideoProviderMock] = &fakeVideoAdapter{pollsUntilSucceed: 1, pollResult: &VideoAdapterResult{Status: VideoStatusFailed, ErrorMessage: "offline failure"}}
	finalizationRepo := &recordingWorkerFinalizationRepository{results: []VideoTaskFinalizationResult{{Applied: true, Status: VideoStatusFailed, Version: 4}}}
	worker := NewVideoGatewayWorker(svc, cfg, NewVideoTaskFinalizer(finalizationRepo))

	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	calls := finalizationRepo.recordedCalls()
	if len(calls) != 1 || calls[0].TerminalStatus != VideoStatusFailed || calls[0].ActualCostUSD.String() != "0.0000000000" {
		t.Fatalf("failed finalization input = %#v", calls)
	}
	if len(repo.balanceClaims) != 0 || len(repo.events) != 0 || len(repo.usage) != 0 {
		t.Fatal("failed flag-on terminal used legacy side effects")
	}
}

func TestVideoGatewayWorkerFlagOffKeepsLegacyTerminalPath(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{ID: 9104, ProviderAccountID: providerID, Provider: VideoProviderMock, Model: "mock-video-v1", Status: VideoStatusRunning, DispatchState: VideoDispatchStateNotRequired, Version: 2, CreatedBy: 84, CreatedAt: time.Now().UTC()}
	repo.tasks[task.ID] = cloneVideoTask(task)
	cfg := &config.Config{VideoGateway: config.VideoGatewayConfig{WorkerEnabled: true}, ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: false}}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.adapters[VideoProviderMock] = &fakeVideoAdapter{pollsUntilSucceed: 1, pollResult: &VideoAdapterResult{Status: VideoStatusFailed, ErrorMessage: "legacy failure"}}
	finalizationRepo := &recordingWorkerFinalizationRepository{}
	worker := NewVideoGatewayWorker(svc, cfg, NewVideoTaskFinalizer(finalizationRepo))

	if err := worker.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if len(finalizationRepo.recordedCalls()) != 0 {
		t.Fatal("flag-off worker called atomic finalizer")
	}
	if stored := repo.tasks[task.ID]; stored.Status != VideoStatusFailed {
		t.Fatalf("legacy task status = %q", stored.Status)
	}
	if len(repo.events) != 1 || len(repo.usage) != 1 {
		t.Fatalf("legacy events/usage = %d/%d, want 1/1", len(repo.events), len(repo.usage))
	}
}

func TestVideoGatewayWorkerPersistsPollResponseDetails(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	tokens := int64(987654)
	actualDuration := 12
	svc.adapters[VideoProviderMock] = &fakeVideoAdapter{
		pollsUntilSucceed: 1,
		pollResult: &VideoAdapterResult{
			Status:           VideoStatusSucceeded,
			ResultURL:        "https://mock.sub2api.local/video/fake.mp4",
			UsageTotalTokens: &tokens,
			ActualResolution: "1080p",
			ActualDuration:   &actualDuration,
			LastFrameURL:     "https://mock.sub2api.local/video/last.png",
		},
	}

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "persist poll response details",
		CreatedBy:         7,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	for range 2 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process task: %v", err)
		}
	}

	task, _, err = svc.GetTask(ctx, task.ID, 7, false)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.UsageTotalTokens == nil || *task.UsageTotalTokens != tokens {
		t.Fatalf("usage_total_tokens = %+v, want %d", task.UsageTotalTokens, tokens)
	}
	if task.ActualResolution != "1080p" {
		t.Fatalf("actual_resolution = %q", task.ActualResolution)
	}
	if task.ActualDuration == nil || *task.ActualDuration != actualDuration {
		t.Fatalf("actual_duration = %+v, want %d", task.ActualDuration, actualDuration)
	}
	if task.LastFrameURL != "https://mock.sub2api.local/video/last.png" {
		t.Fatalf("last_frame_url = %q", task.LastFrameURL)
	}
	if len(repo.usage) != 1 {
		t.Fatalf("expected one usage log, got %d", len(repo.usage))
	}
	if repo.usage[0].UsageTotalTokens == nil || *repo.usage[0].UsageTotalTokens != tokens {
		t.Fatalf("usage log tokens = %+v", repo.usage[0].UsageTotalTokens)
	}
}

func TestVideoGatewayWorkerTerminalUsesMoneyPricingBoundary(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.nextProviderID
	repo.nextProviderID++
	account := &VideoProviderAccount{ID: providerID, Provider: VideoProviderSeedance, DisplayName: "fake seedance", Enabled: true}
	repo.providers[providerID] = cloneVideoProvider(account)
	task := &VideoTask{ID: 8001, ProviderAccountID: providerID, Provider: VideoProviderSeedance, Model: "fake-model", Status: VideoStatusSubmitted, CreatedBy: 8}
	repo.tasks[task.ID] = cloneVideoTask(task)
	original, err := NewMoney("9", Currency("CNY"))
	if err != nil {
		t.Fatalf("original money: %v", err)
	}
	pricing := &recordingWorkerVideoPricing{
		amountUSD: MustUSD("1.25"),
		snapshot: PricingSnapshot{
			AmountOriginal: original,
			ExchangeRate:   "7.2000000000",
			PricingSource:  PricingSourceProviderUsage,
			PricingVersion: "worker-pricing-v1",
		},
	}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	svc.SetVideoTaskPricing(pricing)
	adapter := &fakeVideoAdapter{pollsUntilSucceed: 1, pollResult: &VideoAdapterResult{Status: VideoStatusSucceeded, ResultURL: "https://provider.invalid/result.mp4"}}

	if err := svc.pollTask(ctx, adapter, account, task); err != nil {
		t.Fatalf("poll task: %v", err)
	}
	if pricing.actualCalls != 1 {
		t.Fatalf("ActualPrice calls = %d, want 1", pricing.actualCalls)
	}
	if task.CostEstimate != 9 || task.Currency != BillingCurrencyCNY || task.PricingSource != PricingSourceProviderUsage || task.PricingVersion != "worker-pricing-v1" {
		t.Fatalf("legacy pricing projection = cost=%v currency=%q source=%q version=%q", task.CostEstimate, task.Currency, task.PricingSource, task.PricingVersion)
	}
}

func TestVideoGatewayCapturesSucceededTaskContent(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	contentRepo := &fakeGenContentRepo{}
	cfg := enabledContentCaptureCfg()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.SetGenerationContentCollector(NewGenerationContentCollector(contentRepo, cfg))

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Model:             "mock-video-v1",
		Prompt:            "render launch video for owner@example.test",
		NegativePrompt:    "avoid 13800138000",
		AspectRatio:       "16:9",
		Duration:          5,
		Resolution:        "720p",
		CreatedBy:         7,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	for range 3 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process task: %v", err)
		}
	}

	task, _, err = svc.GetTask(ctx, task.ID, 7, false)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != VideoStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", task.Status)
	}
	if len(contentRepo.rows) != 1 {
		t.Fatalf("expected one content capture row, got %d", len(contentRepo.rows))
	}
	row := contentRepo.rows[0]
	if row.TaskID == nil || *row.TaskID != task.ID {
		t.Fatalf("expected task_id %d, got %+v", task.ID, row.TaskID)
	}
	if row.UserID == nil || *row.UserID != 7 {
		t.Fatalf("expected user_id 7, got %+v", row.UserID)
	}
	if row.AccountID != nil || row.APIKeyID != nil || row.GroupID != nil {
		t.Fatalf("video capture must not write account/api-key/group attribution: %+v", row)
	}
	if strings.Contains(row.PromptRedacted, "owner@example.test") || strings.Contains(row.PromptRedacted, "13800138000") {
		t.Fatalf("video prompt was not redacted: %s", row.PromptRedacted)
	}

	if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
		t.Fatalf("process terminal task again: %v", err)
	}
	if len(contentRepo.rows) != 1 {
		t.Fatalf("terminal task should not duplicate content capture, got %d rows", len(contentRepo.rows))
	}
}

func TestVideoGatewayCaptureDisabledByFlag(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	contentRepo := &fakeGenContentRepo{}
	disabledCfg := &config.Config{}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, disabledCfg)
	svc.SetGenerationContentCollector(NewGenerationContentCollector(contentRepo, disabledCfg))

	if _, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "capture flag disabled path",
		CreatedBy:         7,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	for range 3 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process task: %v", err)
		}
	}
	if len(contentRepo.rows) != 0 {
		t.Fatalf("disabled capture flag should write zero rows, got %d", len(contentRepo.rows))
	}
}

func TestVideoGatewayCaptureFailOpenKeepsSucceededAndUsage(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	contentRepo := &fakeGenContentRepo{err: errors.New("capture db down")}
	cfg := enabledContentCaptureCfg()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	svc.SetGenerationContentCollector(NewGenerationContentCollector(contentRepo, cfg))

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeTextToVideo,
		Prompt:            "capture fail-open path",
		CreatedBy:         7,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	for range 3 {
		if err := svc.ProcessRunnableTasks(ctx, 10, time.Minute); err != nil {
			t.Fatalf("process task: %v", err)
		}
	}
	task, _, err = svc.GetTask(ctx, task.ID, 7, false)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != VideoStatusSucceeded {
		t.Fatalf("capture error must not block task success, got %s", task.Status)
	}
	if len(repo.usage) != 1 {
		t.Fatalf("capture error must not block usage log, got %d", len(repo.usage))
	}
}

func TestVideoProviderKeyNeverReturnedInPlaintext(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	created, err := svc.CreateProviderAccount(ctx, VideoProviderCreateParams{
		Provider:     VideoProviderSeedance,
		DisplayName:  "Seedance Demo",
		Enabled:      false,
		APIKey:       "demo-key-fixture",
		DefaultModel: "seedance-2-0-pro",
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if created.PlainAPIKey != "" {
		t.Fatal("created provider response exposed plaintext key")
	}
	if created.MaskedKey == "" || created.MaskedKey == "demo-key-fixture" {
		t.Fatalf("expected masked key, got %q", created.MaskedKey)
	}

	items, err := svc.ListProviderAccounts(ctx)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one provider, got %d", len(items))
	}
	if items[0].PlainAPIKey != "" {
		t.Fatal("list provider response exposed plaintext key")
	}
	if !items[0].APIKeyConfigured {
		t.Fatal("expected api_key_configured")
	}
}

func TestVideoProviderMaskedPlaceholderDoesNotMarkRealProviderConfigured(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	mockID := repo.seedMockProvider()
	now := time.Now().UTC()
	realProviderID := repo.nextProviderID
	repo.nextProviderID++
	repo.providers[realProviderID] = &VideoProviderAccount{
		ID:           realProviderID,
		Provider:     VideoProviderSeedance,
		DisplayName:  "Seedance Placeholder Only",
		Enabled:      true,
		MaskedKey:    "sdnc***demo",
		DefaultModel: defaultVideoModel(VideoProviderSeedance),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	items, err := svc.ListProviderAccounts(ctx)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	var mockProvider, realProvider *VideoProviderAccount
	for _, item := range items {
		switch item.ID {
		case mockID:
			mockProvider = item
		case realProviderID:
			realProvider = item
		}
	}
	if mockProvider == nil || !mockProvider.APIKeyConfigured || !mockProvider.RouteAvailable {
		t.Fatalf("expected mock provider to remain demo-ready, got %#v", mockProvider)
	}
	if realProvider == nil {
		t.Fatal("expected real provider in list")
	}
	if realProvider.APIKeyConfigured {
		t.Fatalf("masked placeholder should not mark real provider configured: %#v", realProvider)
	}
	if realProvider.KeyStatus != videoKeyStatusMissing || realProvider.RouteAvailable {
		t.Fatalf("expected missing-key real provider to be unavailable, got key_status=%s route_available=%v", realProvider.KeyStatus, realProvider.RouteAvailable)
	}
}

func TestVideoGatewayAutoRouteSkipsUnavailableAccounts(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	availableID := repo.seedMockProvider()
	now := time.Now().UTC()
	unavailable := []*VideoProviderAccount{
		{
			ID:           repo.nextProviderID,
			Provider:     VideoProviderSeedance,
			DisplayName:  "Seedance Missing Key",
			Enabled:      true,
			DefaultModel: defaultVideoModel(VideoProviderSeedance),
			Metadata: map[string]any{
				"key_status":      videoKeyStatusMissing,
				"health_status":   videoHealthStatusNeedsKey,
				"diagnostic_type": "Key 未配置",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:           repo.nextProviderID + 1,
			Provider:     VideoProviderKling,
			DisplayName:  "Kling Disabled",
			Enabled:      false,
			DefaultModel: defaultVideoModel(VideoProviderKling),
			Metadata: map[string]any{
				"key_status":    videoKeyStatusDisabled,
				"health_status": videoHealthStatusDisabled,
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:           repo.nextProviderID + 2,
			Provider:     VideoProviderSeedance,
			DisplayName:  "Seedance Auth Failed",
			Enabled:      true,
			MaskedKey:    "sdnc***demo",
			DefaultModel: defaultVideoModel(VideoProviderSeedance),
			Metadata: map[string]any{
				"key_status":      videoKeyStatusAuthFailed,
				"health_status":   videoHealthStatusAuthFailed,
				"diagnostic_type": "鉴权失败",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:           repo.nextProviderID + 3,
			Provider:     VideoProviderKling,
			DisplayName:  "Kling Rate Limited",
			Enabled:      true,
			MaskedKey:    "klng***demo",
			DefaultModel: defaultVideoModel(VideoProviderKling),
			Metadata: map[string]any{
				"key_status":      videoKeyStatusRateLimited,
				"health_status":   videoHealthStatusRateLimited,
				"diagnostic_type": "触发限流",
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	for _, account := range unavailable {
		repo.providers[account.ID] = cloneVideoProvider(account)
		repo.nextProviderID = account.ID + 1
	}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	var task *VideoTask
	for i := 0; i < 10; i++ {
		var err error
		task, err = svc.CreateTask(ctx, VideoTaskCreateParams{
			TaskType:  VideoTaskTypeTextToVideo,
			Prompt:    "create a short launch video for an enterprise API workflow",
			CreatedBy: 101,
		})
		if err != nil {
			t.Fatalf("auto route create task %d: %v", i+1, err)
		}
	}
	if task.ProviderAccountID != availableID {
		t.Fatalf("expected available provider %d, got %d", availableID, task.ProviderAccountID)
	}
	if len(repo.tasks) != 10 {
		t.Fatalf("expected ten routed tasks, got %d", len(repo.tasks))
	}
	events, err := repo.ListTaskEvents(ctx, task.ID, 20)
	if err != nil {
		t.Fatalf("list task events: %v", err)
	}
	if len(events) == 0 || events[0].EventType != "routed" {
		t.Fatalf("expected first routed event, got %#v", events)
	}
	if events[0].Payload["strategy"] != VideoRouteStrategyLeastInflight {
		t.Fatalf("expected least_inflight strategy, got %#v", events[0].Payload["strategy"])
	}
	skipped, ok := events[0].Payload["skipped_accounts"].([]videoRouteSkip)
	if !ok {
		t.Fatalf("expected skipped account records, got %#v", events[0].Payload["skipped_accounts"])
	}
	if len(skipped) != 4 {
		t.Fatalf("expected four skipped accounts, got %d", len(skipped))
	}
}

func TestDramaGatewayCreatesSkillEventsOnUnifiedAPITask(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	task, err := svc.CreateDramaTask(ctx, DramaTaskCreateParams{
		EmployeeAlias: "E001",
		APIClientID:   "internal_tool_001",
		ProjectID:     "drama_project_demo_001",
		DramaType:     "真人短剧",
		Genre:         "现代都市情感",
		EpisodeNo:     1,
		SceneType:     "情绪爆发",
		ShotRole:      "女主特写反应",
		DramaticGoal:  "表现反转前的情绪冲击",
		ReferenceAssets: []DramaReferenceAsset{{
			AssetType:         "reference_video",
			AssetID:           "ref_demo_001",
			SelectedTimeRange: "00:12-00:15",
		}},
		Prompt: "女主在雨夜街边听到男主离开的消息，缓慢抬头，眼神从震惊转为克制的愤怒。",
		PromptStructure: map[string]any{
			"character_identity": "女主，25岁，现代都市",
			"dramatic_goal":      "反转前情绪冲击",
			"camera":             "slow push-in",
		},
		RequestedEngineCapabilities: DramaEngineCapabilityRequest{
			SupportsRealPerson:    true,
			SupportsImageToVideo:  true,
			SupportsMotionControl: true,
		},
		DurationSeconds: 5,
		AspectRatio:     "9:16",
		CreatedBy:       101,
	})
	if err != nil {
		t.Fatalf("create drama task: %v", err)
	}
	if task.TaskID != "drama_task_1" {
		t.Fatalf("unexpected drama task id: %s", task.TaskID)
	}
	if task.SelectedProvider != "kling_safe_demo" {
		t.Fatalf("expected kling safe demo recommendation, got %s", task.SelectedProvider)
	}
	if !task.SkillEventCreated || task.SkillEventID == "" || task.PromptArtifactID == "" || task.ShotDecisionID == "" {
		t.Fatalf("expected skill/prompt/shot events, got %#v", task)
	}
	if task.EmployeeAlias != "E001" || task.APIClientID != "internal_tool_001" {
		t.Fatalf("expected API-first context to be preserved, got %#v", task)
	}
}

func TestDramaSkillAnalysisExportIsAnonymizedAndDryRun(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	if _, err := svc.CreateDramaTask(ctx, DramaTaskCreateParams{
		EmployeeAlias: "E001",
		APIClientID:   "internal_tool_001",
		ProjectID:     "drama_project_demo_001",
		DramaType:     "AI短剧",
		SceneType:     "结尾钩子",
		ShotRole:      "悬念结尾镜头",
		Prompt:        "主角回头发现门后有第二封信，镜头定格在震惊表情。",
		RequestedEngineCapabilities: DramaEngineCapabilityRequest{
			SupportsGlobalReference: true,
		},
		CreatedBy: 101,
	}); err != nil {
		t.Fatalf("create drama task: %v", err)
	}
	export, err := svc.GenerateDramaSkillAnalysisExport(ctx, DramaSkillAnalysisExportRequest{TargetAIModel: "gpt"})
	if err != nil {
		t.Fatalf("generate export: %v", err)
	}
	if export.SchemaVersion != DramaExportSchemaVersion || !export.Anonymized {
		t.Fatalf("unexpected export metadata: %#v", export)
	}
	records, ok := export.ExportJSON["records"].([]map[string]any)
	if !ok || len(records) != 1 {
		t.Fatalf("expected one anonymized record, got %#v", export.ExportJSON["records"])
	}
	if records[0]["employee_alias"] != "E001" {
		t.Fatalf("expected employee alias only, got %#v", records[0])
	}
	if strings.Contains(fmtAny(export), "CreatedByEmail") || strings.Contains(fmtAny(export), "authorization") {
		t.Fatalf("export should not include identity or authorization details: %#v", export)
	}
	if !strings.Contains(export.AnalysisPrompt, "不调用") && !strings.Contains(export.AnalysisPrompt, "脱敏") {
		t.Fatalf("expected dry-run analysis prompt, got %q", export.AnalysisPrompt)
	}
}

func TestVideoGatewayTaskOwnershipBoundary(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	task, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		TaskType:  VideoTaskTypeTextToVideo,
		Prompt:    "ownership check",
		CreatedBy: 101,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, _, err := svc.GetTask(ctx, task.ID, 202, false); err == nil {
		t.Fatal("expected a different non-admin user to be blocked")
	}
	if _, _, err := svc.GetTask(ctx, task.ID, 202, true); err != nil {
		t.Fatalf("expected admin to read task: %v", err)
	}
}

func TestVideoGatewayTaskListFiltersByEmployeeUnlessAdmin(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	if _, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		TaskType:  VideoTaskTypeTextToVideo,
		Prompt:    "employee one task",
		CreatedBy: 101,
	}); err != nil {
		t.Fatalf("create first task: %v", err)
	}
	if _, err := svc.CreateTask(ctx, VideoTaskCreateParams{
		TaskType:  VideoTaskTypeTextToVideo,
		Prompt:    "employee two task",
		CreatedBy: 202,
	}); err != nil {
		t.Fatalf("create second task: %v", err)
	}

	employeeTasks, total, err := svc.ListTasks(ctx, VideoTaskListParams{CreatedBy: 101, IsAdmin: false})
	if err != nil {
		t.Fatalf("list employee tasks: %v", err)
	}
	if total != 1 || len(employeeTasks) != 1 || employeeTasks[0].CreatedBy != 101 {
		t.Fatalf("expected only employee 101 task, total=%d tasks=%#v", total, employeeTasks)
	}

	adminTasks, total, err := svc.ListTasks(ctx, VideoTaskListParams{CreatedBy: 101, IsAdmin: true})
	if err != nil {
		t.Fatalf("list admin tasks: %v", err)
	}
	if total != 2 || len(adminTasks) != 2 {
		t.Fatalf("expected admin to see both tasks, total=%d tasks=%#v", total, adminTasks)
	}
}

func TestVideoAdapterContractSafeProviderBehavior(t *testing.T) {
	ctx := context.Background()
	registry := NewVideoAdapterRegistry()
	task := &VideoTask{
		ID:          42,
		Model:       "mock-video-v1",
		TaskType:    VideoTaskTypeTextToVideo,
		Prompt:      "safe adapter contract preview",
		AspectRatio: "16:9",
		Duration:    5,
		Resolution:  "720p",
	}

	mock := registry[VideoProviderMock]
	if mock == nil {
		t.Fatal("mock adapter is not registered")
	}
	mockPayload := mock.BuildCreatePayload(&VideoProviderAccount{Provider: VideoProviderMock}, task)
	if containsSecretField(mockPayload) {
		t.Fatalf("mock payload should not expose secret-like fields: %#v", mockPayload)
	}
	result, err := mock.CreateTask(ctx, &VideoProviderAccount{Provider: VideoProviderMock, Enabled: true}, task)
	if err != nil {
		t.Fatalf("mock create task: %v", err)
	}
	if result.Status != VideoStatusSubmitted || result.UpstreamTaskID == "" {
		t.Fatalf("unexpected mock create result: %#v", result)
	}

	for _, provider := range []string{VideoProviderSeedance, VideoProviderKling} {
		adapter := registry[provider]
		if adapter == nil {
			t.Fatalf("%s adapter is not registered", provider)
		}
		preview := adapter.BuildCreatePayload(&VideoProviderAccount{
			Provider:         provider,
			APIKeyConfigured: true,
			PlainAPIKey:      "placeholder-key-should-not-leak",
			BaseURL:          "demo://safe-provider",
		}, task)
		if containsSecretField(preview) || strings.Contains(fmtAny(preview), "placeholder-key-should-not-leak") {
			t.Fatalf("%s payload preview should not expose credentials: %#v", provider, preview)
		}
		if _, err := adapter.CreateTask(ctx, &VideoProviderAccount{Provider: provider, Enabled: true}, task); err == nil || !strings.Contains(err.Error(), "api key is not configured") {
			t.Fatalf("%s without key should be safely disabled, err=%v", provider, err)
		}
		_, err := adapter.CreateTask(ctx, &VideoProviderAccount{
			Provider:         provider,
			Enabled:          true,
			APIKeyConfigured: true,
			PlainAPIKey:      "placeholder-key-should-not-leak",
		}, task)
		if err == nil {
			t.Fatalf("%s with placeholder key should return error", provider)
		}
		errLower := strings.ToLower(err.Error())
		if provider == VideoProviderKling && !strings.Contains(errLower, "disabled") {
			t.Fatalf("%s real call should remain disabled, err=%v", provider, err)
		}
		if provider == VideoProviderSeedance && !strings.Contains(errLower, "smoke") && !strings.Contains(errLower, "authorization") {
			t.Fatalf("%s real call should remain behind the single-smoke gate, err=%v", provider, err)
		}
	}
}

func containsSecretField(payload map[string]any) bool {
	for key, value := range payload {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "api_key") ||
			strings.Contains(lowerKey, "authorization") ||
			strings.Contains(lowerKey, "cookie") ||
			strings.Contains(lowerKey, "token") ||
			strings.Contains(lowerKey, "secret") ||
			strings.Contains(lowerKey, "password") {
			return true
		}
		if nested, ok := value.(map[string]any); ok && containsSecretField(nested) {
			return true
		}
		if nestedSlice, ok := value.([]map[string]any); ok {
			for _, nested := range nestedSlice {
				if containsSecretField(nested) {
					return true
				}
			}
		}
	}
	return false
}

func fmtAny(value any) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(fmt.Sprintf("%#v", value))), "\n", " "), "\t", " "))
}

func cloneVideoProvider(in *VideoProviderAccount) *VideoProviderAccount {
	if in == nil {
		return nil
	}
	out := *in
	if in.Metadata != nil {
		out.Metadata = make(map[string]any, len(in.Metadata))
		for k, v := range in.Metadata {
			out.Metadata[k] = v
		}
	}
	return &out
}

func cloneVideoTask(in *VideoTask) *VideoTask {
	if in == nil {
		return nil
	}
	out := *in
	if in.Content != nil {
		out.Content = append([]VideoTaskContentItem(nil), in.Content...)
	}
	if in.GenerateAudio != nil {
		v := *in.GenerateAudio
		out.GenerateAudio = &v
	}
	if in.Watermark != nil {
		v := *in.Watermark
		out.Watermark = &v
	}
	if in.CameraFixed != nil {
		v := *in.CameraFixed
		out.CameraFixed = &v
	}
	if in.ReturnLastFrame != nil {
		v := *in.ReturnLastFrame
		out.ReturnLastFrame = &v
	}
	if in.UsageTotalTokens != nil {
		v := *in.UsageTotalTokens
		out.UsageTotalTokens = &v
	}
	if in.ActualDuration != nil {
		v := *in.ActualDuration
		out.ActualDuration = &v
	}
	if in.CompletedAt != nil {
		completed := *in.CompletedAt
		out.CompletedAt = &completed
	}
	return &out
}

func cloneVideoEvent(in *VideoTaskEvent) *VideoTaskEvent {
	if in == nil {
		return nil
	}
	out := *in
	if in.Payload != nil {
		out.Payload = make(map[string]any, len(in.Payload))
		for k, v := range in.Payload {
			out.Payload[k] = v
		}
	}
	return &out
}

func seedCancelProvenanceTask(t *testing.T, repo *memoryVideoGatewayRepo, status string, withTrialGate bool) int64 {
	t.Helper()
	providerID := repo.nextProviderID
	repo.nextProviderID++
	repo.providers[providerID] = &VideoProviderAccount{
		ID:           providerID,
		Provider:     VideoProviderSeedance,
		DisplayName:  "Seedance",
		Enabled:      true,
		DefaultModel: "seedance-1-0-pro",
	}
	task := &VideoTask{
		ProviderAccountID: providerID,
		Provider:          VideoProviderSeedance,
		Status:            status,
		CreatedBy:         42,
		Version:           1,
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if withTrialGate {
		if err := repo.AddTaskEvent(context.Background(), &VideoTaskEvent{
			VideoTaskID: task.ID,
			EventType:   "trial_gate",
			Message:     "tiny trial gate passed",
			Payload: map[string]any{
				"trial_mode":  "tiny_real",
				"gate_result": "passed",
			},
		}); err != nil {
			t.Fatalf("AddTaskEvent: %v", err)
		}
	}
	return task.ID
}

func TestCancelAPIKeyTrialTaskPreservesProductionProvenance(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	taskID := seedCancelProvenanceTask(t, repo, VideoStatusRunning, false)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	svc.adapters[VideoProviderSeedance] = &recordingCancelVideoAdapter{
		result: &VideoAdapterResult{Status: VideoStatusCancelled},
	}

	task, events, err := svc.CancelAPIKeyTrialTask(context.Background(), taskID, 42, false)
	if err != nil {
		t.Fatalf("CancelAPIKeyTrialTask: %v", err)
	}
	if task == nil || task.Status != VideoStatusCancelled {
		t.Fatalf("task = %#v, want cancelled", task)
	}
	for _, ev := range events {
		if ev != nil && ev.EventType == "trial_gate" {
			t.Fatalf("production cancel must not invent trial_gate events: %#v", events)
		}
	}
}

func TestCancelAPIKeyTrialTaskPreservesTinyTrialProvenance(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	taskID := seedCancelProvenanceTask(t, repo, VideoStatusRunning, true)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	svc.adapters[VideoProviderSeedance] = &recordingCancelVideoAdapter{
		result: &VideoAdapterResult{Status: VideoStatusCancelled},
	}

	task, events, err := svc.CancelAPIKeyTrialTask(context.Background(), taskID, 42, false)
	if err != nil {
		t.Fatalf("CancelAPIKeyTrialTask: %v", err)
	}
	if task == nil || task.Status != VideoStatusCancelled {
		t.Fatalf("task = %#v, want cancelled", task)
	}
	found := false
	for _, ev := range events {
		if ev != nil && ev.EventType == "trial_gate" {
			found = true
			if mode, _ := ev.Payload["trial_mode"].(string); mode != "tiny_real" {
				t.Fatalf("trial_mode = %v, want tiny_real", ev.Payload["trial_mode"])
			}
		}
	}
	if !found {
		t.Fatalf("tiny trial cancel lost trial_gate provenance: %#v", events)
	}
}

func TestCancelAPIKeyTrialTaskAlreadyCancelledPreservesTinyTrialProvenance(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	taskID := seedCancelProvenanceTask(t, repo, VideoStatusCancelled, true)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	task, events, err := svc.CancelAPIKeyTrialTask(context.Background(), taskID, 42, false)
	if err != nil {
		t.Fatalf("CancelAPIKeyTrialTask: %v", err)
	}
	if task == nil || task.Status != VideoStatusCancelled {
		t.Fatalf("task = %#v, want cancelled", task)
	}
	found := false
	for _, ev := range events {
		if ev != nil && ev.EventType == "trial_gate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("already-cancelled tiny trial lost provenance: %#v", events)
	}
}

func TestCancelAPIKeyTrialTaskProvenanceLookupFailureDoesNotDefaultProduction(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	taskID := seedCancelProvenanceTask(t, repo, VideoStatusRunning, true)
	repo.listEventsErr = errors.New("events unavailable")
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)
	svc.adapters[VideoProviderSeedance] = &recordingCancelVideoAdapter{
		result: &VideoAdapterResult{Status: VideoStatusCancelled},
	}

	task, events, err := svc.CancelAPIKeyTrialTask(context.Background(), taskID, 42, false)
	if err == nil {
		t.Fatal("CancelAPIKeyTrialTask error = nil, want provenance lookup failure")
	}
	if task != nil || events != nil {
		t.Fatalf("on provenance failure must not return task/events that default to production: task=%#v events=%#v", task, events)
	}
}
