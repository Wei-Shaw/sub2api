//go:build unit

package service

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newIntelligenceCheckTestContext 造一个仅用于承载覆盖值的 gin.Context。
// 官方 payload 构造函数本身不需要真实请求，覆盖值也只从 context 上取。
func newIntelligenceCheckTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request, _ = http.NewRequest(http.MethodPost, "/", nil)
	return c
}

// 思考等级必须真的落到上游请求体里，否则这个配置就是个死设置——
// 界面上能改、数据库里有值、记录表里也有值，唯独上游收不到。
// 三家协议的字段名完全不同，写错哪一个都会表现为「配了没效果」。
func TestIntelligenceCheckPayload_InjectsReasoningEffortPerProtocol(t *testing.T) {
	t.Run("claude 用 thinking.budget_tokens", func(t *testing.T) {
		c := newIntelligenceCheckTestContext()
		withIntelligenceCheckOverride(c, intelligenceCheckOverride{
			Prompt:          "draw a pelican",
			MaxTokens:       32000,
			ReasoningEffort: "high",
		})

		payload, err := createClaudeTestPayloadForIntelligenceCheck(c, "claude-sonnet-4-5")
		require.NoError(t, err)
		require.Equal(t, 32000, payload["max_tokens"])

		thinking, ok := payload["thinking"].(map[string]any)
		require.True(t, ok, "claude payload 必须带 thinking 配置")
		require.Equal(t, "enabled", thinking["type"])
		require.Equal(t, 16384, thinking["budget_tokens"])
	})

	t.Run("claude 的思考预算必须严格小于输出上限", func(t *testing.T) {
		c := newIntelligenceCheckTestContext()
		// 输出上限只比最小预算大一点：此时不能硬塞一个更大的预算，
		// 否则上游会因为 budget_tokens >= max_tokens 直接拒绝请求。
		withIntelligenceCheckOverride(c, intelligenceCheckOverride{
			Prompt:          "draw a pelican",
			MaxTokens:       6000,
			ReasoningEffort: "max",
		})

		payload, err := createClaudeTestPayloadForIntelligenceCheck(c, "claude-sonnet-4-5")
		require.NoError(t, err)

		thinking, ok := payload["thinking"].(map[string]any)
		require.True(t, ok)
		budget, ok := thinking["budget_tokens"].(int)
		require.True(t, ok)
		require.Less(t, budget, 6000, "预算必须小于 max_tokens")
		require.GreaterOrEqual(t, budget, claudeThinkingMinBudgetTokens)
	})

	t.Run("claude 输出上限太小时不注入思考配置", func(t *testing.T) {
		c := newIntelligenceCheckTestContext()
		withIntelligenceCheckOverride(c, intelligenceCheckOverride{
			Prompt:          "draw a pelican",
			MaxTokens:       2048,
			ReasoningEffort: "high",
		})

		payload, err := createClaudeTestPayloadForIntelligenceCheck(c, "claude-sonnet-4-5")
		require.NoError(t, err)
		_, hasThinking := payload["thinking"]
		require.False(t, hasThinking, "塞不进合法预算时宁可走上游默认，也不要发非法请求")
	})

	t.Run("openai responses 用嵌套 reasoning.effort", func(t *testing.T) {
		c := newIntelligenceCheckTestContext()
		withIntelligenceCheckOverride(c, intelligenceCheckOverride{
			Prompt:          "draw a pelican",
			MaxTokens:       32000,
			ReasoningEffort: "xhigh",
		})

		payload := createOpenAIResponsesPayloadForIntelligenceCheck(c, "gpt-6-astra", false)
		reasoning, ok := payload["reasoning"].(map[string]any)
		require.True(t, ok, "responses payload 必须带嵌套 reasoning")
		require.Equal(t, "xhigh", reasoning["effort"])
		require.Equal(t, 32000, payload["max_output_tokens"])
	})

	t.Run("openai chat 用顶层 reasoning_effort", func(t *testing.T) {
		c := newIntelligenceCheckTestContext()
		withIntelligenceCheckOverride(c, intelligenceCheckOverride{
			Prompt:          "draw a pelican",
			MaxTokens:       32000,
			ReasoningEffort: "medium",
		})

		payload := createOpenAIChatPayloadForIntelligenceCheck(c, "deepseek-v4.1-flash", "draw a pelican")
		require.Equal(t, "medium", payload["reasoning_effort"])
		require.Equal(t, 32000, payload["max_tokens"])
	})
}

