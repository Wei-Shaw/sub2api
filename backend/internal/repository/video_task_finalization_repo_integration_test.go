//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type videoTaskFinalizationFixture struct {
	user        *service.User
	task        *service.VideoTask
	reservation *service.BillingReservation
}

var _ service.VideoTaskFinalizationRepository = (*videoGatewayRepository)(nil)
var _ service.VideoTaskPollRepository = (*videoGatewayRepository)(nil)

func TestVideoTaskFinalizationFaultMatrixRollsBackEveryWrite(t *testing.T) {
	for _, fault := range []string{"task_cas", "event", "usage", "reservation", "ledger", "balance", "outbox"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newVideoTaskFinalizationFixture(t, "3")
			constraint := installVideoTaskFinalizationFault(t, fault, fixture)
			finalizer := newVideoTaskFinalizerForIntegration(t)

			_, err := finalizer.Finalize(context.Background(), newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "1.25"))
			require.Error(t, err)
			require.Contains(t, err.Error(), constraint, "fault must reach its intended SQL boundary")
			assertVideoTaskFinalizationNoPartialState(t, fixture)
		})
	}
}

func TestVideoTaskFinalizationReplayIsExactlyOnce(t *testing.T) {
	ctx := context.Background()
	fixture := newVideoTaskFinalizationFixture(t, "3")
	finalizer := newVideoTaskFinalizerForIntegration(t)
	input := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "1.25")

	first, err := finalizer.Finalize(ctx, input)
	require.NoError(t, err)
	require.True(t, first.Applied)
	require.False(t, first.Idempotent)
	second, err := finalizer.Finalize(ctx, input)
	require.NoError(t, err)
	require.False(t, second.Applied)
	require.True(t, second.Idempotent)
	require.Equal(t, service.VideoStatusSucceeded, second.Status)
	require.Equal(t, fixture.task.Version+1, second.Version)
	require.Equal(t, service.BillingReservationStatusSettled, second.SettlementStatus)
	require.NotNil(t, second.BalanceChargedAt)
	require.Nil(t, second.WorkerClaimedAt)
	require.Nil(t, second.WorkerClaimedUntil)
	require.Equal(t, service.VideoSideEffectStatusPending, second.ArchiveStatus)
	require.Equal(t, service.VideoSideEffectStatusPending, second.CaptureStatus)
	require.NotNil(t, second.CompletedAt)
	require.Equal(t, input.ProviderResultURL, second.ResultURL)
	require.Equal(t, input.ProviderErrorMessage, second.ErrorMessage)
	require.Equal(t, input.PollCount, second.PollCount)
	require.NotNil(t, second.UsageTotalTokens)
	require.Equal(t, *input.ActualTokens, *second.UsageTotalTokens)
	require.Equal(t, input.ActualResolution, second.ActualResolution)
	require.NotNil(t, second.ActualDuration)
	require.Equal(t, *input.ActualDuration, *second.ActualDuration)
	require.Equal(t, input.LastFrameURL, second.LastFrameURL)

	require.Equal(t, 1, countVideoTaskFinalizationRows(t, "video_task_events", "video_task_id", fixture.task.ID))
	require.Equal(t, 1, countVideoTaskFinalizationRows(t, "video_usage_logs", "video_task_id", fixture.task.ID))
	require.Equal(t, 1, countVideoTaskFinalizationRows(t, "billing_transactions", "source_id", fixture.task.ID))
	require.Equal(t, 4, countVideoTaskFinalizationRows(t, "domain_outbox", "aggregate_id", fixture.task.ID))

	var chargeCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM billing_transactions WHERE transaction_key = $1",
		fmt.Sprintf("video_task:%d:charge", fixture.task.ID),
	).Scan(&chargeCount))
	require.Equal(t, 1, chargeCount)
	require.Equal(t, []string{
		"billing.invalidate_cache",
		"billing.notify_low_balance",
		"video.archive_asset",
		"video.capture_content",
	}, videoTaskFinalizationOutboxTypes(t, fixture.task.ID))

	assertVideoTaskFinalizationSettled(t, fixture, "1.2500000000", "8.75000000", true)
}

