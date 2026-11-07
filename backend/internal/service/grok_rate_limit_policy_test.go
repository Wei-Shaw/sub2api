//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type grokManualRecoveryAccountRepo struct {
	grokQuotaAccountRepo
	setSchedulableCalls int
	lastSchedulableID   int64
	lastSchedulable     bool
}

func (r *grokManualRecoveryAccountRepo) SetSchedulable(_ context.Context, id int64, schedulable bool) error {
	r.setSchedulableCalls++
	r.lastSchedulableID = id
	r.lastSchedulable = schedulable
	return nil
}

func grokRateLimitTestConfig(mutate func(*config.GatewayGrokRateLimitConfig)) *config.Config {
	cfg := &config.Config{}
	cfg.Gateway.Grok.RateLimit = config.GatewayGrokRateLimitConfig{
		FallbackCooldownSeconds:    120,
		RepeatCooldownSeconds:      600,
		SustainedCooldownSeconds:   1800,
		MaxAdaptiveCooldownSeconds: 3600,
		BackoffQuietPeriodSeconds:  3600,
	}
	if mutate != nil {
		mutate(&cfg.Gateway.Grok.RateLimit)
	}
	return cfg
}

func grokRepeated429Account(id int64, accountType string, previousCooldown time.Duration, now time.Time) *Account {
	previousReset := now.Add(-time.Second)
	previousLimited := previousReset.Add(-previousCooldown)
	return &Account{
		ID:               id,
		Platform:         PlatformGrok,
		Type:             accountType,
		RateLimitedAt:    &previousLimited,
		RateLimitResetAt: &previousReset,
	}
}

func TestGrokRateLimitPolicyFromConfigKeepsBuiltInDefaults(t *testing.T) {
	require.Equal(t, defaultGrokRateLimitPolicy(), grokRateLimitPolicyFromConfig(nil))
	// A zero-value config (tests, configs predating gateway.grok.rate_limit)
	// must not produce zero-length cooldowns or disable escalation.
	require.Equal(t, defaultGrokRateLimitPolicy(), grokRateLimitPolicyFromConfig(&config.Config{}))
	require.Equal(t, defaultGrokRateLimitPolicy(), grokRateLimitPolicyFromConfig(grokRateLimitTestConfig(nil)))
}

func TestGrokRateLimitPolicyFromConfigMapsOverrides(t *testing.T) {
	policy := grokRateLimitPolicyFromConfig(grokRateLimitTestConfig(func(rl *config.GatewayGrokRateLimitConfig) {
		rl.FallbackCooldownSeconds = 30
		rl.DisableAdaptiveBackoff = true
		rl.RepeatCooldownSeconds = 60
		rl.SustainedCooldownSeconds = 90
		rl.MaxAdaptiveCooldownSeconds = 120
		rl.BackoffQuietPeriodSeconds = 300
		rl.MinCooldownSeconds = 86400
		rl.ManualRecovery = true
	}))

	require.Equal(t, grokRateLimitPolicy{
		fallbackCooldown:    30 * time.Second,
		adaptiveBackoff:     false,
		repeatCooldown:      time.Minute,
		sustainedCooldown:   90 * time.Second,
		maxAdaptiveCooldown: 2 * time.Minute,
		backoffQuietPeriod:  5 * time.Minute,
		minCooldown:         24 * time.Hour,
		manualRecovery:      true,
	}, policy)
}

