//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestVideoTaskCreationAtomicReservationReplayAndConflict(t *testing.T) {
	ctx := context.Background()
	repo := NewVideoTaskCreationRepository(integrationDB)
	user := newVideoTaskCreationUser(t, 10)
	providerID := newVideoTaskCreationProvider(t)
	creationKey := service.HashIdempotencyKey("video-create-" + uuid.NewString())
	input := newVideoTaskCreationInput(user.ID, providerID, creationKey, service.HashIdempotencyKey("payload-a"), "3")

	first, err := repo.CreateWithReservation(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, first.Task)
	require.NotNil(t, first.Reservation)
	require.NotNil(t, first.Task.ReservationID)
	require.Equal(t, first.Reservation.ID, *first.Task.ReservationID)
	require.Equal(t, first.Task.ID, first.Reservation.SourceID)
	require.Equal(t, "video_task:create:"+creationKey, first.Reservation.ReservationKey)
	require.Equal(t, "3.0000000000", first.Reservation.ReservedAmountUSD.String())
	require.Equal(t, 21.6, first.Task.CostEstimate)
	require.Equal(t, service.BillingCurrencyCNY, first.Task.Currency)
	require.Equal(t, service.PricingSourceProviderUsage, first.Task.PricingSource)
	require.Equal(t, service.VideoPricingVersionSeedance202603, first.Task.PricingVersion)
	var storedCost string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT cost_estimate::text FROM video_tasks WHERE id = $1", first.Task.ID).Scan(&storedCost))
	require.Equal(t, "21.600000", storedCost)

	replayed, err := repo.CreateWithReservation(ctx, input)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, first.Task.ID, replayed.Task.ID)
	require.Equal(t, first.Reservation.ID, replayed.Reservation.ID)

	conflict := input
	conflict.Task = cloneVideoTaskForCreationTest(input.Task)
	conflict.Task.Prompt = "different payload"
	conflict.Task.CreationFingerprint = service.HashIdempotencyKey("payload-b")
	_, err = repo.CreateWithReservation(ctx, conflict)
	require.ErrorIs(t, err, service.ErrIdempotencyKeyConflict)

	require.Equal(t, 1, countVideoTaskCreationRows(t, "video_tasks", "creation_key", creationKey))
	require.Equal(t, 1, countVideoTaskCreationRows(t, "billing_reservations", "reservation_key", "video_task:create:"+creationKey))
}

func TestVideoTaskCreationInsufficientBalanceAndTaskFailureLeaveNoPartialRows(t *testing.T) {
	ctx := context.Background()
	repo := NewVideoTaskCreationRepository(integrationDB)

	t.Run("insufficient balance", func(t *testing.T) {
		user := newVideoTaskCreationUser(t, 1)
		providerID := newVideoTaskCreationProvider(t)
		creationKey := service.HashIdempotencyKey("video-create-" + uuid.NewString())
		input := newVideoTaskCreationInput(user.ID, providerID, creationKey, service.HashIdempotencyKey("payload"), "2")

		_, err := repo.CreateWithReservation(ctx, input)
		require.ErrorIs(t, err, service.ErrBillingReservationInsufficientBalance)
		require.Zero(t, countVideoTaskCreationRows(t, "video_tasks", "creation_key", creationKey))
		require.Zero(t, countVideoTaskCreationRows(t, "billing_reservations", "reservation_key", "video_task:create:"+creationKey))
	})

	t.Run("task insert failure rolls back reservation", func(t *testing.T) {
		user := newVideoTaskCreationUser(t, 10)
		creationKey := service.HashIdempotencyKey("video-create-" + uuid.NewString())
		input := newVideoTaskCreationInput(user.ID, 9_999_999_999, creationKey, service.HashIdempotencyKey("payload"), "2")

		_, err := repo.CreateWithReservation(ctx, input)
		require.Error(t, err)
		require.Zero(t, countVideoTaskCreationRows(t, "video_tasks", "creation_key", creationKey))
		require.Zero(t, countVideoTaskCreationRows(t, "billing_reservations", "reservation_key", "video_task:create:"+creationKey))
	})
}

