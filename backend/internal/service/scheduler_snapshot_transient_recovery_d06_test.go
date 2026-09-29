//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// d06RecoveryCache 记录每个桶最近一次发布的账号 ID。
type d06RecoveryCache struct {
	SchedulerCache

	mu     sync.Mutex
	epoch  int64
	writes map[SchedulerBucket][][]int64
}

func (c *d06RecoveryCache) CaptureBucketWriteToken(_ context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch++
	return SchedulerBucketWriteToken{Bucket: bucket, Epoch: c.epoch}, nil
}

func (c *d06RecoveryCache) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	return true, nil
}

func (c *d06RecoveryCache) UnlockBucket(context.Context, SchedulerBucket) error { return nil }

func (c *d06RecoveryCache) SetAccount(context.Context, *Account) error { return nil }

func (c *d06RecoveryCache) SetSnapshot(_ context.Context, bucket SchedulerBucket, _ SchedulerBucketWriteToken, accounts []Account) error {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes[bucket] = append(c.writes[bucket], ids)
	return nil
}

func (c *d06RecoveryCache) latest(bucket SchedulerBucket) ([]int64, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	writes := c.writes[bucket]
	if len(writes) == 0 {
		return nil, 0
	}
	return append([]int64(nil), writes[len(writes)-1]...), len(writes)
}

// d06RecoveryRepo 模拟 ListSchedulable* 的 SQL 语义：按查询时刻排除限流/过载/临时不可调度账号。
type d06RecoveryRepo struct {
	AccountRepository

	mu       sync.Mutex
	accounts map[int64]Account
}

func (r *d06RecoveryRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	return &account, nil
}

func (r *d06RecoveryRepo) list(groupID int64, platforms ...string) []Account {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Account, 0, len(r.accounts))
	for id := int64(1); id <= int64(len(r.accounts)); id++ {
		account, ok := r.accounts[id]
		if !ok || !account.IsSchedulable() {
			continue
		}
		inGroup := false
		for _, gid := range account.GroupIDs {
			if gid == groupID {
				inGroup = true
			}
		}
		for _, platform := range platforms {
			if inGroup && account.Platform == platform {
				out = append(out, account)
				break
			}
		}
	}
	return out
}

func (r *d06RecoveryRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	return r.list(groupID, platform), nil
}

func (r *d06RecoveryRepo) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, platforms []string) ([]Account, error) {
	return r.list(groupID, platforms...), nil
}

func TestSchedulerTransientRecoveryD06RebuildsBucketAfterRateLimitExpires(t *testing.T) {
	const groupID int64 = 7
	resetAt := time.Now().Add(150 * time.Millisecond)
	repo := &d06RecoveryRepo{accounts: map[int64]Account{
		1: {ID: 1, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}, RateLimitResetAt: &resetAt},
		2: {ID: 2, Platform: PlatformAnthropic, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}},
	}}
	cache := &d06RecoveryCache{writes: make(map[SchedulerBucket][][]int64)}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{RunMode: config.RunModeStandard})
	t.Cleanup(svc.Stop)

	accountID := int64(1)
	require.NoError(t, svc.handleAccountEvent(context.Background(), &accountID, nil, map[batchSeenKey]struct{}{}))

	bucket := SchedulerBucket{GroupID: groupID, Platform: PlatformAnthropic, Mode: SchedulerModeSingle}
	ids, _ := cache.latest(bucket)
	require.Equal(t, []int64{2}, ids, "限流中的账号在事件触发的重建中被排除")

	mixed := SchedulerBucket{GroupID: groupID, Platform: PlatformAnthropic, Mode: SchedulerModeMixed}
	require.Eventually(t, func() bool {
		ids, _ := cache.latest(bucket)
		mixedIDs, _ := cache.latest(mixed)
		return len(ids) == 2 && ids[0] == 1 && ids[1] == 2 &&
			len(mixedIDs) == 2 && mixedIDs[0] == 1 && mixedIDs[1] == 2
	}, 5*time.Second, 20*time.Millisecond, "限流到期后应自动重建桶并重新纳入账号")
}

func TestSchedulerTransientRecoveryD06NewEventReplacesTimerAndStopCancels(t *testing.T) {
	const groupID int64 = 8
	resetAt := time.Now().Add(150 * time.Millisecond)
	repo := &d06RecoveryRepo{accounts: map[int64]Account{
		1: {ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}, RateLimitResetAt: &resetAt},
	}}
	cache := &d06RecoveryCache{writes: make(map[SchedulerBucket][][]int64)}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, &config.Config{RunMode: config.RunModeStandard})

	accountID := int64(1)
	require.NoError(t, svc.handleAccountEvent(context.Background(), &accountID, nil, nil))

	// 管理员清除限流后的新事件：旧定时器应被取消，不再补额外重建。
	repo.mu.Lock()
	cleared := repo.accounts[1]
	cleared.RateLimitResetAt = nil
	repo.accounts[1] = cleared
	repo.mu.Unlock()
	require.NoError(t, svc.handleAccountEvent(context.Background(), &accountID, nil, nil))

	bucket := SchedulerBucket{GroupID: groupID, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}
	_, writesBefore := cache.latest(bucket)
	require.Equal(t, 2, writesBefore)

	// 再次进入临时不可调度，随后 Stop：定时器必须被停止。
	until := time.Now().Add(100 * time.Millisecond)
	repo.mu.Lock()
	blocked := repo.accounts[1]
	blocked.TempUnschedulableUntil = &until
	repo.accounts[1] = blocked
	repo.mu.Unlock()
	require.NoError(t, svc.handleAccountEvent(context.Background(), &accountID, nil, nil))
	svc.Stop()

	time.Sleep(1500 * time.Millisecond)
	_, writesAfter := cache.latest(bucket)
	require.Equal(t, 3, writesAfter, "被替换或 Stop 后的定时器不得再触发重建")
}
