//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ============ shuffleWithinSortGroups 测试 ============

func TestShuffleWithinSortGroups_Empty(t *testing.T) {
	shuffleWithinSortGroups(nil)
	shuffleWithinSortGroups([]accountWithLoad{})
}

func TestShuffleWithinSortGroups_SingleElement(t *testing.T) {
	accounts := []accountWithLoad{
		{account: &Account{ID: 1, Priority: 1}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
	}
	shuffleWithinSortGroups(accounts)
	require.Equal(t, int64(1), accounts[0].account.ID)
}

func TestShuffleWithinSortGroups_DifferentGroups_OrderPreserved(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)

	accounts := []accountWithLoad{
		{account: &Account{ID: 1, Priority: 1, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
		{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
		{account: &Account{ID: 3, Priority: 2, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
	}

	// 每个元素都属于不同组（Priority 或 LoadRate 或 LastUsedAt 不同），顺序不变
	for i := 0; i < 20; i++ {
		cpy := make([]accountWithLoad, len(accounts))
		copy(cpy, accounts)
		shuffleWithinSortGroups(cpy)
		require.Equal(t, int64(1), cpy[0].account.ID)
		require.Equal(t, int64(2), cpy[1].account.ID)
		require.Equal(t, int64(3), cpy[2].account.ID)
	}
}

func TestShuffleWithinSortGroups_SameGroup_Shuffled(t *testing.T) {
	now := time.Now()
	// 同一秒的时间戳视为同一组
	sameSecond := time.Unix(now.Unix(), 0)
	sameSecond2 := time.Unix(now.Unix(), 500_000_000) // 同一秒但不同纳秒

	accounts := []accountWithLoad{
		{account: &Account{ID: 1, Priority: 1, LastUsedAt: &sameSecond}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
		{account: &Account{ID: 2, Priority: 1, LastUsedAt: &sameSecond2}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
		{account: &Account{ID: 3, Priority: 1, LastUsedAt: &sameSecond}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
	}

	// 多次执行，验证所有 ID 都出现在第一个位置（说明确实被打乱了）
	seen := map[int64]bool{}
	for i := 0; i < 100; i++ {
		cpy := make([]accountWithLoad, len(accounts))
		copy(cpy, accounts)
		shuffleWithinSortGroups(cpy)
		seen[cpy[0].account.ID] = true
		// 无论怎么打乱，所有 ID 都应在候选中
		ids := map[int64]bool{}
		for _, a := range cpy {
			ids[a.account.ID] = true
		}
		require.True(t, ids[1] && ids[2] && ids[3])
	}
	// 至少 2 个不同的 ID 出现在首位（随机性验证）
	require.GreaterOrEqual(t, len(seen), 2, "shuffle should produce different orderings")
}

func TestShuffleWithinSortGroups_NilLastUsedAt_SameGroup(t *testing.T) {
	accounts := []accountWithLoad{
		{account: &Account{ID: 1, Priority: 1, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{LoadRate: 0}},
		{account: &Account{ID: 2, Priority: 1, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{LoadRate: 0}},
		{account: &Account{ID: 3, Priority: 1, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{LoadRate: 0}},
	}

	seen := map[int64]bool{}
	for i := 0; i < 100; i++ {
		cpy := make([]accountWithLoad, len(accounts))
		copy(cpy, accounts)
		shuffleWithinSortGroups(cpy)
		seen[cpy[0].account.ID] = true
	}
	require.GreaterOrEqual(t, len(seen), 2, "nil LastUsedAt accounts should be shuffled")
}

func TestShuffleWithinSortGroups_MixedGroups(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	sameAsNow := time.Unix(now.Unix(), 0)

	// 组1: Priority=1, LoadRate=10, LastUsedAt=earlier (ID 1)  — 单元素组
	// 组2: Priority=1, LoadRate=20, LastUsedAt=now (ID 2, 3)   — 双元素组
	// 组3: Priority=2, LoadRate=10, LastUsedAt=earlier (ID 4)  — 单元素组
	accounts := []accountWithLoad{
		{account: &Account{ID: 1, Priority: 1, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
		{account: &Account{ID: 2, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
		{account: &Account{ID: 3, Priority: 1, LastUsedAt: &sameAsNow}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
		{account: &Account{ID: 4, Priority: 2, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
	}

	for i := 0; i < 20; i++ {
		cpy := make([]accountWithLoad, len(accounts))
		copy(cpy, accounts)
		shuffleWithinSortGroups(cpy)

		// 组间顺序不变
		require.Equal(t, int64(1), cpy[0].account.ID, "group 1 position fixed")
		require.Equal(t, int64(4), cpy[3].account.ID, "group 3 position fixed")

		// 组2 内部可以打乱，但仍在位置 1 和 2
		mid := map[int64]bool{cpy[1].account.ID: true, cpy[2].account.ID: true}
		require.True(t, mid[2] && mid[3], "group 2 elements should stay in positions 1-2")
	}
}

// ============ shuffleWithinPriorityAndLastUsed 测试 ============

func TestShuffleWithinPriorityAndLastUsed_Empty(t *testing.T) {
	shuffleWithinPriorityAndLastUsed(nil, false)
	shuffleWithinPriorityAndLastUsed([]*Account{}, false)
}

func TestShuffleWithinPriorityAndLastUsed_SingleElement(t *testing.T) {
	accounts := []*Account{{ID: 1, Priority: 1}}
	shuffleWithinPriorityAndLastUsed(accounts, false)
	require.Equal(t, int64(1), accounts[0].ID)
}

func TestShuffleWithinPriorityAndLastUsed_SameGroup_Shuffled(t *testing.T) {
	accounts := []*Account{
		{ID: 1, Priority: 1, LastUsedAt: nil},
		{ID: 2, Priority: 1, LastUsedAt: nil},
		{ID: 3, Priority: 1, LastUsedAt: nil},
	}

	seen := map[int64]bool{}
	for i := 0; i < 100; i++ {
		cpy := make([]*Account, len(accounts))
		copy(cpy, accounts)
		shuffleWithinPriorityAndLastUsed(cpy, false)
		seen[cpy[0].ID] = true
	}
	require.GreaterOrEqual(t, len(seen), 2, "same group should be shuffled")
}

func TestShuffleWithinPriorityAndLastUsed_DifferentPriority_OrderPreserved(t *testing.T) {
	accounts := []*Account{
		{ID: 1, Priority: 1, LastUsedAt: nil},
		{ID: 2, Priority: 2, LastUsedAt: nil},
		{ID: 3, Priority: 3, LastUsedAt: nil},
	}

	for i := 0; i < 20; i++ {
		cpy := make([]*Account, len(accounts))
		copy(cpy, accounts)
		shuffleWithinPriorityAndLastUsed(cpy, false)
		require.Equal(t, int64(1), cpy[0].ID)
		require.Equal(t, int64(2), cpy[1].ID)
		require.Equal(t, int64(3), cpy[2].ID)
	}
}

func TestShuffleWithinPriorityAndLastUsed_DifferentLastUsedAt_OrderPreserved(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)

	accounts := []*Account{
		{ID: 1, Priority: 1, LastUsedAt: nil},
		{ID: 2, Priority: 1, LastUsedAt: &earlier},
		{ID: 3, Priority: 1, LastUsedAt: &now},
	}

	for i := 0; i < 20; i++ {
		cpy := make([]*Account, len(accounts))
		copy(cpy, accounts)
		shuffleWithinPriorityAndLastUsed(cpy, false)
		require.Equal(t, int64(1), cpy[0].ID)
		require.Equal(t, int64(2), cpy[1].ID)
		require.Equal(t, int64(3), cpy[2].ID)
	}
}

// ============ sameLastUsedAt 测试 ============

func TestSameLastUsedAt(t *testing.T) {
	now := time.Now()
	sameSecond := time.Unix(now.Unix(), 0)
	sameSecondDiffNano := time.Unix(now.Unix(), 999_999_999)
	differentSecond := now.Add(1 * time.Second)

	t.Run("both nil", func(t *testing.T) {
		require.True(t, sameLastUsedAt(nil, nil))
	})

	t.Run("one nil one not", func(t *testing.T) {
		require.False(t, sameLastUsedAt(nil, &now))
		require.False(t, sameLastUsedAt(&now, nil))
	})

	t.Run("same second different nanoseconds", func(t *testing.T) {
		require.True(t, sameLastUsedAt(&sameSecond, &sameSecondDiffNano))
	})

	t.Run("different seconds", func(t *testing.T) {
		require.False(t, sameLastUsedAt(&now, &differentSecond))
	})

	t.Run("exact same time", func(t *testing.T) {
		require.True(t, sameLastUsedAt(&now, &now))
	})
}

// ============ sameAccountWithLoadGroup 测试 ============

func TestSameAccountWithLoadGroup(t *testing.T) {
	now := time.Now()
	sameSecond := time.Unix(now.Unix(), 0)

	t.Run("same group", func(t *testing.T) {
		a := accountWithLoad{account: &Account{Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 10}}
		b := accountWithLoad{account: &Account{Priority: 1, LastUsedAt: &sameSecond}, loadInfo: &AccountLoadInfo{LoadRate: 10}}
		require.True(t, sameAccountWithLoadGroup(a, b))
	})

	t.Run("different priority", func(t *testing.T) {
		a := accountWithLoad{account: &Account{Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 10}}
		b := accountWithLoad{account: &Account{Priority: 2, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 10}}
		require.False(t, sameAccountWithLoadGroup(a, b))
	})

	t.Run("different load rate", func(t *testing.T) {
		a := accountWithLoad{account: &Account{Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 10}}
		b := accountWithLoad{account: &Account{Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 20}}
		require.False(t, sameAccountWithLoadGroup(a, b))
	})

	t.Run("different last used at", func(t *testing.T) {
		later := now.Add(1 * time.Second)
		a := accountWithLoad{account: &Account{Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 10}}
		b := accountWithLoad{account: &Account{Priority: 1, LastUsedAt: &later}, loadInfo: &AccountLoadInfo{LoadRate: 10}}
		require.False(t, sameAccountWithLoadGroup(a, b))
	})

	t.Run("both nil LastUsedAt", func(t *testing.T) {
		a := accountWithLoad{account: &Account{Priority: 1, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{LoadRate: 0}}
		b := accountWithLoad{account: &Account{Priority: 1, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{LoadRate: 0}}
		require.True(t, sameAccountWithLoadGroup(a, b))
	})
}

// ============ sameAccountGroup 测试 ============

func TestSameAccountGroup(t *testing.T) {
	now := time.Now()

	t.Run("same group", func(t *testing.T) {
		a := &Account{Priority: 1, LastUsedAt: nil}
		b := &Account{Priority: 1, LastUsedAt: nil}
		require.True(t, sameAccountGroup(a, b))
	})

	t.Run("different priority", func(t *testing.T) {
		a := &Account{Priority: 1, LastUsedAt: nil}
		b := &Account{Priority: 2, LastUsedAt: nil}
		require.False(t, sameAccountGroup(a, b))
	})

	t.Run("different LastUsedAt", func(t *testing.T) {
		later := now.Add(1 * time.Second)
		a := &Account{Priority: 1, LastUsedAt: &now}
		b := &Account{Priority: 1, LastUsedAt: &later}
		require.False(t, sameAccountGroup(a, b))
	})
}

// ============ sortAccountsByPriorityAndLastUsed 集成随机化测试 ============

func TestSortAccountsByPriorityAndLastUsed_WithShuffle(t *testing.T) {
	t.Run("same priority and nil LastUsedAt are shuffled", func(t *testing.T) {
		accounts := []*Account{
			{ID: 1, Priority: 1, LastUsedAt: nil},
			{ID: 2, Priority: 1, LastUsedAt: nil},
			{ID: 3, Priority: 1, LastUsedAt: nil},
		}

		seen := map[int64]bool{}
		for i := 0; i < 100; i++ {
			cpy := make([]*Account, len(accounts))
			copy(cpy, accounts)
			sortAccountsByPriorityAndLastUsed(cpy, false)
			seen[cpy[0].ID] = true
		}
		require.GreaterOrEqual(t, len(seen), 2, "identical sort keys should produce different orderings after shuffle")
	})

	t.Run("different priorities still sorted correctly", func(t *testing.T) {
		now := time.Now()
		accounts := []*Account{
			{ID: 3, Priority: 3, LastUsedAt: &now},
			{ID: 1, Priority: 1, LastUsedAt: &now},
			{ID: 2, Priority: 2, LastUsedAt: &now},
		}

		sortAccountsByPriorityAndLastUsed(accounts, false)
		require.Equal(t, int64(1), accounts[0].ID)
		require.Equal(t, int64(2), accounts[1].ID)
		require.Equal(t, int64(3), accounts[2].ID)
	})
}
