package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWindowActivationSchedule(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{
		openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": "05:00", "end": "00:00"},
	}}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	for _, tc := range []struct {
		hour   int
		active bool
	}{{4, false}, {5, true}, {23, true}, {0, false}} {
		require.Equal(t, tc.active, shouldActivateOpenAIWindow(account, time.Date(2026, 9, 28, tc.hour, 0, 0, 0, loc), loc), "hour %d", tc.hour)
	}
	account.Extra[openAIWindowActivationExtraKey] = map[string]any{"enabled": true, "start": "22:00", "end": "02:00"}
	require.True(t, shouldActivateOpenAIWindow(account, time.Date(2026, 9, 29, 1, 0, 0, 0, loc), loc))
	require.False(t, shouldActivateOpenAIWindow(account, time.Date(2026, 9, 29, 2, 0, 0, 0, loc), loc))
}

func TestOpenAIWindowActivationValidatesConfig(t *testing.T) {
	for _, value := range []map[string]any{
		{"enabled": true, "start": "05:00", "end": "05:00"},
		{"enabled": true, "start": "25:00", "end": "00:00"},
		{"enabled": true, "start": "05:00", "end": "00:60"},
		{"enabled": "true", "start": "05:00", "end": "00:00"},
		{"enabled": true, "start": "05:00", "end": "00:00", "unexpected": 1},
	} {
		_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{openAIWindowActivationExtraKey: value})
		require.Error(t, err)
	}
	_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, true, map[string]any{openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": "05:00", "end": "00:00"}})
	require.Error(t, err)
}

func TestOpenAIWindowActivationNeedsFreshCountdown(t *testing.T) {
	now := time.Date(2026, 9, 28, 5, 0, 0, 0, time.UTC)
	require.False(t, openAI5hCountdownMissing(&OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
		PrimaryWindow:   &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAt: now.Add(-time.Minute).Unix()},
		SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAt: now.Add(time.Hour).Unix()},
	}}, now))
	require.True(t, openAI5hCountdownMissing(nil, now))
	require.True(t, openAI5hCountdownMissing(&OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 7 * 24 * 3600, ResetAt: now.Add(time.Hour).Unix()}}}, now))
	require.False(t, openAI5hCountdownMissing(&OpenAIQuotaUsage{FetchedAt: now.Unix(), RateLimit: &OpenAIRateLimit{
		SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAfterSeconds: 18000},
	}}, now))
	for _, reset := range []time.Time{now.Add(-time.Second), now.Add(time.Hour)} {
		usage := &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAt: reset.Unix()}}}
		require.Equal(t, !reset.After(now), openAI5hCountdownMissing(usage, now))
	}
}

type windowActivationTestRequester struct {
	calls int
	onRun func()
}

func (r *windowActivationTestRequester) RunTestBackground(_ context.Context, _ int64, model string) (*ScheduledTestResult, error) {
	r.calls++
	if r.onRun != nil {
		r.onRun()
	}
	return &ScheduledTestResult{Status: "success"}, nil
}

func TestOpenAIWindowActivationRequestsUntilCountdown(t *testing.T) {
	local := time.Now().In(time.FixedZone("CST", 8*3600))
	start := local.Add(-time.Minute).Format("15:04")
	end := local.Add(2 * time.Minute).Format("15:04")
	account := &Account{ID: 58, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{
		openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": start, "end": end},
	}}
	quota := &autoResetTestQuota{usage: &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{PrimaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 7 * 24 * 3600, ResetAt: time.Now().Add(24 * time.Hour).Unix()}}}}
	tester := &windowActivationTestRequester{}
	tester.onRun = func() {
		quota.usage = &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
			SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAt: time.Now().Add(5 * time.Hour).Unix()},
		}}
	}
	locks := &fakeLeaderLockCache{}
	svc := NewOpenAIQuotaAutoResetService(nil, quota, nil, nil, nil, nil, locks)
	svc.accountTest = tester
	svc.activationLocation = time.FixedZone("CST", 8*3600)
	svc.activateWindow(context.Background(), account)
	require.Equal(t, 1, tester.calls)
	require.Equal(t, int32(2), quota.queryCalls.Load())
	svc.activateWindow(context.Background(), account)
	require.Equal(t, 1, tester.calls, "same account is throttled across workers")
}

