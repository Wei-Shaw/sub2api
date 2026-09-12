//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestEvaluateAccountSchedulingThreshold_OpenAIChoosesLatestResetWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	wantUntil := now.Add(72 * time.Hour)
	account := &Account{
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_5h_used_percent": 90.0,
			"codex_5h_reset_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
			"codex_7d_used_percent": 85.0,
			"codex_7d_reset_at":     wantUntil.Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{
		PlatformOpenAI: 80,
	}, now)

	require.True(t, decision.ShouldPause)
	require.Equal(t, PlatformOpenAI, decision.Platform)
	require.Equal(t, "7d", decision.Window)
	require.Empty(t, decision.Scope)
	require.Equal(t, 85.0, decision.UsedPercent)
	require.NotNil(t, decision.Until)
	require.True(t, wantUntil.Equal(*decision.Until))
}

func TestEvaluateAccountSchedulingThreshold_OpenAIIgnoresMismatchedCodexSnapshotIdentity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 8, 50, 0, 0, time.UTC)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"email":                "CageLeen9208@outlook.com",
			"chatgpt_account_id":   "1f945aa7-d9a9-4369-9542-0c702ff4adb0",
			"workspace_id":         "org-nU4goUxMmureroyswT5oYPv4",
			"chatgpt_workspace_id": "org-nU4goUxMmureroyswT5oYPv4",
		},
		Extra: map[string]any{
			"email":                 "MasonDobies01@outlook.com",
			"name":                  "Paul Clark",
			"workspace_id":          "org-avRk1G4qdXg7qph3cRIraNKf",
			"codex_7d_used_percent": 100.0,
			"codex_7d_reset_at":     now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{
		PlatformOpenAI: 99,
	}, now)

	require.False(t, decision.ShouldPause)
}

func TestEvaluateAccountSchedulingThreshold_AnthropicIgnoresExpiredFiveHourWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	expiredEnd := now.Add(-30 * time.Minute)
	wantUntil := now.Add(5 * 24 * time.Hour)
	account := &Account{
		Platform:         PlatformAnthropic,
		SessionWindowEnd: &expiredEnd,
		Extra: map[string]any{
			"session_window_utilization":   0.99,
			"passive_usage_7d_utilization": 0.82,
			"passive_usage_7d_reset":       float64(wantUntil.Unix()),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{
		PlatformAnthropic: 80,
	}, now)

	require.True(t, decision.ShouldPause)
	require.Equal(t, PlatformAnthropic, decision.Platform)
	require.Equal(t, "7d", decision.Window)
	require.Empty(t, decision.Scope)
	require.Equal(t, 82.0, decision.UsedPercent)
	require.NotNil(t, decision.Until)
	require.True(t, wantUntil.Equal(*decision.Until))
}

func TestEvaluateAnthropicFableSchedulingThreshold_UsesAccountOverrideWithoutPausingAccount(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	wantUntil := now.Add(4 * 24 * time.Hour)
	account := &Account{
		Platform: PlatformAnthropic,
		Credentials: map[string]any{
			"account_scheduling_threshold": 60,
		},
		Extra: map[string]any{
			"passive_usage_7d_utilization":    0.40,
			"passive_usage_7d_reset":          float64(now.Add(3 * 24 * time.Hour).Unix()),
			"passive_usage_7d_oi_utilization": 0.61,
			"passive_usage_7d_oi_reset":       float64(wantUntil.Unix()),
		},
	}

	thresholds := map[string]int{
		PlatformAnthropic: 100,
	}

	accountDecision := EvaluateAccountSchedulingThreshold(account, thresholds, now)
	require.False(t, accountDecision.ShouldPause)

	decision := evaluateAnthropicFableSchedulingThreshold(account, thresholds, now)

	require.True(t, decision.ShouldPause)
	require.Equal(t, PlatformAnthropic, decision.Platform)
	require.Equal(t, "7d_oi", decision.Window)
	require.Equal(t, anthropicFableRateLimitKey, decision.Scope)
	require.Equal(t, 60, decision.ThresholdPercent)
	require.Equal(t, 61.0, decision.UsedPercent)
	require.NotNil(t, decision.Until)
	require.True(t, wantUntil.Equal(*decision.Until))
}

