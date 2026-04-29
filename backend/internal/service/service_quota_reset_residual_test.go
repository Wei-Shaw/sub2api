//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestResetCountersForUser_ClearsUserScopedKeys 验证：清理某用户残留 counter 时，
// pattern "svcquota:v2:*:*:*:<user_id>" 命中所有规则下该用户的 counter，
// 但不会误命中同 path 下 target=shared 的 key。
//
// 这是该 fix 的核心安全性：删除用户不能让规则内的 shared 计数器一并被清。
func TestResetCountersForUser_ClearsUserScopedKeys(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()

	// 预置三类 key：user 7 / user 8 / shared
	keyUser7 := BuildServiceQuotaCounterKey(1, 100, ServiceQuotaLimiterRPM, ptrInt64Local(7))
	keyUser8 := BuildServiceQuotaCounterKey(1, 100, ServiceQuotaLimiterRPM, ptrInt64Local(8))
	keyShared := BuildServiceQuotaCounterKey(1, 100, ServiceQuotaLimiterRPM, nil)
	limiter.counters[keyUser7] = 5
	limiter.counters[keyUser8] = 9
	limiter.counters[keyShared] = 42

	svc := newQuotaServiceForTest(t, nil, nil, limiter)

	svc.ResetCountersForUser(context.Background(), 7)

	// fire-and-forget：等到 user 7 的 key 被清掉。
	require.Eventually(t, func() bool {
		_, ok := limiter.counterValue(keyUser7)
		return !ok
	}, 500*time.Millisecond, 5*time.Millisecond, "user 7 的 counter 应被清除")

	// user 8 与 shared 必须保留——pattern 不能误伤其他作用域。
	{
		v8, _ := limiter.counterValue(keyUser8)
		require.Equal(t, float64(9), v8, "user 8 不应被波及")
	}
	{
		vs, _ := limiter.counterValue(keyShared)
		require.Equal(t, float64(42), vs, "shared key 不应被波及")
	}
}

// TestResetCountersForPaths_ClearsAllRulesAtPath 验证：path 删除时，
// pattern "svcquota:v2:*:<path_id>:*:*" 跨规则跨 limiter 跨 scope 全部命中。
//
// 实际场景：channel 被删 → 它关联的 path_id=200 在所有规则下的 counter 都该清掉。
func TestResetCountersForPaths_ClearsAllRulesAtPath(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()

	// path=200 跨两条规则、两个 limiter type、user 与 shared 共 4 个 key
	k1 := BuildServiceQuotaCounterKey(1, 200, ServiceQuotaLimiterRPM, ptrInt64Local(7))
	k2 := BuildServiceQuotaCounterKey(1, 200, ServiceQuotaLimiterTPM, nil)
	k3 := BuildServiceQuotaCounterKey(2, 200, ServiceQuotaLimiterRPM, ptrInt64Local(8))
	k4 := BuildServiceQuotaCounterKey(2, 200, ServiceQuotaLimiterDailyUSD, nil)
	// path=300 不该被波及
	k5 := BuildServiceQuotaCounterKey(1, 300, ServiceQuotaLimiterRPM, nil)

	for _, k := range []string{k1, k2, k3, k4, k5} {
		limiter.counters[k] = 1
	}

	svc := newQuotaServiceForTest(t, nil, nil, limiter)
	svc.ResetCountersForPaths(context.Background(), []int64{200})

	require.Eventually(t, func() bool {
		_, ok1 := limiter.counterValue(k1)
		_, ok2 := limiter.counterValue(k2)
		_, ok3 := limiter.counterValue(k3)
		_, ok4 := limiter.counterValue(k4)
		return !ok1 && !ok2 && !ok3 && !ok4
	}, 500*time.Millisecond, 5*time.Millisecond, "path=200 的所有 counter 都应被清除")

	// path=300 必须保留
	{
		v5, _ := limiter.counterValue(k5)
		require.Equal(t, float64(1), v5, "path=300 的 counter 不应被波及")
	}
}

// TestResetCountersForPaths_EmptyOrZero_NoOp 验证空切片 / 0 path_id 不会触发 ResetPattern。
func TestResetCountersForPaths_EmptyOrZero_NoOp(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	keyShared := BuildServiceQuotaCounterKey(1, 100, ServiceQuotaLimiterRPM, nil)
	limiter.counters[keyShared] = 1

	svc := newQuotaServiceForTest(t, nil, nil, limiter)
	svc.ResetCountersForPaths(context.Background(), nil)
	svc.ResetCountersForPaths(context.Background(), []int64{0, -1})

	// 给异步 goroutine 一点时间但都应该是 noop。
	time.Sleep(20 * time.Millisecond)
	{
		vs, _ := limiter.counterValue(keyShared)
		require.Equal(t, float64(1), vs, "空 / 0 / 负 path_id 不应触发任何清理")
	}
}

// TestResetCountersForUser_ZeroOrNegative_NoOp 验证 user_id <= 0 时函数 noop。
func TestResetCountersForUser_ZeroOrNegative_NoOp(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	keyShared := BuildServiceQuotaCounterKey(1, 100, ServiceQuotaLimiterRPM, nil)
	limiter.counters[keyShared] = 1

	svc := newQuotaServiceForTest(t, nil, nil, limiter)
	svc.ResetCountersForUser(context.Background(), 0)
	svc.ResetCountersForUser(context.Background(), -5)

	time.Sleep(20 * time.Millisecond)
	{
		vs, _ := limiter.counterValue(keyShared)
		require.Equal(t, float64(1), vs)
	}
}

// TestBuildServiceQuotaCounterKeyPatternForUser 验证 pattern 文本格式。
func TestBuildServiceQuotaCounterKeyPatternForUser(t *testing.T) {
	t.Parallel()
	require.Equal(t, "svcquota:v2:*:*:*:42", BuildServiceQuotaCounterKeyPatternForUser(42))
}

// TestBuildServiceQuotaCounterKeyPatternForPath 验证 pattern 文本格式。
func TestBuildServiceQuotaCounterKeyPatternForPath(t *testing.T) {
	t.Parallel()
	require.Equal(t, "svcquota:v2:*:200:*:*", BuildServiceQuotaCounterKeyPatternForPath(200))
}

// 复用 ptrInt64 失败时的兜底（同 package 内若已有同名函数会冲突，所以加 Local 后缀）。
func ptrInt64Local(v int64) *int64 { return &v }
