//go:build unit

package repository

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBillingBalanceKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   int64
		expected string
	}{
		{
			name:     "normal_user_id",
			userID:   123,
			expected: "billing:balance:123",
		},
		{
			name:     "zero_user_id",
			userID:   0,
			expected: "billing:balance:0",
		},
		{
			name:     "negative_user_id",
			userID:   -1,
			expected: "billing:balance:-1",
		},
		{
			name:     "max_int64",
			userID:   math.MaxInt64,
			expected: "billing:balance:9223372036854775807",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := billingBalanceKey(tc.userID)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestBillingSubKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   int64
		groupID  int64
		expected string
	}{
		{
			name:     "normal_ids",
			userID:   123,
			groupID:  456,
			expected: "billing:sub:123:456",
		},
		{
			name:     "zero_ids",
			userID:   0,
			groupID:  0,
			expected: "billing:sub:0:0",
		},
		{
			name:     "negative_ids",
			userID:   -1,
			groupID:  -2,
			expected: "billing:sub:-1:-2",
		},
		{
			name:     "max_int64_ids",
			userID:   math.MaxInt64,
			groupID:  math.MaxInt64,
			expected: "billing:sub:9223372036854775807:9223372036854775807",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := billingSubKey(tc.userID, tc.groupID)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestJitteredTTL(t *testing.T) {
	const (
		minTTL = 4*time.Minute + 30*time.Second // 270s = 5min - 30s
		maxTTL = 5*time.Minute + 30*time.Second // 330s = 5min + 30s
	)

	for i := 0; i < 200; i++ {
		ttl := jitteredTTL()
		require.GreaterOrEqual(t, ttl, minTTL, "jitteredTTL() 返回值低于下限: %v", ttl)
		require.LessOrEqual(t, ttl, maxTTL, "jitteredTTL() 返回值超过上限: %v", ttl)
	}
}

func TestJitteredTTL_HasVariation(t *testing.T) {
	// 多次调用应该产生不同的值（验证抖动存在）
	seen := make(map[time.Duration]struct{}, 50)
	for i := 0; i < 50; i++ {
		seen[jitteredTTL()] = struct{}{}
	}
	// 50 次调用中应该至少有 2 个不同的值
	require.Greater(t, len(seen), 1, "jitteredTTL() 应产生不同的 TTL 值")
}