// 留空＝不干预：绝不能凭空塞一个上游不认的字段，把官方连通性测试的请求体也一起改了。
func TestIntelligenceCheckPayload_EmptyEffortKeepsOfficialShape(t *testing.T) {
	c := newIntelligenceCheckTestContext()
	withIntelligenceCheckOverride(c, intelligenceCheckOverride{
		Prompt:    "draw a pelican",
		MaxTokens: 32000,
	})

	claudePayload, err := createClaudeTestPayloadForIntelligenceCheck(c, "claude-sonnet-4-5")
	require.NoError(t, err)
	_, hasThinking := claudePayload["thinking"]
	require.False(t, hasThinking)

	responsesPayload := createOpenAIResponsesPayloadForIntelligenceCheck(c, "gpt-6-astra", false)
	_, hasReasoning := responsesPayload["reasoning"]
	require.False(t, hasReasoning)

	chatPayload := createOpenAIChatPayloadForIntelligenceCheck(c, "deepseek-v4.1-flash", "draw a pelican")
	_, hasEffort := chatPayload["reasoning_effort"]
	require.False(t, hasEffort)
}

// 没有覆盖值时，包装函数必须与官方原函数逐字节等价——
// 官方账号连通性测试、定时连通性测试、用量采样都共用同一条构造链路。
func TestIntelligenceCheckPayload_WithoutOverrideMatchesOfficial(t *testing.T) {
	c := newIntelligenceCheckTestContext()

	wrapped, err := createClaudeTestPayloadForIntelligenceCheck(c, "claude-sonnet-4-5")
	require.NoError(t, err)
	official, err := createTestPayload("claude-sonnet-4-5")
	require.NoError(t, err)

	// sessionID 每次都不同，比较时排除 metadata。
	delete(wrapped, "metadata")
	delete(official, "metadata")
	require.Equal(t, official, wrapped)

	require.Equal(t, createOpenAITestPayload("gpt-6-astra", false),
		createOpenAIResponsesPayloadForIntelligenceCheck(c, "gpt-6-astra", false))

	require.Equal(t, createOpenAIChatCompletionsTestPayload("deepseek-v4.1-flash", "hi"),
		createOpenAIChatPayloadForIntelligenceCheck(c, "deepseek-v4.1-flash", "hi"))
}

// 账号级覆盖优先、留空回落全局——两级回落必须两个入口都实现，
// 否则会出现「手动重跑生效、定时任务不生效」这种只在特定路径失效的诡异现象。
func TestReadIntelligenceCheckAccountConfig_ReasoningEffort(t *testing.T) {
	t.Run("读到账号级覆盖", func(t *testing.T) {
		config := ReadIntelligenceCheckAccountConfig(map[string]any{
			IntelligenceCheckEnabledExtraKey:         true,
			IntelligenceCheckReasoningEffortExtraKey: "low",
		})
		require.Equal(t, "low", config.ReasoningEffort)
	})

	t.Run("缺省时为空即跟随全局", func(t *testing.T) {
		config := ReadIntelligenceCheckAccountConfig(map[string]any{
			IntelligenceCheckEnabledExtraKey: true,
		})
		require.Equal(t, "", config.ReasoningEffort)
	})

	t.Run("类型不符时回退成未覆盖", func(t *testing.T) {
		config := ReadIntelligenceCheckAccountConfig(map[string]any{
			IntelligenceCheckReasoningEffortExtraKey: 42,
		})
		require.Equal(t, "", config.ReasoningEffort)
	})
}