func TestEvaluateAccountSchedulingThreshold_OpenAIPreservesPercentageSemantics(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	openAIUntil := now.Add(24 * time.Hour)
	openAIAccount := &Account{
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_5h_used_percent": 1.0,
			"codex_5h_reset_at":     openAIUntil.Format(time.RFC3339),
		},
	}

	candidate := openAIThresholdCandidate(openAIAccount.Extra, "5h", now)
	require.NotNil(t, candidate)
	require.Equal(t, 1.0, candidate.usedPercent)

	openAIDecision := EvaluateAccountSchedulingThreshold(openAIAccount, map[string]int{
		PlatformOpenAI: 90,
	}, now)
	require.False(t, openAIDecision.ShouldPause)

	openAIAccount.Extra["codex_5h_used_percent"] = 91.0
	openAIDecision = EvaluateAccountSchedulingThreshold(openAIAccount, map[string]int{
		PlatformOpenAI: 90,
	}, now)
	require.True(t, openAIDecision.ShouldPause)
	require.Equal(t, 91.0, openAIDecision.UsedPercent)
}

func TestEvaluateAccountSchedulingThreshold_OpenAISkipsStaleSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	account := &Account{
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_usage_updated_at": now.Add(-2 * time.Hour).Format(time.RFC3339),
			"codex_5h_used_percent":  100.0,
			"codex_5h_reset_at":      now.Add(3 * time.Hour).Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenAI: 90}, now)

	require.False(t, decision.ShouldPause)
}

func TestEvaluateAccountSchedulingThreshold_OpenAISkipsResetWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	account := &Account{
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_usage_updated_at": now.Add(-time.Minute).Format(time.RFC3339),
			"codex_5h_used_percent":  100.0,
			"codex_5h_reset_at":      now.Add(-time.Second).Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenAI: 90}, now)

	require.False(t, decision.ShouldPause)
}

func TestEvaluateAccountSchedulingThreshold_OpenAIPausesFreshExhaustedSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(3 * time.Hour)
	account := &Account{
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_usage_updated_at": now.Add(-time.Minute).Format(time.RFC3339),
			"codex_5h_used_percent":  100.0,
			"codex_5h_reset_at":      resetAt.Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenAI: 90}, now)

	require.True(t, decision.ShouldPause)
	require.Equal(t, "5h", decision.Window)
	require.Equal(t, 100.0, decision.UsedPercent)
	require.NotNil(t, decision.Until)
	require.True(t, resetAt.Equal(*decision.Until))
}

func TestEvaluateAccountSchedulingThreshold_OpenAIPausesFreshExhaustedSevenDayWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(5 * 24 * time.Hour)
	account := &Account{
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_usage_updated_at": now.Add(-time.Minute).Format(time.RFC3339),
			"codex_7d_used_percent":  95.0,
			"codex_7d_reset_at":      resetAt.Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenAI: 90}, now)

	require.True(t, decision.ShouldPause)
	require.Equal(t, "7d", decision.Window)
	require.Equal(t, 95.0, decision.UsedPercent)
	require.NotNil(t, decision.Until)
	require.True(t, resetAt.Equal(*decision.Until))
}

func TestEvaluateAccountSchedulingThreshold_AnthropicPreservesFractionalUtilizationSemantics(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)

	anthropicUntil := now.Add(5 * time.Hour)
	anthropicAccount := &Account{
		Platform:         PlatformAnthropic,
		SessionWindowEnd: &anthropicUntil,
		Extra: map[string]any{
			"session_window_utilization": 0.92,
		},
	}

	anthropicDecision := EvaluateAccountSchedulingThreshold(anthropicAccount, map[string]int{
		PlatformAnthropic: 90,
	}, now)

	require.True(t, anthropicDecision.ShouldPause)
	require.Equal(t, 92.0, anthropicDecision.UsedPercent)
}

func TestEvaluateAccountSchedulingThreshold_AccountOverrideCanLowerOpenAIThreshold(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	wantUntil := now.Add(12 * time.Hour)
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"account_scheduling_threshold": 80,
		},
		Extra: map[string]any{
			"codex_7d_used_percent": 85.0,
			"codex_7d_reset_at":     wantUntil.Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{
		PlatformOpenAI: 90,
	}, now)

	require.True(t, decision.ShouldPause)
	require.Equal(t, PlatformOpenAI, decision.Platform)
	require.Equal(t, 80, decision.ThresholdPercent)
	require.Equal(t, "7d", decision.Window)
	require.Empty(t, decision.Scope)
	require.Equal(t, 85.0, decision.UsedPercent)
	require.NotNil(t, decision.Until)
	require.True(t, wantUntil.Equal(*decision.Until))
}

func TestEvaluateAccountSchedulingThreshold_AccountOverrideHundredDisablesOpenAI(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"account_scheduling_threshold": 100,
		},
		Extra: map[string]any{
			"codex_7d_used_percent": 99.0,
			"codex_7d_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{
		PlatformOpenAI: 80,
	}, now)

	require.False(t, decision.ShouldPause)
	require.Equal(t, 100, decision.ThresholdPercent)
}

