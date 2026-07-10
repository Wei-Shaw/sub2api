//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBillingReservationRepositoryReserve_ConcurrencyFailClosedAndIdempotency(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewBillingReservationRepository(integrationDB)

	newUser := func(t *testing.T, balance float64) *service.User {
		t.Helper()
		return mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("reservation-%s@example.com", uuid.NewString()),
			PasswordHash: "hash",
			Balance:      balance,
		})
	}
	newReservation := func(userID int64, key string, sourceID int64, amount string) *service.BillingReservation {
		return &service.BillingReservation{
			ReservationKey:    key,
			SourceType:        "video_task",
			SourceID:          sourceID,
			UserID:            userID,
			ReservedAmountUSD: service.MustUSD(amount),
			Status:            service.BillingReservationStatusActive,
			ExpiresAt:         time.Now().UTC().Add(time.Hour),
		}
	}

	t.Run("concurrent reservations cannot consume the same balance", func(t *testing.T) {
		user := newUser(t, 10)
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, err := repo.Reserve(ctx, newReservation(user.ID, fmt.Sprintf("reservation-%s", uuid.NewString()), int64(i+1), "7"))
				results <- err
			}(i)
		}
		close(start)
		wg.Wait()
		close(results)

		var successes, insufficient int
		for err := range results {
			switch {
			case err == nil:
				successes++
			case errorIs(err, service.ErrBillingReservationInsufficientBalance):
				insufficient++
			default:
				t.Fatalf("unexpected reservation result: %v", err)
			}
		}
		require.Equal(t, 1, successes)
		require.Equal(t, 1, insufficient)

		var activeTotal string
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(reserved_amount_usd), 0)::text
			FROM billing_reservations
			WHERE user_id = $1 AND status = 'active'
		`, user.ID).Scan(&activeTotal))
		require.Equal(t, "7.0000000000", activeTotal)
	})

	t.Run("unknown balance fails closed", func(t *testing.T) {
		key := "reservation-" + uuid.NewString()
		_, err := repo.Reserve(ctx, newReservation(9_999_999_999, key, 101, "1"))
		require.ErrorIs(t, err, service.ErrBillingReservationBalanceUnavailable)

		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM billing_reservations WHERE reservation_key = $1", key).Scan(&count))
		require.Zero(t, count)
	})

	t.Run("same key replays original and conflicting fields are rejected", func(t *testing.T) {
		user := newUser(t, 20)
		key := "reservation-" + uuid.NewString()
		input := newReservation(user.ID, key, 201, "3.1234567890")

		first, err := repo.Reserve(ctx, input)
		require.NoError(t, err)
		second, err := repo.Reserve(ctx, input)
		require.NoError(t, err)
		require.Equal(t, first.ID, second.ID)
		require.Equal(t, "3.1234567890", second.ReservedAmountUSD.String())

		conflicting := newReservation(user.ID, key, 201, "4")
		_, err = repo.Reserve(ctx, conflicting)
		require.ErrorIs(t, err, service.ErrBillingReservationConflict)

		byID, err := repo.GetByID(ctx, first.ID)
		require.NoError(t, err)
		byKey, err := repo.GetByKey(ctx, key)
		require.NoError(t, err)
		require.Equal(t, first.ID, byID.ID)
		require.Equal(t, first.ID, byKey.ID)

		_, err = integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET expires_at = $1 WHERE id = $2", time.Now().UTC().Add(-time.Minute), first.ID)
		require.NoError(t, err)
		expired, err := repo.ListExpired(ctx, time.Now().UTC(), 10)
		require.NoError(t, err)
		require.Contains(t, reservationIDs(expired), first.ID)
	})

	t.Run("same key replays across PostgreSQL half microsecond boundary", func(t *testing.T) {
		user := newUser(t, 20)
		key := "reservation-half-microsecond-" + uuid.NewString()
		input := newReservation(user.ID, key, 301, "2")
		input.ExpiresAt = time.Unix(1_700_000_000, 123_456_789).UTC()

		first, err := repo.Reserve(ctx, input)
		require.NoError(t, err)
		replayed, err := repo.Reserve(ctx, input)
		require.NoError(t, err)
		require.Equal(t, first.ID, replayed.ID)
		require.Equal(t, input.ExpiresAt.UTC().Truncate(time.Microsecond), replayed.ExpiresAt)
	})

	t.Run("balance query failure fails closed without a reservation", func(t *testing.T) {
		user := newUser(t, 20)
		key := "reservation-query-failure-" + uuid.NewString()
		blocker, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = blocker.Rollback() }()

		var lockedUserID int64
		require.NoError(t, blocker.QueryRowContext(ctx, "SELECT id FROM users WHERE id = $1 FOR UPDATE", user.ID).Scan(&lockedUserID))
		require.Equal(t, user.ID, lockedUserID)

		failureCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_, err = repo.Reserve(failureCtx, newReservation(user.ID, key, 302, "1"))
		require.ErrorIs(t, err, service.ErrBillingReservationBalanceUnavailable)

		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM billing_reservations WHERE reservation_key = $1", key).Scan(&count))
		require.Zero(t, count)
	})
}

type reservationReapFixture struct {
	user        *service.User
	reservation *service.BillingReservation
	task        *service.VideoTask
}

func TestBillingReservationRepositoryReap_StateConcurrencyAndAtomicRollback(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewBillingReservationRepository(integrationDB).(*billingReservationRepository)
	videoRepo := NewVideoGatewayRepository(integrationDB)
	now := time.Now().UTC().Truncate(time.Microsecond)
	provider := &service.VideoProviderAccount{
		Provider:           service.VideoProviderMock,
		DisplayName:        "reservation-reaper-fake-" + uuid.NewString(),
		Enabled:            true,
		BaseURL:            "mock://reservation-reaper",
		DefaultModel:       "mock-video-v1",
		RateLimitPerMinute: 60,
	}
	require.NoError(t, videoRepo.CreateProviderAccount(ctx, provider))

	newFixture := func(t *testing.T, status, dispatch string, expiresAt time.Time) reservationReapFixture {
		t.Helper()
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("reservation-reap-%s@example.com", uuid.NewString()),
			PasswordHash: "hash",
			Balance:      20,
		})
		reservation, err := repo.Reserve(ctx, &service.BillingReservation{
			ReservationKey:    "reservation-reap-" + uuid.NewString(),
			SourceType:        "video_task",
			SourceID:          time.Now().UnixNano(),
			UserID:            user.ID,
			ReservedAmountUSD: service.MustUSD("3.5"),
			Status:            service.BillingReservationStatusActive,
			ExpiresAt:         expiresAt,
		})
		require.NoError(t, err)
		task := &service.VideoTask{
			ProviderAccountID: provider.ID,
			Provider:          service.VideoProviderMock,
			Model:             "mock-video-v1",
			TaskType:          service.VideoTaskTypeTextToVideo,
			Prompt:            "offline reservation reaper fixture",
			AspectRatio:       "16:9",
			Duration:          5,
			Resolution:        "720p",
			Status:            status,
			ReservationID:     &reservation.ID,
			Version:           1,
			DispatchState:     dispatch,
			SettlementStatus:  service.VideoSettlementStatusPending,
			ArchiveStatus:     service.VideoSideEffectStatusPending,
			CaptureStatus:     service.VideoSideEffectStatusPending,
			CreatedBy:         user.ID,
		}
		require.NoError(t, videoRepo.CreateTask(ctx, task))
		_, err = integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET source_id = $1 WHERE id = $2", task.ID, reservation.ID)
		require.NoError(t, err)
		reservation.SourceID = task.ID
		return reservationReapFixture{user: user, reservation: reservation, task: task}
	}

	assertState := func(t *testing.T, fixture reservationReapFixture, reservationStatus, settlementStatus string, version int64) {
		t.Helper()
		var gotReservation, gotSettlement string
		var gotVersion int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM billing_reservations WHERE id = $1", fixture.reservation.ID).Scan(&gotReservation))
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT settlement_status, version FROM video_tasks WHERE id = $1", fixture.task.ID).Scan(&gotSettlement, &gotVersion))
		require.Equal(t, reservationStatus, gotReservation)
		require.Equal(t, settlementStatus, gotSettlement)
		require.Equal(t, version, gotVersion)
	}

	t.Run("release only queued pending and review all dispatch evidence", func(t *testing.T) {
		queued := newFixture(t, service.VideoStatusQueued, service.VideoDispatchStatePending, now.Add(-time.Minute))
		submitted := newFixture(t, service.VideoStatusSubmitted, service.VideoDispatchStateAccepted, now.Add(-time.Minute))
		runningUnknown := newFixture(t, service.VideoStatusRunning, service.VideoDispatchStateUnknown, now.Add(-time.Minute))
		future := newFixture(t, service.VideoStatusQueued, service.VideoDispatchStatePending, now.Add(time.Hour))

		start := make(chan struct{})
		results := make(chan []service.BillingReservationReapResult, 2)
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				items, err := repo.ReapExpiredVideoReservations(ctx, now, 10)
				results <- items
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		applied := map[int64]int{}
		for items := range results {
			for _, item := range items {
				applied[item.TaskID]++
			}
		}
		require.Equal(t, map[int64]int{queued.task.ID: 1, submitted.task.ID: 1, runningUnknown.task.ID: 1}, applied)
		assertState(t, queued, service.BillingReservationStatusReleased, "released", 2)
		assertState(t, submitted, service.BillingReservationStatusReviewRequired, "error", 2)
		assertState(t, runningUnknown, service.BillingReservationStatusReviewRequired, "error", 2)
		assertState(t, future, service.BillingReservationStatusActive, service.VideoSettlementStatusPending, 1)

		for _, fixture := range []reservationReapFixture{queued, submitted, runningUnknown} {
			key := fmt.Sprintf("video_task:%d:reservation_expired", fixture.task.ID)
			var transactionCount, outboxCount int
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM billing_transactions WHERE transaction_key = $1", key).Scan(&transactionCount))
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM domain_outbox WHERE dedup_key = $1", key).Scan(&outboxCount))
			require.Equal(t, 1, transactionCount)
			require.Equal(t, 1, outboxCount)
		}
		var releaseKind, releaseAmount string
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
			SELECT transaction_kind, amount_usd::text
			FROM billing_transactions
			WHERE transaction_key = $1
		`, fmt.Sprintf("video_task:%d:reservation_expired", queued.task.ID)).Scan(&releaseKind, &releaseAmount))
		require.Equal(t, "release", releaseKind)
		require.Equal(t, "3.5000000000", releaseAmount)
		var reviewKind, reviewAmount string
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
			SELECT transaction_kind, amount_usd::text
			FROM billing_transactions
			WHERE transaction_key = $1
		`, fmt.Sprintf("video_task:%d:reservation_expired", submitted.task.ID)).Scan(&reviewKind, &reviewAmount))
		require.Equal(t, "adjustment", reviewKind)
		require.Equal(t, "0.0000000000", reviewAmount)
	})

	t.Run("dispatch and reaper race on task version", func(t *testing.T) {
		expiresAt := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Microsecond)
		fixture := newFixture(t, service.VideoStatusQueued, service.VideoDispatchStatePending, expiresAt)
		dispatchRepo := videoRepo.(service.VideoDispatchRepository)
		dispatchTask := *fixture.task
		start := make(chan struct{})
		dispatchApplied := make(chan bool, 1)
		reapResults := make(chan []service.BillingReservationReapResult, 1)
		errs := make(chan error, 2)
		go func() {
			<-start
			applied, err := dispatchRepo.MarkDispatchingCAS(ctx, &dispatchTask, &service.VideoTaskEvent{
				VideoTaskID: fixture.task.ID,
				EventType:   service.VideoDispatchStateDispatching,
				Message:     "race dispatch claim",
				Payload:     map[string]any{"provider": service.VideoProviderMock},
			})
			dispatchApplied <- applied
			errs <- err
		}()
		go func() {
			<-start
			items, err := repo.ReapExpiredVideoReservations(ctx, expiresAt.Add(time.Minute), 1)
			reapResults <- items
			errs <- err
		}()
		close(start)
		for range 2 {
			require.NoError(t, <-errs)
		}
		applied := <-dispatchApplied
		items := <-reapResults

		var reservationStatus, taskStatus, dispatchState string
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status FROM billing_reservations WHERE id = $1", fixture.reservation.ID).Scan(&reservationStatus))
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT status, dispatch_state FROM video_tasks WHERE id = $1", fixture.task.ID).Scan(&taskStatus, &dispatchState))
		if reservationStatus == service.BillingReservationStatusReleased && dispatchState == service.VideoDispatchStateDispatching {
			t.Fatalf("unsafe race outcome: reservation=%q task=%q dispatch=%q applied=%t reap=%#v", reservationStatus, taskStatus, dispatchState, applied, items)
		}
		require.True(t, applied || len(items) == 1, "one side of the CAS race must apply")
	})

	t.Run("release failure rolls back task ledger and outbox", func(t *testing.T) {
		fixture := newFixture(t, service.VideoStatusQueued, service.VideoDispatchStatePending, now.Add(-time.Minute))
		constraint := "reap_release_guard_" + fmt.Sprint(fixture.reservation.ID)
		_, err := integrationDB.ExecContext(ctx, fmt.Sprintf(
			"ALTER TABLE billing_reservations ADD CONSTRAINT %s CHECK (id <> %d OR status = 'active') NOT VALID",
			constraint, fixture.reservation.ID,
		))
		require.NoError(t, err)
		defer func() {
			_, _ = integrationDB.ExecContext(ctx, "ALTER TABLE billing_reservations DROP CONSTRAINT IF EXISTS "+constraint)
		}()

		_, err = repo.ReapExpiredVideoReservations(ctx, now, 1)
		require.Error(t, err)
		assertState(t, fixture, service.BillingReservationStatusActive, service.VideoSettlementStatusPending, 1)
		assertNoReapSideEffects(t, ctx, fixture.task.ID)
		_, _ = integrationDB.ExecContext(ctx, "ALTER TABLE billing_reservations DROP CONSTRAINT IF EXISTS "+constraint)
		_, _ = integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET status = 'review_required' WHERE id = $1", fixture.reservation.ID)
	})

	t.Run("ledger conflict rolls back release and outbox", func(t *testing.T) {
		fixture := newFixture(t, service.VideoStatusQueued, service.VideoDispatchStatePending, now.Add(-time.Minute))
		key := fmt.Sprintf("video_task:%d:reservation_expired", fixture.task.ID)
		_, err := integrationDB.ExecContext(ctx, `
			INSERT INTO billing_transactions (
				transaction_key, source_type, source_id, transaction_kind, user_id, reservation_id,
				amount_original, currency_original, amount_usd, exchange_rate, exchange_rate_as_of,
				pricing_source, pricing_version, balance_before, balance_after, metadata
			) VALUES ($1, 'video_task', $2, 'adjustment', $3, $4, 0, 'USD', 0, 1, $5, 'conflict', 'v0', 20, 20, '{}')
		`, key, fixture.task.ID, fixture.user.ID, fixture.reservation.ID, now)
		require.NoError(t, err)

		_, err = repo.ReapExpiredVideoReservations(ctx, now, 1)
		require.ErrorIs(t, err, service.ErrBillingTransactionConflict)
		assertState(t, fixture, service.BillingReservationStatusActive, service.VideoSettlementStatusPending, 1)
		var outboxCount int
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM domain_outbox WHERE dedup_key = $1", key).Scan(&outboxCount))
		require.Zero(t, outboxCount)
		_, _ = integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET status = 'review_required' WHERE id = $1", fixture.reservation.ID)
	})

	t.Run("outbox conflict rolls back release and ledger", func(t *testing.T) {
		fixture := newFixture(t, service.VideoStatusQueued, service.VideoDispatchStatePending, now.Add(-time.Minute))
		key := fmt.Sprintf("video_task:%d:reservation_expired", fixture.task.ID)
		_, err := NewDomainOutboxRepository(integrationDB).Enqueue(ctx, &service.DomainOutboxEvent{
			AggregateType: "video_task",
			AggregateID:   fixture.task.ID,
			EventType:     "billing.reservation_expired",
			DedupKey:      key,
			Payload:       json.RawMessage(`{"conflict":true}`),
			Status:        service.DomainOutboxStatusPending,
			NextAttemptAt: now,
		})
		require.NoError(t, err)

		_, err = repo.ReapExpiredVideoReservations(ctx, now, 1)
		require.ErrorIs(t, err, service.ErrDomainOutboxConflict)
		assertState(t, fixture, service.BillingReservationStatusActive, service.VideoSettlementStatusPending, 1)
		var transactionCount int
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM billing_transactions WHERE transaction_key = $1", key).Scan(&transactionCount))
		require.Zero(t, transactionCount)
		_, _ = integrationDB.ExecContext(ctx, "UPDATE billing_reservations SET status = 'review_required' WHERE id = $1", fixture.reservation.ID)
	})
}

func assertNoReapSideEffects(t *testing.T, ctx context.Context, taskID int64) {
	t.Helper()
	key := fmt.Sprintf("video_task:%d:reservation_expired", taskID)
	var transactionCount, outboxCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM billing_transactions WHERE transaction_key = $1", key).Scan(&transactionCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM domain_outbox WHERE dedup_key = $1", key).Scan(&outboxCount))
	require.Zero(t, transactionCount)
	require.Zero(t, outboxCount)
}

func TestExpiredInFlightReservationStopsWorkerBeforeItCanFinalizeWithoutActiveReservation(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	fixture := newVideoTaskFinalizationFixture(t, "3")
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE video_tasks
		SET status = 'submitted', dispatch_state = 'accepted', settlement_status = 'pending',
		    worker_claimed_at = NULL, worker_claimed_until = NULL
		WHERE id = $1;
		UPDATE billing_reservations SET expires_at = $2 WHERE id = $3
	`, fixture.task.ID, now.Add(-time.Minute), fixture.reservation.ID)
	require.NoError(t, err)

	reaper, ok := NewBillingReservationRepository(integrationDB).(service.BillingReservationReaperRepository)
	require.True(t, ok)
	results, err := reaper.ReapExpiredVideoReservations(ctx, now, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, service.BillingReservationReapActionReviewRequired, results[0].Action)

	runnable, err := NewVideoGatewayRepository(integrationDB).ListRunnableTasks(ctx, 20)
	require.NoError(t, err)
	for _, task := range runnable {
		require.NotEqual(t, fixture.task.ID, task.ID, "review-required reservation must not be polled into an un-settleable terminal state")
	}
}

func reservationIDs(items []*service.BillingReservation) []int64 {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func errorIs(err, target error) bool {
	return errors.Is(err, target)
}
