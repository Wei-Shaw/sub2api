//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEstimateKiroCountTokens_AnthropicRequests 覆盖 count_tokens 本地估算
// 的正常路径——不打上游，纯本地估算，对齐 TestEstimateGrokCountTokens_*
// （openai_gateway_count_tokens_test.go）同款用例结构。
func TestEstimateKiroCountTokens_AnthropicRequests(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "simple message",
			body: `{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":"hello world"}]}`,
		},
		{
			name: "system blocks and tools",
			body: `{
				"model":"claude-sonnet-4.6",
				"system":[{"type":"text","text":"You are helpful."}],
				"messages":[{"role":"user","content":[{"type":"text","text":"look up the weather"}]}],
				"tools":[{"name":"lookup_weather","description":"Look up weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}],
				"tool_choice":{"type":"auto"}
			}`,
		},
		{
			// count_tokens 不应该按 kiro.MapModel 的白名单校验模型——客户端
			// 拿这个端点估算 context 大小，不代表真的会把请求发给 Kiro，
			// 不该被本地白名单挡住（与真实转发路径 kiro_gateway_service.go
			// 的 forwardUpstream 是不同的关注点）。
			name: "model not in kiro's real whitelist still estimates",
			body: `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EstimateKiroCountTokens([]byte(tt.body))
			require.NoError(t, err)
			require.Positive(t, got)
		})
	}
}

// TestEstimateKiroCountTokens_EmptyConversationIsZeroNotError 覆盖一个与
// Grok 的 EstimateGrokCountTokens 不同的真实行为差异：kiro.EstimateRequestInput
// 没有 Grok/OpenAI 那套 fallback 最小值兜底（tokens.go 逐段累加，system/
// messages/tools 都为空时总和就是 0），0 是这里的合法结果，不是错误——
// model 非空即可通过校验。写这条测试是因为一开始想当然照抄了 Grok 测试里
// "空对话也应该是正数"的假设，被真实实现的行为证伪。
func TestEstimateKiroCountTokens_EmptyConversationIsZeroNotError(t *testing.T) {
	got, err := EstimateKiroCountTokens([]byte(`{"model":"claude-sonnet-4.6","messages":[]}`))
	require.NoError(t, err)
	require.Zero(t, got)
}

// TestEstimateKiroCountTokens_RejectsInvalidRequests 覆盖必须报错的输入：
// 非法 JSON、缺 model。不校验 messages 内容的合法性——EstimateRequestInput
// 本身对解析失败的消息块是"退化成 0，不报错"的稳健策略（tokens.go 的
// estimateContentTokens 文档），这里只测 EstimateKiroCountTokens 自己
// 新增的两层校验（JSON 合法性 + model 必填）。
func TestEstimateKiroCountTokens_RejectsInvalidRequests(t *testing.T) {
	for _, body := range []string{
		`{`,
		`{"messages":[{"role":"user","content":"hello"}]}`,
		`{"model":"","messages":[{"role":"user","content":"hello"}]}`,
		`{"model":"   ","messages":[]}`,
	} {
		_, err := EstimateKiroCountTokens([]byte(body))
		require.Error(t, err, "body=%s", body)
	}
}
