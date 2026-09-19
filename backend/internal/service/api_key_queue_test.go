//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type apiKeyQueueCacheStub struct {
	stubConcurrencyCacheForTest

	mu             sync.Mutex
	immediate      bool
	immediateNowMs int64
	immediateErr   error
	outcomes       []APIKeyQueueOutcome
	advanceErr     error
	abortErr       error
	statsErr       error
	statsActive    int
	statsWaiting   int

	acquireCalls int
	advanceModes []APIKeyQueueMode
	advanceIDs   []string
	advanceLimit []int
	advanceHook  func()
	abortedIDs   []string
	abortRemove  []bool
	deadlineMs   []int64
	slotTTL      time.Duration
	releasedIDs  []string
}

func (c *apiKeyQueueCacheStub) APIKeySlotTTL() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.slotTTL > 0 {
		return c.slotTTL
	}
	return 60 * time.Second
}
func (c *apiKeyQueueCacheStub) APIKeySlotRefreshInterval() time.Duration {
	return 20 * time.Second
}
func (c *apiKeyQueueCacheStub) RefreshAPIKeySlot(context.Context, int64, string) (bool, error) {
	return true, nil
}
func (c *apiKeyQueueCacheStub) AcquireAPIKeySlot(ctx context.Context, id int64, max int, requestID string) (bool, error) {
	acquired, _, err := c.AcquireAPIKeySlotWithTime(ctx, id, max, requestID)
	return acquired, err
}
func (c *apiKeyQueueCacheStub) AcquireAPIKeySlotWithTime(_ context.Context, _ int64, _ int, _ string) (bool, int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acquireCalls++
	return c.immediate, c.immediateNowMs, c.immediateErr
}
func (c *apiKeyQueueCacheStub) AcquireAPIKeySlotWithTimeCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.acquireCalls
}
func (c *apiKeyQueueCacheStub) AdvanceAPIKeyQueue(_ context.Context, _ int64, requestID string, mode APIKeyQueueMode, deadlineMs int64, keyLimit int, _ int) (APIKeyQueueOutcome, error) {
	c.mu.Lock()
	c.advanceModes = append(c.advanceModes, mode)
	c.advanceIDs = append(c.advanceIDs, requestID)
	c.advanceLimit = append(c.advanceLimit, keyLimit)
	c.deadlineMs = append(c.deadlineMs, deadlineMs)
	hook := c.advanceHook
	if c.advanceErr != nil {
		c.mu.Unlock()
		return 0, c.advanceErr
	}
	var outcome APIKeyQueueOutcome
	if len(c.outcomes) == 0 {
		outcome = APIKeyQueueOutcomeWaiting
	} else {
		outcome = c.outcomes[0]
		c.outcomes = c.outcomes[1:]
	}
	c.mu.Unlock()
	if hook != nil {
		hook()
	}
	return outcome, nil
}
func (c *apiKeyQueueCacheStub) AbortAPIKeyQueueAttempt(_ context.Context, _ int64, requestID string, _ int64, removeSlot bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.abortedIDs = append(c.abortedIDs, requestID)
	c.abortRemove = append(c.abortRemove, removeSlot)
	return c.abortErr
}
func (c *apiKeyQueueCacheStub) GetAPIKeyQueueStats(context.Context, int64) (int, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.statsErr != nil {
		return 0, 0, c.statsErr
	}
	return c.statsActive, c.statsWaiting, nil
}
func (c *apiKeyQueueCacheStub) ReleaseAPIKeySlot(_ context.Context, _ int64, requestID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releasedIDs = append(c.releasedIDs, requestID)
	return nil
}

func queueServiceWithStub(policy APIKeyQueuePolicy, cache *apiKeyQueueCacheStub) *ConcurrencyService {
	svc := NewConcurrencyService(cache)
	svc.SetAPIKeyQueuePolicy(policy)
	return svc
}

func admissionOwnerContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
	t.Cleanup(cancel)
	return ctx, cancel
}

