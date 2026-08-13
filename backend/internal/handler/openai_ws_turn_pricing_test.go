package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestOpenAIWSTurnPricingZeroValue 钉死 WS turn 定价的零值语义：
// 没有 turn 起始回调的 ingress 模式（ws_v2 passthrough 只实现 AfterTurn）
// 必须让 pricingAt 保持零，由 RecordUsage 回退到记录时刻。
//
// 反例（本 PR 引入的回归）：用建连时刻初始化，会把透传连接的所有 turn 钉死在
// 建连时的高峰因子——客户端峰前一分钟建连并保活，整条连接就按谷价结算。
func TestOpenAIWSTurnPricingZeroValue(t *testing.T) {
	var p openAIWSTurnPricing
	require.True(t, p.current().IsZero(),
		"未经 turn 起始回调冻结时必须保持零值，交由 RecordUsage 回退记录时刻")
}

// TestOpenAIWSTurnPricingFreezePerTurn 钉死每个 turn 的 BeforeTurn 都会覆盖
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
