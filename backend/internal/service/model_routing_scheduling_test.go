//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestModelRouteLimitsForAccount(t *testing.T) {
	rule := domain.ModelRoutingRule{
		MaxConcurrency: 10,
		Accounts: []domain.ModelRoutingAccountRef{
			{ID: 1, Priority: 10, Concurrency: 2},
			{ID: 2, Priority: 20, Concurrency: 0},
		},
	}
	p, c := modelRouteLimitsForAccount(rule, 1)
	require.Equal(t, 10, p)
	require.Equal(t, 2, c)
	p, c = modelRouteLimitsForAccount(rule, 99)
	require.Equal(t, domain.DefaultModelRoutingAccountPriority, p)
	require.Equal(t, 0, c)
}

func TestGetMatchingRoutingRuleUsesStructuredPriorityOrder(t *testing.T) {
	group := &Group{
		ModelRoutingEnabled: true,
		ModelRouting: domain.ModelRoutingMap{
			"claude-*": {
				MaxConcurrency: 5,
				Accounts: []domain.ModelRoutingAccountRef{
					{ID: 2, Priority: 20, Concurrency: 1},
					{ID: 1, Priority: 5, Concurrency: 3},
				},
			},
		},
	}
	matched, ok := group.GetMatchingRoutingRule("claude-opus-4")
	require.True(t, ok)
	require.Equal(t, "claude-*", matched.Pattern)
	require.Equal(t, []int64{2, 1}, matched.Rule.AccountIDs())
	require.Equal(t, 5, matched.Rule.MaxConcurrency)
}

type stubModelRouteConcurrencyCache struct {
	stubConcurrencyCacheForTest
	total   int
	account map[int64]int
}

func (c *stubModelRouteConcurrencyCache) AcquireModelRouteSlot(
	_ context.Context, _ int64, _ string, accountID int64, accountMax, totalMax int, _ string,
) (bool, error) {
	if accountMax > 0 && c.account[accountID] >= accountMax {
		return false, nil
	}
	if totalMax > 0 && c.total >= totalMax {
		return false, nil
	}
	if c.account == nil {
		c.account = map[int64]int{}
	}
	c.account[accountID]++
	c.total++
	return true, nil
}

func (c *stubModelRouteConcurrencyCache) ReleaseModelRouteSlot(
	_ context.Context, _ int64, _ string, accountID int64, _ string,
) error {
	if c.account != nil && c.account[accountID] > 0 {
		c.account[accountID]--
	}
	if c.total > 0 {
		c.total--
	}
	return nil
}

func (c *stubModelRouteConcurrencyCache) GetModelRouteAccountConcurrency(
	_ context.Context, _ int64, _ string, accountID int64,
) (int, error) {
	return c.account[accountID], nil
}

func (c *stubModelRouteConcurrencyCache) GetModelRouteTotalConcurrency(
	_ context.Context, _ int64, _ string,
) (int, error) {
	return c.total, nil
}

func TestConcurrencyService_ModelRouteSaturation(t *testing.T) {
	cache := &stubModelRouteConcurrencyCache{total: 2, account: map[int64]int{7: 1}}
	svc := NewConcurrencyService(cache)
	require.True(t, svc.ModelRouteRuleSaturated(context.Background(), 1, "claude-*", 2))
	require.False(t, svc.ModelRouteRuleSaturated(context.Background(), 1, "claude-*", 3))
	require.True(t, svc.ModelRouteAccountSaturated(context.Background(), 1, "claude-*", 7, 1))
	require.False(t, svc.ModelRouteAccountSaturated(context.Background(), 1, "claude-*", 7, 2))
}