func TestAPIKeyQueueImmediateAcquireSkipsQueue(t *testing.T) {
	cache := &apiKeyQueueCacheStub{immediate: true, immediateNowMs: 1_000}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 20, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.NoError(t, err)
	require.NotNil(t, reservation)
	require.NotEmpty(t, reservation.RequestID())
	require.Empty(t, cache.advanceModes, "fast path must not enter the queue")
	reservation.Release()
}

func TestAPIKeyQueueImmediateErrorCompensatesSameID(t *testing.T) {
	cache := &apiKeyQueueCacheStub{immediateErr: errors.New("write reply lost")}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 20, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.Error(t, err)
	require.Nil(t, reservation)
	require.Len(t, cache.releasedIDs, 1, "ambiguous admission must clean up with the same ID")
	require.Empty(t, cache.advanceModes, "must not enter the queue after an unknown admission")
}

func TestAPIKeyQueueDisabledStillEnforcesLimit(t *testing.T) {
	cache := &apiKeyQueueCacheStub{immediate: false}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 0, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)

	result, err := svc.AcquireAPIKeySlotWithWait(ctx, 5, 1)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.Empty(t, cache.advanceModes, "queue disabled must not create tickets")
}

func TestAPIKeyQueueFullReturnsTypedError(t *testing.T) {
	cache := &apiKeyQueueCacheStub{outcomes: []APIKeyQueueOutcome{APIKeyQueueOutcomeQueueFull}}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 1, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)

	_, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorFull))
	require.NotEmpty(t, cache.abortedIDs, "leaving the queue must fence the attempt")
}

func TestAPIKeyQueueConfirmedGrantKeepsSlotOnFence(t *testing.T) {
	cache := &apiKeyQueueCacheStub{
		immediateNowMs: 1_000,
		outcomes:       []APIKeyQueueOutcome{APIKeyQueueOutcomeAcquired},
	}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.NoError(t, err)
	require.NotNil(t, reservation)
	require.Len(t, cache.abortRemove, 1)
	require.False(t, cache.abortRemove[0], "a confirmed grant must keep its regular member")
	reservation.Release()
}

func TestAPIKeyQueuePolicyChangedSurfacesSeparately(t *testing.T) {
	cache := &apiKeyQueueCacheStub{outcomes: []APIKeyQueueOutcome{APIKeyQueueOutcomePolicyChanged}}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)

	_, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorPolicyChanged))
}

func TestAPIKeyQueueRedisFailureIsFailClosedWithSameID(t *testing.T) {
	cache := &apiKeyQueueCacheStub{advanceErr: errors.New("redis unavailable")}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)

	_, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorUnavailable))
	require.Len(t, cache.advanceIDs, 1, "no retry with a new attempt ID")
	require.Len(t, cache.abortedIDs, 1)
	require.Equal(t, cache.advanceIDs[0], cache.abortedIDs[0])
}

func TestAPIKeyQueueTimeoutAndCancel(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cache := &apiKeyQueueCacheStub{}
			svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: 150 * time.Millisecond}, cache)
			ctx, _ := admissionOwnerContext(t)
			_, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
			require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorTimeout))
			require.NotEmpty(t, cache.abortedIDs)
		})
	})

	t.Run("caller cancel keeps original cause", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cache := &apiKeyQueueCacheStub{}
			svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: 5 * time.Second}, cache)
			ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
			defer cancel()
			timer := time.AfterFunc(200*time.Millisecond, cancel)
			defer timer.Stop()
			_, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorTimeout))
			require.NotEmpty(t, cache.abortedIDs)
		})
	})

	t.Run("shutdown stops waiters", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cache := &apiKeyQueueCacheStub{}
			svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: 5 * time.Second}, cache)
			ctx, _ := admissionOwnerContext(t)
			timer := time.AfterFunc(200*time.Millisecond, svc.StopAPIKeyQueue)
			defer timer.Stop()
			_, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
			require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorUnavailable))
		})
	})
}

