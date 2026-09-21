package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLiveCreateAttemptBudgetSeparatesClientVetoesFromUpstreamFailures(t *testing.T) {
	budget := liveCreateAttemptBudget{}
	for i := 0; i < MaxCodexClientAdmissionVetoAttempts-1; i++ {
		budget.clientVetoes++
		require.True(t, budget.canContinue(), "客户端准入否决不得消耗普通上游重试预算")
	}
	require.Zero(t, budget.upstreamAttempts)
	budget.clientVetoes++
	require.False(t, budget.canContinue(), "客户端否决必须受独立的 10 次上限约束")

	budget = liveCreateAttemptBudget{clientVetoes: MaxCodexClientAdmissionVetoAttempts - 1}
	for i := 0; i < maxLiveCreateUpstreamAttempts-1; i++ {
		budget.upstreamAttempts++
		require.True(t, budget.canContinue(), "原有上游创建重试应继续允许最多四次尝试")
	}
	budget.upstreamAttempts++
	require.False(t, budget.canContinue(), "客户端否决预算不得把普通上游失败扩大到十次")
}
