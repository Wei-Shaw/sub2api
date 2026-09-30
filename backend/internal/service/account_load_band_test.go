package service

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func loadBandTestAccount(id int64, weight, current, waiting int) accountWithLoad {
	return accountWithLoad{
		account: &Account{
			ID: id, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true,
			Concurrency: 100, LoadFactor: &weight,
		},
		loadInfo: &AccountLoadInfo{
			AccountID: id, CurrentConcurrency: current, WaitingCount: waiting,
			LoadRate: (current + waiting) * 100 / weight,
		},
	}
}

func TestAccountLoadBandWeightedDistribution(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b int
	}{
		{"idle", 0, 0},
		{"within first band", 4, 0},
		{"within second band", 9, 1},
		{"within third band", 14, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(42))
			const samples = 12000
			selectedA := 0
			lastUsed := time.Now()
			for range samples {
				accounts := []accountWithLoad{
					loadBandTestAccount(1, 5, tc.a, 0),
					loadBandTestAccount(2, 1, tc.b, 0),
				}
				// A recently used account still receives its share even if B
				// has never been used. LRU must not override weighted sampling.
				accounts[0].account.LastUsedAt = &lastUsed
				orderAccountsByLoadBand(accounts, false, rng.Float64)
				if accounts[0].account.ID == 1 {
					selectedA++
				}
			}
			require.InDelta(t, 5.0/6, float64(selectedA)/samples, 0.02)
		})
	}
}

func TestAccountLoadBandPriorityAndMultipleAccounts(t *testing.T) {
	accounts := []accountWithLoad{
		loadBandTestAccount(1, 5, 0, 0),
		loadBandTestAccount(2, 1, 1, 0),
		loadBandTestAccount(3, 2, 0, 0),
		loadBandTestAccount(4, 1, 3, 0),
	}
	accounts[3].account.Priority = -1
	rng := rand.New(rand.NewSource(42))
	counts := map[int64]int{}
	for range 12000 {
		order := append([]accountWithLoad(nil), accounts...)
		orderAccountsByLoadBand(order, false, rng.Float64)
		require.Equal(t, int64(4), order[0].account.ID, "priority precedes load bands")
		require.Equal(t, int64(2), order[3].account.ID, "higher band follows both idle accounts")
		counts[order[1].account.ID]++
	}
	require.InDelta(t, 5.0/7, float64(counts[1])/12000, 0.02)
	require.InDelta(t, 2.0/7, float64(counts[3])/12000, 0.02)
}

func TestAccountLoadBandEffectiveFactorAndWidth(t *testing.T) {
	a := loadBandTestAccount(1, 100, 5, 0)
	b := loadBandTestAccount(2, 20, 0, 0)
	// 100:20 has the same weights as 5:1 but wider bands. Both remain
	// in band zero at 5:0; a draw may still select A.
	order := []accountWithLoad{a, b}
	orderAccountsByLoadBand(order, false, func() float64 { return 0.5 })
	require.Equal(t, int64(1), order[0].account.ID)
	// An unset factor uses Concurrency as both band width and weight.
	a.account.LoadFactor = nil
	a.account.Concurrency = 5
	b.account.LoadFactor = nil
	b.account.Concurrency = 1
	order = []accountWithLoad{a, b}
	orderAccountsByLoadBand(order, false, func() float64 { return 0.5 })
	require.Equal(t, int64(2), order[0].account.ID)
}

func TestOpenAILoadBandSelectionIntegration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		a, b     int
		limitA   int
		denyA    bool
		sticky   int64
		waitingA int
		want     int64
	}{
		{"force B at first boundary", 5, 0, 100, false, 0, 0, 2},
		{"continue beyond 100 percent", 9, 2, 100, false, 0, 0, 1},
		{"force B at second boundary", 10, 1, 100, false, 0, 0, 2},
		{"actual concurrency limit", 4, 1, 4, false, 0, 0, 2},
		{"unlimited concurrency", 9, 2, 0, false, 0, 0, 1},
		{"slot race falls through", 4, 1, 100, true, 0, 0, 2},
		{"sticky precedes load bands", 5, 0, 100, false, 1, 0, 1},
		{"waiting contributes to band", 4, 0, 100, false, 0, 1, 2},
		{"released slot lowers band", 4, 1, 100, false, 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := loadBandTestAccount(1, 5, tc.a, tc.waitingA)
			b := loadBandTestAccount(2, 1, tc.b, 0)
			a.account.Concurrency = tc.limitA
			cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
			if tc.sticky != 0 {
				cache.sessionBindings["openai:test-band-session"] = tc.sticky
			}
			concurrencyCache := stubConcurrencyCache{
				loadMap:        map[int64]*AccountLoadInfo{1: a.loadInfo, 2: b.loadInfo},
				acquireResults: map[int64]bool{1: !tc.denyA, 2: true},
			}
			svc := &OpenAIGatewayService{
				accountRepo:        stubOpenAIAccountRepo{accounts: []Account{*a.account, *b.account}},
				cache:              cache,
				concurrencyService: NewConcurrencyService(concurrencyCache),
			}
			selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "test-band-session", "gpt-4", nil)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.True(t, selection.Acquired)
			t.Cleanup(selection.ReleaseFunc)
			require.Equal(t, tc.want, selection.Account.ID)
			require.Equal(t, tc.want, cache.sessionBindings["openai:test-band-session"])
		})
	}
}

func TestAccountLoadBandOAuthPreference(t *testing.T) {
	a := loadBandTestAccount(1, 5, 0, 0)
	b := loadBandTestAccount(2, 1, 0, 0)
	b.account.Type = AccountTypeOAuth
	order := []accountWithLoad{a, b}
	orderAccountsByLoadBand(order, true, func() float64 { return 0.5 })
	require.Equal(t, int64(2), order[0].account.ID)
	// OAuth preference does not override a lower load band.
	b.loadInfo.CurrentConcurrency = 1
	orderAccountsByLoadBand(order, true, func() float64 { return 0.5 })
	require.Equal(t, int64(1), order[0].account.ID)
}