func TestAPIKeyQueueFixedDeadlineUsesRedisTime(t *testing.T) {
	cache := &apiKeyQueueCacheStub{immediateNowMs: 1_700_000_000_000}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: 2 * time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	// Stop after the first Advance recorded the fixed deadline but before its
	// reply is consumed: the ACK must not be granted.
	cache.advanceHook = svc.StopAPIKeyQueue
	_, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorUnavailable),
		"a stop between the RPC and its reply must not hand over the grant")

	require.Len(t, cache.deadlineMs, 1)
	remaining := cache.deadlineMs[0] - cache.immediateNowMs
	require.Greater(t, remaining, int64(0))
	require.LessOrEqual(t, remaining, int64(2_000), "deadline must not exceed the configured budget")
	require.Len(t, cache.advanceIDs, 1)
	// Every retry keeps one attempt ID and one deadline.
	for _, id := range cache.advanceIDs {
		require.Equal(t, cache.advanceIDs[0], id)
	}
	for _, deadline := range cache.deadlineMs {
		require.Equal(t, cache.deadlineMs[0], deadline)
	}
}

func TestAPIKeyQueueStopBlocksNewAdmissions(t *testing.T) {
	cache := &apiKeyQueueCacheStub{immediate: true, immediateNowMs: 1}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	svc.StopAPIKeyQueue()

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.Nil(t, reservation)
	require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorUnavailable))
	require.Zero(t, cache.acquireCalls, "stopped service must not attempt a fast-path admission")
}

func TestAPIKeyQueueStopReleasesLateQueuedGrant(t *testing.T) {
	cache := &apiKeyQueueCacheStub{
		immediateNowMs: 1_000,
		outcomes:       []APIKeyQueueOutcome{APIKeyQueueOutcomeAcquired},
	}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	cache.advanceHook = svc.StopAPIKeyQueue

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.Nil(t, reservation)
	require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorUnavailable))
	require.Equal(t, []bool{true}, cache.abortRemove, "late ACK must remove the regular member instead of handing it over")
}

func TestAPIKeyQueueDelayedAckReleasesExpiredLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &apiKeyQueueCacheStub{
			immediateNowMs: 1_000,
			outcomes:       []APIKeyQueueOutcome{APIKeyQueueOutcomeAcquired},
			slotTTL:        3 * time.Second,
		}
		// The ACK arrives after the safe lease validity (TTL 3s minus the
		// watchdog margin leaves ~1s): the grant must be released, not handed to
		// a request whose watchdog would cancel it immediately.
		cache.advanceHook = func() { time.Sleep(1200 * time.Millisecond) }
		svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: 30 * time.Second}, cache)
		ctx, _ := admissionOwnerContext(t)

		reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
		require.Nil(t, reservation)
		require.ErrorIs(t, err, ErrAPIKeySlotLeaseLost,
			"an expired lease must never be handed over as a grant")
		require.NotEmpty(t, cache.releasedIDs, "an expired grant is released before any handoff")
	})
}

func TestAPIKeyQueueStatsBatchFailsClosed(t *testing.T) {
	cache := &apiKeyQueueCacheStub{statsErr: errors.New("redis down")}
	svc := queueServiceWithStub(APIKeyQueuePolicy{}, cache)
	stats, err := svc.GetAPIKeyQueueStatsBatch(context.Background(), []int64{1, 2})
	require.Error(t, err)
	require.Nil(t, stats, "statistics failure must not degrade to zeroes")

	cache.statsErr = nil
	cache.statsActive = 2
	cache.statsWaiting = 3
	stats, err = svc.GetAPIKeyQueueStatsBatch(context.Background(), []int64{1})
	require.NoError(t, err)
	require.Equal(t, APIKeyQueueCounts{Active: 2, Waiting: 3}, stats[1])
}

func TestAPIKeyQueueCancelBeforeFirstEnterStillCleansUp(t *testing.T) {
	cache := &apiKeyQueueCacheStub{immediate: true, immediateNowMs: 1}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
	cancel()

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.Error(t, err)
	require.Nil(t, reservation)
	require.Len(t, cache.releasedIDs, 1, "confirmed grant must be released when the caller is gone")
}