func TestEvaluateAccountSchedulingThreshold_AccountOverrideRoundsDecimalThreshold(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	wantUntil := now.Add(12 * time.Hour)
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"account_scheduling_threshold": 75.5,
		},
		Extra: map[string]any{
			"codex_7d_used_percent": 80.0,
			"codex_7d_reset_at":     wantUntil.Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{
		PlatformOpenAI: 90,
	}, now)

	require.True(t, decision.ShouldPause)
	require.Equal(t, 76, decision.ThresholdPercent)
	require.Equal(t, 80.0, decision.UsedPercent)
	require.NotNil(t, decision.Until)
	require.True(t, wantUntil.Equal(*decision.Until))
}

func TestEvaluateAccountSchedulingThreshold_UnsupportedPlatformsDoNotPause(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		platform  string
		threshold int
		extra     map[string]any
	}{
		{
			name:      "gemini",
			platform:  PlatformGemini,
			threshold: 80,
			extra: map[string]any{
				"gemini_usage_raw": map[string]any{
					"buckets": []any{
						map[string]any{
							"modelId":           "gemini-2.5-pro",
							"remainingFraction": 0.05,
							"resetTime":         now.Add(2 * time.Hour).Format(time.RFC3339),
						},
					},
				},
			},
		},
		{
			name:      "kiro",
			platform:  PlatformKiro,
			threshold: 90,
			extra: map[string]any{
				"kiro_sched_utilization": 99.0,
				"kiro_sched_reset_at":    now.Add(24 * time.Hour).Format(time.RFC3339),
			},
		},
		{
			name:      "antigravity",
			platform:  PlatformAntigravity,
			threshold: 90,
			extra: map[string]any{
				"antigravity_sched_utilization": 92.0,
				"antigravity_sched_reset_at":    now.Add(48 * time.Hour).Format(time.RFC3339),
				"antigravity_sched_scope":       "gemini",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			account := &Account{
				Platform: tc.platform,
				Credentials: map[string]any{
					"account_scheduling_threshold": 1,
				},
				Extra: tc.extra,
			}

			decision := EvaluateAccountSchedulingThreshold(account, map[string]int{
				tc.platform: tc.threshold,
			}, now)

			require.False(t, decision.ShouldPause)
			require.Zero(t, decision.ThresholdPercent)
		})
	}
}

func TestEvaluateAccountSchedulingThreshold_GrokUsesConfiguredThresholds(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	wantUntil := now.Add(2 * time.Hour)
	account := &Account{
		Platform: PlatformGrok,
		Extra: map[string]any{
			"grok_sched_utilization": 92.0,
			"grok_sched_reset_at":    wantUntil.Format(time.RFC3339),
		},
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{
		PlatformGrok: 90,
	}, now)

	require.True(t, decision.ShouldPause)
	require.Equal(t, PlatformGrok, decision.Platform)
	require.Equal(t, 90, decision.ThresholdPercent)
	require.Equal(t, "grok", decision.Scope)
	require.Equal(t, 92.0, decision.UsedPercent)
	require.NotNil(t, decision.Until)
	require.True(t, wantUntil.Equal(*decision.Until))
}

func TestEvaluateAccountSchedulingThreshold_GrokUsesOnlyHeaderQuotaWindow(t *testing.T) {
	t.Parallel()
	// Billing seven_day/thirty_day must not drive pause; only grok_sched_* may.
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	weeklyEnd := now.Add(3 * time.Hour)
	weeklyPct := 99.0
	headerUntil := now.Add(2 * time.Hour)
	account := &Account{
		Platform: PlatformGrok,
		Extra: map[string]any{
			"grok_sched_utilization": 50.0, // below threshold
			"grok_sched_reset_at":    headerUntil.Format(time.RFC3339),
			grokBillingExtraKey: &xai.BillingSummary{
				UsagePercent: &weeklyPct,
				PeriodEnd:    weeklyEnd.Format(time.RFC3339),
			},
		},
	}
	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformGrok: 90}, now)
	require.False(t, decision.ShouldPause, "high billing % alone must not pause under scheduling windows")

	account.Extra["grok_sched_utilization"] = 95.0
	decision = EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformGrok: 90}, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, "grok", decision.Scope)
	require.Equal(t, "quota", decision.Window)
	require.NotNil(t, decision.Until)
	require.True(t, headerUntil.Equal(*decision.Until))
}

// ollamaCloudSnapshotExtra 构造 account.Extra 里 ollama_cloud 用量快照的测试夹具，
// 形状与 DB JSONB 反序列化后的嵌套 map 一致（decodeOllamaCloudUsageSnapshot 走
// marshal/unmarshal 往返，map 形式即生产读取路径）。
func ollamaCloudSnapshotExtra(status string, data map[string]any) map[string]any {
	snapshot := map[string]any{"status": status}
	if data != nil {
		snapshot["data"] = data
	}
	return map[string]any{OllamaCloudUsageSnapshotExtraKey: snapshot}
}

