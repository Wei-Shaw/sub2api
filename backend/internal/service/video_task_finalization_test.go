package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type recordingVideoTaskFinalizationRepository struct {
	calls  []VideoTaskFinalizationInput
	result VideoTaskFinalizationResult
	err    error
}

type recordingCancelVideoAdapter struct {
	cancelCalls int
	result      *VideoAdapterResult
	err         error
}

func (a *recordingCancelVideoAdapter) Provider() string { return VideoProviderMock }

func (a *recordingCancelVideoAdapter) CreateTask(context.Context, *VideoProviderAccount, *VideoTask) (*VideoAdapterResult, error) {
	return nil, errors.New("unexpected create")
}

func (a *recordingCancelVideoAdapter) PollTask(context.Context, *VideoProviderAccount, *VideoTask) (*VideoAdapterResult, error) {
	return nil, errors.New("unexpected poll")
}

func (a *recordingCancelVideoAdapter) CancelTask(context.Context, *VideoProviderAccount, *VideoTask) (*VideoAdapterResult, error) {
	a.cancelCalls++
	if a.err != nil {
		return nil, a.err
	}
	return cloneVideoAdapterResult(a.result), nil
}

func (a *recordingCancelVideoAdapter) NormalizeStatus(status string) string {
	return normalizeVideoStatus(status)
}

func (a *recordingCancelVideoAdapter) BuildCreatePayload(*VideoProviderAccount, *VideoTask) map[string]any {
	return map[string]any{}
}

func (r *recordingVideoTaskFinalizationRepository) FinalizeVideoTask(_ context.Context, input VideoTaskFinalizationInput) (VideoTaskFinalizationResult, error) {
	r.calls = append(r.calls, input)
	return r.result, r.err
}

func TestVideoTaskFinalizerRejectsNonTerminalStatus(t *testing.T) {
	repo := &recordingVideoTaskFinalizationRepository{}
	finalizer := NewVideoTaskFinalizer(repo)

	_, err := finalizer.Finalize(context.Background(), validVideoTaskFinalizationInput(VideoStatusRunning))
	if !errors.Is(err, ErrVideoInvalidStatus) {
		t.Fatalf("Finalize error = %v, want ErrVideoInvalidStatus", err)
	}
	if len(repo.calls) != 0 {
		t.Fatalf("repository calls = %d, want 0", len(repo.calls))
	}
}

func TestVideoTaskFinalizerSucceededCallsRepositoryOnce(t *testing.T) {
	repo := &recordingVideoTaskFinalizationRepository{
		result: VideoTaskFinalizationResult{Applied: true, Status: VideoStatusSucceeded, Version: 8},
	}
	finalizer := NewVideoTaskFinalizer(repo)
	input := validVideoTaskFinalizationInput(VideoStatusSucceeded)

	result, err := finalizer.Finalize(context.Background(), input)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if !result.Applied || result.Idempotent || result.Version != 8 {
		t.Fatalf("result = %#v", result)
	}
	if len(repo.calls) != 1 {
		t.Fatalf("repository calls = %d, want 1", len(repo.calls))
	}
	if got := repo.calls[0].ActualCostUSD.String(); got != "1.2500000000" {
		t.Fatalf("actual USD cost = %s", got)
	}
}

func TestVideoTaskFinalizerFailedAndCancelledCannotCharge(t *testing.T) {
	for _, status := range []string{VideoStatusFailed, VideoStatusCancelled} {
		t.Run(status, func(t *testing.T) {
			repo := &recordingVideoTaskFinalizationRepository{
				result: VideoTaskFinalizationResult{Applied: true, Status: status, Version: 8},
			}
			finalizer := NewVideoTaskFinalizer(repo)
			input := validVideoTaskFinalizationInput(status)
			input.ActualCostUSD = MustUSD("99")

			_, err := finalizer.Finalize(context.Background(), input)
			if err != nil {
				t.Fatalf("Finalize: %v", err)
			}
			if len(repo.calls) != 1 {
				t.Fatalf("repository calls = %d, want 1", len(repo.calls))
			}
			if got := repo.calls[0].ActualCostUSD.String(); got != "0.0000000000" {
				t.Fatalf("terminal %s cost = %s, want zero", status, got)
			}
		})
	}
}