func TestVideoTaskCreationConcurrentReservationsCannotOversubscribe(t *testing.T) {
	ctx := context.Background()
	repo := NewVideoTaskCreationRepository(integrationDB)
	user := newVideoTaskCreationUser(t, 10)
	providerID := newVideoTaskCreationProvider(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			key := service.HashIdempotencyKey(fmt.Sprintf("video-create-%s-%d", uuid.NewString(), index))
			input := newVideoTaskCreationInput(user.ID, providerID, key, service.HashIdempotencyKey(fmt.Sprintf("payload-%d", index)), "7")
			_, err := repo.CreateWithReservation(ctx, input)
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var succeeded, insufficient int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, service.ErrBillingReservationInsufficientBalance):
			insufficient++
		default:
			t.Fatalf("unexpected create result: %v", err)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, insufficient)

	var activeTotal string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(reserved_amount_usd), 0)::text
		FROM billing_reservations
		WHERE user_id = $1 AND status = 'active'
	`, user.ID).Scan(&activeTotal))
	require.Equal(t, "7.0000000000", activeTotal)
}

func TestVideoTaskCreationConcurrentSameKeyReturnsSameTaskAndReservation(t *testing.T) {
	ctx := context.Background()
	repo := NewVideoTaskCreationRepository(integrationDB)
	user := newVideoTaskCreationUser(t, 10)
	providerID := newVideoTaskCreationProvider(t)
	key := service.HashIdempotencyKey("video-create-" + uuid.NewString())
	base := newVideoTaskCreationInput(user.ID, providerID, key, service.HashIdempotencyKey("same-payload"), "3")
	type outcome struct {
		result *service.VideoTaskCreationResult
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
			input := base
			input.Task = cloneVideoTaskForCreationTest(base.Task)
			result, err := repo.CreateWithReservation(ctx, input)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)

	results := make([]*service.VideoTaskCreationResult, 0, 2)
	for item := range outcomes {
		require.NoError(t, item.err)
		require.NotNil(t, item.result)
		results = append(results, item.result)
	}
	require.Len(t, results, 2)
	require.Equal(t, results[0].Task.ID, results[1].Task.ID)
	require.Equal(t, results[0].Reservation.ID, results[1].Reservation.ID)
}

func TestVideoTaskCreationKeepsDailyTrialGateInsideAtomicCreate(t *testing.T) {
	ctx := context.Background()
	repo := NewVideoTaskCreationRepository(integrationDB)
	user := newVideoTaskCreationUser(t, 20)
	providerID := newVideoTaskCreationProvider(t)
	trialDate := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

	firstKey := service.HashIdempotencyKey("video-create-" + uuid.NewString())
	first := newVideoTaskCreationInput(user.ID, providerID, firstKey, service.HashIdempotencyKey("trial-a"), "2")
	first.DailyTrialProvider = service.VideoProviderSeedance
	first.DailyTrialDate = trialDate
	_, err := repo.CreateWithReservation(ctx, first)
	require.NoError(t, err)

	secondKey := service.HashIdempotencyKey("video-create-" + uuid.NewString())
	second := newVideoTaskCreationInput(user.ID, providerID, secondKey, service.HashIdempotencyKey("trial-b"), "2")
	second.DailyTrialProvider = service.VideoProviderSeedance
	second.DailyTrialDate = trialDate
	_, err = repo.CreateWithReservation(ctx, second)
	require.ErrorIs(t, err, service.ErrVideoTrialLimitExceeded)
	require.Zero(t, countVideoTaskCreationRows(t, "video_tasks", "creation_key", secondKey))
	require.Zero(t, countVideoTaskCreationRows(t, "billing_reservations", "reservation_key", "video_task:create:"+secondKey))

	var dailyReservations int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM video_daily_trial_reservations
		WHERE provider = $1 AND created_by = $2 AND trial_date = $3
	`, service.VideoProviderSeedance, user.ID, trialDate).Scan(&dailyReservations))
	require.Equal(t, 1, dailyReservations)
}

