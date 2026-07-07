//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClampAntigravityRateLimitDuration(t *testing.T) {
	tests := []struct {
		name      string
		input     time.Duration
		want      time.Duration
		wantClamp bool
	}{
		{
			name:      "short delay passes through",
			input:     500 * time.Millisecond,
			want:      500 * time.Millisecond,
			wantClamp: false,
		},
		{
			name:      "30s passes through",
			input:     30 * time.Second,
			want:      30 * time.Second,
			wantClamp: false,
		},
		{
			name:      "exactly the cap is not clamped",
			input:     antigravityMaxRateLimitCooldown,
			want:      antigravityMaxRateLimitCooldown,
			wantClamp: false,
		},
		{
			name:      "5h clamped down to cap",
			input:     5 * time.Hour,
			want:      antigravityMaxRateLimitCooldown,
			wantClamp: true,
		},
		{
			name:      "24h clamped down to cap",
			input:     24 * time.Hour,
			want:      antigravityMaxRateLimitCooldown,
			wantClamp: true,
		},
		{
			name:      "7d clamped down to cap",
			input:     7 * 24 * time.Hour,
			want:      antigravityMaxRateLimitCooldown,
			wantClamp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, did := clampAntigravityRateLimitDuration(tt.input)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantClamp, did)
		})
	}
}

func TestClampAntigravityRateLimitResetAt(t *testing.T) {
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		resetAt   time.Time
		want      time.Time
		wantClamp bool
	}{
		{
			name:      "near future passes through",
			resetAt:   now.Add(30 * time.Minute),
			want:      now.Add(30 * time.Minute),
			wantClamp: false,
		},
		{
			name:      "exactly the cap is not clamped",
			resetAt:   now.Add(antigravityMaxRateLimitCooldown),
			want:      now.Add(antigravityMaxRateLimitCooldown),
			wantClamp: false,
		},
		{
			name:      "5h clamped to now+cap",
			resetAt:   now.Add(5 * time.Hour),
			want:      now.Add(antigravityMaxRateLimitCooldown),
			wantClamp: true,
		},
		{
			name:      "7d clamped to now+cap",
			resetAt:   now.Add(7 * 24 * time.Hour),
			want:      now.Add(antigravityMaxRateLimitCooldown),
			wantClamp: true,
		},
		{
			name:      "past time passes through (caller decides)",
			resetAt:   now.Add(-1 * time.Hour),
			want:      now.Add(-1 * time.Hour),
			wantClamp: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, did := clampAntigravityRateLimitResetAt(tt.resetAt, now)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantClamp, did)
		})
	}
}

func TestParseAntigravitySmartRetryInfo_ClampsExcessiveRetryDelay(t *testing.T) {
	// 模拟上游配额耗尽时返回 24h retryDelay 的情况：解析后应被截断到本地上限。
	body := `{
		"error": {
			"code": 429,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-6"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "86400s"}
			]
		}
	}`

	info := parseAntigravitySmartRetryInfo([]byte(body))
	require.NotNil(t, info)
	require.Equal(t, "claude-sonnet-4-6", info.ModelName)
	require.Equal(t, antigravityMaxRateLimitCooldown, info.RetryDelay,
		"24h retryDelay should be clamped to local cap")
}

func TestParseAntigravitySmartRetryInfo_KeepsShortRetryDelay(t *testing.T) {
	// 短 retryDelay 应原样保留，不受 clamp 影响。
	body := `{
		"error": {
			"code": 429,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-6"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.4s"}
			]
		}
	}`

	info := parseAntigravitySmartRetryInfo([]byte(body))
	require.NotNil(t, info)
	require.Equal(t, 400*time.Millisecond, info.RetryDelay)
}
