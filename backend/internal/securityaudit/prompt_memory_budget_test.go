package securityaudit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/stretchr/testify/require"
)

type budgetGatedAuditConfig struct {
	*fakeConfigStore
	reads   atomic.Int32
	entered chan struct{}
	resume  chan struct{}
}

func (c *budgetGatedAuditConfig) Active() (ActiveConfig, bool) {
	if c.reads.Add(1) == 2 {
		close(c.entered)
		<-c.resume
	}
	return c.fakeConfigStore.Active()
}

func TestAsyncAuditMemoryBudgetOutlivesHandlerAndReleases(t *testing.T) {
	cfg := asyncConfig()
	store := &budgetGatedAuditConfig{fakeConfigStore: &fakeConfigStore{cfg: cfg, active: true}, entered: make(chan struct{}), resume: make(chan struct{})}
	var unblock sync.Once
	defer unblock.Do(func() { close(store.resume) })
	svc := memoryTestPromptService(cfg, &fakeJobRepository{})
	svc.config = store
	svc.enqueuer.config = store
	budget := httputil.NewBodyMemoryBudget(1 << 20)
	ctx, release := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release()
	require.NoError(t, svc.Enqueue(ctx, asyncRequest()))
	select {
	case <-store.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("background enqueue did not start")
	}
	release() // The HTTP request is now finished.
	other, releaseOther := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
	defer releaseOther()
	require.ErrorIs(t, httputil.ReserveRequestBodyMemory(other, 1<<20), httputil.ErrRequestBodyMemoryBudget)
	unblock.Do(func() { close(store.resume) })
	done := make(chan struct{})
	go func() { svc.enqueueWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("audit work did not release")
	}
	require.NoError(t, httputil.ReserveRequestBodyMemory(other, 1<<20))
	require.Equal(t, AuditMetricsSnapshot{Enqueued: 1}, svc.metrics.AuditSnapshot())
}

func TestAsyncAuditMemoryBudgetDropsBeforeCopyAndRefunds(t *testing.T) {
	svc := memoryTestPromptService(asyncConfig(), &fakeJobRepository{})
	budget := httputil.NewBodyMemoryBudget(128)
	ctx, release := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release()
	require.NoError(t, svc.Enqueue(ctx, asyncRequest()))
	svc.enqueueWG.Wait()
	require.Equal(t, AuditMetricsSnapshot{Dropped: 1}, svc.metrics.AuditSnapshot())
	require.Empty(t, svc.enqueueSlots)
	require.NoError(t, httputil.ReserveRequestBodyMemory(ctx, 128), "failed audit admission must refund clone/workspace charges")
}
