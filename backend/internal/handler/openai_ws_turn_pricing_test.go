package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestOpenAIWSTurnPricingZeroValue 钉死未冻结时的兼容回退语义。
func TestOpenAIWSTurnPricingZeroValue(t *testing.T) {
	var p openAIWSTurnPricing
	require.True(t, p.current().IsZero(),
		"未经风险准入冻结时必须保持零值，交由 RecordUsage 回退记录时刻")
}

// TestOpenAIWSTurnPricingFreezePerTurn 钉死每个 turn 的风险准入都会覆盖
// 上一个 turn 的定价时刻：长连接跨峰谷时后续 turn 不得沿用旧时刻。
func TestOpenAIWSTurnPricingFreezePerTurn(t *testing.T) {
	var p openAIWSTurnPricing
	turn1 := time.Now().Add(-time.Hour)
	turn2 := time.Now()

	p.freeze(turn1)
	require.Equal(t, turn1, p.current())

	p.freeze(turn2)
	require.Equal(t, turn2, p.current(), "后续 turn 必须使用自己的定价时刻")
}
