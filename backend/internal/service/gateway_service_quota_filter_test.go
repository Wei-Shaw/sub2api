//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeQuotaSvcForFilter 是 FilterAccountsByServiceQuotaSchedulability 测试用的最小桩。
//
// 仅实现 ServiceQuotaService.SnapshotForAccounts —— 其他方法在调度过滤路径不被调用，
// 用 panic 兜底确保意外调用立即暴露（避免静默走错分支）。
type fakeQuotaSvcForFilter struct {
	ServiceQuotaService
	blocks map[int64]*ServiceQuotaPredictedBlock
	err    error
}

func (f *fakeQuotaSvcForFilter) SnapshotForAccounts(_ context.Context, _ ServiceQuotaCheckRequest, _ *ServiceQuotaPreCheckPlan, _ []int64) (map[int64]*ServiceQuotaPredictedBlock, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.blocks, nil
}

func makeAccountsForFilterTest(ids ...int64) []Account {
	out := make([]Account, 0, len(ids))
	for _, id := range ids {
		out = append(out, Account{ID: id, Status: StatusActive, Schedulable: true})
	}
	return out
}

// TestFilterAccountsByServiceQuotaSchedulability_NilQuotaService_NoOp：
// quotaSvc == nil 时直接返回原 slice。
func TestFilterAccountsByServiceQuotaSchedulability_NilQuotaService_NoOp(t *testing.T) {
	t.Parallel()
	accounts := makeAccountsForFilterTest(1, 2, 3)
	got := FilterAccountsByServiceQuotaSchedulability(context.Background(), nil, &ServiceQuotaPreCheckPlan{Rules: []*ServiceQuotaRule{{}}}, ServiceQuotaCheckRequest{}, accounts)
	require.Equal(t, accounts, got)
}

// TestFilterAccountsByServiceQuotaSchedulability_NilPlan_NoOp：plan == nil 时直接返回原 slice。
func TestFilterAccountsByServiceQuotaSchedulability_NilPlan_NoOp(t *testing.T) {
	t.Parallel()
	accounts := makeAccountsForFilterTest(1, 2, 3)
	got := FilterAccountsByServiceQuotaSchedulability(context.Background(), &fakeQuotaSvcForFilter{}, nil, ServiceQuotaCheckRequest{}, accounts)
	require.Equal(t, accounts, got)
}

// TestFilterAccountsByServiceQuotaSchedulability_PartialBlock_RemovesBlocked：
// SnapshotForAccounts 返回部分阻塞 → 仅剔除阻塞账号，其他保留。
func TestFilterAccountsByServiceQuotaSchedulability_PartialBlock_RemovesBlocked(t *testing.T) {
	t.Parallel()
	accounts := makeAccountsForFilterTest(1, 2, 3)
	quotaSvc := &fakeQuotaSvcForFilter{
		blocks: map[int64]*ServiceQuotaPredictedBlock{
			2: {RuleID: 11, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount},
		},
	}
	plan := &ServiceQuotaPreCheckPlan{Rules: []*ServiceQuotaRule{{ID: 11, Enabled: true}}}
	got := FilterAccountsByServiceQuotaSchedulability(context.Background(), quotaSvc, plan, ServiceQuotaCheckRequest{}, accounts)
	require.Len(t, got, 2)
	for _, acc := range got {
		require.NotEqual(t, int64(2), acc.ID)
	}
}

// TestFilterAccountsByServiceQuotaSchedulability_AllBlocked_ReturnsEmpty：
// 全部候选都被阻塞 → 空切片。caller 据此走 ErrNoAvailableAccounts 链路（**禁止**新增专用错误码）。
func TestFilterAccountsByServiceQuotaSchedulability_AllBlocked_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	accounts := makeAccountsForFilterTest(1, 2, 3)
	quotaSvc := &fakeQuotaSvcForFilter{
		blocks: map[int64]*ServiceQuotaPredictedBlock{
			1: {RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount},
			2: {RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount},
			3: {RuleID: 1, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount},
		},
	}
	plan := &ServiceQuotaPreCheckPlan{Rules: []*ServiceQuotaRule{{ID: 1, Enabled: true}}}
	got := FilterAccountsByServiceQuotaSchedulability(context.Background(), quotaSvc, plan, ServiceQuotaCheckRequest{}, accounts)
	require.Empty(t, got)
}

// TestFilterAccountsByServiceQuotaSchedulability_SnapshotError_FailOpen：
// SnapshotForAccounts 报错 → 返回原 slice（fail-open，调度照常进行）。
func TestFilterAccountsByServiceQuotaSchedulability_SnapshotError_FailOpen(t *testing.T) {
	t.Parallel()
	accounts := makeAccountsForFilterTest(1, 2, 3)
	quotaSvc := &fakeQuotaSvcForFilter{err: errors.New("snapshot many failed")}
	plan := &ServiceQuotaPreCheckPlan{Rules: []*ServiceQuotaRule{{ID: 1, Enabled: true}}}
	got := FilterAccountsByServiceQuotaSchedulability(context.Background(), quotaSvc, plan, ServiceQuotaCheckRequest{}, accounts)
	require.Equal(t, accounts, got)
}

// TestAccountSchedChecksFromBlocks_BuildsExpectedMap：blocks → schedChecks 透明转换。
func TestAccountSchedChecksFromBlocks_BuildsExpectedMap(t *testing.T) {
	t.Parallel()

	t.Run("nil/empty input → nil map", func(t *testing.T) {
		require.Nil(t, AccountSchedChecksFromBlocks(nil))
		require.Nil(t, AccountSchedChecksFromBlocks(map[int64]*ServiceQuotaPredictedBlock{}))
	})

	t.Run("populated map", func(t *testing.T) {
		block := &ServiceQuotaPredictedBlock{RuleID: 11, LimiterType: ServiceQuotaLimiterRPM, ScopeKind: ServiceQuotaPredictedBlockScopeAccount}
		got := AccountSchedChecksFromBlocks(map[int64]*ServiceQuotaPredictedBlock{7: block})
		require.Len(t, got, 1)
		require.NotNil(t, got[7])
		require.Same(t, block, got[7].QuotaBlock)
	})
}
