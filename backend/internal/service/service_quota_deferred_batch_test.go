//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// ruleDeferredFanout 构造一条带 N 个 deferred limiter 的规则（用 path 数 × limiter 数撑批量）。
//
// 用 TPM/TPD/DailyUSD 三种 limiter type 同时存在；path 数由调用方控制。N path × 3 deferred limiter
// 形成笛卡尔积，PreCheckAcquire 第三轮应该把 3N 个 SnapshotKey 一次发给 SnapshotMany。
func ruleDeferredFanout(id int64, paths int) *ServiceQuotaRule {
	pathDefs := make([]ServiceQuotaPathDef, paths)
	for i := 0; i < paths; i++ {
		pathDefs[i] = ServiceQuotaPathDef{ID: id*100 + int64(i+1), RuleID: id}
	}
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModeShared,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*10 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterTPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: 1000000},
			{ID: id*10 + 2, RuleID: id, LimiterType: ServiceQuotaLimiterTPD, WindowMode: ServiceQuotaWindowFixed, LimitValue: 50000000},
			{ID: id*10 + 3, RuleID: id, LimiterType: ServiceQuotaLimiterDailyUSD, WindowMode: ServiceQuotaWindowFixed, LimitValue: 100},
		},
		Paths: pathDefs,
	}
}

// TestPreCheckAcquire_DeferredBatch_SingleRoundTrip 验证 deferred 轮一次 SnapshotMany 拿全部用量，
// round-trip 数从 N 降到 1。
//
// 旧实现：N 个 deferred limiter 各自调一次 Current → N 次 Redis RTT。
// 新实现：collectDeferredEntries → 一次 SnapshotMany → 逐个判超限。
//
// 验证手段：fakeLimiter.snapshotManyCalls 长度恰为 1，单次调用包含的 keys 数等于 path × limiter 笛卡尔积。
func TestPreCheckAcquire_DeferredBatch_SingleRoundTrip(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleDeferredFanout(1, 4) // 4 path × 3 deferred limiter = 12 deferred entries
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	lease, err := svc.PreCheckAcquire(context.Background(), plan, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, lease)

	// 关键断言：SnapshotMany 只调用 1 次（不是逐个调 N 次）。
	require.Len(t, limiter.snapshotManyCalls, 1, "deferred 轮应一次 SnapshotMany 拿全部用量")
	require.Len(t, limiter.snapshotManyCalls[0], 12, "4 path × 3 deferred limiter 应展开为 12 个 SnapshotKey")
}

// TestPreCheckAcquire_DeferredBatch_DetectsExceeded 验证批量路径仍能正确判超限并短路返回。
//
// 预置某个 deferred key 的 used=LimitValue → SnapshotMany 返回该值 → 主流程 used >= limit 拒绝。
// 验证返回的 ApplicationError 是 ErrServiceQuotaExceeded（与旧逐个路径错误码一致）。
func TestPreCheckAcquire_DeferredBatch_DetectsExceeded(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleDeferredFanout(2, 1) // 1 path × 3 deferred limiter
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	// 预置 TPD key 用量 = LimitValue（会触发 used >= LimitValue 拒绝）。
	tpdKey := BuildServiceQuotaCounterKey(2, 201, ServiceQuotaLimiterTPD, nil)
	limiter.counters[tpdKey] = 50000000 // 等于 ruleDeferredFanout 配的 LimitValue

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)

	lease, err := svc.PreCheckAcquire(context.Background(), plan, 0, 0)
	require.Error(t, err, "TPD 用量达上限应被拒绝")
	require.Nil(t, lease)
	require.ErrorIs(t, err, ErrServiceQuotaExceeded)

	// 仍然只 SnapshotMany 一次（短路是在 collect 之后的逐个判定阶段）。
	require.Len(t, limiter.snapshotManyCalls, 1)
}

// TestPreCheckAcquire_DeferredBatch_NoDeferredLimiters_NoCall 验证当 matched 全是 concurrency/RPM 时，
// SnapshotMany 不会被调用（避免空批次浪费）。
func TestPreCheckAcquire_DeferredBatch_NoDeferredLimiters_NoCall(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	// ruleConcurrencyAccount 只有 concurrency limiter，无 deferred。
	rule := ruleConcurrencyAccount(3, 7, 5)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	lease, err := svc.PreCheckAcquire(context.Background(), plan, 0, 7)
	require.NoError(t, err)
	require.NotNil(t, lease)

	require.Empty(t, limiter.snapshotManyCalls, "无 deferred limiter 时不应触发 SnapshotMany")
}
