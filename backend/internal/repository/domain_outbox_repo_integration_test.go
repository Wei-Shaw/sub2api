//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestDomainOutboxRepository_ClaimLeaseCompleteRetryAndRedaction(t *testing.T) {
	ctx := context.Background()
	repo := NewDomainOutboxRepository(integrationDB)
	now := time.Now().UTC().Truncate(time.Microsecond)

	enqueue := func(t *testing.T, suffix string, next time.Time) *service.DomainOutboxEvent {
		t.Helper()
		event, err := repo.Enqueue(ctx, &service.DomainOutboxEvent{
			AggregateType: "video_task",
			AggregateID:   time.Now().UnixNano(),
			EventType:     "video.archive_asset",
			DedupKey:      "outbox-" + suffix + "-" + uuid.NewString(),
			Payload:       json.RawMessage(`{"asset":"local"}`),
			Status:        service.DomainOutboxStatusPending,
			NextAttemptAt: next,
		})
		require.NoError(t, err)
		return event
	}

	t.Run("claims only due rows and concurrent workers do not duplicate", func(t *testing.T) {
		due := []*service.DomainOutboxEvent{
			enqueue(t, "due-1", now.Add(-time.Second)),
			enqueue(t, "due-2", now.Add(-time.Second)),
			enqueue(t, "due-3", now.Add(-time.Second)),
			enqueue(t, "due-4", now.Add(-time.Second)),
		}
		future := enqueue(t, "future", now.Add(time.Hour))

		start := make(chan struct{})
		claimed := make(chan []*service.DomainOutboxEvent, 2)
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for _, worker := range []string{"worker-a", "worker-b"} {
			worker := worker
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				items, err := repo.ClaimBatch(ctx, worker, now, 2, 2*time.Minute)
				claimed <- items
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(claimed)
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}

		seen := map[int64]struct{}{}
		for batch := range claimed {
			for _, event := range batch {
				_, duplicate := seen[event.ID]
				require.False(t, duplicate, "FOR UPDATE SKIP LOCKED must not double-claim")
				seen[event.ID] = struct{}{}
			}
		}
		require.Len(t, seen, len(due))
		_, claimedFuture := seen[future.ID]
		require.False(t, claimedFuture)
	})

	t.Run("lease ownership, reaping, and complete are safe", func(t *testing.T) {
		event := enqueue(t, "lease", now.Add(-time.Second))
		claimed, err := repo.ClaimBatch(ctx, "lease-owner", now, 1, time.Minute)
		require.NoError(t, err)
		require.Equal(t, event.ID, claimed[0].ID)

		other, err := repo.ClaimBatch(ctx, "other-worker", now.Add(30*time.Second), 10, time.Minute)
		require.NoError(t, err)
		require.NotContains(t, outboxIDs(other), event.ID)

		reaped, err := repo.ReapExpiredLeases(ctx, now.Add(30*time.Second), 10)
		require.NoError(t, err)
		require.Zero(t, reaped)
		reaped, err = repo.ReapExpiredLeases(ctx, now.Add(61*time.Second), 10)
		require.NoError(t, err)
		require.EqualValues(t, 1, reaped)

		reclaimed, err := repo.ClaimBatch(ctx, "other-worker", now.Add(61*time.Second), 10, time.Minute)
		require.NoError(t, err)
		require.Contains(t, outboxIDs(reclaimed), event.ID)
		applied, err := repo.Complete(ctx, event.ID, "other-worker", now.Add(62*time.Second))
		require.NoError(t, err)
		require.True(t, applied)
		applied, err = repo.Complete(ctx, event.ID, "other-worker", now.Add(63*time.Second))
		require.NoError(t, err)
		require.False(t, applied)
	})

	t.Run("expired lease rejects completion with a backdated completed time", func(t *testing.T) {
		claimAt := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
		event := enqueue(t, "expired-complete", claimAt.Add(-time.Second))
		claimed, err := repo.ClaimBatch(ctx, "expired-owner", claimAt, 1, time.Minute)
		require.NoError(t, err)
		require.Equal(t, event.ID, claimed[0].ID)

		applied, err := repo.Complete(ctx, event.ID, "expired-owner", claimAt.Add(30*time.Second))
		require.ErrorIs(t, err, service.ErrDomainOutboxLeaseConflict)
		require.False(t, applied)
		stored, err := repo.GetByID(ctx, event.ID)
		require.NoError(t, err)
		require.Equal(t, service.DomainOutboxStatusProcessing, stored.Status)
	})

	t.Run("enqueue replay remains idempotent after lifecycle changes", func(t *testing.T) {
		input := &service.DomainOutboxEvent{
			AggregateType: "video_task",
			AggregateID:   time.Now().UnixNano(),
			EventType:     "video.capture_content",
			DedupKey:      "outbox-lifecycle-" + uuid.NewString(),
			Payload:       json.RawMessage(`{"capture":true}`),
			Status:        service.DomainOutboxStatusPending,
			NextAttemptAt: now.Add(-time.Second),
		}
		first, err := repo.Enqueue(ctx, input)
		require.NoError(t, err)
		conflicting := *input
		conflicting.Payload = json.RawMessage(`{"capture":false}`)
		_, err = repo.Enqueue(ctx, &conflicting)
		require.ErrorIs(t, err, service.ErrDomainOutboxConflict)

		claimed, err := repo.ClaimBatch(ctx, "lifecycle-worker", now, 1, time.Minute)
		require.NoError(t, err)
		require.Equal(t, first.ID, claimed[0].ID)
		applied, err := repo.Complete(ctx, first.ID, "lifecycle-worker", now.Add(time.Second))
		require.NoError(t, err)
		require.True(t, applied)

		replayed, err := repo.Enqueue(ctx, input)
		require.NoError(t, err)
		require.Equal(t, first.ID, replayed.ID)
		require.Equal(t, service.DomainOutboxStatusCompleted, replayed.Status)
	})

	t.Run("payload numeric forms are equivalent without losing large integer precision", func(t *testing.T) {
		input := &service.DomainOutboxEvent{
			AggregateType: "video_task",
			AggregateID:   time.Now().UnixNano(),
			EventType:     "billing.invalidate_cache",
			DedupKey:      "outbox-json-equivalent-" + uuid.NewString(),
			Payload:       json.RawMessage(`{"value":1,"nested":[1.0,{"n":1e0}]}`),
			Status:        service.DomainOutboxStatusPending,
			NextAttemptAt: now.Add(time.Hour),
		}
		first, err := repo.Enqueue(ctx, input)
		require.NoError(t, err)
		for _, payload := range []json.RawMessage{
			json.RawMessage(`{"value":1.0,"nested":[1e0,{"n":1}]}`),
			json.RawMessage(`{"value":1e0,"nested":[1,{"n":1.00}]}`),
		} {
			replayedInput := *input
			replayedInput.Payload = payload
			replayed, err := repo.Enqueue(ctx, &replayedInput)
			require.NoError(t, err)
			require.Equal(t, first.ID, replayed.ID)
		}

		largeInput := *input
		largeInput.DedupKey = "outbox-json-large-" + uuid.NewString()
		largeInput.Payload = json.RawMessage(`{"value":9007199254740992}`)
		_, err = repo.Enqueue(ctx, &largeInput)
		require.NoError(t, err)
		largeConflict := largeInput
		largeConflict.Payload = json.RawMessage(`{"value":9007199254740993}`)
		_, err = repo.Enqueue(ctx, &largeConflict)
		require.ErrorIs(t, err, service.ErrDomainOutboxConflict)
	})

	t.Run("worker dead decision at attempt eight stores only a safe error summary", func(t *testing.T) {
		event := enqueue(t, "retry", now.Add(-time.Second))
		attemptAt := now
		for attempt := 1; attempt <= 8; attempt++ {
			claimed, err := repo.ClaimBatch(ctx, "retry-worker", attemptAt, 1, time.Minute)
			require.NoError(t, err)
			require.Equal(t, event.ID, claimed[0].ID)
			require.Equal(t, attempt, claimed[0].AttemptCount)

			errorText := "provider https://example.test/path?access_token=secret&trace=private password=hunter2\n" +
				"request https://url-user:url-password@example.test/path?token=query-secret#fragment-secret\n" +
				"fragment https://example.test/path#fragment-only-secret\n" +
				"Authorization: Basic dXNlcjpwYXNz\nX-API-Key: topsecret\n" +
				"Cookie: session=secret; csrf=still-leaked\n" +
				"Authorization: Digest username=x, response=digestsecret\n" + strings.Repeat("x", 1500)
			nextAttempt := attemptAt.Add(time.Second)
			applied, err := repo.Retry(ctx, event.ID, "retry-worker", nextAttempt, attempt == 8, errorText)
			require.NoError(t, err)
			require.True(t, applied)
			attemptAt = nextAttempt
		}

		stored, err := repo.GetByID(ctx, event.ID)
		require.NoError(t, err)
		require.Equal(t, service.DomainOutboxStatusDead, stored.Status)
		require.Equal(t, 8, stored.AttemptCount)
		require.NotNil(t, stored.LastError)
		require.LessOrEqual(t, len(*stored.LastError), service.DomainOutboxMaxErrorSummaryBytes)
		require.NotContains(t, *stored.LastError, "access_token")
		require.NotContains(t, *stored.LastError, "trace=private")
		require.NotContains(t, *stored.LastError, "hunter2")
		require.NotContains(t, *stored.LastError, "dXNlcjpwYXNz")
		require.NotContains(t, *stored.LastError, "topsecret")
		require.NotContains(t, *stored.LastError, "still-leaked")
		require.NotContains(t, *stored.LastError, "digestsecret")
		require.NotContains(t, *stored.LastError, "url-user")
		require.NotContains(t, *stored.LastError, "url-password")
		require.NotContains(t, *stored.LastError, "query-secret")
		require.NotContains(t, *stored.LastError, "fragment-secret")
		require.NotContains(t, *stored.LastError, "fragment-only-secret")
		require.Contains(t, *stored.LastError, "https://example.test/path")

		counts, err := repo.Counts(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, counts.Dead, int64(1))
	})
}

func outboxIDs(items []*service.DomainOutboxEvent) []int64 {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
