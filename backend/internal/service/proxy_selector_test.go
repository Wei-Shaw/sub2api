//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mkSelectableProxy(id int64, status string, expiresInDays *int, now time.Time) Proxy {
	p := Proxy{
		ID:     id,
		Name:   "p",
		Status: status,
	}
	if expiresInDays != nil {
		t := now.AddDate(0, 0, *expiresInDays)
		p.ExpiresAt = &t
	}
	return p
}

func TestSelectProxyFromGroup_EmptyAndUnhealthy(t *testing.T) {
	t.Parallel()
	now := time.Now()

	t.Run("empty candidates", func(t *testing.T) {
		t.Parallel()
		p, ok := SelectProxyFromGroup(nil, ProxyGroupStrategyRoundRobin, now, 0)
		require.False(t, ok)
		require.Nil(t, p)
	})

	t.Run("all expired", func(t *testing.T) {
		t.Parallel()
		cands := []Proxy{
			mkSelectableProxy(1, StatusActive, di(-1), now),
			mkSelectableProxy(2, StatusActive, di(-2), now),
		}
		p, ok := SelectProxyFromGroup(cands, ProxyGroupStrategyRoundRobin, now, 0)
		require.False(t, ok)
		require.Nil(t, p)
	})

	t.Run("all inactive", func(t *testing.T) {
		t.Parallel()
		cands := []Proxy{
			mkSelectableProxy(1, "inactive", di(30), now),
			mkSelectableProxy(2, StatusExpired, nil, now),
		}
		p, ok := SelectProxyFromGroup(cands, ProxyGroupStrategyRandom, now, 1)
		require.False(t, ok)
		require.Nil(t, p)
	})
}

func TestSelectProxyFromGroup_SingleHealthy(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cands := []Proxy{
		mkSelectableProxy(1, "inactive", di(30), now),
		mkSelectableProxy(2, StatusActive, di(30), now),
		mkSelectableProxy(3, StatusActive, di(-1), now),
	}
	p, ok := SelectProxyFromGroup(cands, ProxyGroupStrategyRoundRobin, now, 99)
	require.True(t, ok)
	require.NotNil(t, p)
	require.Equal(t, int64(2), p.ID)
}

func TestSelectProxyFromGroup_RoundRobin(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cands := []Proxy{
		mkSelectableProxy(10, StatusActive, nil, now),
		mkSelectableProxy(20, StatusActive, nil, now),
		mkSelectableProxy(30, StatusActive, nil, now),
	}

	seen := map[int64]int{}
	for seed := uint64(0); seed < 9; seed++ {
		p, ok := SelectProxyFromGroup(cands, ProxyGroupStrategyRoundRobin, now, seed)
		require.True(t, ok)
		seen[p.ID]++
	}
	require.Equal(t, 3, seen[10])
	require.Equal(t, 3, seen[20])
	require.Equal(t, 3, seen[30])
}

func TestSelectProxyFromGroup_StickyStable(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cands := []Proxy{
		mkSelectableProxy(1, StatusActive, nil, now),
		mkSelectableProxy(2, StatusActive, nil, now),
		mkSelectableProxy(3, StatusActive, nil, now),
		mkSelectableProxy(4, StatusActive, nil, now),
	}

	accountID := uint64(42)
	first, ok := SelectProxyFromGroup(cands, ProxyGroupStrategySticky, now, accountID)
	require.True(t, ok)
	for i := 0; i < 20; i++ {
		p, ok := SelectProxyFromGroup(cands, ProxyGroupStrategySticky, now, accountID)
		require.True(t, ok)
		require.Equal(t, first.ID, p.ID, "sticky must be stable for same account+candidates")
	}
}

func TestSelectProxyFromGroup_StickySpreadsAccounts(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cands := []Proxy{
		mkSelectableProxy(1, StatusActive, nil, now),
		mkSelectableProxy(2, StatusActive, nil, now),
		mkSelectableProxy(3, StatusActive, nil, now),
	}
	seen := map[int64]struct{}{}
	for accountID := uint64(1); accountID <= 30; accountID++ {
		p, ok := SelectProxyFromGroup(cands, ProxyGroupStrategySticky, now, accountID)
		require.True(t, ok)
		seen[p.ID] = struct{}{}
	}
	require.GreaterOrEqual(t, len(seen), 2, "different accounts should spread across proxies")
}

func TestSelectProxyFromGroup_StickyRemapsOnCandidateChange(t *testing.T) {
	t.Parallel()
	now := time.Now()
	full := []Proxy{
		mkSelectableProxy(1, StatusActive, nil, now),
		mkSelectableProxy(2, StatusActive, nil, now),
		mkSelectableProxy(3, StatusActive, nil, now),
	}
	accountID := uint64(7)
	before, ok := SelectProxyFromGroup(full, ProxyGroupStrategySticky, now, accountID)
	require.True(t, ok)

	// 去掉原先命中的成员，允许重新映射。
	reduced := make([]Proxy, 0, 2)
	for _, p := range full {
		if p.ID != before.ID {
			reduced = append(reduced, p)
		}
	}
	after, ok := SelectProxyFromGroup(reduced, ProxyGroupStrategySticky, now, accountID)
	require.True(t, ok)
	require.NotEqual(t, before.ID, after.ID)
}

func TestSelectProxyFromGroup_UnknownStrategyFallsBack(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cands := []Proxy{
		mkSelectableProxy(1, StatusActive, nil, now),
		mkSelectableProxy(2, StatusActive, nil, now),
	}
	p, ok := SelectProxyFromGroup(cands, "not-a-real-strategy", now, 1)
	require.True(t, ok)
	require.Equal(t, int64(2), p.ID) // seed=1 → index 1 under round_robin fallback
}

func TestProxyGroupEffectiveStrategy(t *testing.T) {
	t.Parallel()
	require.Equal(t, ProxyGroupStrategyRoundRobin, (*ProxyGroup)(nil).EffectiveStrategy())
	require.Equal(t, ProxyGroupStrategySticky, (&ProxyGroup{StickyByAccount: true, Strategy: ProxyGroupStrategyRandom}).EffectiveStrategy())
	require.Equal(t, ProxyGroupStrategyRandom, (&ProxyGroup{Strategy: ProxyGroupStrategyRandom}).EffectiveStrategy())
	require.Equal(t, ProxyGroupStrategyRoundRobin, (&ProxyGroup{Strategy: "weird"}).EffectiveStrategy())
}
