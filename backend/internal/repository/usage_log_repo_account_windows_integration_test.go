//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGetAccountStatsByWindowsIntegrationDifferingRangesAndBoundaries(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)
	user := mustCreateUser(t, client, &service.User{Email: "account-windows-boundaries@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-account-windows-boundaries"})
	first := mustCreateAccount(t, client, &service.Account{Name: "account-windows-first"})
	second := mustCreateAccount(t, client, &service.Account{Name: "account-windows-second"})
	unrelated := mustCreateAccount(t, client, &service.Account{Name: "account-windows-unrelated"})
	start := time.Date(2026, 9, 10, 9, 23, 41, 123456000, time.FixedZone("UTC+8", 8*60*60))
	end := start.Add(3 * time.Hour)
	zero, half, oneHalf, two := 0.0, 0.5, 1.5, 2.0

	for i, row := range []struct {
		accountID  int64
		at         time.Time
		tokens     [4]int
		standard   float64
		user       float64
		pricing    *float64
		multiplier *float64
	}{
		{first.ID, start.Add(-time.Microsecond), [4]int{900, 900, 900, 900}, 900, 900, nil, nil},
		{first.ID, start, [4]int{1, 2, 3, 4}, 2, 1, nil, nil},
		{first.ID, start.Add(time.Hour), [4]int{5, 6, 7, 8}, 3, 0.6, &oneHalf, &two},
		{first.ID, end.Add(-time.Microsecond), [4]int{9, 10, 11, 12}, 4, 3, &two, nil},
		{first.ID, end, [4]int{900, 900, 900, 900}, 900, 900, nil, nil},
		{second.ID, start.Add(30 * time.Minute), [4]int{900, 900, 900, 900}, 900, 900, nil, nil},
		{second.ID, start.Add(2 * time.Hour), [4]int{20, 30, 40, 50}, 5, 4, nil, &half},
		{second.ID, start.Add(4*time.Hour - time.Microsecond), [4]int{2, 4, 6, 8}, 7, 2, &zero, &two},
		{second.ID, start.Add(4 * time.Hour), [4]int{900, 900, 900, 900}, 900, 900, nil, nil},
		{unrelated.ID, start.Add(time.Hour), [4]int{900, 900, 900, 900}, 900, 900, nil, nil},
	} {
		inserted, err := repo.Create(ctx, &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: row.accountID,
			RequestID:   fmt.Sprintf("account-windows-boundary-%d", i),
			Model:       fmt.Sprintf("model-%d", i%2),
			InputTokens: row.tokens[0], OutputTokens: row.tokens[1],
			CacheCreationTokens: row.tokens[2], CacheReadTokens: row.tokens[3],
			TotalCost: row.standard, ActualCost: row.user,
			AccountStatsCost: row.pricing, AccountRateMultiplier: row.multiplier,
			CreatedAt: row.at,
		})
		require.NoError(t, err)
		require.True(t, inserted)
	}

	windows := []usagestats.AccountStatsWindow{
		{AccountID: second.ID, StartAt: start.Add(2 * time.Hour), EndAt: start.Add(4 * time.Hour)},
		{AccountID: first.ID, StartAt: start, EndAt: end},
	}
	result, err := repo.GetAccountStatsByWindows(ctx, windows)
	require.NoError(t, err)
	require.Len(t, result, 2)
	expected := map[int64]usagestats.AccountStats{
		first.ID:  {Requests: 3, Tokens: 78, Cost: 7, StandardCost: 9, UserCost: 4.6},
		second.ID: {Requests: 2, Tokens: 160, Cost: 2.5, StandardCost: 12, UserCost: 6},
	}
	for _, window := range windows {
		stats := result[window.AccountID]
		require.NotNil(t, stats)
		want := expected[window.AccountID]
		require.Equal(t, want.Requests, stats.Requests)
		require.Equal(t, want.Tokens, stats.Tokens)
		require.InDelta(t, want.Cost, stats.Cost, 1e-9)
		require.InDelta(t, want.StandardCost, stats.StandardCost, 1e-9)
		require.InDelta(t, want.UserCost, stats.UserCost, 1e-9)

		legacy, err := repo.GetModelStatsWithFilters(ctx, window.StartAt, window.EndAt, 0, 0, window.AccountID, 0, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, legacy, 2, "totals must combine different models")
		var requests, tokens int64
		var accountCost, standardCost float64
		for _, model := range legacy {
			requests += model.Requests
			tokens += model.TotalTokens
			// With only an account filter, legacy ActualCost is account cost.
			accountCost += model.ActualCost
			standardCost += model.Cost
		}
		require.Equal(t, requests, stats.Requests)
		require.Equal(t, tokens, stats.Tokens)
		require.InDelta(t, accountCost, stats.Cost, 1e-9)
		require.InDelta(t, standardCost, stats.StandardCost, 1e-9)

		var userCost float64
		require.NoError(t, scanSingleRow(ctx, tx, `
			SELECT COALESCE(SUM(actual_cost), 0)
			FROM usage_logs
			WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
		`, []any{window.AccountID, window.StartAt, window.EndAt}, &userCost))
		require.InDelta(t, userCost, stats.UserCost, 1e-9)
		require.NotEqual(t, accountCost, stats.UserCost, "user and account billing must remain distinct")
	}
}

