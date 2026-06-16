package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Task 6.2 验证: math/rand/v2 迁移后 nextBackoff 行为正确 ---

func TestNextBackoff_ExponentialGrowth(t *testing.T) {
	// 验证退避时间指数增长（乘数 1.5）
	// 由于有随机抖动（±20%），需要验证范围
	current := initialBackoff // 100ms

	for i := 0; i < 10; i++ {
		next := nextBackoff(current)

		// 退避结果应在 [initialBackoff, maxBackoff] 范围内
		assert.GreaterOrEqual(t, int64(next), int64(initialBackoff),
			"第 %d 次退避不应低于初始值 %v", i, initialBackoff)
		assert.LessOrEqual(t, int64(next), int64(maxBackoff),
			"第 %d 次退避不应超过最大值 %v", i, maxBackoff)

		// 为下一轮提供当前退避值
		current = next
	}
}

func TestNextBackoff_BoundedByMaxBackoff(t *testing.T) {
	// 即使输入非常大，输出也不超过 maxBackoff
	for i := 0; i < 100; i++ {
		result := nextBackoff(10 * time.Second)
		assert.LessOrEqual(t, int64(result), int64(maxBackoff),
			"退避值不应超过 maxBackoff")
	}
}

func TestNextBackoff_BoundedByInitialBackoff(t *testing.T) {
	// 即使输入非常小，输出也不低于 initialBackoff
	for i := 0; i < 100; i++ {
		result := nextBackoff(1 * time.Millisecond)
		assert.GreaterOrEqual(t, int64(result), int64(initialBackoff),
			"退避值不应低于 initialBackoff")
	}
}

func TestNextBackoff_HasJitter(t *testing.T) {
	// 验证多次调用会产生不同的值（随机抖动生效）
	// 使用相同的输入调用 50 次，收集结果
	results := make(map[time.Duration]bool)
	current := 500 * time.Millisecond

	for i := 0; i < 50; i++ {
		result := nextBackoff(current)
		results[result] = true
	}

	// 50 次调用应该至少有 2 个不同的值（抖动存在）
	require.Greater(t, len(results), 1,
		"nextBackoff 应产生随机抖动，但所有 50 次调用结果相同")
}

func TestNextBackoff_InitialValueGrows(t *testing.T) {
	// 验证从初始值开始，退避趋势是增长的
	current := initialBackoff
	var sum time.Duration

	runs := 100
	for i := 0; i < runs; i++ {
		next := nextBackoff(current)
		sum += next
		current = next
	}

	avg := sum / time.Duration(runs)
	// 平均退避时间应大于初始值（因为指数增长 + 上限）
	assert.Greater(t, int64(avg), int64(initialBackoff),
		"平均退避时间应大于初始退避值")
}

func TestNextBackoff_ConvergesToMaxBackoff(t *testing.T) {
	// 从初始值开始，经过多次退避后应收敛到 maxBackoff 附近
	current := initialBackoff
	for i := 0; i < 20; i++ {
		current = nextBackoff(current)
	}

	// 经过 20 次迭代后，应该已经到达 maxBackoff 区间
	// 由于抖动，允许 ±20% 的范围
	lowerBound := time.Duration(float64(maxBackoff) * 0.8)
	assert.GreaterOrEqual(t, int64(current), int64(lowerBound),
		"经过多次退避后应收敛到 maxBackoff 附近")
}

func BenchmarkNextBackoff(b *testing.B) {
	current := initialBackoff
	for i := 0; i < b.N; i++ {
		current = nextBackoff(current)
		if current > maxBackoff {
			current = initialBackoff
		}
	}
}