func TestGrokRateLimitResetAtForAccountDisabledBackoffKeepsFallback(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	policy := grokRateLimitPolicyFromConfig(grokRateLimitTestConfig(func(rl *config.GatewayGrokRateLimitConfig) {
		rl.DisableAdaptiveBackoff = true
	}))
	snapshot := &xai.QuotaSnapshot{StatusCode: http.StatusTooManyRequests, UpdatedAt: now.Format(time.RFC3339)}

	// Even right after a sustained cooldown, a bare 429 only costs the fallback.
	for _, previous := range []time.Duration{2 * time.Minute, grokRateLimitRepeatCooldown, grokRateLimitSustainedCooldown, time.Hour} {
		account := grokRepeated429Account(640, AccountTypeOAuth, previous, now)
		resetAt, limited := grokRateLimitResetAtForAccount(account, snapshot, now, policy)
		require.True(t, limited)
		require.WithinDuration(t, now.Add(grokRateLimitFallbackCooldown), resetAt, time.Second, "previous cooldown %s", previous)
	}
}

func TestGrokRateLimitResetAtForAccountUsesConfiguredLadder(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	policy := grokRateLimitPolicyFromConfig(grokRateLimitTestConfig(func(rl *config.GatewayGrokRateLimitConfig) {
		rl.FallbackCooldownSeconds = 60
		rl.RepeatCooldownSeconds = 180
		rl.SustainedCooldownSeconds = 300
		rl.MaxAdaptiveCooldownSeconds = 420
		rl.BackoffQuietPeriodSeconds = 600
	}))
	snapshot := &xai.QuotaSnapshot{StatusCode: http.StatusTooManyRequests, UpdatedAt: now.Format(time.RFC3339)}

	tests := []struct {
		previous time.Duration
		want     time.Duration
	}{
		{previous: time.Minute, want: 3 * time.Minute},
		{previous: 3 * time.Minute, want: 5 * time.Minute},
		{previous: 5 * time.Minute, want: 7 * time.Minute},
		{previous: 7 * time.Minute, want: 7 * time.Minute},
	}
	for _, tt := range tests {
		account := grokRepeated429Account(641, AccountTypeOAuth, tt.previous, now)
		resetAt, limited := grokRateLimitResetAtForAccount(account, snapshot, now, policy)
		require.True(t, limited)
		require.WithinDuration(t, now.Add(tt.want), resetAt, time.Second, "previous cooldown %s", tt.previous)
	}

	// Outside the configured quiet period the ladder restarts at the fallback.
	quietReset := now.Add(-11 * time.Minute)
	quietLimited := quietReset.Add(-7 * time.Minute)
	account := &Account{ID: 642, Platform: PlatformGrok, Type: AccountTypeOAuth, RateLimitedAt: &quietLimited, RateLimitResetAt: &quietReset}
	resetAt, limited := grokRateLimitResetAtForAccount(account, snapshot, now, policy)
	require.True(t, limited)
	require.WithinDuration(t, now.Add(time.Minute), resetAt, time.Second)
}

func TestGrokRateLimitResetAtForAccountAppliesMinCooldownFloor(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	policy := grokRateLimitPolicyFromConfig(grokRateLimitTestConfig(func(rl *config.GatewayGrokRateLimitConfig) {
		rl.MinCooldownSeconds = 86400
	}))
	retryAfter := 60

	t.Run("short Retry-After is raised to the floor", func(t *testing.T) {
		snapshot := &xai.QuotaSnapshot{StatusCode: http.StatusTooManyRequests, RetryAfterSeconds: &retryAfter, UpdatedAt: now.Format(time.RFC3339)}
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
			account := &Account{ID: 643, Platform: PlatformGrok, Type: accountType}
			resetAt, limited := grokRateLimitResetAtForAccount(account, snapshot, now, policy)
			require.True(t, limited)
			require.WithinDuration(t, now.Add(24*time.Hour), resetAt, time.Second, accountType)
		}
	})

	t.Run("longer upstream window reset wins", func(t *testing.T) {
		windowReset := now.Add(48 * time.Hour)
		snapshot := &xai.QuotaSnapshot{
			StatusCode: http.StatusTooManyRequests,
			UpdatedAt:  now.Format(time.RFC3339),
			Requests:   &xai.QuotaWindow{Remaining: grokInt64PtrForTest(0), ResetUnix: grokInt64PtrForTest(windowReset.Unix())},
		}
		account := &Account{ID: 644, Platform: PlatformGrok, Type: AccountTypeOAuth}
		resetAt, limited := grokRateLimitResetAtForAccount(account, snapshot, now, policy)
		require.True(t, limited)
		require.WithinDuration(t, windowReset, resetAt, time.Second)
	})

	t.Run("exhausted window on a successful response is not a 429", func(t *testing.T) {
		windowReset := now.Add(10 * time.Minute)
		snapshot := &xai.QuotaSnapshot{
			StatusCode: http.StatusOK,
			UpdatedAt:  now.Format(time.RFC3339),
			Requests:   &xai.QuotaWindow{Remaining: grokInt64PtrForTest(0), ResetUnix: grokInt64PtrForTest(windowReset.Unix())},
		}
		account := &Account{ID: 645, Platform: PlatformGrok, Type: AccountTypeOAuth}
		resetAt, limited := grokRateLimitResetAtForAccount(account, snapshot, now, policy)
		require.True(t, limited)
		require.WithinDuration(t, windowReset, resetAt, time.Second)
	})
}

