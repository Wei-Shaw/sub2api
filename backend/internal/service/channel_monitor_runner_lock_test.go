//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubLeaderLockCache struct {
	acquire  bool
	err      error
	acquires int
	releases int
}

func (s *stubLeaderLockCache) TryAcquireLeaderLock(context.Context, string, string, time.Duration) (bool, error) {
	s.acquires++
	return s.acquire, s.err
}

func (s *stubLeaderLockCache) ReleaseLeaderLock(context.Context, string, string) error {
	s.releases++
	return nil
}

func TestChannelMonitorRunnerSkipsWhenProbeLockHeld(t *testing.T) {
	svc := &stubMonitorSvc{}
	cache := &stubLeaderLockCache{acquire: false}
	r := newRunnerForTest(svc)
	r.SetLeaderLock(cache, nil)

	r.runOne(42, "codex-官方0.3折")

	require.Zero(t, svc.runCount.Load(), "选主锁被别的实例持有时不得发探针（否则重复消费）")
	require.Equal(t, 1, cache.acquires)
	require.Zero(t, cache.releases, "没拿到锁就不该去释放（会误删别人的锁）")
}

func TestChannelMonitorRunnerRunsAndReleasesWhenLockAcquired(t *testing.T) {
	svc := &stubMonitorSvc{}
	cache := &stubLeaderLockCache{acquire: true}
	r := newRunnerForTest(svc)
	r.SetLeaderLock(cache, nil)

	r.runOne(42, "codex-官方0.3折")

	require.Equal(t, int64(1), svc.runCount.Load())
	require.Equal(t, 1, cache.releases, "探针结束必须释放选主锁")
}

func TestChannelMonitorRunnerUngatedWithoutLockBackend(t *testing.T) {
	svc := &stubMonitorSvc{}
	r := newRunnerForTest(svc) // lockCache / lockDB 均为 nil

	r.runOne(42, "codex-官方0.3折")

	require.Equal(t, int64(1), svc.runCount.Load(), "没有选主后端时应退化为不设闸门，而不是饿死探针")
}

func TestChannelMonitorRunnerFallsBackWhenCacheErrors(t *testing.T) {
	// cache 报错 → tryAcquireSingletonLeaderLock 会退到 DB advisory；db 也是 nil
	// → 最终退化为不设闸门，探针照跑（Redis 抖动不该让监控停摆）。
	svc := &stubMonitorSvc{}
	cache := &stubLeaderLockCache{err: errors.New("redis down")}
	r := newRunnerForTest(svc)
	r.SetLeaderLock(cache, nil)

	r.runOne(42, "codex-官方0.3折")

	require.Equal(t, int64(1), svc.runCount.Load())
}
