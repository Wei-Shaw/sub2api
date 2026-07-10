package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type videoReliabilityMockHarness struct {
	router       *gin.Engine
	repo         *videoReliabilityMockRepo
	worker       *service.VideoGatewayWorker
	outbox       *videoReliabilityOutboxRepo
	outboxWorker *service.DomainOutboxWorker
}

func newVideoReliabilityMockHarness(t *testing.T) *videoReliabilityMockHarness {
	t.Helper()
	base := newAPIKeyVideoGatewayMemoryRepo()
	base.seedProvider(service.VideoProviderMock, true, map[string]any{"priority": 10, "key_status": "normal", "health_status": "healthy"})
	outbox := &videoReliabilityOutboxRepo{}
	repo := &videoReliabilityMockRepo{
		apiKeyVideoGatewayMemoryRepo: base,
		outbox:                       outbox,
		balance:                      service.MustUSD("50.00000000"),
		reservations:                 make(map[int64]*service.BillingReservation),
		transactions:                 make(map[string]*service.BillingTransaction),
	}
	cfg := &config.Config{}
	cfg.Gateway.MaxBodySize = 1 << 20
	cfg.VideoGateway.WorkerEnabled = true
	cfg.ReliabilityCore.VideoEnabled = true
	videoSvc := service.NewVideoGatewayService(repo, apiKeyVideoGatewayNoopEncryptor{}, cfg)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{}, Video: handler.NewVideoHandler(videoSvc)},
		apiKeyVideoGatewayAuthMiddleware(), nil, nil, nil, nil, cfg,
	)
	handler := &videoReliabilityLocalAssetHandler{repo: repo}
	return &videoReliabilityMockHarness{
		router: router,
		repo:   repo,
		worker: service.NewVideoGatewayWorker(videoSvc, cfg),
		outbox: outbox,
		outboxWorker: service.NewDomainOutboxWorker(outbox, service.DomainOutboxHandlerRegistry{
			service.VideoOutboxEventCapture: handler,
			service.VideoOutboxEventArchive: handler,
		}, cfg, service.DomainOutboxWorkerOptions{WorkerID: "mock-proof"}),
	}
}

func (h *videoReliabilityMockHarness) finalizationInput(taskID, expectedVersion int64) service.VideoTaskFinalizationInput {
	zero := service.MustUSD("0")
	return service.VideoTaskFinalizationInput{
		TaskID:          taskID,
		ExpectedVersion: expectedVersion,
		TerminalStatus:  service.VideoStatusSucceeded,
		ActualCostUSD:   zero,
		PricingSnapshot: service.PricingSnapshot{AmountOriginal: zero, ExchangeRate: "1.0000000000", PricingSource: "mock", PricingVersion: "mock-v1"},
		CompletedAt:     time.Now().UTC(),
	}
}

// videoReliabilityMockRepo extends the existing HTTP memory repository with
// only the flag-on poll/finalization contracts. It contains no network client.
type videoReliabilityMockRepo struct {
	*apiKeyVideoGatewayMemoryRepo
	mu                sync.Mutex
	outbox            *videoReliabilityOutboxRepo
	balance           service.Money
	reservations      map[int64]*service.BillingReservation
	transactions      map[string]*service.BillingTransaction
	nextReservationID int64
}

