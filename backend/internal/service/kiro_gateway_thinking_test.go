//go:build unit

package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestKiroFakeThinkingPlan 覆盖"推理强度有没有生效"这个真实缺口的修复：
// 此前 forwardUpstream 无论请求携带什么 thinking/output_config，一律给
// kiro.BuildRequest 传固定的 4000——kiroFakeThinkingPlan 现在按请求实际
// 表达的诉求决定预算，账号开关仍是总闸。
func TestKiroFakeThinkingPlan(t *testing.T) {
	thinkingAccount := kiroAccount(map[string]any{"fake_thinking": true})
	noThinkingAccount := kiroAccount(map[string]any{"fake_thinking": false})

	t.Run("账号关闭时无论请求带什么都不启用", func(t *testing.T) {
		enabled, budget := kiroFakeThinkingPlan(noThinkingAccount, &apicompat.AnthropicRequest{
			Thinking: &apicompat.AnthropicThinking{Type: "enabled", BudgetTokens: 9000},
		})
		require.False(t, enabled)
		require.Zero(t, budget)
	})

	t.Run("账号开启但请求什么都没带时退回旧的默认值", func(t *testing.T) {
		enabled, budget := kiroFakeThinkingPlan(thinkingAccount, &apicompat.AnthropicRequest{})
		require.True(t, enabled)
		require.Equal(t, kiroFakeThinkingDefaultBudget, budget)
	})

	t.Run("请求显式 thinking.type=disabled 时尊重客户端，即使账号开着", func(t *testing.T) {
		enabled, budget := kiroFakeThinkingPlan(thinkingAccount, &apicompat.AnthropicRequest{
			Thinking: &apicompat.AnthropicThinking{Type: "disabled"},
		})
		require.False(t, enabled)
		require.Zero(t, budget)
	})

	t.Run("请求带原生 thinking.budget_tokens 时直接使用这个数字", func(t *testing.T) {
		enabled, budget := kiroFakeThinkingPlan(thinkingAccount, &apicompat.AnthropicRequest{
			Thinking: &apicompat.AnthropicThinking{Type: "enabled", BudgetTokens: 7777},
		})
		require.True(t, enabled)
		require.Equal(t, 7777, budget)
	})

	t.Run("请求带 output_config.effort 时按档位换算预算", func(t *testing.T) {
		cases := map[string]int{
			"low":    1024,
			"medium": 4096,
			"high":   10240,
			"xhigh":  20480,
			"max":    32768,
		}
		for effort, want := range cases {
			enabled, budget := kiroFakeThinkingPlan(thinkingAccount, &apicompat.AnthropicRequest{
				OutputConfig: &apicompat.AnthropicOutputConfig{Effort: effort},
			})
			require.True(t, enabled, effort)
			require.Equal(t, want, budget, effort)
		}
	})

	t.Run("未知的 effort 档位退回默认值而不是 0", func(t *testing.T) {
		enabled, budget := kiroFakeThinkingPlan(thinkingAccount, &apicompat.AnthropicRequest{
			OutputConfig: &apicompat.AnthropicOutputConfig{Effort: "totally-made-up"},
		})
		require.True(t, enabled)
		require.Equal(t, kiroFakeThinkingDefaultBudget, budget)
	})

	t.Run("同时带 budget_tokens 时优先于 output_config.effort", func(t *testing.T) {
		enabled, budget := kiroFakeThinkingPlan(thinkingAccount, &apicompat.AnthropicRequest{
			Thinking:     &apicompat.AnthropicThinking{Type: "enabled", BudgetTokens: 555},
			OutputConfig: &apicompat.AnthropicOutputConfig{Effort: "max"},
		})
		require.True(t, enabled)
		require.Equal(t, 555, budget)
	})
}

// TestKiroForwardUpstreamThreadsRequestedEffortIntoFakeThinkingBudget 是
// kiroFakeThinkingPlan 的接线验证：真的走一遍 forwardUpstream，确认请求携带
// 的 output_config.effort 最终体现在打给 Kiro 真实上游的 payload 里
// （<max_thinking_length> 标签里的数字），而不只是 kiroFakeThinkingPlan
// 这个纯函数本身算对了、却没被接到请求路径上。
func TestKiroForwardUpstreamThreadsRequestedEffortIntoFakeThinkingBudget(t *testing.T) {
	frames := kiroTestConcatFrames(
		kiroTestEventFrame("assistantResponseEvent", `{"content":"ok"}`),
		kiroTestEventFrame("metadataEvent", `{"stopReason":"end_turn"}`),
	)
	srv, calls, bodies := kiroTestFakeUpstreamCapturingBody(t, http.StatusOK, frames)

	svc := &KiroGatewayService{}
	svc.callEndpointOverride = kiroTestOverrideCallingServer(srv)

	account := kiroTestOAuthAccount(605)
	account.Credentials["fake_thinking"] = true

	reqBody := `{"model":"claude-sonnet-4-5-20250929","max_tokens":100,"output_config":{"effort":"high"},"messages":[{"role":"user","content":"hi"}],"stream":false}`
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(reqBody)))

	result, err := svc.ForwardUpstream(context.Background(), c, account, []byte(reqBody))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.EqualValues(t, 1, atomic.LoadInt32(calls))
	require.Len(t, *bodies, 1)
	// json.Marshal 默认把请求体里的 < > 转义成 < >（HTML-safe
	// 转义），字面尖括号形态断言不到，改成分别断言标签名和数值都出现。
	upstreamBody := string((*bodies)[0])
	require.Contains(t, upstreamBody, "max_thinking_length")
	require.Contains(t, upstreamBody, "10240",
		"output_config.effort=high 应该换算成 10240 token 预算，而不是旧的固定 4000")
	require.NotContains(t, upstreamBody, "4000", "不应该再是旧的固定默认预算")
}
