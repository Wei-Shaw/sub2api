//go:build unit

package service_test

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCountZhipuMCPBillableCalls(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "单对象 tools/call 请求计 1",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_search"}}`,
			want: 1,
		},
		{
			name: "initialize 免费",
			body: `{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
			want: 0,
		},
		{
			name: "tools/list 免费",
			body: `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
			want: 0,
		},
		{
			name: "notifications 通知（无 id）免费",
			body: `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			want: 0,
		},
		{
			name: "tools/call 形态的通知（无 id）不计费：JSON-RPC 语义上不是请求",
			body: `{"jsonrpc":"2.0","method":"tools/call"}`,
			want: 0,
		},
		{
			name: "id 显式 null 等价于通知，不计费",
			body: `{"jsonrpc":"2.0","id":null,"method":"tools/call"}`,
			want: 0,
		},
		{
			name: "batch 混合：2 个 tools/call + initialize + 通知",
			body: `[
				{"jsonrpc":"2.0","id":1,"method":"tools/call"},
				{"jsonrpc":"2.0","id":2,"method":"tools/call"},
				{"jsonrpc":"2.0","id":3,"method":"initialize"},
				{"jsonrpc":"2.0","method":"notifications/message"}
			]`,
			want: 2,
		},
		{
			name: "batch 内 tools/call 通知（无 id）不计费",
			body: `[
				{"jsonrpc":"2.0","id":1,"method":"tools/call"},
				{"jsonrpc":"2.0","method":"tools/call"}
			]`,
			want: 1,
		},
		{
			name: "空 batch 计 0",
			body: `[]`,
			want: 0,
		},
		{
			name: "非法 JSON 宁可漏计",
			body: `{"jsonrpc":"2.0","id":1,"method":`,
			want: 0,
		},
		{
			name: "标量 body 不计费",
			body: `"tools/call"`,
			want: 0,
		},
		{
			name: "数字 body 不计费",
			body: `42`,
			want: 0,
		},
		{
			name: "null body 不计费",
			body: `null`,
			want: 0,
		},
		{
			name: "空 body 不计费",
			body: ``,
			want: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, service.CountZhipuMCPBillableCalls([]byte(tc.body)))
		})
	}

	// nil body（DELETE / GET 无 body 的常规形态）不计费。
	require.Equal(t, 0, service.CountZhipuMCPBillableCalls(nil))
}

// TestZhipuMCPSearchCostViaGroupPrice 验证 MCP 计费通道：zhipu 分组的
// search_price_per_1k 语义即 "MCP tools/call 每千次价格"，经
// CalculateSearchCost 按 (price/1000) × 次数 × 倍率 计费。
// 该函数本身已有 TestCalculateSearchCost 覆盖数值边界，这里锁定 MCP 视角的
// 语义组合（显式分组价 + 倍率 + batch N 次），与 handler 入账字段对齐。
func TestZhipuMCPSearchCostViaGroupPrice(t *testing.T) {
	price := 2.0 // USD / 1k calls
	s := service.NewBillingService(nil, nil)

	// batch 含 3 个 tools/call → 3 × 2.0 / 1000 = 0.006（倍率前）。
	cost := s.CalculateSearchCost(3, &price, 1.5)
	require.Equal(t, "per_request", cost.BillingMode)
	require.InDelta(t, 0.006, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.009, cost.ActualCost, 1e-12)
}
