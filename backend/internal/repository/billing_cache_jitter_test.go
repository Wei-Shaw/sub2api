package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Task 6.1 验证: math/rand/v2 迁移后 jitteredTTL 行为正确 ---

func TestJitteredTTL_WithinExpectedRange(t *testing.T) {
	// jitteredTTL 使用减法抖动: billingCacheTTL - [0, billingCacheJitter)
	// 所以结果应在 [billingCacheTTL - billingCacheJitter, billingCacheTTL] 范围内
	lowerBound := billingCacheTTL - billingCacheJitter // 5min - 30s = 4min30s
	upperBound := billingCacheTTL                      // 5min

	for i := 0; i < 200; i++ {
		ttl := jitteredTTL()
		assert.GreaterOrEqual(t, int64(ttl), int64(lowerBound),
			"TTL 不应低于 %v，实际得到 %v", lowerBound, ttl)
		assert.LessOrEqual(t, int64(ttl), int64(upperBound),
			"TTL 不应超过 %v（上界不变保证），实际得到 %v", upperBound, ttl)
	}
}

func TestJitteredTTL_NeverExceedsBase(t *testing.T) {
	// 关键安全性测试：jitteredTTL 使用减法抖动，确保永远不超过 billingCacheTTL
	for i := 0; i < 500; i++ {
		ttl := jitteredTTL()
		assert.LessOrEqual(t, int64(ttl), int64(billingCacheTTL),
			"jitteredTTL 不应超过基础 TTL（上界预期不被打破）")
	}
}

func TestJitteredTTL_HasVariance(t *testing.T) {
	// 验证抖动确实产生了不同的值
	results := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		ttl := jitteredTTL()
		results[ttl] = true
	}

	require.Greater(t, len(results), 1,
		"jitteredTTL 应产生不同的值（抖动生效），但 100 次调用结果全部相同")
}

func TestJitteredTTL_AverageNearCenter(t *testing.T) {
	// 验证平均值大约在抖动范围中间
	var sum time.Duration
	runs := 1000
	for i := 0; i < runs; i++ {
		sum += jitteredTTL()
	}

	avg := sum / time.Duration(runs)
	expectedCenter := billingCacheTTL - billingCacheJitter/2 // 4min45s

	// 允许 ±5s 的误差
	tolerance := 5 * time.Second
	assert.InDelta(t, float64(expectedCenter), float64(avg), float64(tolerance),
		"平均 TTL 应接近抖动范围中心 %v", expectedCenter)
}

func TestBillingKeyGeneration(t *testing.T) {
	t.Run("balance_key", func(t *testing.T) {
		key := billingBalanceKey(12345)
		assert.Equal(t, "billing:balance:12345", key)
	})

	t.Run("sub_key", func(t *testing.T) {
		key := billingSubKey(100, 200)
		assert.Equal(t, "billing:sub:100:200", key)
	})
}

func BenchmarkJitteredTTL(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = jitteredTTL()
	}
}