// 留空即「跟随全局」，所以归一化要把空值对应的键删掉，
// 而不是留一个空字符串在 extra 里制造「配过了」的假象。
func TestNormalizeIntelligenceCheckExtra_ReasoningEffort(t *testing.T) {
	t.Run("空串删除键", func(t *testing.T) {
		normalized, err := NormalizeIntelligenceCheckExtra(map[string]any{
			IntelligenceCheckReasoningEffortExtraKey: "   ",
		})
		require.NoError(t, err)
		_, exists := normalized[IntelligenceCheckReasoningEffortExtraKey]
		require.False(t, exists)
	})

	t.Run("非空值去除首尾空白后保留", func(t *testing.T) {
		normalized, err := NormalizeIntelligenceCheckExtra(map[string]any{
			IntelligenceCheckReasoningEffortExtraKey: "  high  ",
		})
		require.NoError(t, err)
		require.Equal(t, "high", normalized[IntelligenceCheckReasoningEffortExtraKey])
	})

	t.Run("类型不符直接拒绝", func(t *testing.T) {
		_, err := NormalizeIntelligenceCheckExtra(map[string]any{
			IntelligenceCheckReasoningEffortExtraKey: 123,
		})
		require.Error(t, err)
	})
}

// 智力检测必须走非流式：请求体 stream 被关掉，官方路径保持流式。
func TestIntelligenceCheckPayload_DisableStream(t *testing.T) {
	t.Run("开启时把 stream 关掉", func(t *testing.T) {
		c := newIntelligenceCheckTestContext()
		withIntelligenceCheckOverride(c, intelligenceCheckOverride{
			Prompt:        "draw a pelican",
			MaxTokens:     32000,
			DisableStream: true,
		})

		payload := createOpenAIResponsesPayloadForIntelligenceCheck(c, "gpt-6-astra", false)
		require.Equal(t, false, payload["stream"])
	})

	t.Run("关闭时保持官方流式", func(t *testing.T) {
		c := newIntelligenceCheckTestContext()
		withIntelligenceCheckOverride(c, intelligenceCheckOverride{
			Prompt:    "draw a pelican",
			MaxTokens: 32000,
		})

		payload := createOpenAIResponsesPayloadForIntelligenceCheck(c, "gpt-6-astra", false)
		require.Equal(t, true, payload["stream"])
	})

	t.Run("只有 DisableStream 的覆盖也不能被丢弃", func(t *testing.T) {
		c := newIntelligenceCheckTestContext()
		withIntelligenceCheckOverride(c, intelligenceCheckOverride{DisableStream: true})

		payload := createOpenAIResponsesPayloadForIntelligenceCheck(c, "gpt-6-astra", false)
		require.Equal(t, false, payload["stream"])
	})
}

// 判形态：SSE 走原路，整体 JSON 走新路；不确定时默认落流式以保证官方行为不变。
func TestIsNonStreamOpenAIResponse(t *testing.T) {
	build := func(contentType, body string) (*http.Response, *bufio.Reader) {
		resp := &http.Response{Header: http.Header{}}
		if contentType != "" {
			resp.Header.Set("Content-Type", contentType)
		}
		return resp, bufio.NewReader(strings.NewReader(body))
	}

	t.Run("Content-Type 是 SSE 时判流式", func(t *testing.T) {
		resp, reader := build("text/event-stream", `{"id":"resp_1"}`)
		require.False(t, isNonStreamOpenAIResponse(resp, reader))
	})

	t.Run("整体 JSON 判非流式", func(t *testing.T) {
		resp, reader := build("application/json", `{"id":"resp_1","status":"completed"}`)
		require.True(t, isNonStreamOpenAIResponse(resp, reader))
	})

	t.Run("无 Content-Type 时按首字节判断", func(t *testing.T) {
		resp, reader := build("", "data: {\"type\":\"response.completed\"}\n\n")
		require.False(t, isNonStreamOpenAIResponse(resp, reader))
	})

	t.Run("前导空白不影响判断", func(t *testing.T) {
		resp, reader := build("", "\n  {\"id\":\"resp_1\"}")
		require.True(t, isNonStreamOpenAIResponse(resp, reader))
	})

	t.Run("空响应默认落流式", func(t *testing.T) {
		resp, reader := build("", "")
		require.False(t, isNonStreamOpenAIResponse(resp, reader))
	})
}

