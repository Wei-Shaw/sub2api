package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsAccountSupportedForModelFilter(t *testing.T) {
	t.Run("Anthropic OAuth 支持短 ID 命中带日期后缀模型", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"claude-3-7-sonnet-20250219": "claude-3-7-sonnet-20250219",
				},
			},
		}

		require.True(t, IsAccountSupportedForModelFilter(account, "claude-3-7-sonnet"))
	})
}

func TestMatchesOpenAIQuotaStrategyFilter(t *testing.T) {
	account5h := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_quota_strategy": "prefer_5h",
		},
	}
	account7d := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_quota_strategy": "prefer_7d",
		},
	}
	accountDisabled := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{},
	}

	tests := []struct {
		name     string
		account  *Account
		filter   string
		expected bool
	}{
		{name: "no_restriction", account: account5h, filter: "", expected: true},
		{name: "prefer_5h", account: account5h, filter: "prefer_5h", expected: true},
		{name: "prefer_5h_mismatch", account: account7d, filter: "prefer_5h", expected: false},
		{name: "prefer_7d", account: account7d, filter: "prefer_7d", expected: true},
		{name: "enabled_matches_5h", account: account5h, filter: "enabled", expected: true},
		{name: "enabled_matches_7d", account: account7d, filter: "enabled", expected: true},
		{name: "disabled_matches_empty", account: accountDisabled, filter: "disabled", expected: true},
		{name: "disabled_rejects_enabled", account: account5h, filter: "disabled", expected: false},
		{name: "unknown_filter", account: account5h, filter: "unknown", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesOpenAIQuotaStrategyFilter(tt.account, tt.filter); got != tt.expected {
				t.Fatalf("MatchesOpenAIQuotaStrategyFilter() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchesModelRestrictionFilter(t *testing.T) {
	accountLimited := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5": "gpt-5",
			},
		},
	}
	accountUnlimited := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{},
	}

	tests := []struct {
		name     string
		account  *Account
		filter   string
		expected bool
	}{
		{name: "all_models_matches_everything", account: accountLimited, filter: "", expected: false},
		{name: "limited_matches_explicit_mapping", account: accountLimited, filter: AccountModelFilterLimited, expected: true},
		{name: "limited_rejects_missing_mapping", account: accountUnlimited, filter: AccountModelFilterLimited, expected: false},
		{name: "unlimited_matches_missing_mapping", account: accountUnlimited, filter: AccountModelFilterUnlimited, expected: true},
		{name: "unlimited_rejects_explicit_mapping", account: accountLimited, filter: AccountModelFilterUnlimited, expected: false},
		{name: "specific_model_still_uses_support_check", account: accountLimited, filter: "gpt-5", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAccountSupportedForModelFilter(tt.account, tt.filter); got != tt.expected {
				t.Fatalf("IsAccountSupportedForModelFilter() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchesAccountListStatusFilter(t *testing.T) {
	now := time.Date(2026, 4, 26, 21, 0, 0, 0, time.UTC)
	rateLimitedUntil := now.Add(5 * time.Minute)
	tempUnschedUntil := now.Add(5 * time.Minute)

	activeQuotaOK := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"openai_quota_strategy":               "prefer_5h",
			"openai_quota_stop_threshold_percent": 10,
			"codex_5h_used_percent":               80,
		},
	}
	activeQuotaStopped := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"openai_quota_strategy":               "prefer_5h",
			"openai_quota_stop_threshold_percent": 10,
			"codex_5h_used_percent":               95,
		},
	}
	rateLimited := &Account{
		Status:           StatusActive,
		Schedulable:      true,
		RateLimitResetAt: &rateLimitedUntil,
	}
	tempUnsched := &Account{
		Status:                 StatusActive,
		Schedulable:            true,
		TempUnschedulableUntil: &tempUnschedUntil,
	}
	unschedulable := &Account{
		Status:      StatusActive,
		Schedulable: false,
	}

	require.True(t, MatchesAccountListStatusFilter(activeQuotaOK, AccountStatusFilterActiveExcludingQuotaStopped, now))
	require.False(t, MatchesAccountListStatusFilter(activeQuotaStopped, AccountStatusFilterActiveExcludingQuotaStopped, now))
	require.True(t, MatchesAccountListStatusFilter(rateLimited, "rate_limited", now))
	require.True(t, MatchesAccountListStatusFilter(tempUnsched, "temp_unschedulable", now))
	require.True(t, MatchesAccountListStatusFilter(unschedulable, "unschedulable", now))
}