func TestVideoReliabilityBillableFakeSettlementAndOutboxRetryProof(t *testing.T) {
	ctx := context.Background()
	user := newVideoTaskCreationUser(t, 10)
	providerID := newVideoTaskCreationProvider(t)
	repo := NewVideoGatewayRepository(integrationDB)
	cfg := &config.Config{ReliabilityCore: config.ReliabilityCoreConfig{VideoEnabled: true}}
	video := service.NewVideoGatewayService(repo, billableFakeEncryptor{}, cfg)
	pricing := billableFakePricing{amount: service.MustUSD("1.25")}
	video.SetVideoTaskPricing(pricing)
	adapter := &billableFakeAdapter{}
	require.NoError(t, video.RegisterVideoAdapter(adapter))

	created, err := video.CreateTask(ctx, service.VideoTaskCreateParams{
		ProviderAccountID:   providerID,
		TaskType:            service.VideoTaskTypeTextToVideo,
		Model:               "doubao-seedance-2-0-260128",
		Prompt:              "offline billable fake settlement",
		AspectRatio:         "16:9",
		Duration:            5,
		Resolution:          "720p",
		CreatedBy:           user.ID,
		CreationKey:         service.HashIdempotencyKey("billable-fake-create-" + uuid.NewString()),
		CreationFingerprint: service.HashIdempotencyKey("billable-fake-payload-" + uuid.NewString()),
	})
	require.NoError(t, err)
	require.NotNil(t, created.ReservationID)

	worker := service.NewVideoGatewayWorker(video, cfg)
	require.NoError(t, worker.ProcessOnce(ctx), "offline fake submits through the real worker")
	require.NoError(t, worker.ProcessOnce(ctx), "offline fake polls and finalizes through the real finalizer")
	terminal, err := repo.GetTask(ctx, created.ID)
	require.NoError(t, err)
	assertVideoReliabilityTaskAndSideEffects(t, terminal.ID, service.VideoStatusSucceeded, service.VideoSideEffectStatusPending, service.VideoSideEffectStatusPending)
	require.Equal(t, 1, adapter.createCalls)
	require.Equal(t, 1, adapter.pollCalls)
	require.Equal(t, 1, videoTaskFinalizationTransactionCount(t, terminal.ID, "charge"))
	require.Equal(t, "8.75000000", videoTaskFinalizationBalance(t, user.ID))

	finalizerRepo, ok := repo.(service.VideoTaskFinalizationRepository)
	require.True(t, ok)
	replay, err := service.NewVideoTaskFinalizer(finalizerRepo).Finalize(ctx, billableFakeFinalizationReplayInput(terminal.ID))
	require.NoError(t, err)
	require.True(t, replay.Idempotent)
	require.Equal(t, 1, videoTaskFinalizationTransactionCount(t, terminal.ID, "charge"))

	now := time.Now().UTC().Add(time.Second)
	sideEffects, ok := repo.(service.VideoOutboxSideEffectRepository)
	require.True(t, ok)
	handler := &billableFakeOutboxHandler{sideEffects: sideEffects, failedOnce: make(map[string]bool)}
	outboxWorker := service.NewDomainOutboxWorker(
		NewDomainOutboxRepository(integrationDB), handler, cfg,
		service.DomainOutboxWorkerOptions{WorkerID: "billable-fake-outbox", Now: func() time.Time { return now }},
	)
	require.NoError(t, outboxWorker.RunOnce(ctx), "first local side-effect attempt fails without mutating terminal state")
	assertVideoReliabilityTaskAndSideEffects(t, terminal.ID, service.VideoStatusSucceeded, service.VideoSideEffectStatusPending, service.VideoSideEffectStatusPending)
	require.Equal(t, 1, videoTaskFinalizationTransactionCount(t, terminal.ID, "charge"))
	require.Equal(t, "8.75000000", videoTaskFinalizationBalance(t, user.ID))

	now = now.Add(6 * time.Second)
	require.NoError(t, outboxWorker.RunOnce(ctx), "real outbox retry completes the local side effects")
	assertVideoReliabilityTaskAndSideEffects(t, terminal.ID, service.VideoStatusSucceeded, "succeeded", "succeeded")
	require.Equal(t, 1, videoTaskFinalizationTransactionCount(t, terminal.ID, "charge"))
	require.Equal(t, "8.75000000", videoTaskFinalizationBalance(t, user.ID))
	require.Equal(t, 1, adapter.createCalls, "retries never re-dispatch or invoke a network adapter")
	require.Equal(t, 1, adapter.pollCalls)

	var completed int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM domain_outbox
		WHERE aggregate_type = 'video_task' AND aggregate_id = $1
		  AND event_type IN ('video.capture_content', 'video.archive_asset')
		  AND status = 'completed' AND attempt_count = 2
	`, terminal.ID).Scan(&completed))
	require.Equal(t, 2, completed)
}

type billableFakeEncryptor struct{}

func (billableFakeEncryptor) Encrypt(plaintext string) (string, error)  { return plaintext, nil }
func (billableFakeEncryptor) Decrypt(ciphertext string) (string, error) { return ciphertext, nil }

type billableFakePricing struct{ amount service.Money }

func (p billableFakePricing) EstimatePrice(context.Context, *service.VideoTask) (service.Money, service.PricingSnapshot, error) {
	return p.price()
}

func (p billableFakePricing) ActualPrice(context.Context, *service.VideoTask) (service.Money, service.PricingSnapshot, error) {
	return p.price()
}

func (p billableFakePricing) price() (service.Money, service.PricingSnapshot, error) {
	return p.amount, service.PricingSnapshot{AmountOriginal: p.amount, ExchangeRate: "1.0000000000", PricingSource: "billable_fake", PricingVersion: "fixed-v1"}, nil
}

type billableFakeAdapter struct {
	createCalls int
	pollCalls   int
}

func (*billableFakeAdapter) Provider() string { return service.VideoProviderSeedance }
func (a *billableFakeAdapter) CreateTask(context.Context, *service.VideoProviderAccount, *service.VideoTask) (*service.VideoAdapterResult, error) {
	a.createCalls++
	return &service.VideoAdapterResult{UpstreamTaskID: "offline-billable-fake", Status: service.VideoStatusSubmitted, Payload: map[string]any{"offline": true}}, nil
}
func (a *billableFakeAdapter) PollTask(context.Context, *service.VideoProviderAccount, *service.VideoTask) (*service.VideoAdapterResult, error) {
	a.pollCalls++
	return &service.VideoAdapterResult{Status: service.VideoStatusSucceeded, ResultURL: "local://billable-fake/result.mp4", Payload: map[string]any{"offline": true}}, nil
}
func (*billableFakeAdapter) CancelTask(context.Context, *service.VideoProviderAccount, *service.VideoTask) (*service.VideoAdapterResult, error) {
	return &service.VideoAdapterResult{Status: service.VideoStatusCancelled}, nil
}
func (*billableFakeAdapter) NormalizeStatus(status string) string { return status }
func (*billableFakeAdapter) BuildCreatePayload(*service.VideoProviderAccount, *service.VideoTask) map[string]any {
	return map[string]any{"offline": true}
}

type billableFakeOutboxHandler struct {
	sideEffects service.VideoOutboxSideEffectRepository
	failedOnce  map[string]bool
}

func (h *billableFakeOutboxHandler) Handle(_ context.Context, event *service.DomainOutboxEvent) error {
	if event == nil {
		return service.NonRetryableDomainOutboxError(errors.New("billable fake outbox event is required"))
	}
	if event.EventType != service.VideoOutboxEventCapture && event.EventType != service.VideoOutboxEventArchive {
		return nil
	}
	if !h.failedOnce[event.EventType] {
		h.failedOnce[event.EventType] = true
		return service.RetryableDomainOutboxError(errors.New("injected local side effect failure"))
	}
	return nil
}

func (h *billableFakeOutboxHandler) Complete(ctx context.Context, event *service.DomainOutboxEvent, workerID string, completedAt time.Time) (bool, error) {
	if event == nil || (event.EventType != service.VideoOutboxEventCapture && event.EventType != service.VideoOutboxEventArchive) {
		return false, nil
	}
	effect := "capture"
	if event.EventType == service.VideoOutboxEventArchive {
		effect = "archive"
	}
	return h.sideEffects.CompleteVideoOutboxSideEffect(ctx, event.ID, workerID, completedAt, event.AggregateID, effect)
}

func billableFakeFinalizationReplayInput(taskID int64) service.VideoTaskFinalizationInput {
	price := service.MustUSD("1.25")
	return service.VideoTaskFinalizationInput{
		TaskID: taskID, ExpectedVersion: 1, TerminalStatus: service.VideoStatusSucceeded,
		ActualCostUSD: price, PricingSnapshot: service.PricingSnapshot{AmountOriginal: price, ExchangeRate: "1.0000000000", PricingSource: "billable_fake", PricingVersion: "fixed-v1"},
		CompletedAt: time.Now().UTC(),
	}
}

func assertVideoReliabilityTaskAndSideEffects(t *testing.T, taskID int64, wantTask, wantCapture, wantArchive string) {
	t.Helper()
	var taskStatus, captureStatus, archiveStatus string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `
		SELECT status, capture_status, archive_status FROM video_tasks WHERE id = $1
	`, taskID).Scan(&taskStatus, &captureStatus, &archiveStatus))
	require.Equal(t, wantTask, taskStatus)
	require.Equal(t, wantCapture, captureStatus)
	require.Equal(t, wantArchive, archiveStatus)
}

func TestVideoTaskFinalizationConcurrentReplayAndTerminalConflict(t *testing.T) {
	ctx := context.Background()
	fixture := newVideoTaskFinalizationFixture(t, "3")
	finalizer := newVideoTaskFinalizerForIntegration(t)
	input := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "1.25")
	type outcome struct {
		result service.VideoTaskFinalizationResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := finalizer.Finalize(ctx, input)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)

	var applied, idempotent int
	for item := range outcomes {
		require.NoError(t, item.err)
		if item.result.Applied {
			applied++
		}
		if item.result.Idempotent {
			idempotent++
		}
	}
	require.Equal(t, 1, applied)
	require.Equal(t, 1, idempotent)

	conflicting := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusFailed, "0")
	_, err := finalizer.Finalize(ctx, conflicting)
	require.ErrorIs(t, err, service.ErrVideoTaskTerminalConflict)
	var typed *service.VideoTaskTerminalConflictError
	require.True(t, errors.As(err, &typed))
	require.Equal(t, service.VideoStatusSucceeded, typed.CurrentStatus)

	var storedStatus string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM video_tasks WHERE id = $1", fixture.task.ID).Scan(&storedStatus))
	require.Equal(t, service.VideoStatusSucceeded, storedStatus)
	require.Equal(t, 1, countVideoTaskFinalizationRows(t, "video_task_events", "video_task_id", fixture.task.ID))
}

func TestVideoTaskFinalizationWorkerSuccessRacesCancellationConsistently(t *testing.T) {
	ctx := context.Background()
	fixture := newVideoTaskFinalizationFixture(t, "3")
	finalizer := newVideoTaskFinalizerForIntegration(t)
	succeeded := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "1.25")
	cancelled := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusCancelled, "0")
	type outcome struct {
		status string
		result service.VideoTaskFinalizationResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, input := range []service.VideoTaskFinalizationInput{succeeded, cancelled} {
		input := input
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := finalizer.Finalize(ctx, input)
			outcomes <- outcome{status: input.TerminalStatus, result: result, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)

	var winner string
	var conflicts int
	for item := range outcomes {
		if item.err == nil && item.result.Applied {
			winner = item.status
			continue
		}
		require.ErrorIs(t, item.err, service.ErrVideoTaskTerminalConflict)
		conflicts++
	}
	require.NotEmpty(t, winner)
	require.Equal(t, 1, conflicts)

	var taskStatus, reservationStatus string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM video_tasks WHERE id = $1", fixture.task.ID).Scan(&taskStatus))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM billing_reservations WHERE id = $1", fixture.reservation.ID).Scan(&reservationStatus))
	require.Equal(t, winner, taskStatus)
	require.Equal(t, 1, countVideoTaskFinalizationRows(t, "video_task_events", "video_task_id", fixture.task.ID))
	require.Equal(t, 1, countVideoTaskFinalizationRows(t, "video_usage_logs", "video_task_id", fixture.task.ID))
	require.Equal(t, 1, countVideoTaskFinalizationRows(t, "billing_transactions", "source_id", fixture.task.ID))
	if winner == service.VideoStatusSucceeded {
		require.Equal(t, service.BillingReservationStatusSettled, reservationStatus)
		require.Equal(t, 1, videoTaskFinalizationTransactionCount(t, fixture.task.ID, "charge"))
		require.Equal(t, 0, videoTaskFinalizationTransactionCount(t, fixture.task.ID, "release"))
		require.Equal(t, "8.75000000", videoTaskFinalizationBalance(t, fixture.user.ID))
		require.Equal(t, 4, countVideoTaskFinalizationRows(t, "domain_outbox", "aggregate_id", fixture.task.ID))
	} else {
		require.Equal(t, service.VideoStatusCancelled, winner)
		require.Equal(t, service.BillingReservationStatusReleased, reservationStatus)
		require.Equal(t, 0, videoTaskFinalizationTransactionCount(t, fixture.task.ID, "charge"))
		require.Equal(t, 1, videoTaskFinalizationTransactionCount(t, fixture.task.ID, "release"))
		require.Equal(t, "10.00000000", videoTaskFinalizationBalance(t, fixture.user.ID))
		require.Equal(t, 0, countVideoTaskFinalizationRows(t, "domain_outbox", "aggregate_id", fixture.task.ID))
	}
}

func TestVideoTaskPollCASRacesCancellationConsistently(t *testing.T) {
	ctx := context.Background()

	t.Run("cancel commits before stale poll", func(t *testing.T) {
		fixture := newVideoTaskFinalizationFixture(t, "3")
		markVideoTaskRunningForPollRace(t, fixture.task)
		expectedVersion := fixture.task.Version
		cancelInput := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusCancelled, "0")
		cancelInput.ProviderResultURL = ""
		cancelInput.ProviderErrorMessage = ""
		cancelInput.ActualDuration = nil
		cancelInput.ActualResolution = ""
		cancelInput.ActualTokens = nil
		cancelInput.LastFrameURL = ""

		cancelled, err := newVideoTaskFinalizerForIntegration(t).Finalize(ctx, cancelInput)
		require.NoError(t, err)
		require.True(t, cancelled.Applied)

		candidate := *fixture.task
		candidate.Status = service.VideoStatusRunning
		candidate.ResultURL = "https://provider.invalid/stale-poll.mp4"
		candidate.PollCount++
		polled, err := newVideoTaskPollRepositoryForIntegration(t).UpdatePolledTaskCAS(ctx, expectedVersion, &candidate, &service.VideoTaskEvent{
			VideoTaskID: fixture.task.ID,
			EventType:   service.VideoStatusRunning,
			Message:     "stale poll must not commit",
			Payload:     map[string]any{"request_id": "cancel-first"},
		})
		require.NoError(t, err)
		require.False(t, polled.Applied)
		require.Equal(t, service.VideoStatusCancelled, polled.Status)
		require.Equal(t, expectedVersion+1, polled.Version)
		require.Empty(t, polled.ResultURL)
		require.Equal(t, service.BillingReservationStatusReleased, polled.SettlementStatus)
		require.Equal(t, service.VideoSideEffectStatusNotNeeded, polled.ArchiveStatus)
		require.Equal(t, service.VideoSideEffectStatusNotNeeded, polled.CaptureStatus)
		require.Nil(t, polled.BalanceChargedAt)
		require.Nil(t, polled.WorkerClaimedAt)
		require.Nil(t, polled.WorkerClaimedUntil)
		require.NotNil(t, polled.CompletedAt)

		var taskStatus, reservationStatus string
		var version int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status, version FROM video_tasks WHERE id = $1", fixture.task.ID).Scan(&taskStatus, &version))
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM billing_reservations WHERE id = $1", fixture.reservation.ID).Scan(&reservationStatus))
		require.Equal(t, service.VideoStatusCancelled, taskStatus)
		require.Equal(t, expectedVersion+1, version)
		require.Equal(t, service.BillingReservationStatusReleased, reservationStatus)
		require.Equal(t, []string{service.VideoStatusCancelled}, videoTaskFinalizationEventTypes(t, fixture.task.ID))
		require.Equal(t, 1, countVideoTaskFinalizationRows(t, "video_usage_logs", "video_task_id", fixture.task.ID))
		require.Equal(t, 1, videoTaskFinalizationTransactionCount(t, fixture.task.ID, "release"))
	})

	t.Run("poll commits before stale cancel", func(t *testing.T) {
		fixture := newVideoTaskFinalizationFixture(t, "3")
		markVideoTaskRunningForPollRace(t, fixture.task)
		expectedVersion := fixture.task.Version
		candidate := *fixture.task
		candidate.Status = service.VideoStatusRunning
		candidate.ResultURL = "https://provider.invalid/current-poll.mp4"
		candidate.PollCount++

		polled, err := newVideoTaskPollRepositoryForIntegration(t).UpdatePolledTaskCAS(ctx, expectedVersion, &candidate, &service.VideoTaskEvent{
			VideoTaskID: fixture.task.ID,
			EventType:   service.VideoStatusRunning,
			Message:     "poll committed",
			Payload:     map[string]any{"request_id": "poll-first"},
		})
		require.NoError(t, err)
		require.True(t, polled.Applied)
		require.Equal(t, expectedVersion+1, polled.Version)

		_, err = newVideoTaskFinalizerForIntegration(t).Finalize(ctx, newVideoTaskFinalizationInput(fixture.task, service.VideoStatusCancelled, "0"))
		require.ErrorIs(t, err, service.ErrVideoTaskTerminalConflict)
		var typed *service.VideoTaskTerminalConflictError
		require.True(t, errors.As(err, &typed))
		require.Equal(t, service.VideoStatusRunning, typed.CurrentStatus)
		require.Equal(t, expectedVersion+1, typed.CurrentVersion)

		var taskStatus, reservationStatus string
		var version int64
		var pollCount int
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status, version, poll_count FROM video_tasks WHERE id = $1", fixture.task.ID).Scan(&taskStatus, &version, &pollCount))
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM billing_reservations WHERE id = $1", fixture.reservation.ID).Scan(&reservationStatus))
		require.Equal(t, service.VideoStatusRunning, taskStatus)
		require.Equal(t, expectedVersion+1, version)
		require.Equal(t, 1, pollCount)
		require.Equal(t, service.BillingReservationStatusActive, reservationStatus)
		require.Equal(t, []string{service.VideoStatusRunning}, videoTaskFinalizationEventTypes(t, fixture.task.ID))
		require.Equal(t, 0, countVideoTaskFinalizationRows(t, "video_usage_logs", "video_task_id", fixture.task.ID))
		require.Equal(t, 0, countVideoTaskFinalizationRows(t, "billing_transactions", "source_id", fixture.task.ID))
	})
}

func TestVideoTaskPollCASEventFailureRollsBackTaskUpdate(t *testing.T) {
	ctx := context.Background()
	fixture := newVideoTaskFinalizationFixture(t, "3")
	markVideoTaskRunningForPollRace(t, fixture.task)
	constraint := installVideoTaskFinalizationFault(t, "event", fixture)
	candidate := *fixture.task
	candidate.PollCount++
	candidate.ResultURL = "https://provider.invalid/rolled-back-poll.mp4"

	_, err := newVideoTaskPollRepositoryForIntegration(t).UpdatePolledTaskCAS(ctx, fixture.task.Version, &candidate, &service.VideoTaskEvent{
		VideoTaskID: fixture.task.ID,
		EventType:   service.VideoStatusRunning,
		Message:     "poll event fault",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), constraint)
	assertVideoTaskFinalizationNoPartialState(t, fixture)
}

func TestVideoTaskCancellationFaultMatrixRollsBackEveryWrite(t *testing.T) {
	for _, fault := range []string{"task_cas", "event", "usage", "reservation", "ledger"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newVideoTaskFinalizationFixture(t, "3")
			constraint := installVideoTaskFinalizationFault(t, fault, fixture)
			finalizer := newVideoTaskFinalizerForIntegration(t)

			_, err := finalizer.Finalize(context.Background(), newVideoTaskFinalizationInput(fixture.task, service.VideoStatusCancelled, "0"))
			require.Error(t, err)
			require.Contains(t, err.Error(), constraint, "cancel fault must reach its intended SQL boundary")
			assertVideoTaskFinalizationNoPartialState(t, fixture)
		})
	}
}

func TestVideoTaskFinalizationSettlementReleaseOverrunAndDecimalProjection(t *testing.T) {
	ctx := context.Background()
	finalizer := newVideoTaskFinalizerForIntegration(t)

	t.Run("actual below reservation settles actual and releases the difference", func(t *testing.T) {
		fixture := newVideoTaskFinalizationFixture(t, "3")
		result, err := finalizer.Finalize(ctx, newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "1.25"))
		require.NoError(t, err)
		require.False(t, result.ReservationOverrun)
		assertVideoTaskFinalizationSettled(t, fixture, "1.2500000000", "8.75000000", true)
	})

	t.Run("actual over reservation still charges actual and queues warning", func(t *testing.T) {
		fixture := newVideoTaskFinalizationFixture(t, "1")
		result, err := finalizer.Finalize(ctx, newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "1.25"))
		require.NoError(t, err)
		require.True(t, result.ReservationOverrun)
		assertVideoTaskFinalizationSettled(t, fixture, "1.2500000000", "8.75000000", true)

		var metadata []byte
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT metadata FROM billing_transactions WHERE transaction_key = $1",
			fmt.Sprintf("video_task:%d:charge", fixture.task.ID),
		).Scan(&metadata))
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(metadata, &decoded))
		require.Equal(t, true, decoded["reservation_overrun"])
		require.Contains(t, videoTaskFinalizationOutboxTypes(t, fixture.task.ID), "billing.notify_reservation_overrun")
	})

	t.Run("users balance is rounded half up to eight places using decimal SQL", func(t *testing.T) {
		fixture := newVideoTaskFinalizationFixture(t, "2")
		input := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "1.2345678950")
		input.PricingSnapshot.AmountOriginal = service.MustUSD("1.2345678950")
		input.PricingSnapshot.ExchangeRate = "1.0000000000"
		_, err := finalizer.Finalize(ctx, input)
		require.NoError(t, err)
		assertVideoTaskFinalizationSettled(t, fixture, "1.2345678950", "8.76543211", true)

		var ledgerAmount string
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT amount_usd::text FROM billing_transactions WHERE transaction_key = $1",
			fmt.Sprintf("video_task:%d:charge", fixture.task.ID),
		).Scan(&ledgerAmount))
		require.Equal(t, "1.2345678950", ledgerAmount)
	})

	for _, status := range []string{service.VideoStatusFailed, service.VideoStatusCancelled} {
		t.Run(status+" releases reservation without charge", func(t *testing.T) {
			fixture := newVideoTaskFinalizationFixture(t, "3")
			input := newVideoTaskFinalizationInput(fixture.task, status, "99")
			_, err := finalizer.Finalize(ctx, input)
			require.NoError(t, err)

			var reservationStatus, settledAmount string
			require.NoError(t, integrationDB.QueryRowContext(ctx,
				"SELECT status, settled_amount_usd::text FROM billing_reservations WHERE id = $1",
				fixture.reservation.ID,
			).Scan(&reservationStatus, &settledAmount))
			require.Equal(t, service.BillingReservationStatusReleased, reservationStatus)
			require.Equal(t, "0.0000000000", settledAmount)
			require.Equal(t, 0, videoTaskFinalizationTransactionCount(t, fixture.task.ID, "charge"))
			require.Equal(t, 1, videoTaskFinalizationTransactionCount(t, fixture.task.ID, "release"))
			require.Equal(t, "10.00000000", videoTaskFinalizationBalance(t, fixture.user.ID))

			var chargedAt sql.NullTime
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance_charged_at FROM video_tasks WHERE id = $1", fixture.task.ID).Scan(&chargedAt))
			require.False(t, chargedAt.Valid)
		})
	}
}

func TestVideoTaskFinalizationMockSkipsReservationAndChargeButQueuesDelivery(t *testing.T) {
	ctx := context.Background()
	user := newVideoTaskCreationUser(t, 10)
	gatewayRepo := NewVideoGatewayRepository(integrationDB)
	provider := &service.VideoProviderAccount{
		Provider:           service.VideoProviderMock,
		DisplayName:        "finalization mock " + uuid.NewString(),
		Enabled:            true,
		BaseURL:            "mock://video-finalization",
		DefaultModel:       "mock-video-v1",
		RateLimitPerMinute: 1,
	}
	require.NoError(t, gatewayRepo.CreateProviderAccount(ctx, provider))
	task := &service.VideoTask{
		ProviderAccountID:   provider.ID,
		Provider:            service.VideoProviderMock,
		Model:               provider.DefaultModel,
		TaskType:            service.VideoTaskTypeTextToVideo,
		Prompt:              "offline mock finalization",
		AspectRatio:         "16:9",
		Duration:            5,
		Resolution:          "720p",
		Status:              service.VideoStatusRunning,
		CreatedBy:           user.ID,
		CreationKey:         service.HashIdempotencyKey("mock-finalize-" + uuid.NewString()),
		CreationFingerprint: service.HashIdempotencyKey("mock-finalize-payload"),
		Version:             4,
		DispatchState:       service.VideoDispatchStateNotRequired,
		SettlementStatus:    service.VideoSettlementStatusPending,
		ArchiveStatus:       service.VideoSideEffectStatusPending,
		CaptureStatus:       service.VideoSideEffectStatusPending,
	}
	require.NoError(t, gatewayRepo.CreateTask(ctx, task))
	zero := service.MustUSD("0")
	input := newVideoTaskFinalizationInput(task, service.VideoStatusSucceeded, "0")
	input.PricingSnapshot = service.PricingSnapshot{
		AmountOriginal: zero,
		ExchangeRate:   "1.0000000000",
		PricingSource:  service.PricingSourceFallback,
		PricingVersion: "mock-v1",
	}

	result, err := newVideoTaskFinalizerForIntegration(t).Finalize(ctx, input)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Equal(t, 0, countVideoTaskFinalizationRows(t, "billing_reservations", "source_id", task.ID))
	require.Equal(t, 0, countVideoTaskFinalizationRows(t, "billing_transactions", "source_id", task.ID))
	require.Equal(t, "10.00000000", videoTaskFinalizationBalance(t, user.ID))
	require.Equal(t, []string{"video.archive_asset", "video.capture_content"}, videoTaskFinalizationOutboxTypes(t, task.ID))

	var settlement string
	var chargedAt sql.NullTime
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT settlement_status, balance_charged_at FROM video_tasks WHERE id = $1",
		task.ID,
	).Scan(&settlement, &chargedAt))
	require.Equal(t, service.VideoSettlementStatusNotNeeded, settlement)
	require.False(t, chargedAt.Valid)
}

func TestVideoTaskFinalizationMockReservationAnomalyRequiresReviewWithoutFinancialMutation(t *testing.T) {
	ctx := context.Background()
	fixture := newMockVideoTaskFinalizationFixtureWithReservation(t)
	finalizer := newVideoTaskFinalizerForIntegration(t)
	input := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "0")
	input.PricingSnapshot.AmountOriginal = service.MustUSD("0")
	input.PricingSnapshot.ExchangeRate = "1.0000000000"

	first, err := finalizer.Finalize(ctx, input)
	require.NoError(t, err)
	require.True(t, first.Applied)
	replayed, err := finalizer.Finalize(ctx, input)
	require.NoError(t, err)
	require.True(t, replayed.Idempotent)

	var taskStatus, settlementStatus, reservationStatus, settledAmount string
	var chargedAt sql.NullTime
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT status, settlement_status, balance_charged_at
		FROM video_tasks WHERE id = $1
	`, fixture.task.ID).Scan(&taskStatus, &settlementStatus, &chargedAt))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT status, settled_amount_usd::text
		FROM billing_reservations WHERE id = $1
	`, fixture.reservation.ID).Scan(&reservationStatus, &settledAmount))
	require.Equal(t, service.VideoStatusSucceeded, taskStatus)
	require.Equal(t, service.VideoSettlementStatusNotNeeded, settlementStatus)
	require.False(t, chargedAt.Valid)
	require.Equal(t, service.BillingReservationStatusReviewRequired, reservationStatus)
	require.Equal(t, "0.0000000000", settledAmount)
	require.Equal(t, "10.00000000", videoTaskFinalizationBalance(t, fixture.user.ID))
	require.Equal(t, 0, countVideoTaskFinalizationRows(t, "billing_transactions", "source_id", fixture.task.ID))
	require.Equal(t, []string{
		"billing.reservation_review_required",
		"video.archive_asset",
		"video.capture_content",
	}, videoTaskFinalizationOutboxTypes(t, fixture.task.ID))

	var warningCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM domain_outbox WHERE dedup_key = $1",
		fmt.Sprintf("video_task:%d:reservation_review_required", fixture.task.ID),
	).Scan(&warningCount))
	require.Equal(t, 1, warningCount)
}

func TestVideoTaskFinalizationMockReservationReviewWarningFailureRollsBack(t *testing.T) {
	fixture := newMockVideoTaskFinalizationFixtureWithReservation(t)
	constraint := installVideoTaskFinalizationFault(t, "outbox", fixture)
	input := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "0")
	input.PricingSnapshot.AmountOriginal = service.MustUSD("0")
	input.PricingSnapshot.ExchangeRate = "1.0000000000"

	_, err := newVideoTaskFinalizerForIntegration(t).Finalize(context.Background(), input)
	require.Error(t, err)
	require.Contains(t, err.Error(), constraint)
	assertVideoTaskFinalizationNoPartialState(t, fixture)
}

func TestVideoTaskFinalizationMockCancelledReservationAnomalyStillWarnsReview(t *testing.T) {
	ctx := context.Background()
	fixture := newMockVideoTaskFinalizationFixtureWithReservation(t)
	input := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusCancelled, "0")

	_, err := newVideoTaskFinalizerForIntegration(t).Finalize(ctx, input)
	require.NoError(t, err)
	var taskSettlement, reservationStatus string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT settlement_status FROM video_tasks WHERE id = $1", fixture.task.ID).Scan(&taskSettlement))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM billing_reservations WHERE id = $1", fixture.reservation.ID).Scan(&reservationStatus))
	require.Equal(t, service.VideoSettlementStatusNotNeeded, taskSettlement)
	require.Equal(t, service.BillingReservationStatusReviewRequired, reservationStatus)
	require.Equal(t, 0, countVideoTaskFinalizationRows(t, "billing_transactions", "source_id", fixture.task.ID))
	require.Equal(t, "10.00000000", videoTaskFinalizationBalance(t, fixture.user.ID))
	require.Equal(t, []string{"billing.reservation_review_required"}, videoTaskFinalizationOutboxTypes(t, fixture.task.ID))
}

func TestVideoTaskFinalizationMockMismatchedReservationOwnerStillRequiresReview(t *testing.T) {
	ctx := context.Background()
	fixture := newMockVideoTaskFinalizationFixtureWithReservation(t)
	otherUser := newVideoTaskCreationUser(t, 7)
	_, err := integrationDB.ExecContext(ctx, "UPDATE video_tasks SET created_by = $1 WHERE id = $2", otherUser.ID, fixture.task.ID)
	require.NoError(t, err)
	fixture.task.CreatedBy = otherUser.ID
	input := newVideoTaskFinalizationInput(fixture.task, service.VideoStatusSucceeded, "0")
	input.PricingSnapshot.AmountOriginal = service.MustUSD("0")
	input.PricingSnapshot.ExchangeRate = "1.0000000000"

	_, err = newVideoTaskFinalizerForIntegration(t).Finalize(ctx, input)
	require.NoError(t, err)
	var reservationStatus string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM billing_reservations WHERE id = $1", fixture.reservation.ID).Scan(&reservationStatus))
	require.Equal(t, service.BillingReservationStatusReviewRequired, reservationStatus)
	require.Equal(t, 0, countVideoTaskFinalizationRows(t, "billing_transactions", "source_id", fixture.task.ID))
	require.Equal(t, "10.00000000", videoTaskFinalizationBalance(t, fixture.user.ID))
	require.Equal(t, "7.00000000", videoTaskFinalizationBalance(t, otherUser.ID))
	require.Contains(t, videoTaskFinalizationOutboxTypes(t, fixture.task.ID), "billing.reservation_review_required")
}

func newVideoTaskFinalizerForIntegration(t *testing.T) *service.VideoTaskFinalizer {
	t.Helper()
	gatewayRepo := NewVideoGatewayRepository(integrationDB)
	finalizationRepo, ok := gatewayRepo.(service.VideoTaskFinalizationRepository)
	require.True(t, ok, "video gateway repository must implement VideoTaskFinalizationRepository")
	return service.NewVideoTaskFinalizer(finalizationRepo)
}

func newVideoTaskPollRepositoryForIntegration(t *testing.T) service.VideoTaskPollRepository {
	t.Helper()
	gatewayRepo := NewVideoGatewayRepository(integrationDB)
	pollRepo, ok := gatewayRepo.(service.VideoTaskPollRepository)
	require.True(t, ok, "video gateway repository must implement VideoTaskPollRepository")
	return pollRepo
}

func markVideoTaskRunningForPollRace(t *testing.T, task *service.VideoTask) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), "UPDATE video_tasks SET status = $2 WHERE id = $1", task.ID, service.VideoStatusRunning)
	require.NoError(t, err)
	task.Status = service.VideoStatusRunning
}

func newVideoTaskFinalizationFixture(t *testing.T, reservedAmount string) videoTaskFinalizationFixture {
	t.Helper()
	user := newVideoTaskCreationUser(t, 10)
	providerID := newVideoTaskCreationProvider(t)
	input := newVideoTaskCreationInput(
		user.ID,
		providerID,
		service.HashIdempotencyKey("finalize-create-"+uuid.NewString()),
		service.HashIdempotencyKey("finalize-payload-"+uuid.NewString()),
		reservedAmount,
	)
	created, err := NewVideoTaskCreationRepository(integrationDB).CreateWithReservation(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, created.Reservation)
	return videoTaskFinalizationFixture{user: user, task: created.Task, reservation: created.Reservation}
}

func newMockVideoTaskFinalizationFixtureWithReservation(t *testing.T) videoTaskFinalizationFixture {
	t.Helper()
	ctx := context.Background()
	user := newVideoTaskCreationUser(t, 10)
	reservation, err := NewBillingReservationRepository(integrationDB).Reserve(ctx, &service.BillingReservation{
		ReservationKey:    "mock-finalization-anomaly-" + uuid.NewString(),
		SourceType:        "video_task",
		SourceID:          time.Now().UnixNano(),
		UserID:            user.ID,
		ReservedAmountUSD: service.MustUSD("3"),
		Status:            service.BillingReservationStatusActive,
		ExpiresAt:         time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)
	gatewayRepo := NewVideoGatewayRepository(integrationDB)
	provider := &service.VideoProviderAccount{Provider: service.VideoProviderMock, DisplayName: "mock anomaly " + uuid.NewString(), Enabled: true, BaseURL: "mock://video", DefaultModel: "mock-video-v1", RateLimitPerMinute: 1}
	require.NoError(t, gatewayRepo.CreateProviderAccount(ctx, provider))
	task := &service.VideoTask{
		ProviderAccountID:   provider.ID,
		Provider:            service.VideoProviderMock,
		Model:               provider.DefaultModel,
		TaskType:            service.VideoTaskTypeTextToVideo,
		Prompt:              "mock reservation anomaly",
		AspectRatio:         "16:9",
		Duration:            5,
		Resolution:          "720p",
		Status:              service.VideoStatusRunning,
		CreatedBy:           user.ID,
		CreationKey:         service.HashIdempotencyKey("mock-anomaly-" + uuid.NewString()),
		CreationFingerprint: service.HashIdempotencyKey("mock-anomaly-payload"),
		ReservationID:       &reservation.ID,
		Version:             4,
		DispatchState:       service.VideoDispatchStateNotRequired,
		SettlementStatus:    service.VideoSettlementStatusPending,
		ArchiveStatus:       service.VideoSideEffectStatusPending,
		CaptureStatus:       service.VideoSideEffectStatusPending,
	}
	require.NoError(t, gatewayRepo.CreateTask(ctx, task))
	_, err = integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET source_id = $1 WHERE id = $2", task.ID, reservation.ID)
	require.NoError(t, err)
	reservation.SourceID = task.ID
	return videoTaskFinalizationFixture{user: user, task: task, reservation: reservation}
}

func newVideoTaskFinalizationInput(task *service.VideoTask, status, actualUSD string) service.VideoTaskFinalizationInput {
	original, err := service.NewMoney("9", service.Currency("CNY"))
	if err != nil {
		panic(err)
	}
	duration := 12
	tokens := int64(987)
	return service.VideoTaskFinalizationInput{
		TaskID:               task.ID,
		ExpectedVersion:      task.Version,
		TerminalStatus:       status,
		ProviderResultURL:    "https://provider.invalid/result.mp4",
		ProviderErrorMessage: "offline provider failure",
		ProviderPayload:      map[string]any{"request_id": "offline-finalize"},
		ActualDuration:       &duration,
		ActualResolution:     "1080p",
		ActualTokens:         &tokens,
		LastFrameURL:         "https://provider.invalid/last-frame.png",
		PollCount:            5,
		ActualCostUSD:        service.MustUSD(actualUSD),
		PricingSnapshot: service.PricingSnapshot{
			AmountOriginal: original,
			ExchangeRate:   "7.2000000000",
			PricingSource:  service.PricingSourceProviderUsage,
			PricingVersion: "finalization-test-v1",
		},
		CompletedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func installVideoTaskFinalizationFault(t *testing.T, fault string, fixture videoTaskFinalizationFixture) string {
	t.Helper()
	constraint := fmt.Sprintf("finalization_%s_%d", fault, fixture.task.ID)
	var table, expression string
	switch fault {
	case "task_cas":
		table = "video_tasks"
		expression = fmt.Sprintf("id <> %d OR status = 'queued'", fixture.task.ID)
	case "event":
		table = "video_task_events"
		expression = fmt.Sprintf("video_task_id <> %d", fixture.task.ID)
	case "usage":
		table = "video_usage_logs"
		expression = fmt.Sprintf("video_task_id <> %d", fixture.task.ID)
	case "reservation":
		table = "billing_reservations"
		expression = fmt.Sprintf("id <> %d OR status = 'active'", fixture.reservation.ID)
	case "ledger":
		table = "billing_transactions"
		expression = fmt.Sprintf("source_id <> %d", fixture.task.ID)
	case "balance":
		table = "users"
		expression = fmt.Sprintf("id <> %d OR balance = 10.00000000", fixture.user.ID)
	case "outbox":
		table = "domain_outbox"
		expression = fmt.Sprintf("aggregate_id <> %d", fixture.task.ID)
	default:
		t.Fatalf("unknown finalization fault %q", fault)
	}
	_, err := integrationDB.ExecContext(context.Background(), fmt.Sprintf(
		"ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s) NOT VALID",
		table,
		constraint,
		expression,
	))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, dropErr := integrationDB.ExecContext(context.Background(), fmt.Sprintf(
			"ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s",
			table,
			constraint,
		))
		require.NoError(t, dropErr)
	})
	return constraint
}

func assertVideoTaskFinalizationNoPartialState(t *testing.T, fixture videoTaskFinalizationFixture) {
	t.Helper()
	ctx := context.Background()
	var status, settlement string
	var version int64
	var chargedAt sql.NullTime
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT status, version, settlement_status, balance_charged_at FROM video_tasks WHERE id = $1",
		fixture.task.ID,
	).Scan(&status, &version, &settlement, &chargedAt))
	require.Equal(t, fixture.task.Status, status)
	require.Equal(t, fixture.task.Version, version)
	require.Equal(t, service.VideoSettlementStatusPending, settlement)
	require.False(t, chargedAt.Valid)
	require.Equal(t, 0, countVideoTaskFinalizationRows(t, "video_task_events", "video_task_id", fixture.task.ID))
	require.Equal(t, 0, countVideoTaskFinalizationRows(t, "video_usage_logs", "video_task_id", fixture.task.ID))
	require.Equal(t, 0, countVideoTaskFinalizationRows(t, "billing_transactions", "source_id", fixture.task.ID))
	require.Equal(t, 0, countVideoTaskFinalizationRows(t, "domain_outbox", "aggregate_id", fixture.task.ID))
	require.Equal(t, "10.00000000", videoTaskFinalizationBalance(t, fixture.user.ID))

	var reservationStatus, settledAmount string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT status, settled_amount_usd::text FROM billing_reservations WHERE id = $1",
		fixture.reservation.ID,
	).Scan(&reservationStatus, &settledAmount))
	require.Equal(t, service.BillingReservationStatusActive, reservationStatus)
	require.Equal(t, "0.0000000000", settledAmount)
}

func assertVideoTaskFinalizationSettled(t *testing.T, fixture videoTaskFinalizationFixture, settledAmount, balance string, charged bool) {
	t.Helper()
	ctx := context.Background()
	var reservationStatus, actualSettled string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT status, settled_amount_usd::text FROM billing_reservations WHERE id = $1",
		fixture.reservation.ID,
	).Scan(&reservationStatus, &actualSettled))
	require.Equal(t, service.BillingReservationStatusSettled, reservationStatus)
	require.Equal(t, settledAmount, actualSettled)
	require.Equal(t, balance, videoTaskFinalizationBalance(t, fixture.user.ID))

	var taskStatus, settlementStatus string
	var chargedAt sql.NullTime
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT status, settlement_status, balance_charged_at FROM video_tasks WHERE id = $1",
		fixture.task.ID,
	).Scan(&taskStatus, &settlementStatus, &chargedAt))
	require.Equal(t, service.VideoStatusSucceeded, taskStatus)
	require.Equal(t, service.BillingReservationStatusSettled, settlementStatus)
	require.Equal(t, charged, chargedAt.Valid)
}

func countVideoTaskFinalizationRows(t *testing.T, table, column string, value int64) int {
	t.Helper()
	allowed := map[string]map[string]bool{
		"video_task_events":    {"video_task_id": true},
		"video_usage_logs":     {"video_task_id": true},
		"billing_reservations": {"source_id": true},
		"billing_transactions": {"source_id": true},
		"domain_outbox":        {"aggregate_id": true},
	}
	require.True(t, allowed[table][column], "unsafe finalization count query target")
	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", table, column),
		value,
	).Scan(&count))
	return count
}

func videoTaskFinalizationTransactionCount(t *testing.T, taskID int64, kind string) int {
	t.Helper()
	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM billing_transactions
		WHERE source_type = 'video_task' AND source_id = $1 AND transaction_kind = $2
	`, taskID, kind).Scan(&count))
	return count
}

func videoTaskFinalizationBalance(t *testing.T, userID int64) string {
	t.Helper()
	var balance string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT balance::text FROM users WHERE id = $1",
		userID,
	).Scan(&balance))
	return balance
}

func videoTaskFinalizationOutboxTypes(t *testing.T, taskID int64) []string {
	t.Helper()
	rows, err := integrationDB.QueryContext(context.Background(),
		"SELECT event_type FROM domain_outbox WHERE aggregate_type = 'video_task' AND aggregate_id = $1",
		taskID,
	)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var items []string
	for rows.Next() {
		var item string
		require.NoError(t, rows.Scan(&item))
		items = append(items, item)
	}
	require.NoError(t, rows.Err())
	sort.Strings(items)
	return items
}

func videoTaskFinalizationEventTypes(t *testing.T, taskID int64) []string {
	t.Helper()
	rows, err := integrationDB.QueryContext(context.Background(), "SELECT event_type FROM video_task_events WHERE video_task_id = $1 ORDER BY id", taskID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var items []string
	for rows.Next() {
		var item string
		require.NoError(t, rows.Scan(&item))
		items = append(items, item)
	}
	require.NoError(t, rows.Err())
	return items
}
