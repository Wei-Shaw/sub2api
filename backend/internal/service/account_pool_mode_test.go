//go:build unit

package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetPoolModeRetryCount(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected int
	}{
		{
			name: "default_when_not_pool_mode",
			account: &Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformOpenAI,
				Credentials: map[string]any{}, Extra: map[string]any{"openai_responses_mode": "force_responses"},
			},
			expected: defaultPoolModeRetryCount,
		},
		{
			name: "default_when_missing_retry_count",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode": true,
				}, Extra: map[string]any{"openai_responses_mode": "force_responses"},
			},
			expected: defaultPoolModeRetryCount,
		},
		{
			name: "supports_float64_from_json_credentials",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode":             true,
					"pool_mode_retry_count": float64(5),
				}, Extra: map[string]any{"openai_responses_mode": "force_responses"},
			},
			expected: 5,
		},
		{
			name: "supports_json_number",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode":             true,
					"pool_mode_retry_count": json.Number("4"),
				}, Extra: map[string]any{"openai_responses_mode": "force_responses"},
			},
			expected: 4,
		},
		{
			name: "supports_string_value",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode":             true,
					"pool_mode_retry_count": "2",
				}, Extra: map[string]any{"openai_responses_mode": "force_responses"},
			},
			expected: 2,
		},
		{
			name: "negative_value_is_clamped_to_zero",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode":             true,
					"pool_mode_retry_count": -1,
				}, Extra: map[string]any{"openai_responses_mode": "force_responses"},
			},
			expected: 0,
		},
		{
			name: "oversized_value_is_clamped_to_max",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode":             true,
					"pool_mode_retry_count": 99,
				}, Extra: map[string]any{"openai_responses_mode": "force_responses"},
			},
			expected: maxPoolModeRetryCount,
		},
		{
			name: "invalid_value_falls_back_to_default",
			account: &Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"pool_mode":             true,
					"pool_mode_retry_count": "oops",
				}, Extra: map[string]any{"openai_responses_mode": "force_responses"},
			},
			expected: defaultPoolModeRetryCount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.GetPoolModeRetryCount())
		})
	}
}

func TestGetPoolModeRetryDelay(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected time.Duration
	}{
		{
			name:     "default_when_not_pool_mode",
			account:  &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{}, Extra: map[string]any{"openai_responses_mode": "force_responses"}},
			expected: defaultPoolModeRetryDelay,
		},
		{
			name: "configured_delay_in_milliseconds",
			account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{
				"pool_mode": true, "pool_mode_retry_delay_ms": 1500,
			}, Extra: map[string]any{"openai_responses_mode": "force_responses"}},
			expected: 1500 * time.Millisecond,
		},
		{
			name: "zero_disables_delay",
			account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{
				"pool_mode": true, "pool_mode_retry_delay_ms": 0,
			}, Extra: map[string]any{"openai_responses_mode": "force_responses"}},
			expected: 0,
		},
		{
			name: "oversized_delay_is_clamped",
			account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{
				"pool_mode": true, "pool_mode_retry_delay_ms": 120000,
			}, Extra: map[string]any{"openai_responses_mode": "force_responses"}},
			expected: maxPoolModeRetryDelay,
		},
		{
			name: "invalid_or_negative_delay_uses_default",
			account: &Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{
				"pool_mode": true, "pool_mode_retry_delay_ms": "oops",
			}, Extra: map[string]any{"openai_responses_mode": "force_responses"}},
			expected: defaultPoolModeRetryDelay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.account.GetPoolModeRetryDelay())
		})
	}
}