func TestAPIKeySlotReservationConsumeProtectsSuccessor(t *testing.T) {
	released := 0
	paused := 0
	reservation := &APIKeySlotReservation{
		requestID:    "attempt-1",
		limit:        1,
		release:      func() { released++ },
		pauseRenewal: func() { paused++ },
	}

	reservation.PauseRenewal()
	require.Equal(t, "attempt-1", reservation.RequestID(), "paused reservation is still transferable")
	reservation.Consume()
	require.Empty(t, reservation.RequestID(), "consumed reservation cannot be transferred twice")
	reservation.Release()
	require.Zero(t, released, "the original request release must not delete the transferred Live member")
	require.GreaterOrEqual(t, paused, 1)
}

func TestAPIKeyQueueReservationExposesEffectiveLimit(t *testing.T) {
	cache := &apiKeyQueueCacheStub{immediate: true, immediateNowMs: 1}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 2)
	require.NoError(t, err)
	require.NotNil(t, reservation)
	require.Equal(t, 2, reservation.EffectiveLimit())
	require.False(t, reservation.StatsOnly())
	reservation.Release()

	// A limit that dropped to 0 while waiting yields a stats-only handle with an
	// exact member identity and a stop-only lifecycle.
	stats := svc.trackAPIKeyReservation(ctx, 5)
	require.Zero(t, stats.EffectiveLimit())
	require.True(t, stats.StatsOnly())
	require.NotEmpty(t, stats.RequestID())
	stats.Release()
	require.Len(t, cache.releasedIDs, 2, "both reservations stop/remove their own member")
}

// authRevalidatorSequence adapts a per-call script into the context callback
// and reports how many times it was invoked.
func authRevalidatorSequence(next func(call int) (int, error)) (APIKeyQueueAuthRevalidator, *int) {
	calls := 0
	return func(context.Context) (int, error) {
		calls++
		return next(calls)
	}, &calls
}

func TestAPIKeyQueueAuthRevalidationUsesFreshLimit(t *testing.T) {
	cache := &apiKeyQueueCacheStub{
		immediateNowMs: 1_000,
		outcomes:       []APIKeyQueueOutcome{APIKeyQueueOutcomeWaiting, APIKeyQueueOutcomeAcquired},
	}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	revalidator, calls := authRevalidatorSequence(func(call int) (int, error) {
		if call == 1 {
			return 2, nil
		}
		return 3, nil
	})
	ctx = WithAPIKeyQueueAuthRevalidator(ctx, revalidator)

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.NoError(t, err)
	require.NotNil(t, reservation)
	require.Equal(t, []int{3, 3}, cache.advanceLimit, "queued retries use the refreshed limit")
	require.Equal(t, 4, *calls, "initial, ENTER, POLL and pre-handoff revalidations")
	reservation.Release()
}

func TestAPIKeyQueueAuthRevalidationPolicyChangedRecovers(t *testing.T) {
	cache := &apiKeyQueueCacheStub{
		immediateNowMs: 1_000,
		outcomes:       []APIKeyQueueOutcome{APIKeyQueueOutcomePolicyChanged, APIKeyQueueOutcomeAcquired},
	}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	revalidator, _ := authRevalidatorSequence(func(int) (int, error) { return 1, nil })
	ctx = WithAPIKeyQueueAuthRevalidator(ctx, revalidator)

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.NoError(t, err, "POLICY_CHANGED must re-check the auth cache instead of dead-ending")
	require.NotNil(t, reservation)
	require.Len(t, cache.advanceIDs, 2, "same attempt is retried after revalidation")
	require.Equal(t, cache.advanceIDs[0], cache.advanceIDs[1])
	reservation.Release()
}

