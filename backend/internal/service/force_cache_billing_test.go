//go:build unit

package service

import (
	"context"
	"testing"
)

func TestIsForceCacheBilling(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected bool
	}{
		{
			name:     "context without force cache billing",
			ctx:      context.Background(),
			expected: false,
		},
		{
			name:     "context with force cache billing set to true",
			ctx:      context.WithValue(context.Background(), ForceCacheBillingContextKey, true),
			expected: true,
		},
		{
			name:     "context with force cache billing set to false",
			ctx:      context.WithValue(context.Background(), ForceCacheBillingContextKey, false),
			expected: false,
		},
		{
			name:     "context with wrong type value",
			ctx:      context.WithValue(context.Background(), ForceCacheBillingContextKey, "true"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsForceCacheBilling(tt.ctx)
			if result != tt.expected {
				t.Errorf("IsForceCacheBilling() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWithForceCacheBilling(t *testing.T) {
	ctx := context.Background()

	// 原始上下文没有标记
	if IsForceCacheBilling(ctx) {
		t.Error("original context should not have force cache billing")
	}

	// 使用 WithForceCacheBilling 后应该有标记
	newCtx := WithForceCacheBilling(ctx)
	if !IsForceCacheBilling(newCtx) {
		t.Error("new context should have force cache billing")
	}

	// 原始上下文应该不受影响
	if IsForceCacheBilling(ctx) {
		t.Error("original context should still not have force cache billing")
	}
}

func TestForceCacheBilling_TokenConversion(t *testing.T) {
	tests := []struct {
		name                    string
		forceCacheBilling       bool
		inputTokens             int
		cacheReadInputTokens    int
		expectedInputTokens     int
		expectedCacheReadTokens int
	}{
		{
			name:                    "force cache billing converts input to cache_read",
			forceCacheBilling:       true,
			inputTokens:             1000,
			cacheReadInputTokens:    500,
			expectedInputTokens:     0,
			expectedCacheReadTokens: 1500, // 500 + 1000
		},
		{
			name:                    "no force cache billing keeps tokens unchanged",
			forceCacheBilling:       false,
			inputTokens:             1000,
			cacheReadInputTokens:    500,
			expectedInputTokens:     1000,
			expectedCacheReadTokens: 500,
		},
		{
			name:                    "force cache billing with zero input tokens does nothing",
			forceCacheBilling:       true,
			inputTokens:             0,
			cacheReadInputTokens:    500,
			expectedInputTokens:     0,
			expectedCacheReadTokens: 500,
		},
		{
			name:                    "force cache billing with zero cache_read tokens",
			forceCacheBilling:       true,
			inputTokens:             1000,
			cacheReadInputTokens:    0,
			expectedInputTokens:     0,
			expectedCacheReadTokens: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟 RecordUsage 中的 ForceCacheBilling 逻辑
			usage := ClaudeUsage{
				InputTokens:          tt.inputTokens,
				CacheReadInputTokens: tt.cacheReadInputTokens,
			}

			// 这是 RecordUsage 中的实际逻辑
			if tt.forceCacheBilling && usage.InputTokens > 0 {
				usage.CacheReadInputTokens += usage.InputTokens
				usage.InputTokens = 0
			}

			if usage.InputTokens != tt.expectedInputTokens {
				t.Errorf("InputTokens = %d, want %d", usage.InputTokens, tt.expectedInputTokens)
			}
			if usage.CacheReadInputTokens != tt.expectedCacheReadTokens {
				t.Errorf("CacheReadInputTokens = %d, want %d", usage.CacheReadInputTokens, tt.expectedCacheReadTokens)
			}
		})
	}
}
