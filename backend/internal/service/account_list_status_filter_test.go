package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
	openAI5HZero := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 0.0,
		},
	}
	openAI7DZero := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_7d_used_percent": 0.0,
		},
	}
	openAINonZero := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 12.0,
			"codex_7d_used_percent": 8.0,
		},
	}
	expired7DWindow := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_7d_used_percent": 91.0,
			"codex_7d_reset_at":     now.Add(-1 * time.Hour).Format(time.RFC3339),
		},
	}

	require.True(t, MatchesAccountListStatusFilter(activeQuotaOK, AccountStatusFilterActiveExcludingQuotaStopped, now))
	require.False(t, MatchesAccountListStatusFilter(activeQuotaStopped, AccountStatusFilterActiveExcludingQuotaStopped, now))
	require.True(t, MatchesAccountListStatusFilter(rateLimited, "rate_limited", now))
	require.True(t, MatchesAccountListStatusFilter(tempUnsched, "temp_unschedulable", now))
	require.True(t, MatchesAccountListStatusFilter(unschedulable, "unschedulable", now))
	require.True(t, MatchesAccountListStatusFilter(openAI5HZero, AccountStatusFilterOpenAI5HUsedZero, now))
	require.True(t, MatchesAccountListStatusFilter(openAI7DZero, AccountStatusFilterOpenAI7DUsedZero, now))
	require.True(t, MatchesAccountListStatusFilter(expired7DWindow, AccountStatusFilterOpenAI7DUsedZero, now))
	require.False(t, MatchesAccountListStatusFilter(openAINonZero, AccountStatusFilterOpenAI5HUsedZero, now))
	require.False(t, MatchesAccountListStatusFilter(openAINonZero, AccountStatusFilterOpenAI7DUsedZero, now))
}

func TestMatchesAccountListStatusFilter_OpenAIQuotaRange(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	quotaOKInRange := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"openai_quota_strategy":               "prefer_7d",
			"openai_quota_stop_threshold_percent": 10,
			"codex_7d_used_percent":               42.0,
		},
	}
	quotaStoppedInRange := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"openai_quota_strategy":               "prefer_7d",
			"openai_quota_stop_threshold_percent": 10,
			"codex_7d_used_percent":               95.0,
		},
	}
	selectedWindowIndependentFromStrategy := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"openai_quota_strategy":               "prefer_5h",
			"openai_quota_stop_threshold_percent": 10,
			"codex_5h_used_percent":               42.0,
			"codex_7d_used_percent":               42.0,
		},
	}

	require.True(t, MatchesAccountListStatusFilter(quotaOKInRange, "openai_quota_used_range:7d:40:45", now))
	require.True(t, MatchesAccountListStatusFilter(quotaOKInRange, "openai_quota_used_range:7d:42:42", now))
	require.False(t, MatchesAccountListStatusFilter(quotaOKInRange, "openai_quota_used_range:7d:43:50", now))
	require.False(t, MatchesAccountListStatusFilter(quotaStoppedInRange, "openai_quota_used_range:7d:90:100", now), "指定额度应先套用正常（排除超额度）")
	require.True(t, MatchesAccountListStatusFilter(selectedWindowIndependentFromStrategy, "openai_quota_used_range:7d:40:45", now), "指定额度窗口由弹窗决定，不强制等于账号额度策略窗口")
}

func TestMatchesAccountListStatusFilter_OpenAIQuotaFull(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	fullError := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusError,
		Schedulable: false,
		Extra: map[string]any{
			"codex_5h_used_percent": 100.0,
			"codex_7d_used_percent": 88.0,
		},
	}
	overFull := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusDisabled,
		Extra: map[string]any{
			"codex_7d_used_percent": 105.0,
		},
	}
	notFull := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Extra: map[string]any{
			"codex_5h_used_percent": 99.9,
			"codex_7d_used_percent": 99.9,
		},
	}

	require.True(t, MatchesAccountListStatusFilter(fullError, "openai_quota_full:5h", now), "额度已满应覆盖全部账号状态")
	require.True(t, MatchesAccountListStatusFilter(overFull, "openai_quota_full:7d", now), "超过 100% 也应视为额度已满")
	require.False(t, MatchesAccountListStatusFilter(notFull, "openai_quota_full:5h", now))
	require.False(t, MatchesAccountListStatusFilter(fullError, "openai_quota_full:7d", now))
}