func TestAPIKeyQueueAuthRevalidationStopsDisabledKey(t *testing.T) {
	cache := &apiKeyQueueCacheStub{outcomes: []APIKeyQueueOutcome{APIKeyQueueOutcomeWaiting}}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	rejection := NewAPIKeyQueueAuthRejected(infraerrors.Unauthorized("API_KEY_DISABLED", "API key is disabled"))
	revalidator, calls := authRevalidatorSequence(func(call int) (int, error) {
		if call >= 2 {
			return 0, rejection
		}
		return 1, nil
	})
	ctx = WithAPIKeyQueueAuthRevalidator(ctx, revalidator)

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.Nil(t, reservation)
	require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorAuthRejected))
	require.ErrorIs(t, err, rejection)
	require.Equal(t, 401, infraerrors.Code(err), "the original authentication status is preserved")
	require.Equal(t, "API_KEY_DISABLED", infraerrors.Reason(err))
	require.GreaterOrEqual(t, *calls, 2)
	require.Empty(t, cache.advanceIDs, "a rejected key never reaches the queue RPC")
	require.NotEmpty(t, cache.abortedIDs, "leaving the wait must fence the attempt")
}

func TestAPIKeyQueueAuthRevalidationInfrastructureFailureIsUnavailable(t *testing.T) {
	cache := &apiKeyQueueCacheStub{}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	ctx = WithAPIKeyQueueAuthRevalidator(ctx, func(context.Context) (int, error) {
		return 0, errors.New("auth cache unavailable")
	})

	_, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorUnavailable))
	require.Empty(t, cache.advanceIDs)
}

func TestAPIKeyQueueAuthRevalidationLimitZeroSwitchesToTracking(t *testing.T) {
	cache := &apiKeyQueueCacheStub{
		immediateNowMs: 1_000,
		outcomes:       []APIKeyQueueOutcome{APIKeyQueueOutcomeWaiting},
	}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	revalidator, _ := authRevalidatorSequence(func(call int) (int, error) {
		if call >= 2 {
			return 0, nil
		}
		return 1, nil
	})
	ctx = WithAPIKeyQueueAuthRevalidator(ctx, revalidator)

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.NoError(t, err)
	require.NotNil(t, reservation, "limit 0 continues on the unlimited tracking path")
	require.True(t, reservation.StatsOnly(), "tracking handle enforces no key capacity")
	require.NotEmpty(t, reservation.RequestID(), "stats handle still has an exact transfer identity")
	require.NotEmpty(t, cache.abortRemove)
	require.True(t, cache.abortRemove[0], "queued attempt is closed before tracking starts")
	require.NotEmpty(t, cache.trackedAPIKeyRequestIDs)

	reservation.Release()
	require.Len(t, cache.releasedIDs, 1, "tracking member is released by its owner")
}

func TestAPIKeyQueueAuthRevalidationAfterGrantReleasesInsteadOfHandoff(t *testing.T) {
	cache := &apiKeyQueueCacheStub{
		immediateNowMs: 1_000,
		outcomes:       []APIKeyQueueOutcome{APIKeyQueueOutcomeAcquired},
	}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	revalidator, calls := authRevalidatorSequence(func(call int) (int, error) {
		if call >= 3 {
			return 0, NewAPIKeyQueueAuthRejected(infraerrors.Forbidden("API_KEY_EXPIRED", "API key 已过期"))
		}
		return 1, nil
	})
	ctx = WithAPIKeyQueueAuthRevalidator(ctx, revalidator)

	reservation, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.Nil(t, reservation)
	require.True(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorAuthRejected))
	require.Equal(t, 403, infraerrors.Code(err))
	require.Equal(t, 3, *calls)
	require.Len(t, cache.releasedIDs, 1, "a confirmed member is released, never handed to a stale request")
	require.Equal(t, []bool{false}, cache.abortRemove, "the confirmed grant keeps its regular member from the fence")
}

func TestAPIKeyQueuePreservesTypedCancellationCause(t *testing.T) {
	cache := &apiKeyQueueCacheStub{}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: 5 * time.Second}, cache)
	parent, cancelParent := context.WithCancelCause(context.Background())
	defer cancelParent(context.Canceled)
	ctx, _ := WithAPIKeyAdmissionOwner(parent)
	peerGone := errors.New("ws peer gone while queued")
	timer := time.AfterFunc(150*time.Millisecond, func() { cancelParent(peerGone) })
	defer timer.Stop()

	_, err := svc.ReserveAPIKeySlotWithWait(ctx, 5, 1)
	require.ErrorIs(t, err, peerGone, "typed peer-gone cause must not flatten to context.Canceled")
	require.False(t, IsAPIKeyQueueErrorKind(err, APIKeyQueueErrorTimeout))
	require.NotEmpty(t, cache.abortedIDs)
}

