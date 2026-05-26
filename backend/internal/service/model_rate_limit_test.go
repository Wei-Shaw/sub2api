package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func TestIsModelRateLimited(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * time.Minute).Format(time.RFC3339)
	past := now.Add(-10 * time.Minute).Format(time.RFC3339)

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		expected       bool
	}{
		{
			name: "official model ID hit - claude-sonnet-4-5",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			expected:       true,
		},
		{
			name: "official model ID hit via mapping - request claude-3-5-sonnet, mapped to claude-sonnet-4-5",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"claude-3-5-sonnet": "claude-sonnet-4-5",
					},
				},
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "claude-3-5-sonnet",
			expected:       true,
		},
		{
			name: "no rate limit - expired",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": past,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			expected:       false,
		},
		{
			name: "no rate limit - no matching key",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-flash": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			expected:       false,
		},
		{
			name:           "no rate limit - unsupported model",
			account:        &Account{},
			requestedModel: "gpt-4",
			expected:       false,
		},
		{
			name:           "no rate limit - empty model",
			account:        &Account{},
			requestedModel: "",
			expected:       false,
		},
		{
			name: "gemini model hit",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-pro-high": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "gemini-3-pro-high",
			expected:       true,
		},
		{
			name: "antigravity platform - gemini-3-pro-preview mapped to gemini-3-pro-high",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-pro-high": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "gemini-3-pro-preview",
			expected:       true,
		},
		{
			name: "non-antigravity platform - gemini-3-pro-preview NOT mapped",
			account: &Account{
				Platform: PlatformGemini,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"gemini-3-pro-high": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "gemini-3-pro-preview",
			expected:       false, // gemini 平台不走 antigravity 映射
		},
		{
			name: "antigravity platform - claude-opus-4-5-thinking mapped to opus-4-6-thinking",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-opus-4-6-thinking": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "claude-opus-4-5-thinking",
			expected:       true,
		},
		{
			name: "no scope fallback - claude_sonnet should not match",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude_sonnet": map[string]any{
							"rate_limit_reset_at": future,
						},
					},
				},
			},
			requestedModel: "claude-3-5-sonnet-20241022",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.isModelRateLimitedWithContext(context.Background(), tt.requestedModel)
			if result != tt.expected {
				t.Errorf("isModelRateLimited(%q) = %v, want %v", tt.requestedModel, result, tt.expected)
			}
		})
	}
}

func TestIsModelRateLimited_Antigravity_ThinkingAffectsModelKey(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * time.Minute).Format(time.RFC3339)

	account := &Account{
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5-thinking": map[string]any{
					"rate_limit_reset_at": future,
				},
			},
		},
	}

	ctx := context.WithValue(context.Background(), ctxkey.ThinkingEnabled, true)
	if !account.isModelRateLimitedWithContext(ctx, "claude-sonnet-4-5") {
		t.Errorf("expected model to be rate limited")
	}
}

func TestGetModelRateLimitRemainingTime(t *testing.T) {
	now := time.Now()
	future10m := now.Add(10 * time.Minute).Format(time.RFC3339)
	future5m := now.Add(5 * time.Minute).Format(time.RFC3339)
	past := now.Add(-10 * time.Minute).Format(time.RFC3339)

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		minExpected    time.Duration
		maxExpected    time.Duration
	}{
		{
			name:           "nil account",
			account:        nil,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
		},
		{
			name: "model rate limited - direct hit",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future10m,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			minExpected:    9 * time.Minute,
			maxExpected:    11 * time.Minute,
		},
		{
			name: "model rate limited - via mapping",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"claude-3-5-sonnet": "claude-sonnet-4-5",
					},
				},
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future5m,
						},
					},
				},
			},
			requestedModel: "claude-3-5-sonnet",
			minExpected:    4 * time.Minute,
			maxExpected:    6 * time.Minute,
		},
		{
			name: "expired rate limit",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": past,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
		},
		{
			name:           "no rate limit data",
			account:        &Account{},
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
		},
		{
			name: "no scope fallback",
			account: &Account{
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude_sonnet": map[string]any{
							"rate_limit_reset_at": future5m,
						},
					},
				},
			},
			requestedModel: "claude-3-5-sonnet-20241022",
			minExpected:    0,
			maxExpected:    0,
		},
		{
			name: "antigravity platform - claude-opus-4-5-thinking mapped to opus-4-6-thinking",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-opus-4-6-thinking": map[string]any{
							"rate_limit_reset_at": future5m,
						},
					},
				},
			},
			requestedModel: "claude-opus-4-5-thinking",
			minExpected:    4 * time.Minute,
			maxExpected:    6 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetModelRateLimitRemainingTimeWithContext(context.Background(), tt.requestedModel)
			if result < tt.minExpected || result > tt.maxExpected {
				t.Errorf("GetModelRateLimitRemainingTime() = %v, want between %v and %v", result, tt.minExpected, tt.maxExpected)
			}
		})
	}
}

func TestGetRateLimitRemainingTime(t *testing.T) {
	now := time.Now()
	future15m := now.Add(15 * time.Minute).Format(time.RFC3339)
	future5m := now.Add(5 * time.Minute).Format(time.RFC3339)

	tests := []struct {
		name           string
		account        *Account
		requestedModel string
		minExpected    time.Duration
		maxExpected    time.Duration
	}{
		{
			name:           "nil account",
			account:        nil,
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
		},
		{
			name: "model rate limited - 15 minutes",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future15m,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			minExpected:    14 * time.Minute,
			maxExpected:    16 * time.Minute,
		},
		{
			name: "only model rate limited",
			account: &Account{
				Platform: PlatformAntigravity,
				Extra: map[string]any{
					modelRateLimitsKey: map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": future5m,
						},
					},
				},
			},
			requestedModel: "claude-sonnet-4-5",
			minExpected:    4 * time.Minute,
			maxExpected:    6 * time.Minute,
		},
		{
			name: "neither rate limited",
			account: &Account{
				Platform: PlatformAntigravity,
			},
			requestedModel: "claude-sonnet-4-5",
			minExpected:    0,
			maxExpected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetRateLimitRemainingTimeWithContext(context.Background(), tt.requestedModel)
			if result < tt.minExpected || result > tt.maxExpected {
				t.Errorf("GetRateLimitRemainingTime() = %v, want between %v and %v", result, tt.minExpected, tt.maxExpected)
			}
		})
	}
}
