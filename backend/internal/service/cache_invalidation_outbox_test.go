//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ── Stubs ────────────────────────────────────────────────────────────────────

// outboxRepoStub is a fake CacheInvalidationOutboxRepository for worker tests.
type outboxRepoStub struct {
	mu sync.Mutex

	claimResult []CacheInvalidationEvent
	claimErr    error
	claimCalls  int

	markSucceededIDs  []int64
	markFailedIDs     []int64
	markDeadIDs       []int64
	requeueStaleCalls int
}

func (s *outboxRepoStub) Enqueue(_ context.Context, _ CacheInvalidationEvent) error {
	return nil
}

func (s *outboxRepoStub) ClaimReady(_ context.Context, _ string, _ int, _ time.Duration) ([]CacheInvalidationEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimCalls++
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	// Return the events once, then empty to avoid infinite loop.
	evs := s.claimResult
	s.claimResult = nil
	return evs, nil
}

func (s *outboxRepoStub) MarkSucceeded(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markSucceededIDs = append(s.markSucceededIDs, id)
	return nil
}

func (s *outboxRepoStub) MarkFailed(_ context.Context, id int64, _ error, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markFailedIDs = append(s.markFailedIDs, id)
	return nil
}

func (s *outboxRepoStub) MarkDead(_ context.Context, id int64, _ error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markDeadIDs = append(s.markDeadIDs, id)
	return nil
}

func (s *outboxRepoStub) RequeueStaleProcessing(_ context.Context, _ time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requeueStaleCalls++
	return 0, nil
}

// strictAuthStub implements StrictAPIKeyAuthCacheInvalidator.
type strictAuthStub struct {
	mu            sync.Mutex
	cacheKeysSeen []string
	userIDsSeen   []int64
	err           error
}

func (s *strictAuthStub) InvalidateAuthCacheByUserIDStrict(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userIDsSeen = append(s.userIDsSeen, userID)
	return s.err
}

func (s *strictAuthStub) InvalidateAuthCacheByUserIDsStrict(_ context.Context, userIDs []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userIDsSeen = append(s.userIDsSeen, userIDs...)
	return s.err
}

func (s *strictAuthStub) InvalidateAuthCacheByCacheKeysStrict(_ context.Context, cacheKeys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheKeysSeen = append(s.cacheKeysSeen, cacheKeys...)
	return s.err
}

// rateStub implements UserGroupRateCacheInvalidator.
type rateStub struct {
	mu         sync.Mutex
	pairsSeen  []RatePair
	err        error
}

func (s *rateStub) InvalidateUserGroupRateCache(_ context.Context, pairs []RatePair) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairsSeen = append(s.pairsSeen, pairs...)
	return s.err
}

// newTestWorker creates a worker with very short poll interval for tests.
func newTestWorker(repo CacheInvalidationOutboxRepository, auth StrictAPIKeyAuthCacheInvalidator, rate UserGroupRateCacheInvalidator) *CacheInvalidationOutboxWorker {
	return NewCacheInvalidationOutboxWorker(
		repo, auth, rate,
		10*time.Millisecond, // fast poll for tests
		10,
		30*time.Second,
		3, // low max_attempts for tests
		1,
		"test-worker",
	)
}

// ── Tests ────────────────────────────────────────────────────────────────────

// TestWorker_SuccessfulAuthCacheInvalidation: event with auth_snapshot cache type
// and pre-hashed cache keys → InvalidateAuthCacheByCacheKeysStrict called, row succeeded.
func TestWorker_SuccessfulAuthCacheInvalidation(t *testing.T) {
	auth := &strictAuthStub{}
	rate := &rateStub{}
	repo := &outboxRepoStub{
		claimResult: []CacheInvalidationEvent{
			{
				ID:          1,
				EventType:   EventTypeAuthCacheInvalidate,
				CacheTypes:  []string{CacheTypeAuthSnapshot},
				Attempts:    0,
				MaxAttempts: 3,
				Payload: EventPayload{
					AuthCacheKeys: []string{"deadbeef01", "cafebabe02"},
				},
			},
		},
	}
	w := newTestWorker(repo, auth, rate)
	w.processBatch(context.Background())

	require.Equal(t, []string{"deadbeef01", "cafebabe02"}, auth.cacheKeysSeen,
		"pre-hashed cache keys must be passed directly to InvalidateAuthCacheByCacheKeysStrict")
	require.Equal(t, []int64{1}, repo.markSucceededIDs,
		"event must be marked succeeded on success")
	require.Empty(t, repo.markFailedIDs)
}

