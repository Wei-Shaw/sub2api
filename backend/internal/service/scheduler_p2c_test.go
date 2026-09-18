package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectedAccountPressureUsesSchedulingCapacityAndQueue(t *testing.T) {
	loadFactor := 20
	candidate := accountWithLoad{
		account:  &Account{Concurrency: 5, LoadFactor: &loadFactor},
		loadInfo: &AccountLoadInfo{CurrentConcurrency: 4, WaitingCount: 1},
	}
	require.InDelta(t, 0.30, projectedAccountPressure(candidate), 0.0001)
}

func TestAccountHasHardCapacityIgnoresVirtualLoadFactor(t *testing.T) {
	lowVirtualCapacity := 2
	candidate := accountWithLoad{
		account:  &Account{Concurrency: 10, LoadFactor: &lowVirtualCapacity},
		loadInfo: &AccountLoadInfo{CurrentConcurrency: 3, LoadRate: 150},
	}
	require.True(t, accountHasHardCapacity(candidate), "virtual scheduling capacity must not close real concurrency slots")
	candidate.loadInfo.CurrentConcurrency = 10
	require.False(t, accountHasHardCapacity(candidate))
}

func TestBuildPowerOfTwoSelectionOrderPreservesPriorityTiers(t *testing.T) {
	available := []accountWithLoad{
		{account: &Account{ID: 1, Priority: 2, Concurrency: 5}, loadInfo: &AccountLoadInfo{}},
		{account: &Account{ID: 2, Priority: 1, Concurrency: 5}, loadInfo: &AccountLoadInfo{}},
		{account: &Account{ID: 3, Priority: 1, Concurrency: 5}, loadInfo: &AccountLoadInfo{}},
	}
	order := buildPowerOfTwoSelectionOrder(available, false, false)
	require.Len(t, order, 3)
	require.Equal(t, 1, order[0].account.Priority)
	require.Equal(t, 1, order[1].account.Priority)
	require.Equal(t, int64(1), order[2].account.ID)
}

func TestNewAccountPoolWaitPlanDeduplicatesCandidates(t *testing.T) {
	first := &Account{ID: 1, Concurrency: 2}
	second := &Account{ID: 2, Concurrency: 4}
	plan := newAccountPoolWaitPlan([]*Account{first, first, second}, 0, 10)
	require.NotNil(t, plan)
	require.Equal(t, int64(1), plan.AccountID)
	require.Len(t, plan.Candidates, 2)
	require.Equal(t, int64(2), plan.Candidates[1].Account.ID)
}