// 非流式正文提取：只认 message 里的 output_text，思考内容不得污染作品。
func TestCollectOpenAIResponsesOutputText(t *testing.T) {
	t.Run("提取 message 正文并忽略 reasoning", func(t *testing.T) {
		var data map[string]any
		require.NoError(t, json.Unmarshal([]byte(`{
			"status":"completed",
			"output":[
				{"type":"reasoning","summary":[]},
				{"type":"message","role":"assistant","content":[
					{"type":"output_text","text":"<svg>"},
					{"type":"output_text","text":"</svg>"}
				]}
			]
		}`), &data))
		require.Equal(t, "<svg></svg>", collectOpenAIResponsesOutputText(data))
	})

	t.Run("没有 output 时返回空串", func(t *testing.T) {
		require.Equal(t, "", collectOpenAIResponsesOutputText(map[string]any{}))
	})
}

// 非流式解析端到端：产出的事件形态必须与流式路径一致，
// 这样上层 parseIntelligenceCheckSSEOutput 无需区分两种形态。
func TestProcessOpenAINonStream_EmitsSameEventShape(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	body := `{"id":"resp_1","status":"completed","model":"gpt-6-astra","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"<svg>pelican</svg>"}]}]}`
	require.NoError(t, (&AccountTestService{}).processOpenAINonStream(c, strings.NewReader(body)))

	text, errMsg, _ := parseIntelligenceCheckSSEOutput(recorder.Body.String())
	require.Equal(t, "<svg>pelican</svg>", text)
	require.Equal(t, "", errMsg)
}

// 非流式下的上游错误必须被报出来，而不是当成一次成功的空作品。
// sendErrorAndEnd 自身会把错误作为返回值上抛，同时写入 error 事件。
func TestProcessOpenAINonStream_SurfacesError(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	body := `{"error":{"message":"upstream overloaded","type":"server_error"}}`
	require.Error(t, (&AccountTestService{}).processOpenAINonStream(c, strings.NewReader(body)))

	_, errMsg, _ := parseIntelligenceCheckSSEOutput(recorder.Body.String())
	require.Equal(t, "upstream overloaded", errMsg)
}

// 流式开关默认必须是「开」：默认值一旦漂移成非流式，长思考跑测会撞上游网关超时。
func TestIntelligenceCheckStreamSetting_DefaultsToStreaming(t *testing.T) {
	t.Run("全默认配置走流式", func(t *testing.T) {
		require.True(t, DefaultIntelligenceCheckGlobalSettings().StreamEnabled)
	})

	t.Run("键缺失时按默认值兜底为开启", func(t *testing.T) {
		// 老库升级后 settings 表里还没有这个键，读出来是空串。
		// 若按 isTrueSettingValue 的语义处理会得到 false，把行为静默改成非流式。
		require.True(t, isTrueSettingValueOrDefault("", true))
	})

	t.Run("显式关闭能被识别", func(t *testing.T) {
		require.False(t, isTrueSettingValueOrDefault("false", true))
	})

	t.Run("显式开启能被识别", func(t *testing.T) {
		require.True(t, isTrueSettingValueOrDefault("true", false))
	})
}

// DisableStream 是「零值即流式」的字段，这个不变式是整条开关链路的地基。
func TestIntelligenceCheckRequest_ZeroValueMeansStreaming(t *testing.T) {
	var req IntelligenceCheckRequest
	require.False(t, req.DisableStream)

	c := newIntelligenceCheckTestContext()
	withIntelligenceCheckOverride(c, intelligenceCheckOverride{
		Prompt:    "draw a pelican",
		MaxTokens: 32000,
	})
	payload := createOpenAIResponsesPayloadForIntelligenceCheck(c, "gpt-6-astra", false)
	require.Equal(t, true, payload["stream"], "未开启 DisableStream 时必须保持官方流式请求体")
}