func TestVideoTaskFinalizerReturnsIdempotentReplay(t *testing.T) {
	repo := &recordingVideoTaskFinalizationRepository{
		result: VideoTaskFinalizationResult{Idempotent: true, Status: VideoStatusSucceeded, Version: 8},
	}
	finalizer := NewVideoTaskFinalizer(repo)

	result, err := finalizer.Finalize(context.Background(), validVideoTaskFinalizationInput(VideoStatusSucceeded))
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if result.Applied || !result.Idempotent || result.Status != VideoStatusSucceeded {
		t.Fatalf("result = %#v", result)
	}
	if len(repo.calls) != 1 {
		t.Fatalf("repository calls = %d, want 1", len(repo.calls))
	}
}

func TestVideoTaskFinalizerPreservesTypedTerminalConflict(t *testing.T) {
	conflict := &VideoTaskTerminalConflictError{
		TaskID:          41,
		RequestedStatus: VideoStatusSucceeded,
		CurrentStatus:   VideoStatusFailed,
		CurrentVersion:  8,
	}
	repo := &recordingVideoTaskFinalizationRepository{err: conflict}
	finalizer := NewVideoTaskFinalizer(repo)

	_, err := finalizer.Finalize(context.Background(), validVideoTaskFinalizationInput(VideoStatusSucceeded))
	if !errors.Is(err, ErrVideoTaskTerminalConflict) {
		t.Fatalf("Finalize error = %v, want terminal conflict", err)
	}
	var typed *VideoTaskTerminalConflictError
	if !errors.As(err, &typed) || typed.CurrentStatus != VideoStatusFailed || typed.CurrentVersion != 8 {
		t.Fatalf("typed conflict = %#v", typed)
	}
}

func TestVideoGatewayCancelFlagOnUsesAtomicFinalizerOnly(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{ID: 4201, ProviderAccountID: providerID, Provider: VideoProviderMock, Model: "mock-video-v1", Status: VideoStatusRunning, Version: 5, CreatedBy: 77, CreatedAt: time.Now().UTC()}
	repo.tasks[task.ID] = cloneVideoTask(task)
	cfg := &config.Config{ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true}}
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, cfg)
	adapter := &recordingCancelVideoAdapter{result: &VideoAdapterResult{Status: VideoStatusRunning, Payload: map[string]any{"cancel_id": "offline-cancel"}}}
	svc.adapters[VideoProviderMock] = adapter
	finalizationRepo := &recordingVideoTaskFinalizationRepository{result: VideoTaskFinalizationResult{Applied: true, Status: VideoStatusCancelled, Version: 6}}
	svc.SetVideoTaskFinalizer(NewVideoTaskFinalizer(finalizationRepo))

	result, err := svc.CancelTask(ctx, task.ID, task.CreatedBy, false)
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if adapter.cancelCalls != 1 {
		t.Fatalf("provider cancel calls = %d, want 1", adapter.cancelCalls)
	}
	if len(finalizationRepo.calls) != 1 {
		t.Fatalf("finalization calls = %d, want 1", len(finalizationRepo.calls))
	}
	call := finalizationRepo.calls[0]
	if call.TaskID != task.ID || call.ExpectedVersion != 5 || call.TerminalStatus != VideoStatusCancelled {
		t.Fatalf("finalization input = %#v", call)
	}
	if call.ActualCostUSD.String() != "0.0000000000" {
		t.Fatalf("cancel cost = %s, want zero", call.ActualCostUSD.String())
	}
	if result.Status != VideoStatusCancelled || result.Version != 6 {
		t.Fatalf("cancel result = %#v", result)
	}
	stored := repo.tasks[task.ID]
	if stored.Status != VideoStatusRunning || stored.Version != 5 || len(repo.events) != 0 || len(repo.usage) != 0 {
		t.Fatalf("flag-on cancel used legacy writes: task=%#v events=%d usage=%d", stored, len(repo.events), len(repo.usage))
	}
}

func TestVideoGatewayCancelFlagOnFinalizerUnavailableFailsClosed(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{ID: 4202, ProviderAccountID: providerID, Provider: VideoProviderMock, Status: VideoStatusRunning, Version: 3, CreatedBy: 78, CreatedAt: time.Now().UTC()}
	repo.tasks[task.ID] = cloneVideoTask(task)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, &config.Config{ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true}})
	adapter := &recordingCancelVideoAdapter{result: &VideoAdapterResult{Status: VideoStatusCancelled}}
	svc.adapters[VideoProviderMock] = adapter

	_, err := svc.CancelTask(context.Background(), task.ID, task.CreatedBy, false)
	if err == nil {
		t.Fatal("CancelTask error = nil, want unavailable finalizer")
	}
	if adapter.cancelCalls != 0 {
		t.Fatalf("provider cancel calls = %d, want fail-closed before provider", adapter.cancelCalls)
	}
	if stored := repo.tasks[task.ID]; stored.Status != VideoStatusRunning || len(repo.events) != 0 || len(repo.usage) != 0 {
		t.Fatalf("unavailable finalizer polluted task: %#v", stored)
	}
}

