//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type grokMediaCleanupRunnerStub struct {
	calls     atomic.Int32
	active    atomic.Int32
	maxActive atomic.Int32
	started   chan struct{}
	delay     time.Duration
	block     bool
	ctxErr    chan error
}

func (s *grokMediaCleanupRunnerStub) CleanupGrokMediaExpiredRecords(ctx context.Context) GrokMediaExpiredCleanupStats {
	active := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		maximum := s.maxActive.Load()
		if active <= maximum || s.maxActive.CompareAndSwap(maximum, active) {
			break
		}
	}
	s.calls.Add(1)
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}

	if s.block {
		<-ctx.Done()
		if s.ctxErr != nil {
			s.ctxErr <- ctx.Err()
		}
		return GrokMediaExpiredCleanupStats{}
	}
	if s.delay > 0 {
		timer := time.NewTimer(s.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
	return GrokMediaExpiredCleanupStats{}
}

func waitForGrokMediaCleanupCalls(t *testing.T, started <-chan struct{}, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for cleanup call %d", i+1)
		}
	}
}

func TestGrokMediaCleanupServiceRunsAtStartupAndPeriodicallyWithoutRequests(t *testing.T) {
	cleaner := &grokMediaCleanupRunnerStub{started: make(chan struct{}, 8)}
	svc := NewGrokMediaCleanupService(cleaner, nil)
	svc.interval = 10 * time.Millisecond
	svc.Start()
	t.Cleanup(svc.Stop)

	waitForGrokMediaCleanupCalls(t, cleaner.started, 3)
	require.GreaterOrEqual(t, cleaner.calls.Load(), int32(3))
}

func TestGrokMediaCleanupServiceStartupIncludesPureImageRepository(t *testing.T) {
	imageRepo := &grokImageCreateRepoStub{}
	seedGrokMediaCleanupRecords(&grokVideoOwnerRepoStub{}, imageRepo, 1, 1)
	gateway := &OpenAIGatewayService{grokMediaImageCreateRepo: imageRepo}
	svc := NewGrokMediaCleanupService(gateway, nil)
	svc.interval = time.Hour
	svc.Start()

	require.Eventually(t, func() bool {
		imageRepo.mu.Lock()
		defer imageRepo.mu.Unlock()
		return imageRepo.cleanupCalls == 1
	}, time.Second, 5*time.Millisecond)
	svc.Stop()
	imageRepo.mu.Lock()
	defer imageRepo.mu.Unlock()
	require.Len(t, imageRepo.creates, 1, "only the unexpired image record should remain")
}

func TestGrokMediaCleanupServiceStopCancelsRunningCleanupAndPreventsLaterTicks(t *testing.T) {
	cleaner := &grokMediaCleanupRunnerStub{
		started: make(chan struct{}, 4),
		block:   true,
		ctxErr:  make(chan error, 1),
	}
	svc := NewGrokMediaCleanupService(cleaner, nil)
	svc.interval = 5 * time.Millisecond
	svc.Start()
	waitForGrokMediaCleanupCalls(t, cleaner.started, 1)

	stopped := make(chan struct{})
	go func() {
		svc.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel and wait for the running cleanup")
	}
	require.ErrorIs(t, <-cleaner.ctxErr, context.Canceled)
	callsAfterStop := cleaner.calls.Load()
	time.Sleep(25 * time.Millisecond)
	require.Equal(t, callsAfterStop, cleaner.calls.Load())
}

func TestGrokMediaCleanupServiceStartAndStopAreIdempotent(t *testing.T) {
	cleaner := &grokMediaCleanupRunnerStub{started: make(chan struct{}, 4)}
	svc := NewGrokMediaCleanupService(cleaner, nil)
	svc.interval = time.Hour

	svc.Start()
	svc.Start()
	waitForGrokMediaCleanupCalls(t, cleaner.started, 1)
	svc.Stop()
	svc.Stop()
	svc.Start()
	time.Sleep(20 * time.Millisecond)

	require.Equal(t, int32(1), cleaner.calls.Load())
}

func TestGrokMediaCleanupServiceRapidTicksNeverOverlap(t *testing.T) {
	cleaner := &grokMediaCleanupRunnerStub{
		started: make(chan struct{}, 8),
		delay:   25 * time.Millisecond,
	}
	svc := NewGrokMediaCleanupService(cleaner, nil)
	svc.interval = 2 * time.Millisecond
	svc.cleanupTimeout = time.Second
	svc.Start()

	waitForGrokMediaCleanupCalls(t, cleaner.started, 3)
	svc.Stop()
	require.Equal(t, int32(1), cleaner.maxActive.Load())
}

func TestGrokMediaCleanupServiceBoundsEachCleanupWithTimeout(t *testing.T) {
	cleaner := &grokMediaCleanupRunnerStub{
		started: make(chan struct{}, 1),
		block:   true,
		ctxErr:  make(chan error, 1),
	}
	svc := NewGrokMediaCleanupService(cleaner, nil)
	svc.interval = time.Hour
	svc.cleanupTimeout = 20 * time.Millisecond
	svc.Start()
	t.Cleanup(svc.Stop)
	waitForGrokMediaCleanupCalls(t, cleaner.started, 1)

	select {
	case err := <-cleaner.ctxErr:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("cleanup context was not bounded by its timeout")
	}
}
