//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newSchedulingThresholdFilterFixture(n int) (*GatewayService, []Account) {
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})

	settingsRepo := newMockSettingRepo()
	settingsRepo.data[SettingKeyAccountSchedulingThresholds] = `{"anthropic":90,"openai":90}`
	rl := NewRateLimitService(&rateLimitAccountRepoStub{}, nil, &config.Config{}, nil, nil)
	rl.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))

	accounts := make([]Account, n)
	for i := range accounts {
		accounts[i] = Account{ID: int64(i + 1), Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	}
	return &GatewayService{rateLimitService: rl}, accounts
}

func TestFilterAccountsBySchedulingThreshold_ReadsThresholdsOncePerPass(t *testing.T) {
	svc, accounts := newSchedulingThresholdFilterFixture(200)
	windowEnd := time.Now().Add(2 * time.Hour)
	accounts[0].SessionWindowEnd = &windowEnd
	accounts[0].Extra = map[string]any{"session_window_utilization": 0.95}
	ctx := context.Background()

	filtered := svc.filterAccountsBySchedulingThreshold(ctx, accounts)
	require.Len(t, filtered, len(accounts)-1, "the account above the 90% threshold is filtered")
	require.NotEqual(t, accounts[0].ID, filtered[0].ID)

	// Result slice plus one thresholds clone; cloning per account costs 400+.
	allocs := testing.AllocsPerRun(20, func() {
		svc.filterAccountsBySchedulingThreshold(ctx, accounts[1:])
	})
	require.LessOrEqual(t, allocs, float64(5))
}

func BenchmarkSchedulingThresholdFilter(b *testing.B) {
	svc, accounts := newSchedulingThresholdFilterFixture(200)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := svc.filterAccountsBySchedulingThreshold(ctx, accounts); len(got) != len(accounts) {
			b.Fatalf("filtered %d of %d accounts", len(accounts)-len(got), len(accounts))
		}
	}
}
