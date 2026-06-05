//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// ruleRPMChannelWithArrivalToggle 构造 channel-scope 的 RPM 规则，可控制 CountOnArrival。
//
// 不复用 ruleRPMChannel：那个工厂为兼容老断言固定 CountOnArrival=true，
// 这里需要把开关作为参数显式表达。
func ruleRPMChannelWithArrivalToggle(id, channelID int64, limit float64, countOnArrival bool) *ServiceQuotaRule {
	ch := channelID
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModePerUser,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit, CountOnArrival: countOnArrival},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, ChannelID: &ch},
		},
	}
}

// TestRPMCountOnArrival_True 验证旧"到达即计"语义：PreCheckAcquire 阶段对每次匹配请求 +1，
// Record 不再重复 +1（避免双倍计数）。
func TestRPMCountOnArrival_True(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleRPMChannelWithArrivalToggle(101, 99, 100, true)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	// PreCheckAcquire：channel 命中 → +1。
	lease, err := svc.PreCheckAcquire(context.Background(), plan, 99, 0)
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Len(t, limiter.incrementCalls, 1, "CountOnArrival=true 时 arrival 阶段应 Increment(+1)")

	// Record：CountOnArrival=true 走 Increment-on-arrival 旧语义，Record 不再重复计数。
	prev := len(limiter.incrementCalls)
	svc.Record(context.Background(), ServiceQuotaRecordRequest{
		ServiceQuotaCheckRequest: ServiceQuotaCheckRequest{UserID: 42, ChannelID: 99},
		InputTokens:              10,
	})
	require.Equal(t, prev, len(limiter.incrementCalls), "CountOnArrival=true 时 Record 不应再次 +1")
}

// TestRPMCountOnArrival_False 验证新"成功才计"语义：PreCheckAcquire 阶段不写入计数（仅判超限），
// Record 阶段成功落账后 +1。
func TestRPMCountOnArrival_False(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleRPMChannelWithArrivalToggle(102, 99, 100, false)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	// PreCheckAcquire：channel 命中但 CountOnArrival=false → 仅 Current 只读判超限，不写入。
	lease, err := svc.PreCheckAcquire(context.Background(), plan, 99, 0)
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Empty(t, limiter.incrementCalls, "CountOnArrival=false 时 arrival 阶段不应 Increment")

	// Record：成功落账后 +1。
	svc.Record(context.Background(), ServiceQuotaRecordRequest{
		ServiceQuotaCheckRequest: ServiceQuotaCheckRequest{UserID: 42, ChannelID: 99},
		InputTokens:              10,
	})
	require.Len(t, limiter.incrementCalls, 1, "CountOnArrival=false 时 Record 应 +1")
}

// TestRPMCountOnArrival_False_BlocksWhenAlreadyAtLimit 验证只读路径同样能拦住超限请求：
// 当计数器已经累计到 limit，CountOnArrival=false 的下一次请求必须被拒。
func TestRPMCountOnArrival_False_BlocksWhenAlreadyAtLimit(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleRPMChannelWithArrivalToggle(103, 99, 2, false)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)

	// 模拟两次成功调用：每次 PreCheckAcquire（不写）+ Record（+1） → 计数器达 limit=2。
	for i := 0; i < 2; i++ {
		lease, err := svc.PreCheckAcquire(context.Background(), plan, 99, 0)
		require.NoError(t, err)
		require.NotNil(t, lease)
		svc.Record(context.Background(), ServiceQuotaRecordRequest{
			ServiceQuotaCheckRequest: ServiceQuotaCheckRequest{UserID: 42, ChannelID: 99},
			InputTokens:              10,
		})
	}
	require.Len(t, limiter.incrementCalls, 2)

	// 第三次 PreCheckAcquire 应被拒：used=2 已 >= limit=2。
	lease, err := svc.PreCheckAcquire(context.Background(), plan, 99, 0)
	require.Error(t, err)
	require.Nil(t, lease)
	require.True(t, errors.Is(err, ErrServiceQuotaExceeded))
	// 拒绝路径不应触发额外 Increment（区别于旧 to-arrival 路径会先 +1 再判超）。
	require.Len(t, limiter.incrementCalls, 2, "只读路径下被拒请求不写计数")
}

// TestShouldIncrementOnArrival_OnlyRPM 验证 helper 仅对 RPM 生效，其他 limiter type 恒返回 false。
func TestShouldIncrementOnArrival_OnlyRPM(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		def  ServiceQuotaLimiterDef
		want bool
	}{
		{"rpm count_on_arrival=true", ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterRPM, CountOnArrival: true}, true},
		{"rpm count_on_arrival=false", ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterRPM, CountOnArrival: false}, false},
		{"tpm 忽略 count_on_arrival", ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterTPM, CountOnArrival: true}, false},
		{"tpd 忽略 count_on_arrival", ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterTPD, CountOnArrival: true}, false},
		{"daily_usd 忽略 count_on_arrival", ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterDailyUSD, CountOnArrival: true}, false},
		{"concurrency 忽略 count_on_arrival", ServiceQuotaLimiterDef{LimiterType: ServiceQuotaLimiterConcurrency, CountOnArrival: true}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.def.ShouldIncrementOnArrival())
		})
	}
}

// TestNormalizeLimiterCountOnArrival_NonRPMCleared 验证 normalize 阶段把非 RPM 的 CountOnArrival 强制置 nil，
// 避免管理员误填污染存储。
func TestNormalizeLimiterCountOnArrival_NonRPMCleared(t *testing.T) {
	t.Parallel()
	truthy := true
	for _, kind := range []string{
		ServiceQuotaLimiterTPM,
		ServiceQuotaLimiterTPD,
		ServiceQuotaLimiterDailyUSD,
		ServiceQuotaLimiterConcurrency,
	} {
		l := &ServiceQuotaLimiterInput{LimiterType: kind, CountOnArrival: &truthy}
		normalizeLimiterCountOnArrival(l)
		require.Nil(t, l.CountOnArrival, "non-RPM limiter type %q must clear CountOnArrival", kind)
	}

	// RPM 保留传入值。
	l := &ServiceQuotaLimiterInput{LimiterType: ServiceQuotaLimiterRPM, CountOnArrival: &truthy}
	normalizeLimiterCountOnArrival(l)
	require.NotNil(t, l.CountOnArrival)
	require.True(t, *l.CountOnArrival)
}