func TestAPIKeyQueueAuthRevalidatorSurvivesContextDerivation(t *testing.T) {
	cache := &apiKeyQueueCacheStub{immediate: true, immediateNowMs: 1}
	svc := queueServiceWithStub(APIKeyQueuePolicy{MaxWaiting: 5, Timeout: time.Second}, cache)
	ctx, _ := admissionOwnerContext(t)
	calls := 0
	ctx = WithAPIKeyQueueAuthRevalidator(ctx, func(context.Context) (int, error) {
		calls++
		return 1, nil
	})

	// WS admission derives a cancelable gate context and some forwarding paths
	// detach client cancellation; values must survive both derivations.
	gateCtx, cancelGate := context.WithCancelCause(ctx)
	defer cancelGate(context.Canceled)
	detached, cancelDetached := context.WithCancelCause(context.WithoutCancel(gateCtx))
	defer cancelDetached(context.Canceled)

	reservation, err := svc.ReserveAPIKeySlotWithWait(detached, 5, 1)
	require.NoError(t, err)
	require.NotNil(t, reservation)
	require.Equal(t, 1, calls, "revalidator must stay visible through WithCancelCause and WithoutCancel")
	reservation.Release()

	// The upstream-detachment wrapper reads values from the WithoutCancel side
	// while using the owner control context for lifetime; admission through it
	// must still see the revalidator and the admission owner.
	upstreamCtx, releaseUpstream := detachAPIKeyUpstreamContext(ctx)
	defer releaseUpstream()
	reservation, err = svc.ReserveAPIKeySlotWithWait(upstreamCtx, 5, 1)
	require.NoError(t, err)
	require.NotNil(t, reservation)
	require.Equal(t, 2, calls, "revalidator must stay visible through detachAPIKeyUpstreamContext")
	reservation.Release()
}

// TestGroupAllowsImageGenerationLatestPrefersRevalidatedPermission pins the
// service-forwarder correction: once the queue revalidator observed the latest
// image permission, gates must use it instead of the stale handshake snapshot.
func TestGroupAllowsImageGenerationLatestPrefersRevalidatedPermission(t *testing.T) {
	disabled := &Group{AllowImageGeneration: false}
	enabled := &Group{AllowImageGeneration: true}

	require.False(t, GroupAllowsImageGenerationLatest(context.Background(), disabled), "no holder falls back to the group snapshot")
	require.True(t, GroupAllowsImageGenerationLatest(context.Background(), enabled))

	holder := NewAPIKeyQueueImagePermission()
	ctx := WithAPIKeyQueueImagePermission(context.Background(), holder)
	require.False(t, GroupAllowsImageGenerationLatest(ctx, disabled), "unresolved holder keeps the fallback")
	require.True(t, GroupAllowsImageGenerationLatest(ctx, enabled))

	holder.Set(true)
	require.True(t, GroupAllowsImageGenerationLatest(ctx, disabled), "a revalidated relaxation must win over the handshake snapshot")
	holder.Set(false)
	require.False(t, GroupAllowsImageGenerationLatest(ctx, enabled), "a revalidated revocation must win over the handshake snapshot")
}

func TestRevalidateAPIKeyQueueTurnReportsInstalledCallback(t *testing.T) {
	svc := NewConcurrencyService(nil)
	limit, ok, err := svc.RevalidateAPIKeyQueueTurn(context.Background())
	require.NoError(t, err)
	require.False(t, ok, "no installed callback means no per-turn revalidation")
	require.Zero(t, limit)

	ctx := WithAPIKeyQueueAuthRevalidator(context.Background(), func(context.Context) (int, error) {
		return 2, nil
	})
	limit, ok, err = svc.RevalidateAPIKeyQueueTurn(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 2, limit)
}
