//go:build unit

package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// fallbackCoalesceCache 总是快照未命中，并统计发布令牌与回写次数。
type fallbackCoalesceCache struct {
	SchedulerCache
	reads    atomic.Int32
	tokens   atomic.Int32
	publish  atomic.Int32
	snapshot []Account
}

func (c *fallbackCoalesceCache) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	c.reads.Add(1)
	return nil, false, nil
}

func (c *fallbackCoalesceCache) CaptureBucketWriteToken(_ context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	c.tokens.Add(1)
	return SchedulerBucketWriteToken{Bucket: bucket, Epoch: 1}, nil
}

func (c *fallbackCoalesceCache) SetSnapshot(_ context.Context, _ SchedulerBucket, _ SchedulerBucketWriteToken, accounts []Account) error {
	c.publish.Add(1)
	c.snapshot = accounts
	return nil
}

// fallbackCoalesceRepo 的分组查询停在 release 关闭之前，并统计调用次数。
type fallbackCoalesceRepo struct {
	AccountRepository
	release  chan struct{}
	calls    atomic.Int32
	accounts []Account
}

func (r *fallbackCoalesceRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, _ string) ([]Account, error) {
	r.calls.Add(1)
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return r.accounts, nil
}

func newFallbackCoalesceService(cache SchedulerCache, repo AccountRepository) *SchedulerSnapshotService {
	return NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{
		RunMode: config.RunModeStandard,
		Gateway: config.GatewayConfig{Scheduling: config.GatewaySchedulingConfig{
			DbFallbackEnabled: true,
		}},
	})
}

func TestSchedulerFallbackConcurrentMissesShareOneDBLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &fallbackCoalesceCache{}
		repo := &fallbackCoalesceRepo{
			release: make(chan struct{}),
			accounts: []Account{
				{ID: 7101, Name: "a", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true},
				{ID: 7102, Name: "b", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true},
			},
		}
		svc := newFallbackCoalesceService(cache, repo)
		groupID := int64(71)

		const callers = 12
		results := make([][]Account, callers)
		errs := make([]error, callers)
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i], _, errs[i] = svc.ListSchedulableAccounts(context.Background(), &groupID, PlatformOpenAI, false)
			}(i)
		}
		synctest.Wait()
		require.Equal(t, int32(callers), cache.reads.Load())
		require.Equal(t, int32(1), repo.calls.Load())
		close(repo.release)
		wg.Wait()

		require.Equal(t, int32(1), repo.calls.Load())
		require.Equal(t, int32(1), cache.tokens.Load())
		require.Equal(t, int32(1), cache.publish.Load())
		for i := 0; i < callers; i++ {
			require.NoError(t, errs[i])
			require.Len(t, results[i], 2)
		}

		// 每个调用方的切片相互独立，也不与回写的快照共用底层数组。
		results[0][0].Name = "mutated"
		results[0] = append(results[0][:1], Account{ID: 7199})
		for i := 1; i < callers; i++ {
			require.Equal(t, "a", results[i][0].Name)
			require.Equal(t, int64(7102), results[i][1].ID)
		}
		require.Equal(t, "a", cache.snapshot[0].Name)
		require.Equal(t, int64(7102), cache.snapshot[1].ID)
	})
}

func TestSchedulerFallbackCanceledWaiterLeavesSharedLoadRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &fallbackCoalesceCache{}
		repo := &fallbackCoalesceRepo{
			release:  make(chan struct{}),
			accounts: []Account{{ID: 7201, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true}},
		}
		svc := newFallbackCoalesceService(cache, repo)
		groupID := int64(72)

		ctx, cancel := context.WithCancel(context.Background())
		canceled := make(chan error, 1)
		go func() {
			_, _, err := svc.ListSchedulableAccounts(ctx, &groupID, PlatformOpenAI, false)
			canceled <- err
		}()
		synctest.Wait()
		require.Equal(t, int32(1), repo.calls.Load())

		joined := make(chan []Account, 1)
		go func() {
			accounts, _, err := svc.ListSchedulableAccounts(context.Background(), &groupID, PlatformOpenAI, false)
			if err != nil {
				accounts = nil
			}
			joined <- accounts
		}()
		synctest.Wait()

		cancel()
		require.ErrorIs(t, <-canceled, context.Canceled)
		close(repo.release)
		accounts := <-joined
		require.Len(t, accounts, 1)
		require.Equal(t, int32(1), repo.calls.Load(), "the canceled caller must not abort the shared load")
		require.Equal(t, int32(1), cache.publish.Load())
	})
}

// valuesReaderCache 直接返回值切片；GetSnapshot 不应被调用。
type valuesReaderCache struct {
	SchedulerCache
	values        []Account
	snapshotCalls atomic.Int32
}

func (c *valuesReaderCache) GetSnapshotValues(context.Context, SchedulerBucket) ([]Account, bool, error) {
	return c.values, true, nil
}

func (c *valuesReaderCache) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	c.snapshotCalls.Add(1)
	return nil, false, nil
}

func TestSchedulerSnapshotListUsesValuesReaderWithoutCopy(t *testing.T) {
	cache := &valuesReaderCache{values: []Account{{ID: 7301, Platform: PlatformOpenAI}, {ID: 7302, Platform: PlatformOpenAI}}}
	svc := newFallbackCoalesceService(cache, nil)
	groupID := int64(73)

	accounts, _, err := svc.ListSchedulableAccounts(context.Background(), &groupID, PlatformOpenAI, false)
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.Same(t, &cache.values[0], &accounts[0])
	require.Zero(t, cache.snapshotCalls.Load())
}

func TestFilterAccountsBySchedulingThresholdCompactsInPlace(t *testing.T) {
	svc := &GatewayService{}
	accounts := []Account{{ID: 1}, {ID: 2}, {ID: 3}}
	filtered := svc.filterAccountsBySchedulingThreshold(context.Background(), accounts)
	require.Equal(t, []int64{1, 2, 3}, []int64{filtered[0].ID, filtered[1].ID, filtered[2].ID})
	require.Same(t, &accounts[0], &filtered[0])
}