func TestGetAccountStatsByWindowsIntegrationPricingFallbacks(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)
	user := mustCreateUser(t, client, &service.User{Email: "account-windows-pricing@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-account-windows-pricing"})
	start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	zero, half, three := 0.0, 0.5, 3.0
	cases := []struct {
		name       string
		pricing    *float64
		multiplier *float64
		cost       float64
	}{
		{name: "both_null", cost: 10},
		{name: "pricing_null", multiplier: &half, cost: 5},
		{name: "multiplier_null", pricing: &three, cost: 3},
		{name: "both_present", pricing: &three, multiplier: &half, cost: 1.5},
		{name: "zero_pricing", pricing: &zero, multiplier: &half, cost: 0},
		{name: "zero_multiplier", pricing: &three, multiplier: &zero, cost: 0},
	}
	windows := make([]usagestats.AccountStatsWindow, 0, len(cases))
	for _, test := range cases {
		account := mustCreateAccount(t, client, &service.Account{Name: "account-windows-" + test.name})
		inserted, err := repo.Create(ctx, &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
			RequestID: "account-windows-" + test.name, Model: "pricing-model",
			InputTokens: 1, OutputTokens: 2, CacheCreationTokens: 4, CacheReadTokens: 8,
			TotalCost: 10, ActualCost: 7,
			AccountStatsCost: test.pricing, AccountRateMultiplier: test.multiplier,
			CreatedAt: start,
		})
		require.NoError(t, err)
		require.True(t, inserted)
		windows = append(windows, usagestats.AccountStatsWindow{
			AccountID: account.ID, StartAt: start, EndAt: start.Add(time.Hour),
		})
	}

	result, err := repo.GetAccountStatsByWindows(ctx, windows)
	require.NoError(t, err)
	require.Len(t, result, len(cases))
	for i, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, &usagestats.AccountStats{
				Requests: 1, Tokens: 15, Cost: test.cost, StandardCost: 10, UserCost: 7,
			}, result[windows[i].AccountID])
		})
	}
}

func TestGetAccountStatsByWindowsIntegrationZeroRows(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)
	account := mustCreateAccount(t, client, &service.Account{Name: "account-windows-empty"})
	start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	const missingAccountID int64 = 9223372036854775807
	windows := []usagestats.AccountStatsWindow{
		{AccountID: account.ID, StartAt: start, EndAt: start.Add(31 * 24 * time.Hour)},
		{AccountID: missingAccountID, StartAt: start.Add(time.Hour), EndAt: start.Add(2 * time.Hour)},
	}
	result, err := repo.GetAccountStatsByWindows(ctx, windows)
	require.NoError(t, err)
	require.Equal(t, map[int64]*usagestats.AccountStats{
		account.ID:       {},
		missingAccountID: {},
	}, result)

	// PostgreSQL must infer parameter types correctly for a single VALUES tuple.
	result, err = repo.GetAccountStatsByWindows(ctx, windows[:1])
	require.NoError(t, err)
	require.Equal(t, map[int64]*usagestats.AccountStats{account.ID: {}}, result)
}
