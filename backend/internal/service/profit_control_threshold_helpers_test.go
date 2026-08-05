package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClampProfitControlThreshold 钉死阈值清洗口径：非有限或为负一律按 0
// （等价于只放行免费上游），合法值原样保留。
func TestClampProfitControlThreshold(t *testing.T) {
	require.Equal(t, 0.0, clampProfitControlThreshold(math.NaN()))
	require.Equal(t, 0.0, clampProfitControlThreshold(math.Inf(1)))
	require.Equal(t, 0.0, clampProfitControlThreshold(math.Inf(-1)))
	require.Equal(t, 0.0, clampProfitControlThreshold(-0.5))
	require.Equal(t, 0.0, clampProfitControlThreshold(0))
	require.Equal(t, 0.7, clampProfitControlThreshold(0.7))
}

// TestProfitControlOverThreshold 钉死「越线」的唯一定义：相对 epsilon 吸收
// decimal(10,4) 落库与浮点乘法的边界误差，U == 阈值 判定为合格。
//
// 线上否决点（openAIProfitControlVetoReason）与 profit-preview 现在共用这个
// 函数与 clampProfitControlThreshold；此前各复制了一份，会随线上口径漂移，
// 导致预演结果与真实调度不一致。
func TestProfitControlOverThreshold(t *testing.T) {
	require.False(t, profitControlOverThreshold(0.7, 0.7), "U == 阈值 必须判定为合格")
	require.False(t, profitControlOverThreshold(0.6999, 0.7))
	require.True(t, profitControlOverThreshold(0.7001, 0.7))

	// 0.3*3 的浮点尾数不得被判成越线。
	require.False(t, profitControlOverThreshold(0.1*3, 0.3))

	// 阈值为 0 时只放行 0 成本上游。
	require.False(t, profitControlOverThreshold(0, 0))
	require.True(t, profitControlOverThreshold(0.0001, 0))
}
