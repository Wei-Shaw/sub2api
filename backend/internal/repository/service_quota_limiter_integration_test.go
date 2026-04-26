//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ServiceQuotaLimiterSuite struct {
	IntegrationRedisSuite
	limiter service.ServiceQuotaLimiter
}

func TestServiceQuotaLimiterSuite(t *testing.T) {
	suite.Run(t, new(ServiceQuotaLimiterSuite))
}

func (s *ServiceQuotaLimiterSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.limiter = NewServiceQuotaLimiter(s.rdb)
}

// TestServiceQuotaLimiter_Acquire_AtomicUnderConcurrency 验证 Acquire 的 Lua
// 脚本在高并发下严格保证 limit 上限。
//
// 用 100 个 goroutine 同时 Acquire limit=10：旧实现 ZRemRangeByScore + ZCard +
// ZAdd 三步分离会让超出 10 个请求通过；新 Lua 实现必须恰好 10 个返回 true、其
// 余 90 个返回 false。
func (s *ServiceQuotaLimiterSuite) TestServiceQuotaLimiter_Acquire_AtomicUnderConcurrency() {
	const (
		concurrency = 100
		limit       = int64(10)
	)
	key := "svcquota:test:concurrency:atomic"

	var (
		wg           sync.WaitGroup
		acquiredHits int64
	)
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			member := fmt.Sprintf("req-%d", i)
			ok, err := s.limiter.Acquire(s.ctx, key, member, limit)
			require.NoError(s.T(), err)
			if ok {
				atomic.AddInt64(&acquiredHits, 1)
			}
		}()
	}
	wg.Wait()

	require.Equal(s.T(), limit, atomic.LoadInt64(&acquiredHits),
		"Acquire 在 100 并发下必须严格放行 limit=10 个请求")

	// 验证 ZSET 内确实只剩 limit 个 member
	count, err := s.rdb.ZCard(s.ctx, key).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), limit, count, "ZSET 实际占用槽位数必须等于 limit")
}

// TestServiceQuotaLimiter_Acquire_ReleaseFreesSlot 验证 Acquire/Release 配对
// 后槽位可以被新请求复用，确认原子化没有破坏 Release 语义。
func (s *ServiceQuotaLimiterSuite) TestServiceQuotaLimiter_Acquire_ReleaseFreesSlot() {
	ctx := context.Background()
	key := "svcquota:test:concurrency:release"

	for i := 0; i < 3; i++ {
		ok, err := s.limiter.Acquire(ctx, key, fmt.Sprintf("m%d", i), 3)
		require.NoError(s.T(), err)
		require.True(s.T(), ok)
	}
	// 第 4 个请求应被拒
	ok, err := s.limiter.Acquire(ctx, key, "m3", 3)
	require.NoError(s.T(), err)
	require.False(s.T(), ok)

	// 释放一个槽位后，新请求应通过
	require.NoError(s.T(), s.limiter.Release(ctx, key, "m1"))
	ok, err = s.limiter.Acquire(ctx, key, "m4", 3)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
}

// TestServiceQuotaLimiter_Acquire_ZeroLimitFailsOpen 验证 limit<=0 时直接放行
// （fail-open），与历史行为一致。
func (s *ServiceQuotaLimiterSuite) TestServiceQuotaLimiter_Acquire_ZeroLimitFailsOpen() {
	ctx := context.Background()
	ok, err := s.limiter.Acquire(ctx, "svcquota:test:concurrency:zero", "m", 0)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
}
