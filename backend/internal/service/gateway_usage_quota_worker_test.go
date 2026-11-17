package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type blockingUsageQuotaRepo struct {
	UserPlatformQuotaRepository
	entered chan context.Context
	release chan struct{}
}

func (r *blockingUsageQuotaRepo) IncrementUsageWithReset(ctx context.Context, _ int64, _ string, _ float64, _ time.Time) error {
	r.entered <- ctx
	select {
	case <-r.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPostUsageQuotaPersistenceStaysInUsageWorker(t *testing.T) {
	repo := &blockingUsageQuotaRepo{entered: make(chan context.Context, 1), release: make(chan struct{})}
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount: 1, QueueSize: 8, TaskTimeout: time.Second, OverflowPolicy: "sync",
	})
	t.Cleanup(pool.Stop)
	t.Cleanup(func() { close(repo.release) })
	pool.Submit(func(ctx context.Context) {
		finalizePostUsageBilling(ctx, &postUsageBillingParams{
			Cost: &CostBreakdown{ActualCost: 1}, User: &User{ID: 1}, Account: &Account{ID: 2}, Platform: PlatformOpenAI,
		}, &billingDeps{
			userPlatformQuotaRepo: repo,
			billingCacheService:   &BillingCacheService{cfg: &config.Config{}},
			deferredService:       &DeferredService{},
		}, nil)
	})
	select {
	case ctx := <-repo.entered:
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, 5, time.Until(deadline).Seconds(), 1)
	case <-time.After(2 * time.Second):
		t.Fatal("quota persistence did not start")
	}
	next := make(chan struct{})
	pool.Submit(func(context.Context) { close(next) })
	select {
	case <-next:
		t.Fatal("quota persistence escaped the usage worker and released its concurrency slot")
	case <-time.After(20 * time.Millisecond):
	}
}

type failedUsageQuotaRepo struct {
	UserPlatformQuotaRepository
	ctx       context.Context
	errAtCall error
}

func (r *failedUsageQuotaRepo) IncrementUsageWithReset(ctx context.Context, _ int64, _ string, _ float64, _ time.Time) error {
	r.ctx = ctx
	r.errAtCall = ctx.Err()
	return errors.New("database unavailable")
}

func TestPostUsageQuotaPersistenceHasIndependentDeadlineAndReportsFailure(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &failedUsageQuotaRepo{}
	before := userPlatformQuotaDBIncrErrorTotal.Load()
	persistPostUsagePlatformQuota(parent, repo, 1, PlatformOpenAI, 1)
	require.NotNil(t, repo.ctx)
	require.NoError(t, repo.errAtCall, "committed charge must attempt persistence after parent cancellation")
	deadline, ok := repo.ctx.Deadline()
	require.True(t, ok)
	require.InDelta(t, 5, time.Until(deadline).Seconds(), 1)
	require.Equal(t, before+1, userPlatformQuotaDBIncrErrorTotal.Load())
}
