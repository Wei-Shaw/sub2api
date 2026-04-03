package service

import (
	"testing"
	"time"
)

func TestAccountTargetGroup_IsExhausted_OAuthCodex7DThreshold(t *testing.T) {
	a := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"codex_7d_used_percent": 100.0},
	}

	if !a.IsExhausted() {
		t.Fatalf("expected oauth account to be exhausted when codex_7d_used_percent >= 100")
	}
}

func TestAccountTargetGroup_IsExhausted_OAuthCodexPrimaryThreshold(t *testing.T) {
	a := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"codex_primary_used_percent": 100.0},
	}

	if !a.IsExhausted() {
		t.Fatalf("expected oauth account to be exhausted when codex_primary_used_percent >= 100")
	}
}

func TestAccountTargetGroup_IsExhausted_APIKeyQuotaExceeded(t *testing.T) {
	now := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	a := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_limit":        10.0,
			"quota_used":         10.0,
			"quota_daily_start":  now,
			"quota_weekly_start": now,
		},
	}

	if !a.IsExhausted() {
		t.Fatalf("expected api key account to be exhausted when quota is exceeded")
	}
}

func TestAccountTargetGroup_IsExhausted_BelowThresholdNotExhausted(t *testing.T) {
	oauth := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"codex_7d_used_percent": 99.9, "codex_primary_used_percent": 99.9},
	}
	if oauth.IsExhausted() {
		t.Fatalf("expected oauth account below threshold to be not exhausted")
	}

	apiKey := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_limit": 10.0,
			"quota_used":  9.0,
		},
	}
	if apiKey.IsExhausted() {
		t.Fatalf("expected api key account below quota to be not exhausted")
	}
}

func TestAccountTargetGroup_IsSchedulableForTargetGroup_ExhaustedSkipsRateLimitReset(t *testing.T) {
	future := time.Now().Add(5 * time.Minute)
	a := &Account{
		Status:           StatusActive,
		Schedulable:      true,
		Platform:         PlatformOpenAI,
		Type:             AccountTypeOAuth,
		Extra:            map[string]any{"codex_7d_used_percent": 100.0},
		RateLimitResetAt: &future,
	}

	if !a.IsSchedulableForTargetGroup(TargetGroupExhausted) {
		t.Fatalf("expected exhausted account to remain schedulable for exhausted group despite rate limit reset")
	}
}

func TestAccountTargetGroup_IsSchedulableForTargetGroup_ExhaustedNotInActiveGroup(t *testing.T) {
	future := time.Now().Add(5 * time.Minute)
	a := &Account{
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Extra:       map[string]any{"codex_7d_used_percent": 100.0},

		RateLimitResetAt: &future,
	}

	if a.IsSchedulableForTargetGroup(TargetGroupActive) {
		t.Fatalf("expected exhausted account to be blocked from active group")
	}
}

func TestAccountTargetGroup_IsSchedulableForTargetGroup_TempUnschedulableBlocksExhaustedGroup(t *testing.T) {
	futureRateLimit := time.Now().Add(5 * time.Minute)
	futureTempUnsched := time.Now().Add(2 * time.Minute)
	a := &Account{
		Status:      StatusActive,
		Schedulable: true,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Extra:       map[string]any{"codex_7d_used_percent": 100.0},

		RateLimitResetAt:       &futureRateLimit,
		TempUnschedulableUntil: &futureTempUnsched,
	}

	if a.IsSchedulableForTargetGroup(TargetGroupExhausted) {
		t.Fatalf("expected temp unschedulable exhausted account to be blocked in exhausted group")
	}
}