// TestWorker_RateCacheInvalidation: event with user_group_rate cache type
// → InvalidateUserGroupRateCache called, row succeeded.
func TestWorker_RateCacheInvalidation(t *testing.T) {
	auth := &strictAuthStub{}
	rate := &rateStub{}
	repo := &outboxRepoStub{
		claimResult: []CacheInvalidationEvent{
			{
				ID:          2,
				EventType:   EventTypeRateCacheInvalidate,
				CacheTypes:  []string{CacheTypeUserGroupRate},
				Attempts:    0,
				MaxAttempts: 3,
				Payload: EventPayload{
					RatePairs: []RatePair{{UserID: 10, GroupID: 20}},
				},
			},
		},
	}
	w := newTestWorker(repo, auth, rate)
	w.processBatch(context.Background())

	require.Equal(t, []RatePair{{UserID: 10, GroupID: 20}}, rate.pairsSeen)
	require.Equal(t, []int64{2}, repo.markSucceededIDs)
}

// TestWorker_FailureRetry: auth invalidation fails → row marked failed (not dead)
// when attempts < max_attempts.
func TestWorker_FailureRetry(t *testing.T) {
	authErr := errors.New("redis unavailable")
	auth := &strictAuthStub{err: authErr}
	repo := &outboxRepoStub{
		claimResult: []CacheInvalidationEvent{
			{
				ID:          3,
				CacheTypes:  []string{CacheTypeAuthSnapshot},
				Attempts:    1, // still under max
				MaxAttempts: 3,
				Payload: EventPayload{
					AuthCacheKeys: []string{"abc"},
				},
			},
		},
	}
	w := newTestWorker(repo, auth, nil)
	w.processBatch(context.Background())

	require.Empty(t, repo.markSucceededIDs)
	require.Empty(t, repo.markDeadIDs)
	require.Equal(t, []int64{3}, repo.markFailedIDs, "failed event must be scheduled for retry")
}

// TestWorker_ExceedsMaxAttempts: auth fails when attempts+1 >= max_attempts → dead.
func TestWorker_ExceedsMaxAttempts(t *testing.T) {
	authErr := errors.New("permanent error")
	auth := &strictAuthStub{err: authErr}
	repo := &outboxRepoStub{
		claimResult: []CacheInvalidationEvent{
			{
				ID:          4,
				CacheTypes:  []string{CacheTypeAuthSnapshot},
				Attempts:    2, // next attempt = 3 = max_attempts
				MaxAttempts: 3,
				Payload: EventPayload{
					AuthCacheKeys: []string{"xyz"},
				},
			},
		},
	}
	w := newTestWorker(repo, auth, nil)
	w.processBatch(context.Background())

	require.Empty(t, repo.markSucceededIDs)
	require.Empty(t, repo.markFailedIDs)
	require.Equal(t, []int64{4}, repo.markDeadIDs, "exhausted event must be moved to dead")
}

// TestWorker_StartStop: Start/Stop lifecycle — worker polls and stops cleanly.
func TestWorker_StartStop(t *testing.T) {
	auth := &strictAuthStub{}
	rate := &rateStub{}
	repo := &outboxRepoStub{}

	w := newTestWorker(repo, auth, rate)
	w.Start()
	time.Sleep(50 * time.Millisecond) // let the worker poll at least once
	w.Stop()

	repo.mu.Lock()
	calls := repo.claimCalls
	repo.mu.Unlock()
	require.Greater(t, calls, 0, "worker must have called ClaimReady at least once before Stop")
}

// TestWorker_RateCacheFailure_EntersRetry: rate invalidation failure causes retry,
// not dead-letter, when attempts < max_attempts.
func TestWorker_RateCacheFailure_EntersRetry(t *testing.T) {
	rate := &rateStub{err: errors.New("rate cache down")}
	repo := &outboxRepoStub{
		claimResult: []CacheInvalidationEvent{
			{
				ID:          5,
				CacheTypes:  []string{CacheTypeUserGroupRate},
				Attempts:    0,
				MaxAttempts: 3,
				Payload: EventPayload{
					RatePairs: []RatePair{{UserID: 1, GroupID: 2}},
				},
			},
		},
	}
	w := newTestWorker(repo, nil, rate)
	w.processBatch(context.Background())

	require.Equal(t, []int64{5}, repo.markFailedIDs, "failed rate cache event must enter retry chain")
	require.Empty(t, repo.markDeadIDs)
}
