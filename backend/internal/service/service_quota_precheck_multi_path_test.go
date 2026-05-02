//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 本文件覆盖 PreCheckAcquire 的"同一规则内多 path 计数互不干扰"场景。
// 复用 service_quota_two_phase_test.go 的 newQuotaServiceForTest 与
// service_quota_fakes_test.go 的 fakeServiceQuotaLimiter（其 incrementCalls
// 记录每次 Increment 调用的 redis key，便于按 key 分组断言）。

// ruleRPMTwoModelPaths 构造一条规则：
//   - 1 个 RPM limiter（CountOnArrival=true，超 limit=3 触发拒绝）
//   - 2 条 path：account_id=accountID + 不同 model_pattern（pathA / pathB）
//
// 用于验证同一规则下不同 path 的 redis counter key 互不重叠（key 前缀含 path_id）。
func ruleRPMTwoModelPaths(id, accountID int64, limit float64, patternA, patternB string) *ServiceQuotaRule {
	acc := accountID
	pa, pb := patternA, patternB
	return &ServiceQuotaRule{
		ID:          id,
		Enabled:     true,
		CounterMode: ServiceQuotaCounterModePerUser,
		Limiters: []ServiceQuotaLimiterDef{
			{ID: id*100 + 1, RuleID: id, LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: limit, CountOnArrival: true},
		},
		Paths: []ServiceQuotaPathDef{
			{ID: id*100 + 11, RuleID: id, AccountID: &acc, ModelPattern: &pa},
			{ID: id*100 + 12, RuleID: id, AccountID: &acc, ModelPattern: &pb},
		},
	}
}

// countKeysContaining 数 incrementCalls 中包含 substr 的 key 数量——
// path_id 编码进 key（svcquota:v2:<rule>:<path>:<type>:<scope>），按 path_id 子串
// 即可区分不同 path 的写入次数。
func countKeysContaining(keys []string, substr string) int {
	n := 0
	for _, k := range keys {
		if strings.Contains(k, substr) {
			n++
		}
	}
	return n
}

// TestPreCheckAcquire_SameRuleMultiPath_IndependentRPMCounters：
// 同一规则 r、1 个 RPM limiter（limit=3）、2 条 path（model="alpha"、model="beta"）。
//   - 发 3 个 model="alpha" → counter A == 3，counter B == 0
//   - 第 4 个 alpha → 超限拒绝
//   - 紧接着 1 个 model="beta" → 通过，counter B == 1（独立计数）
func TestPreCheckAcquire_SameRuleMultiPath_IndependentRPMCounters(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleRPMTwoModelPaths(30, 100, 3, "alpha", "beta")
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	// 3 个 alpha 请求都通过，每次只增 path A 的计数。
	for i := 0; i < 3; i++ {
		// PreCheckAcquire 接口签名只接 channelID/accountID；req.Model 走 plan.Req
		// → 重置 plan.Req.Model 模拟新请求的 model 维度。
		plan.Req.Model = "alpha"
		_, err := svc.PreCheckAcquire(context.Background(), plan, 0, 100)
		require.NoError(t, err, "第 %d 次 alpha 请求应该通过", i+1)
	}
	pathAID := ":3011:" // svcquota:v2:30:3011:rpm:42
	pathBID := ":3012:"
	require.Equal(t, 3, countKeysContaining(limiter.incrementCalls, pathAID), "path A 必须计数 3 次")
	require.Equal(t, 0, countKeysContaining(limiter.incrementCalls, pathBID), "path B 必须 0 次")

	// 第 4 个 alpha 触发超限。CountOnArrival=true 路径会先 Increment(+1) 再判越限——
	// 越限分支会做 best-effort -1 回滚，但 fake limiter 的 incrementCalls 已记录 +1 那次写。
	plan.Req.Model = "alpha"
	_, err = svc.PreCheckAcquire(context.Background(), plan, 0, 100)
	require.Error(t, err, "第 4 个 alpha 请求必须超限拒绝")
	require.True(t, errors.Is(err, ErrServiceQuotaExceeded))

	// 紧接着 model=beta：path B 独立计数，应通过。
	plan.Req.Model = "beta"
	_, err = svc.PreCheckAcquire(context.Background(), plan, 0, 100)
	require.NoError(t, err, "model=beta 走独立 path B 计数器应通过")
	require.Equal(t, 1, countKeysContaining(limiter.incrementCalls, pathBID), "path B 仅 1 次写入")
}

// TestPreCheckAcquire_AccountScopeMismatch_NoCount：path.AccountID=100 但
// 请求 accountID=200 时，PreCheckAcquire 不命中任何 path → Increment 调用 0 次。
// 用 fake limiter 的 incrementCalls 长度断言。
func TestPreCheckAcquire_AccountScopeMismatch_NoCount(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleRPMTwoModelPaths(31, 100, 3, "alpha", "beta")
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	plan.Req.Model = "alpha"
	lease, err := svc.PreCheckAcquire(context.Background(), plan, 0, 200)
	require.NoError(t, err)
	require.Nil(t, lease, "account 不命中 → 无 path → 无 lease")
	require.Empty(t, limiter.incrementCalls, "account scope mismatch 必须不写计数")
}
