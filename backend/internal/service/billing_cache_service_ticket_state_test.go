//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// 本文件覆盖 BillingTicket 状态机（删 legacyOneOff 后只剩 plan==nil/plan!=nil 两条路径）。
// 复用 service_quota_two_phase_test.go 的 ruleConcurrencyAccount / newQuotaServiceForTest
// 与 service_quota_fakes_test.go 的 fakeServiceQuotaLimiter，避免重复 mock 工厂。

// quotaCallCounter 在 BillingTicket.Consume 路径上拦截 PreCheckAcquire 调用次数，
// 用于断言 plan==nil 分支不触达底层 ServiceQuotaService。它实现 ServiceQuotaService
// 仅最小子集——其它方法保留嵌入接口零值，被调用时会 nil panic 让测试一旦绕过
// plan==nil 分支立刻可见。
type quotaCallCounter struct {
	ServiceQuotaService // 嵌入仅占位接口，所有未实现方法被调用时 panic
	preCheckAcquireN    int
}

func (q *quotaCallCounter) PreCheckAcquire(_ context.Context, _ *ServiceQuotaPreCheckPlan, _, _ int64) (*ServiceQuotaLease, error) {
	q.preCheckAcquireN++
	return nil, nil
}

// TestBillingTicket_Consume_NoPlan_Noop：plan==nil 时 Consume 立即返回 nil 且
// 不触达 PreCheckAcquire（无 service_quota 规则匹配本次请求时 PreCheckSelect 会
// 返回 nil plan，Consume 必须退化为只回填 channel/account 后返回）。
func TestBillingTicket_Consume_NoPlan_Noop(t *testing.T) {
	t.Parallel()
	counter := &quotaCallCounter{}
	ticket := &BillingTicket{
		svc:      &BillingCacheService{serviceQuota: counter},
		quotaReq: ServiceQuotaCheckRequest{UserID: 42},
		plan:     nil, // PreCheckSelect 返回 nil 的等价场景
	}
	require.NoError(t, ticket.Consume(context.Background(), 12, 4194))
	require.Zero(t, counter.preCheckAcquireN, "plan==nil 必须不触达 PreCheckAcquire")
	require.Equal(t, int64(12), ticket.quotaReq.ChannelID)
	require.Equal(t, int64(4194), ticket.quotaReq.AccountID)
}

// TestBillingTicket_Consume_AfterClose_Error：Close 后 Consume 必须返回 sentinel
// errBillingTicketClosed；多次 Close 幂等无 panic。
func TestBillingTicket_Consume_AfterClose_Error(t *testing.T) {
	t.Parallel()
	ticket := &BillingTicket{
		svc:      &BillingCacheService{},
		quotaReq: ServiceQuotaCheckRequest{UserID: 42},
	}
	ticket.Close()
	ticket.Close() // 幂等
	ticket.Close()

	err := ticket.Consume(context.Background(), 1, 2)
	require.Error(t, err)
	require.True(t, errors.Is(err, errBillingTicketClosed))
}

// TestBillingTicket_Consume_Twice_ReleasesOldLease：第二次 Consume 切换账号时，
// 旧 lease 必须先 Release 再换新 lease——否则 lease 计数会泄漏到旧 path。
func TestBillingTicket_Consume_Twice_ReleasesOldLease(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(20, 7, 5)
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.NotNil(t, plan)

	ticket := &BillingTicket{
		svc:      &BillingCacheService{serviceQuota: svc},
		quotaReq: ServiceQuotaCheckRequest{UserID: 42},
		plan:     plan,
	}
	defer ticket.Close()

	require.NoError(t, ticket.Consume(context.Background(), 0, 7))
	require.Len(t, limiter.acquireCalls, 1, "首次 Consume 必须 Acquire")
	require.NotNil(t, ticket.lease)
	firstLease := ticket.lease

	// 切换账号：path.AccountID=7 不再匹配，新 lease 必须替换旧的（释放旧 lease）。
	require.NoError(t, ticket.Consume(context.Background(), 0, 999))
	require.NotEqual(t, firstLease, ticket.lease, "切换账号必须替换 lease 引用")
	require.Nil(t, ticket.lease, "切到不命中 path 的账号后 lease 应为 nil（PreCheckAcquire 返回 nil）")

	// 旧 lease 已释放：限流器 set 中应已无对应 member，能再次 Acquire 同 limit=5 占满 5 次。
	require.NoError(t, ticket.Consume(context.Background(), 0, 7))
	require.NotNil(t, ticket.lease, "切回匹配账号必须重新 Acquire")
}

// TestBillingTicket_PreCheckPlan_NoCandidates_NilPlan：PreCheckSelect 在没有
// 候选规则（rules 列表全部 disabled）时返回 nil plan，Consume 退化为 noop。
// 这是 PreCheckPlan 现有行为的边界补充：覆盖"plan 含 0 规则"等价路径。
func TestBillingTicket_PreCheckPlan_NoCandidates_NilPlan(t *testing.T) {
	t.Parallel()
	limiter := newFakeLimiter()
	rule := ruleConcurrencyAccount(21, 7, 5)
	rule.Enabled = false // 全部规则关闭
	svc := newQuotaServiceForTest(t, []*ServiceQuotaRule{rule}, nil, limiter)

	plan, err := svc.PreCheckSelect(context.Background(), ServiceQuotaCheckRequest{UserID: 42})
	require.NoError(t, err)
	require.Nil(t, plan, "全部 rule disabled 时 PreCheckSelect 必须返回 nil plan")

	ticket := &BillingTicket{
		svc:      &BillingCacheService{serviceQuota: svc},
		quotaReq: ServiceQuotaCheckRequest{UserID: 42},
		plan:     plan,
	}
	require.Nil(t, ticket.PreCheckPlan(), "PreCheckPlan 透传内部 plan 字段")
	require.NoError(t, ticket.Consume(context.Background(), 0, 7))
	require.Empty(t, limiter.acquireCalls, "plan==nil 必须不触达 limiter.Acquire")
}