func TestOpenAIWindowActivationRetriesWhenNoCountdownAppears(t *testing.T) {
	local := time.Now().In(time.FixedZone("CST", 8*3600))
	account := &Account{ID: 59, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{
		openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": local.Add(-time.Minute).Format("15:04"), "end": local.Add(2 * time.Minute).Format("15:04")},
	}}
	quota := &autoResetTestQuota{usage: &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{}}}
	tester := &windowActivationTestRequester{}
	locks := &fakeLeaderLockCache{}
	svc := NewOpenAIQuotaAutoResetService(nil, quota, nil, nil, nil, nil, locks)
	svc.accountTest = tester
	svc.activationLocation = time.FixedZone("CST", 8*3600)
	svc.activateWindow(context.Background(), account)
	require.Equal(t, 1, tester.calls)
	require.NoError(t, locks.ReleaseLeaderLock(context.Background(), "jobs:openai-window-activation:59", svc.owner)) // simulate TTL elapsing
	svc.activateWindow(context.Background(), account)
	require.Equal(t, 2, tester.calls)
}

func TestOpenAIWindowActivationDoesNotProbeExhaustedWindowWithoutResetTime(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name   string
		window *OpenAIRateLimitWindow
		probe  bool
	}{
		{"active countdown at 100 percent", &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, UsedPercent: 100, ResetAt: now.Add(time.Hour).Unix()}, false},
		{"exhausted without reset time", &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, UsedPercent: 100}, false},
		{"expired at 100 percent", &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, UsedPercent: 100, ResetAt: now.Add(-time.Minute).Unix()}, true},
		{"missing window", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{SecondaryWindow: tc.window}}
			require.Equal(t, tc.probe, shouldProbeOpenAI5hActivation(usage, now))
		})
	}
}

func TestOpenAIWindowActivationSkipsExhaustedUnknownResetInWorker(t *testing.T) {
	local := time.Now().In(time.FixedZone("CST", 8*3600))
	account := &Account{ID: 60, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{
		openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": local.Add(-time.Minute).Format("15:04"), "end": local.Add(2 * time.Minute).Format("15:04")},
	}}
	quota := &autoResetTestQuota{usage: &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
		SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, UsedPercent: 100},
	}}}
	tester := &windowActivationTestRequester{}
	svc := NewOpenAIQuotaAutoResetService(nil, quota, nil, nil, nil, nil, &fakeLeaderLockCache{})
	svc.accountTest = tester
	svc.activationLocation = time.FixedZone("CST", 8*3600)
	svc.activateWindow(context.Background(), account)
	require.Zero(t, tester.calls)
	require.Equal(t, int32(1), quota.queryCalls.Load())
}

func TestOpenAIWindowActivationJitterConfiguration(t *testing.T) {
	legacy := map[string]any{openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": "05:00", "end": "00:00"}}
	normalized, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, legacy)
	require.NoError(t, err)
	require.NotContains(t, normalized[openAIWindowActivationExtraKey], "jitter_minutes")

	for _, valid := range []any{0, 30, 60, float64(30)} {
		extra := map[string]any{openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": "05:00", "end": "00:00", "jitter_minutes": valid}}
		_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, extra)
		require.NoError(t, err, "valid jitter %v", valid)
	}
	for _, invalid := range []any{-1, 61, 1.5, "30", nil} {
		extra := map[string]any{openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": "05:00", "end": "00:00", "jitter_minutes": invalid}}
		_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, extra)
		require.Error(t, err, "invalid jitter %v", invalid)
	}
}

func TestOpenAIWindowActivationJitterSpreadsAccountsAndCycles(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 9, 30, 9, 0, 0, 0, loc)
	config := map[string]any{"enabled": true, "start": "05:00", "end": "20:00", "jitter_minutes": 30}
	first := &Account{ID: 101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{openAIWindowActivationExtraKey: config}}
	second := &Account{ID: 202, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{openAIWindowActivationExtraKey: config}}
	start := time.Date(2026, 9, 30, 5, 0, 0, 0, loc)
	a := openAIWindowActivationAt(first, nil, now, loc)
	b := openAIWindowActivationAt(second, nil, now, loc)
	require.WithinDuration(t, a, openAIWindowActivationAt(first, nil, now, loc), 0, "restart/replica must retain the same delay")
	require.True(t, !a.Before(start) && !a.After(start.Add(30*time.Minute)))
	require.True(t, !b.Before(start) && !b.After(start.Add(30*time.Minute)))
	require.NotEqual(t, a, b, "different accounts should spread out")

	reset := time.Date(2026, 9, 30, 10, 7, 0, 0, loc)
	usage := &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAt: reset.Unix()}}}
	next := openAIWindowActivationAt(first, usage, reset.Add(time.Hour), loc)
	require.True(t, !next.Before(reset) && !next.After(reset.Add(30*time.Minute)))
	require.NotEqual(t, a.Sub(start), next.Sub(reset), "each window gets a fresh delay")
}