func TestVideoGatewayCancelFlagOnFinalizationFailureDoesNotUseLegacyWrites(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{ID: 4203, ProviderAccountID: providerID, Provider: VideoProviderMock, Status: VideoStatusRunning, Version: 8, CreatedBy: 79, CreatedAt: time.Now().UTC()}
	repo.tasks[task.ID] = cloneVideoTask(task)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, &config.Config{ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true}})
	adapter := &recordingCancelVideoAdapter{result: &VideoAdapterResult{Status: VideoStatusCancelled}}
	svc.adapters[VideoProviderMock] = adapter
	finalizationRepo := &recordingVideoTaskFinalizationRepository{err: errors.New("injected cancel finalization failure")}
	svc.SetVideoTaskFinalizer(NewVideoTaskFinalizer(finalizationRepo))

	_, err := svc.CancelTask(context.Background(), task.ID, task.CreatedBy, false)
	if err == nil {
		t.Fatal("CancelTask error = nil, want finalization failure")
	}
	if adapter.cancelCalls != 1 || len(finalizationRepo.calls) != 1 {
		t.Fatalf("provider/finalization calls = %d/%d, want 1/1", adapter.cancelCalls, len(finalizationRepo.calls))
	}
	if stored := repo.tasks[task.ID]; stored.Status != VideoStatusRunning || stored.Version != 8 || len(repo.events) != 0 || len(repo.usage) != 0 {
		t.Fatalf("failed finalization polluted task: %#v", stored)
	}
}

func TestVideoGatewayCancelFlagOffKeepsLegacyWrites(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	task := &VideoTask{ID: 4204, ProviderAccountID: providerID, Provider: VideoProviderMock, Status: VideoStatusRunning, Version: 2, CreatedBy: 80, CreatedAt: time.Now().UTC()}
	repo.tasks[task.ID] = cloneVideoTask(task)
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, &config.Config{ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: false}})
	adapter := &recordingCancelVideoAdapter{result: &VideoAdapterResult{Status: VideoStatusCancelled}}
	svc.adapters[VideoProviderMock] = adapter
	finalizationRepo := &recordingVideoTaskFinalizationRepository{}
	svc.SetVideoTaskFinalizer(NewVideoTaskFinalizer(finalizationRepo))

	result, err := svc.CancelTask(context.Background(), task.ID, task.CreatedBy, false)
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if result.Status != VideoStatusCancelled || repo.tasks[task.ID].Status != VideoStatusCancelled {
		t.Fatalf("legacy cancel result/stored = %q/%q", result.Status, repo.tasks[task.ID].Status)
	}
	if len(repo.events) != 1 || len(repo.usage) != 1 || len(finalizationRepo.calls) != 0 {
		t.Fatalf("legacy events/usage/finalizer = %d/%d/%d", len(repo.events), len(repo.usage), len(finalizationRepo.calls))
	}
}

func validVideoTaskFinalizationInput(status string) VideoTaskFinalizationInput {
	original, err := NewMoney("9", Currency("CNY"))
	if err != nil {
		panic(err)
	}
	duration := 12
	tokens := int64(987)
	return VideoTaskFinalizationInput{
		TaskID:               41,
		ExpectedVersion:      7,
		TerminalStatus:       status,
		ProviderResultURL:    "https://provider.invalid/result.mp4",
		ProviderErrorMessage: "provider rejected request",
		ProviderPayload:      map[string]any{"request_id": "req-offline-1"},
		ActualDuration:       &duration,
		ActualResolution:     "1080p",
		ActualTokens:         &tokens,
		LastFrameURL:         "https://provider.invalid/last-frame.png",
		PollCount:            3,
		ActualCostUSD:        MustUSD("1.25"),
		PricingSnapshot: PricingSnapshot{
			AmountOriginal: original,
			ExchangeRate:   "7.2000000000",
			PricingSource:  PricingSourceProviderUsage,
			PricingVersion: "test-pricing-v1",
		},
		CompletedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	}
}