func TestHandleGrokAccountUpstreamError429ManualRecoveryPausesScheduling(t *testing.T) {
	account := &Account{ID: 646, Platform: PlatformGrok, Type: AccountTypeOAuth, Schedulable: true}
	repo := &grokManualRecoveryAccountRepo{}
	svc := &OpenAIGatewayService{
		accountRepo: repo,
		cfg: grokRateLimitTestConfig(func(rl *config.GatewayGrokRateLimitConfig) {
			rl.ManualRecovery = true
		}),
	}
	before := time.Now()

	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusTooManyRequests, nil, nil)

	// The cooldown is still installed so re-enabling early honors the reset.
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.WithinDuration(t, before.Add(grokRateLimitFallbackCooldown), repo.lastRateLimitResetAt, time.Second)
	require.Equal(t, 1, repo.setSchedulableCalls)
	require.Equal(t, int64(646), repo.lastSchedulableID)
	require.False(t, repo.lastSchedulable)
	require.False(t, account.Schedulable)

	// Already paused accounts are not written again.
	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusTooManyRequests, nil, nil)
	require.Equal(t, 1, repo.setSchedulableCalls)
}

func TestHandleGrokAccountUpstreamError429ManualRecoverySkipsNonAccountLimits(t *testing.T) {
	manualCfg := grokRateLimitTestConfig(func(rl *config.GatewayGrokRateLimitConfig) {
		rl.ManualRecovery = true
	})
	tests := []struct {
		name    string
		account *Account
		cfg     *config.Config
		status  int
		body    []byte
	}{
		{
			name:    "disabled by default",
			account: &Account{ID: 647, Platform: PlatformGrok, Type: AccountTypeOAuth, Schedulable: true},
			cfg:     grokRateLimitTestConfig(nil),
			status:  http.StatusTooManyRequests,
		},
		{
			name:    "model capacity 429 is model pressure",
			account: &Account{ID: 648, Platform: PlatformGrok, Type: AccountTypeOAuth, Schedulable: true},
			cfg:     manualCfg,
			status:  http.StatusTooManyRequests,
			body:    []byte(`{"error":{"message":"The model is currently at capacity due to high demand"}}`),
		},
		{
			name: "pool mode leaves account health upstream",
			account: &Account{
				ID: 649, Platform: PlatformGrok, Type: AccountTypeAPIKey, Schedulable: true,
				Credentials: map[string]any{"pool_mode": true},
			},
			cfg:    manualCfg,
			status: http.StatusTooManyRequests,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &grokManualRecoveryAccountRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo, cfg: tt.cfg}

			svc.handleGrokAccountUpstreamError(context.Background(), tt.account, tt.status, nil, tt.body)

			require.Zero(t, repo.setSchedulableCalls)
			require.True(t, tt.account.Schedulable)
		})
	}
}