func TestOpenAIWindowActivationJitterLegacyAndOvernight(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 10, 1, 1, 0, 0, 0, loc)
	account := &Account{ID: 303, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{
		openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": "22:00", "end": "02:00"},
	}}
	yesterday := time.Date(2026, 9, 30, 22, 0, 0, 0, loc)
	require.Equal(t, yesterday, openAIWindowActivationAt(account, nil, now, loc), "legacy configuration has no delay")
	reset := time.Date(2026, 10, 1, 0, 30, 0, 0, loc)
	usage := &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAt: reset.Unix()}}}
	require.True(t, reset.Equal(openAIWindowActivationAt(account, usage, now, loc)), "zero jitter retains the actual reset instant")
	activation, ok := account.Extra[openAIWindowActivationExtraKey].(map[string]any)
	require.True(t, ok)
	activation["jitter_minutes"] = 60
	due := openAIWindowActivationAt(account, usage, now, loc)
	require.True(t, !due.Before(reset) && !due.After(reset.Add(time.Hour)), "cross-midnight delay uses the current interval")
}

func TestOpenAIWindowActivationJitterSkipsBeyondEndAndExistingCountdown(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 9, 30, 5, 58, 0, 0, loc)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{
		openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": "05:00", "end": "07:00", "jitter_minutes": 60},
	}}
	reset := now.Add(-time.Minute)
	expired := &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAt: reset.Unix()}}}
	end := time.Date(2026, 9, 30, 6, 0, 0, 0, loc)
	for id := int64(1); id < 1000; id++ {
		account.ID = id
		if openAIWindowActivationAt(account, expired, now, loc).After(end) {
			break
		}
	}
	require.True(t, openAIWindowActivationAt(account, expired, now, loc).After(end))
	activation, ok := account.Extra[openAIWindowActivationExtraKey].(map[string]any)
	require.True(t, ok)
	activation["end"] = "06:00"
	require.True(t, openAIWindowActivationAt(account, expired, now, loc).IsZero(), "do not activate when the delay crosses the configured end")
	active := &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAt: now.Add(time.Hour).Unix()}}}
	require.False(t, shouldProbeOpenAI5hActivation(active, now), "a user-triggered live countdown must prevent a probe")
}

func TestOpenAIWindowActivationWaitsForEachAccountAndExpiredWindow(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Now().In(loc)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{
		openAIWindowActivationExtraKey: map[string]any{"enabled": true, "start": now.Add(-time.Minute).Format("15:04"), "end": now.Add(2 * time.Hour).Format("15:04"), "jitter_minutes": 60},
	}}
	for id := int64(1); id < 1000; id++ {
		account.ID = id
		if openAIWindowActivationAt(account, nil, now, loc).After(now.Add(5 * time.Minute)) {
			break
		}
	}
	require.True(t, openAIWindowActivationAt(account, nil, now, loc).After(now.Add(5*time.Minute)))
	quota := &autoResetTestQuota{usage: &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{}}}
	tester := &windowActivationTestRequester{}
	svc := NewOpenAIQuotaAutoResetService(nil, quota, nil, nil, nil, nil, &fakeLeaderLockCache{})
	svc.accountTest = tester
	svc.activationLocation = loc
	svc.activateWindow(context.Background(), account)
	require.Zero(t, tester.calls)
	require.Zero(t, quota.queryCalls.Load(), "first daily delay should not lock or query usage")

	activation, ok := account.Extra[openAIWindowActivationExtraKey].(map[string]any)
	require.True(t, ok)
	activation["start"] = now.Add(-2 * time.Hour).Format("15:04")
	reset := now.Add(-time.Minute).Truncate(time.Second)
	quota.usage = &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{SecondaryWindow: &OpenAIRateLimitWindow{LimitWindowSeconds: 5 * 3600, ResetAt: reset.Unix()}}}
	for id := int64(1); id < 1000; id++ {
		account.ID = id
		if openAIWindowActivationAt(account, quota.usage, now, loc).After(now.Add(5 * time.Minute)) {
			break
		}
	}
	require.True(t, openAIWindowActivationAt(account, quota.usage, now, loc).After(now.Add(5*time.Minute)))
	svc.activateWindow(context.Background(), account)
	require.Zero(t, tester.calls)
	require.Equal(t, int32(1), quota.queryCalls.Load(), "expired window should be queried and delayed")
}