func ollamaCloudUsageWindowFixture(usedPercent float64, resetAt time.Time) map[string]any {
	return map[string]any{
		"used_percent": usedPercent,
		"reset_at":     resetAt.Format(time.RFC3339),
	}
}

func TestEvaluateAccountSchedulingThreshold_OllamaCloud(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(2 * time.Hour)
	sevenDayReset := now.Add(5 * 24 * time.Hour)
	newSnapshotAccount := func(mode string, extra map[string]any) *Account {
		account := &Account{
			Platform: PlatformOllamaCloud,
			Type:     AccountTypeAPIKey,
			Extra:    extra,
		}
		if mode != "" {
			account.Credentials = map[string]any{"account_mode": mode}
		}
		return account
	}
	okExtra := ollamaCloudSnapshotExtra(OllamaCloudUsageStatusOK, map[string]any{
		"five_hour": ollamaCloudUsageWindowFixture(95, fiveHourReset),
		"seven_day": ollamaCloudUsageWindowFixture(40, sevenDayReset),
	})

	cases := []struct {
		name        string
		account     *Account
		shouldPause bool
		wantWindow  string
		wantUntil   time.Time
	}{
		{
			name:        "legacy pauses at exhausted five hour window",
			account:     newSnapshotAccount("ollama_legacy", okExtra),
			shouldPause: true,
			wantWindow:  "5h",
			wantUntil:   fiveHourReset,
		},
		{
			name:        "mode unset defaults to legacy and pauses",
			account:     newSnapshotAccount("", okExtra),
			shouldPause: true,
			wantWindow:  "5h",
			wantUntil:   fiveHourReset,
		},
		{
			name:        "unknown mode value falls back to legacy and pauses",
			account:     newSnapshotAccount("bogus", okExtra),
			shouldPause: true,
			wantWindow:  "5h",
			wantUntil:   fiveHourReset,
		},
		{
			// 防回归护栏：credits 型是月度美元信用池、没有滚动窗口 reset，
			// 即使快照里解析出 5h/7d 数据也绝不停调。
			name:        "credits never pauses despite fully populated snapshot",
			account:     newSnapshotAccount("ollama_credits", okExtra),
			shouldPause: false,
		},
		{
			name:        "failed snapshot does not pause",
			account:     newSnapshotAccount("ollama_legacy", ollamaCloudSnapshotExtra(OllamaCloudUsageStatusFailed, nil)),
			shouldPause: false,
		},
		{
			name:        "ok snapshot without window data does not pause",
			account:     newSnapshotAccount("ollama_legacy", ollamaCloudSnapshotExtra(OllamaCloudUsageStatusOK, map[string]any{})),
			shouldPause: false,
		},
		{
			// 防回归护栏：A6 纳入白名单后，无快照的账号同样不得停调。
			name:        "missing snapshot does not pause",
			account:     newSnapshotAccount("ollama_legacy", map[string]any{}),
			shouldPause: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			decision := EvaluateAccountSchedulingThreshold(tc.account, map[string]int{
				PlatformOllamaCloud: 80,
			}, now)

			require.Equal(t, tc.shouldPause, decision.ShouldPause)
			if !tc.shouldPause {
				return
			}
			require.Equal(t, PlatformOllamaCloud, decision.Platform)
			require.Equal(t, tc.wantWindow, decision.Window)
			require.Equal(t, PlatformOllamaCloud, decision.Scope)
			require.NotNil(t, decision.Until)
			require.True(t, tc.wantUntil.Equal(*decision.Until))
		})
	}
}

func TestEvaluateAccountSchedulingThreshold_OllamaCloudPicksLatestResetWindow(t *testing.T) {
	t.Parallel()

	// 与 openai/anthropic/kimi 同类取最晚 reset（不是 CN 供应商的最早 reset）。
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(2 * time.Hour)
	sevenDayReset := now.Add(5 * 24 * time.Hour)
	account := &Account{
		Platform: PlatformOllamaCloud,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode": "ollama_legacy",
		},
		Extra: ollamaCloudSnapshotExtra(OllamaCloudUsageStatusOK, map[string]any{
			"five_hour": ollamaCloudUsageWindowFixture(95, fiveHourReset),
			"seven_day": ollamaCloudUsageWindowFixture(90, sevenDayReset),
		}),
	}

	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{
		PlatformOllamaCloud: 80,
	}, now)

	require.True(t, decision.ShouldPause)
	require.Equal(t, "weekly", decision.Window)
	require.Equal(t, PlatformOllamaCloud, decision.Scope)
	require.NotNil(t, decision.Until)
	require.True(t, sevenDayReset.Equal(*decision.Until))
}
