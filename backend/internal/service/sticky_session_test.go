//go:build unit

// Package service 提供 API 网关核心服务。
// 本文件包含 shouldClearStickySession 函数的单元测试，
// 验证粘性会话清理逻辑在各种账号状态下的正确行为。
//
// This file contains unit tests for the shouldClearStickySession function,
// verifying correct sticky session clearing behavior under various account states.
package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestShouldClearStickySession tests sticky session clearing via IsSchedulable() delegation
// plus model-level rate limiting.
func TestShouldClearStickySession(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	// 短限流时间（有限流即清除粘性会话）
	shortRateLimitReset := now.Add(5 * time.Second).Format(time.RFC3339)
	// 长限流时间（有限流即清除粘性会话）
	longRateLimitReset := now.Add(30 * time.Second).Format(time.RFC3339)

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		want           bool
	}{
		{name: "nil account", account: nil, requestedModel: "", want: false},
		{name: "status error", account: &Account{Status: StatusError, Schedulable: true}, requestedModel: "", want: true},
		{name: "status disabled", account: &Account{Status: StatusDisabled, Schedulable: true}, requestedModel: "", want: true},
		{name: "schedulable false", account: &Account{Status: StatusActive, Schedulable: false}, requestedModel: "", want: true},
		{name: "temp unschedulable", account: &Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &future}, requestedModel: "", want: true},
		{name: "temp unschedulable expired", account: &Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: &past}, requestedModel: "", want: false},
		{name: "active schedulable", account: &Account{Status: StatusActive, Schedulable: true}, requestedModel: "", want: false},
		// 模型限流测试：有限流即清除
		{
			name: "model rate limited short duration",
			account: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					"model_rate_limits": map[string]any{
						"claude-sonnet-4": map[string]any{
							"rate_limit_reset_at": shortRateLimitReset,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4",
			want:           true, // 有限流即清除
		},
		{
			name: "model rate limited long duration",
			account: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					"model_rate_limits": map[string]any{
						"claude-sonnet-4": map[string]any{
							"rate_limit_reset_at": longRateLimitReset,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4",
			want:           true, // 有限流即清除
		},
		{
			name: "model rate limited different model",
			account: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					"model_rate_limits": map[string]any{
						"claude-sonnet-4": map[string]any{
							"rate_limit_reset_at": longRateLimitReset,
						},
					},
				},
			},
			requestedModel: "claude-opus-4", // 请求不同模型
			want:           false,           // 不同模型不受影响
		},
		{
			name: "apikey quota exceeded",
			account: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Type:        AccountTypeAPIKey,
				Extra: map[string]any{
					"quota_daily_limit": 10.0,
					"quota_daily_used":  10.0,
					"quota_daily_start": now.Add(-1 * time.Hour).Format(time.RFC3339),
				},
			},
			requestedModel: "",
			want:           true,
		},
		{
			name: "oauth quota exceeded not cleared",
			account: &Account{
				Status:      StatusActive,
				Schedulable: true,
				Type:        AccountTypeOAuth,
				Extra: map[string]any{
					"quota_daily_limit": 10.0,
					"quota_daily_used":  10.0,
					"quota_daily_start": now.Add(-1 * time.Hour).Format(time.RFC3339),
				},
			},
			requestedModel: "",
			want:           false,
		},
		{
			name: "overloaded account",
			account: &Account{
				Status:       StatusActive,
				Schedulable:  true,
				OverloadUntil: &future,
			},
			requestedModel: "",
			want:           true,
		},
		{
			name: "account-level rate limited",
			account: &Account{
				Status:           StatusActive,
				Schedulable:      true,
				RateLimitResetAt: &future,
			},
			requestedModel: "",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldClearStickySession(tt.account, tt.requestedModel))
		})
	}
}