// CreateWithReservation is the actual service entry point for billable
// reliability creation. Mock requests must never reach it: if a future change
// accidentally removes the mock exemption, this repository creates observable
// financial state and the HTTP proof below fails instead of inspecting a
// disconnected harness variable.
func (r *videoReliabilityMockRepo) CreateWithReservation(_ context.Context, input service.VideoTaskCreationInput) (*service.VideoTaskCreationResult, error) {
	if input.Task == nil {
		return nil, errors.New("mock reservation task is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nextReservationID == 0 {
		r.nextReservationID = 1
	}
	reservationID := r.nextReservationID
	r.nextReservationID++
	now := time.Now().UTC()
	task := cloneAPIKeyVideoTask(input.Task)
	task.ID = r.nextTaskID
	r.nextTaskID++
	task.CreatedAt, task.UpdatedAt = now, now
	task.ReservationID = &reservationID
	if provider, ok := r.providers[task.ProviderAccountID]; ok {
		task.ProviderAccountName = provider.DisplayName
	}
	zero := service.MustUSD("0")
	reservation := &service.BillingReservation{
		ID: reservationID, ReservationKey: fmt.Sprintf("mock-proof:%d", task.ID), SourceType: "video_task", SourceID: task.ID,
		UserID: task.CreatedBy, APIKeyID: task.APIKeyID, ReservedAmountUSD: input.ReservedAmountUSD, SettledAmountUSD: zero,
		Status: service.BillingReservationStatusActive, ExpiresAt: input.ReservationExpiresAt, CreatedAt: now, UpdatedAt: now,
	}
	r.tasks[task.ID] = cloneAPIKeyVideoTask(task)
	r.reservations[reservationID] = reservation
	return &service.VideoTaskCreationResult{Task: cloneAPIKeyVideoTask(task), Reservation: reservation}, nil
}

func (r *videoReliabilityMockRepo) financialSnapshot() (service.Money, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.balance, len(r.reservations), len(r.transactions)
}

func (r *videoReliabilityMockRepo) UpdatePolledTaskCAS(_ context.Context, expectedVersion int64, candidate *service.VideoTask, event *service.VideoTaskEvent) (service.VideoTaskPollUpdateResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.tasks[candidate.ID]
	if !ok {
		return service.VideoTaskPollUpdateResult{}, service.ErrVideoTaskNotFound
	}
	applied := false
	if current.Version == expectedVersion && !service.IsTerminalVideoStatus(current.Status) {
		updated := cloneAPIKeyVideoTask(candidate)
		updated.Version = expectedVersion + 1
		updated.UpdatedAt = time.Now().UTC()
		r.tasks[candidate.ID] = updated
		current = updated
		_ = r.addEventLocked(event)
		applied = true
	}
	return videoReliabilityPollResult(current, applied), nil
}

func (r *videoReliabilityMockRepo) FinalizeVideoTask(_ context.Context, input service.VideoTaskFinalizationInput) (service.VideoTaskFinalizationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.tasks[input.TaskID]
	if !ok {
		return service.VideoTaskFinalizationResult{}, service.ErrVideoTaskNotFound
	}
	if service.IsTerminalVideoStatus(current.Status) {
		if current.Status != input.TerminalStatus {
			return service.VideoTaskFinalizationResult{}, &service.VideoTaskTerminalConflictError{TaskID: input.TaskID, RequestedStatus: input.TerminalStatus, CurrentStatus: current.Status, CurrentVersion: current.Version}
		}
		result := videoReliabilityFinalizationResult(current)
		result.Idempotent = true
		return result, nil
	}
	if current.Version != input.ExpectedVersion {
		return service.VideoTaskFinalizationResult{}, fmt.Errorf("version conflict: expected %d, current %d", input.ExpectedVersion, current.Version)
	}
	updated := cloneAPIKeyVideoTask(current)
	updated.Status = input.TerminalStatus
	updated.Version++
	updated.ResultURL = input.ProviderResultURL
	updated.ErrorMessage = input.ProviderErrorMessage
	updated.PollCount = input.PollCount
	updated.UsageTotalTokens = input.ActualTokens
	updated.ActualResolution = input.ActualResolution
	updated.ActualDuration = input.ActualDuration
	updated.LastFrameURL = input.LastFrameURL
	completed := input.CompletedAt.UTC()
	updated.CompletedAt = &completed
	updated.SettlementStatus = service.VideoSettlementStatusNotNeeded
	updated.ArchiveStatus = service.VideoSideEffectStatusPending
	updated.CaptureStatus = service.VideoSideEffectStatusPending
	updated.CostEstimate = 0
	updated.UpdatedAt = completed
	// This branch is unreachable for a correct mock request. It deliberately
	// models the same observable reservation/ledger/balance effects as the
	// billable path so a regression that creates a mock reservation makes the
	// zero-finance assertions below fail against state the finalizer actually
	// touched, rather than against an isolated harness fixture.
	if updated.ReservationID != nil && input.TerminalStatus == service.VideoStatusSucceeded {
		reservation, ok := r.reservations[*updated.ReservationID]
		if !ok {
			return service.VideoTaskFinalizationResult{}, errors.New("mock billable reservation is missing")
		}
		balanceBefore := r.balance
		balanceAfter, err := balanceBefore.Subtract(input.ActualCostUSD)
		if err != nil {
			return service.VideoTaskFinalizationResult{}, err
		}
		reservation.Status = service.BillingReservationStatusSettled
		reservation.SettledAmountUSD = input.ActualCostUSD
		reservation.UpdatedAt, reservation.SettledAt = completed, &completed
		transactionKey := fmt.Sprintf("video_task:%d:charge", updated.ID)
		r.transactions[transactionKey] = &service.BillingTransaction{
			TransactionKey: transactionKey, SourceType: "video_task", SourceID: updated.ID, TransactionKind: "charge",
			UserID: updated.CreatedBy, ReservationID: updated.ReservationID, AmountOriginal: input.PricingSnapshot.AmountOriginal,
			AmountUSD: input.ActualCostUSD, PricingSource: input.PricingSnapshot.PricingSource, PricingVersion: input.PricingSnapshot.PricingVersion,
			BalanceBefore: balanceBefore, BalanceAfter: balanceAfter, CreatedAt: completed,
		}
		r.balance = balanceAfter
	}
	r.tasks[input.TaskID] = updated
	_ = r.addEventLocked(&service.VideoTaskEvent{VideoTaskID: input.TaskID, EventType: input.TerminalStatus, Message: "video task succeeded", Payload: input.ProviderPayload})
	for _, spec := range []struct{ eventType, suffix string }{
		{service.VideoOutboxEventCapture, "capture"},
		{service.VideoOutboxEventArchive, "archive"},
	} {
		payload, _ := json.Marshal(map[string]any{"task_id": input.TaskID})
		_, err := r.outbox.Enqueue(context.Background(), &service.DomainOutboxEvent{AggregateType: "video_task", AggregateID: input.TaskID, EventType: spec.eventType, DedupKey: fmt.Sprintf("video_task:%d:%s", input.TaskID, spec.suffix), Payload: payload, Status: service.DomainOutboxStatusPending, NextAttemptAt: completed})
		if err != nil {
			return service.VideoTaskFinalizationResult{}, err
		}
	}
	result := videoReliabilityFinalizationResult(updated)
	result.Applied = true
	return result, nil
}

func (r *videoReliabilityMockRepo) addEventLocked(event *service.VideoTaskEvent) error {
	if event == nil {
		return nil
	}
	event.ID = r.nextEventID
	r.nextEventID++
	event.CreatedAt = time.Now().UTC()
	r.events[event.VideoTaskID] = append(r.events[event.VideoTaskID], cloneAPIKeyVideoEvent(event))
	return nil
}

func videoReliabilityPollResult(task *service.VideoTask, applied bool) service.VideoTaskPollUpdateResult {
	return service.VideoTaskPollUpdateResult{Applied: applied, Status: task.Status, Version: task.Version, ResultURL: task.ResultURL, ErrorMessage: task.ErrorMessage, CostEstimate: task.CostEstimate, PollCount: task.PollCount, UsageTotalTokens: task.UsageTotalTokens, ActualResolution: task.ActualResolution, ActualDuration: task.ActualDuration, LastFrameURL: task.LastFrameURL, SettlementStatus: task.SettlementStatus, BalanceChargedAt: task.BalanceChargedAt, WorkerClaimedAt: task.WorkerClaimedAt, WorkerClaimedUntil: task.WorkerClaimedUntil, ArchiveStatus: task.ArchiveStatus, CaptureStatus: task.CaptureStatus, CompletedAt: task.CompletedAt}
}

func videoReliabilityFinalizationResult(task *service.VideoTask) service.VideoTaskFinalizationResult {
	return service.VideoTaskFinalizationResult{Status: task.Status, Version: task.Version, SettlementStatus: task.SettlementStatus, BalanceChargedAt: task.BalanceChargedAt, ArchiveStatus: task.ArchiveStatus, CaptureStatus: task.CaptureStatus, CompletedAt: task.CompletedAt, ResultURL: task.ResultURL, ErrorMessage: task.ErrorMessage, CostEstimate: task.CostEstimate, PollCount: task.PollCount, UsageTotalTokens: task.UsageTotalTokens, ActualResolution: task.ActualResolution, ActualDuration: task.ActualDuration, LastFrameURL: task.LastFrameURL}
}

type videoReliabilityOutboxRepo struct {
	mu     sync.Mutex
	events []*service.DomainOutboxEvent
}

func (r *videoReliabilityOutboxRepo) Enqueue(_ context.Context, event *service.DomainOutboxEvent) (*service.DomainOutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.events {
		if existing.DedupKey == event.DedupKey {
			return cloneReliabilityOutboxEvent(existing), nil
		}
	}
	stored := cloneReliabilityOutboxEvent(event)
	stored.ID = int64(len(r.events) + 1)
	stored.CreatedAt = time.Now().UTC()
	r.events = append(r.events, stored)
	return cloneReliabilityOutboxEvent(stored), nil
}

func (r *videoReliabilityOutboxRepo) GetByID(_ context.Context, id int64) (*service.DomainOutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.events {
		if event.ID == id {
			return cloneReliabilityOutboxEvent(event), nil
		}
	}
	return nil, service.ErrDomainOutboxNotFound
}

func (r *videoReliabilityOutboxRepo) ClaimBatch(_ context.Context, workerID string, now time.Time, limit int, lease time.Duration) ([]*service.DomainOutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	claimed := make([]*service.DomainOutboxEvent, 0, limit)
	for _, event := range r.events {
		if event.Status != service.DomainOutboxStatusPending || event.NextAttemptAt.After(now) || len(claimed) >= limit {
			continue
		}
		lockedAt, lockedUntil, owner := now, now.Add(lease), workerID
		event.Status, event.AttemptCount = service.DomainOutboxStatusProcessing, event.AttemptCount+1
		event.LockedAt, event.LockedUntil, event.LockedBy = &lockedAt, &lockedUntil, &owner
		claimed = append(claimed, cloneReliabilityOutboxEvent(event))
	}
	return claimed, nil
}

func (r *videoReliabilityOutboxRepo) Complete(_ context.Context, id int64, workerID string, completedAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.events {
		if event.ID == id && event.Status == service.DomainOutboxStatusProcessing && event.LockedBy != nil && *event.LockedBy == workerID {
			event.Status, event.CompletedAt = service.DomainOutboxStatusCompleted, &completedAt
			event.LockedAt, event.LockedUntil, event.LockedBy = nil, nil, nil
			return true, nil
		}
	}
	return false, nil
}

func (r *videoReliabilityOutboxRepo) Retry(_ context.Context, id int64, workerID string, next time.Time, dead bool, lastError string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.events {
		if event.ID != id || event.LockedBy == nil || *event.LockedBy != workerID {
			continue
		}
		status := service.DomainOutboxStatusPending
		if dead {
			status = service.DomainOutboxStatusDead
		}
		event.Status, event.NextAttemptAt, event.LastError = status, next, &lastError
		event.LockedAt, event.LockedUntil, event.LockedBy = nil, nil, nil
		return true, nil
	}
	return false, nil
}

func (r *videoReliabilityOutboxRepo) ReapExpiredLeases(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (r *videoReliabilityOutboxRepo) Counts(context.Context) (service.DomainOutboxCounts, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var counts service.DomainOutboxCounts
	for _, event := range r.events {
		switch event.Status {
		case service.DomainOutboxStatusPending:
			counts.Pending++
		case service.DomainOutboxStatusProcessing:
			counts.Processing++
		case service.DomainOutboxStatusCompleted:
			counts.Completed++
		case service.DomainOutboxStatusDead:
			counts.Dead++
		}
	}
	return counts, nil
}

func cloneReliabilityOutboxEvent(event *service.DomainOutboxEvent) *service.DomainOutboxEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	cloned.Payload = append([]byte(nil), event.Payload...)
	return &cloned
}

type videoReliabilityLocalAssetHandler struct{ repo *videoReliabilityMockRepo }

func (h *videoReliabilityLocalAssetHandler) Handle(ctx context.Context, event *service.DomainOutboxEvent) error {
	if h == nil || h.repo == nil || event == nil {
		return errors.New("local mock asset handler is required")
	}
	task, err := h.repo.GetTask(ctx, event.AggregateID)
	if err != nil {
		return err
	}
	switch event.EventType {
	case service.VideoOutboxEventCapture:
		task.CaptureStatus = "succeeded"
		return h.repo.UpdateTask(ctx, task)
	case service.VideoOutboxEventArchive:
		if err := h.repo.SetTaskLocalAsset(ctx, task.ID, fmt.Sprintf("local://mock-assets/%d.svg", task.ID), time.Now().UTC()); err != nil {
			return err
		}
		task, err = h.repo.GetTask(ctx, task.ID)
		if err != nil {
			return err
		}
		task.ArchiveStatus = "succeeded"
		return h.repo.UpdateTask(ctx, task)
	default:
		return service.NonRetryableDomainOutboxError(fmt.Errorf("unexpected local mock event %s", event.EventType))
	}
}

func TestVideoReliabilityMockOnlyEndToEnd(t *testing.T) {
	harness := newVideoReliabilityMockHarness(t)
	initialBalance, initialReservations, initialTransactions := harness.repo.financialSnapshot()
	require.Zero(t, initialReservations)
	require.Zero(t, initialTransactions)

	create := apiKeyVideoGatewayRequest(harness.router, http.MethodPost, "/v1/video/tasks", []byte(`{
		"provider":"mock",
		"task_type":"text_to_video",
		"model":"mock-video-v1",
		"prompt":"local reliability proof",
		"aspect_ratio":"16:9",
		"duration":5,
		"resolution":"720p"
	}`))
	require.Equal(t, http.StatusCreated, create.Code, create.Body.String())
	var envelope struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(create.Body.Bytes(), &envelope))
	require.Positive(t, envelope.Data.ID, "create must return a real local task ID")

	for range 3 { // submit -> running -> terminal finalization
		require.NoError(t, harness.worker.ProcessOnce(context.Background()))
	}
	terminal, err := harness.repo.GetTask(context.Background(), envelope.Data.ID)
	require.NoError(t, err)
	require.Equal(t, service.VideoStatusSucceeded, terminal.Status)
	require.Equal(t, service.VideoSettlementStatusNotNeeded, terminal.SettlementStatus)
	require.Empty(t, terminal.LocalAssetPath, "asset must be produced by the outbox, not terminal finalization")
	require.Len(t, harness.outbox.events, 2)

	require.NoError(t, harness.outboxWorker.RunOnce(context.Background()))
	delivered, err := harness.repo.GetTask(context.Background(), envelope.Data.ID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", delivered.CaptureStatus)
	require.Equal(t, "succeeded", delivered.ArchiveStatus)
	require.Equal(t, "local://mock-assets/1.svg", delivered.LocalAssetPath)
	balance, reservationCount, transactionCount := harness.repo.financialSnapshot()
	require.Equal(t, initialBalance, balance)
	require.Zero(t, reservationCount, "mock path must not call CreateWithReservation")
	require.Zero(t, transactionCount, "mock path must not create a charge transaction")

	get := apiKeyVideoGatewayRequest(harness.router, http.MethodGet, "/v1/video/tasks/1", nil)
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	require.Contains(t, get.Body.String(), `"delivery_status":"deliverable"`)
	require.Contains(t, get.Body.String(), `"local_asset_available":true`)

	// A repeated terminal delivery must remain idempotent and financially inert.
	replayed, err := harness.repo.FinalizeVideoTask(context.Background(), harness.finalizationInput(envelope.Data.ID, terminal.Version-1))
	require.NoError(t, err)
	require.True(t, replayed.Idempotent)
	balance, reservationCount, transactionCount = harness.repo.financialSnapshot()
	require.Equal(t, initialBalance, balance)
	require.Zero(t, reservationCount)
	require.Zero(t, transactionCount)

	for _, event := range harness.outbox.events {
		require.Equal(t, service.DomainOutboxStatusCompleted, event.Status)
		require.NotNil(t, event.CompletedAt)
		require.WithinDuration(t, time.Now().UTC(), *event.CompletedAt, time.Minute)
	}
}