func TestVideoTaskCreationReplayExistingSupportsMockWithoutReservation(t *testing.T) {
	ctx := context.Background()
	user := newVideoTaskCreationUser(t, 10)
	providerRepo := NewVideoGatewayRepository(integrationDB)
	provider := &service.VideoProviderAccount{Provider: service.VideoProviderMock, DisplayName: "mock replay " + uuid.NewString(), Enabled: true, BaseURL: "mock://video", DefaultModel: "mock-video-v1", RateLimitPerMinute: 1}
	require.NoError(t, providerRepo.CreateProviderAccount(ctx, provider))
	key := service.HashIdempotencyKey("mock-create-" + uuid.NewString())
	fingerprint := service.HashIdempotencyKey("mock-payload")
	task := &service.VideoTask{ProviderAccountID: provider.ID, Provider: service.VideoProviderMock, Model: "mock-video-v1", TaskType: service.VideoTaskTypeTextToVideo, Prompt: "mock idempotency", Status: service.VideoStatusQueued, CreatedBy: user.ID, CreationKey: key, CreationFingerprint: fingerprint, Version: 1}
	require.NoError(t, providerRepo.CreateTask(ctx, task))

	replayRepo := NewVideoTaskCreationRepository(integrationDB).(service.VideoTaskCreationReplayRepository)
	result, found, err := replayRepo.ReplayExisting(ctx, service.VideoTaskCreationReplayInput{CreationKey: key, CreationFingerprint: fingerprint, CreatedBy: user.ID})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, task.ID, result.Task.ID)
	require.Nil(t, result.Reservation)
}

func newVideoTaskCreationUser(t *testing.T, balance float64) *service.User {
	t.Helper()
	return mustCreateUser(t, testEntClient(t), &service.User{
		Email:        fmt.Sprintf("video-creation-%s@example.com", uuid.NewString()),
		PasswordHash: "hash",
		Balance:      balance,
	})
}

func newVideoTaskCreationProvider(t *testing.T) int64 {
	return newVideoTaskCreationProviderFixture(t, "")
}

func newConfiguredVideoTaskCreationProvider(t *testing.T) int64 {
	return newVideoTaskCreationProviderFixture(t, "offline-test-placeholder")
}

func newVideoTaskCreationProviderFixture(t *testing.T, encryptedAPIKey string) int64 {
	t.Helper()
	repo := NewVideoGatewayRepository(integrationDB)
	displayName := "creation integration fake " + uuid.NewString()
	provider := &service.VideoProviderAccount{
		Provider:           service.VideoProviderSeedance,
		DisplayName:        displayName,
		Enabled:            true,
		EncryptedAPIKey:    encryptedAPIKey,
		BaseURL:            "https://provider.invalid",
		DefaultModel:       "doubao-seedance-2-0-260128",
		RateLimitPerMinute: 1,
		Metadata:           map[string]any{"test_only": true},
	}
	require.NoError(t, repo.CreateProviderAccount(context.Background(), provider))
	return provider.ID
}

func newVideoTaskCreationInput(userID, providerID int64, creationKey, fingerprint, amount string) service.VideoTaskCreationInput {
	original, err := service.NewMoney("21.6", service.Currency("CNY"))
	if err != nil {
		panic(err)
	}
	return service.VideoTaskCreationInput{
		Task: &service.VideoTask{
			ProviderAccountID:   providerID,
			ProviderAccountName: "creation integration fake",
			Provider:            service.VideoProviderSeedance,
			Model:               "doubao-seedance-2-0-260128",
			TaskType:            service.VideoTaskTypeTextToVideo,
			Prompt:              "atomic create",
			AspectRatio:         "16:9",
			Duration:            5,
			Resolution:          "720p",
			Status:              service.VideoStatusQueued,
			CreatedBy:           userID,
			CreationKey:         creationKey,
			CreationFingerprint: fingerprint,
			Version:             1,
			DispatchState:       "pending",
			SettlementStatus:    "pending",
			ArchiveStatus:       "pending",
			CaptureStatus:       "pending",
		},
		ReservedAmountUSD:    service.MustUSD(amount),
		PricingSnapshot:      service.PricingSnapshot{AmountOriginal: original, ExchangeRate: "7.2000000000", PricingSource: service.PricingSourceProviderUsage, PricingVersion: service.VideoPricingVersionSeedance202603},
		ReservationExpiresAt: time.Now().UTC().Add(6 * time.Hour),
	}
}

func cloneVideoTaskForCreationTest(input *service.VideoTask) *service.VideoTask {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Content = append([]service.VideoTaskContentItem(nil), input.Content...)
	return &cloned
}

func countVideoTaskCreationRows(t *testing.T, table, column, value string) int {
	t.Helper()
	allowed := map[string]map[string]bool{
		"video_tasks":          {"creation_key": true},
		"billing_reservations": {"reservation_key": true},
	}
	require.True(t, allowed[table][column], "unsafe count query target")
	var count int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", table, column), value,
	).Scan(&count))
	return count
}
