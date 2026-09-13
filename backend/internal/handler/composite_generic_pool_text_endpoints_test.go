//go:build unit

package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ---- composite generic Responses / ChatCompletions 池化（B2）测试 ----
//
// 复用 composite_generic_pool_messages_test.go（B1）的假上游 / usage 捕获 / harness
// 组装手法：池候选 ctx + composite 分组身份 + RunModeSimple 跳过计费，委派链与
// generic 转换链分别挂 openAIUpstream / gwUpstream 假上游。

func poolResponsesBody() string {
	return `{"model":"my-model","stream":false,"input":"hello","max_output_tokens":64}`
}

func poolChatCompletionsBody() string {
	return `{"model":"my-model","stream":false,"messages":[{"role":"user","content":"hello"}]}`
}

// poolKimiResponsesAccount 固定 responses 协议的 kimi 账号（UsesNativeCNResponses）：
// Responses 入站按原生 Responses 端点委派。
func poolKimiResponsesAccount(id int64, groupID int64, priority int) *service.Account {
	return &service.Account{
		ID: id, Name: fmt.Sprintf("kimi-resp-%d", id), Platform: service.PlatformKimi,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Concurrency: 1, Priority: priority,
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		Credentials: map[string]any{
			"api_key":       "kimi-resp-key",
			"base_url":      "https://kimi.example/v1",
			"api_protocol":  service.APIProtocolResponses,
			"model_mapping": map[string]any{"my-model": "kimi-k3"},
		},
	}
}

// poolZhipuAdaptiveAccount 无原生 Responses 端点的 adaptive 国产账号（GLM 平台）：
// Responses 入站不委派（走既有 generic 转换链）。
func poolZhipuAdaptiveAccount(id int64, groupID int64, priority int) *service.Account {
	return &service.Account{
		ID: id, Name: fmt.Sprintf("zhipu-adaptive-%d", id), Platform: service.PlatformZhipu,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Concurrency: 1, Priority: priority,
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		Credentials: map[string]any{
			"api_key":       "zhipu-adaptive-key",
			"api_protocol":  service.APIProtocolAdaptive,
			"api_base_urls": map[string]any{"anthropic": "https://zhipu-adaptive.example"},
			"model_mapping": map[string]any{"my-model": "glm-4.7"},
		},
	}
}

// anthropicMessagesSSEStreamResponse 返回一段完整的 Anthropic Messages SSE 流。
// generic Responses/CC → Anthropic 转换链强制上游流式（clientStream 无关），假上游
// 必须以 SSE 回包，缓冲转换链聚合成入站协议格式写回。
func anthropicMessagesSSEStreamResponse() *http.Response {
	sse := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_pool_sse","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[],"usage":{"input_tokens":12,"output_tokens":1}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi from anthropic"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"req-native-pool-sse"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}
}

// 池选中纯 OpenAI 族账号（国产 CC 固定）：Responses 入站委派 OpenAI 网关
// Responses 链（链内降级 Responses→CC 直转原生 CC 端点），响应以 Responses 格式
// 写回，usage 适配进既有 RecordUsage 流（token 计数与模型映射链不丢）。
func TestCompositePoolResponsesDelegatesPureOpenAIFamilyAccount(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolKimiAccount(42101, 41011, 0)})
	h.openAIUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return chatCompletionsOKResponse("kimi-k3", 11, 7)
	}

	c, rec := h.newRequest(t, "/v1/responses", poolResponsesBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.Responses(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "response", gjson.GetBytes(rec.Body.Bytes(), "object").String(),
		"委派链响应必须是 OpenAI Responses 格式")
	require.Equal(t, int64(11), gjson.GetBytes(rec.Body.Bytes(), "usage.input_tokens").Int())
	require.Equal(t, int64(7), gjson.GetBytes(rec.Body.Bytes(), "usage.output_tokens").Int())

	// 假上游收到的是 Responses→CC 降级转换后的 Chat Completions 请求（链内分流生效）。
	calls := h.openAIUpstream.snapshot()
	require.Len(t, calls, 1)
	require.True(t, strings.HasSuffix(calls[0].Path, "/chat/completions"), "upstream path=%s", calls[0].Path)
	require.Contains(t, string(calls[0].Body), `"model":"kimi-k3"`, "账号级 model_mapping 必须生效")
	require.Contains(t, calls[0].Authorization, "kimi-key")
	require.Empty(t, h.gwUpstream.snapshot(), "纯 OpenAI 族账号不得走 generic 转换链")

	// usage 适配记录已提交：token 计数与模型映射链进入 usage 行。
	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.Equal(t, 11, logs[0].InputTokens)
	require.Equal(t, 7, logs[0].OutputTokens)
	require.EqualValues(t, 42101, logs[0].AccountID)
	require.Equal(t, "my-model", logs[0].Model, "公开请求模型作为计费/展示模型记录")
	require.NotNil(t, logs[0].UpstreamModel)
	require.Equal(t, "kimi-k3", *logs[0].UpstreamModel)
}

// 池选中固定 responses 协议的国产账号（UsesNativeCNResponses）：Responses 入站
// 委派 OpenAI 链直连原生 Responses 端点（零转换）。
func TestCompositePoolResponsesDelegatesNativeCNResponsesAccount(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolKimiResponsesAccount(42151, 41011, 0)})
	h.openAIUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return openAIResponsesOKResponse("my-model", 13, 8)
	}

	c, rec := h.newRequest(t, "/v1/responses", poolResponsesBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.Responses(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(13), gjson.GetBytes(rec.Body.Bytes(), "usage.input_tokens").Int())
	require.Equal(t, int64(8), gjson.GetBytes(rec.Body.Bytes(), "usage.output_tokens").Int())

	calls := h.openAIUpstream.snapshot()
	require.Len(t, calls, 1)
	require.True(t, strings.HasSuffix(calls[0].Path, "/responses"), "upstream path=%s", calls[0].Path)
	require.False(t, strings.HasSuffix(calls[0].Path, "/chat/completions"), "原生 Responses 账号不得降级 CC")
	require.Empty(t, h.gwUpstream.snapshot())

	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.Equal(t, 13, logs[0].InputTokens)
	require.Equal(t, 8, logs[0].OutputTokens)
	require.EqualValues(t, 42151, logs[0].AccountID)
}

// 池选中 anthropic 原生账号：走既有 generic Responses→Anthropic 转换链（假上游
// 收到 {base}/v1/messages），不委派 OpenAI 链。
func TestCompositePoolResponsesAnthropicAccountUsesGenericConversion(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolAnthropicAccount(42201, 41011, 0)})
	h.gwUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return anthropicMessagesSSEStreamResponse()
	}

	c, rec := h.newRequest(t, "/v1/responses", poolResponsesBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.Responses(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(12), gjson.GetBytes(rec.Body.Bytes(), "usage.input_tokens").Int())
	require.Equal(t, int64(5), gjson.GetBytes(rec.Body.Bytes(), "usage.output_tokens").Int())

	calls := h.gwUpstream.snapshot()
	require.Len(t, calls, 1)
	require.True(t, strings.HasSuffix(calls[0].Path, "/v1/messages"), "upstream path=%s", calls[0].Path)
	require.Equal(t, "anthropic-key", calls[0].APIKeyHeader)
	require.Empty(t, h.openAIUpstream.snapshot(), "anthropic 账号不得委派 OpenAI 链")

	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.Equal(t, 12, logs[0].InputTokens)
	require.Equal(t, 5, logs[0].OutputTokens)
}

// Responses 委派链返回 failover 错误：走既有 failover 语义换号重选，改用
// anthropic 原生账号的 generic 转换链完成请求。
func TestCompositePoolResponsesDelegationFailoverSwitchesAccount(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{
		poolKimiAccount(42301, 41011, 0),
		poolAnthropicAccount(42302, 41011, 5),
	})
	h.gwUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return anthropicMessagesSSEStreamResponse()
	}
	// kimi 假上游默认回 500（respond 为 nil 时的兜底响应即 500）。

	c, rec := h.newRequest(t, "/v1/responses", poolResponsesBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.Responses(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(12), gjson.GetBytes(rec.Body.Bytes(), "usage.input_tokens").Int(),
		"failover 后应由 anthropic 账号的 generic 转换链完成请求")

	// 第一次委派失败（CC 上游 500），第二次走 generic 转换链。
	require.Len(t, h.openAIUpstream.snapshot(), 1)
	require.Len(t, h.gwUpstream.snapshot(), 1)
	require.True(t, strings.HasSuffix(h.gwUpstream.snapshot()[0].Path, "/v1/messages"))

	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.EqualValues(t, 42302, selected, "ops 记录最终选中的 anthropic 账号")

	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.EqualValues(t, 42302, logs[0].AccountID)
}

// 池选中纯 OpenAI 族账号（国产 CC 固定）：CC 入站委派 OpenAI 网关 CC 链，同族
// 零转换直转原生 CC 端点，响应与 usage 走既有链路。
func TestCompositePoolChatCompletionsDelegatesPureOpenAIFamilyAccount(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolKimiAccount(42401, 41011, 0)})
	h.openAIUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return chatCompletionsOKResponse("kimi-k3", 11, 7)
	}

	c, rec := h.newRequest(t, "/v1/chat/completions", poolChatCompletionsBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.ChatCompletions(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "hi from kimi-k3", gjson.GetBytes(rec.Body.Bytes(), "choices.0.message.content").String(),
		"同族 CC 入站零转换，响应为 Chat Completions 格式")
	require.Equal(t, int64(11), gjson.GetBytes(rec.Body.Bytes(), "usage.prompt_tokens").Int())

	calls := h.openAIUpstream.snapshot()
	require.Len(t, calls, 1)
	require.True(t, strings.HasSuffix(calls[0].Path, "/chat/completions"), "upstream path=%s", calls[0].Path)
	require.Contains(t, string(calls[0].Body), `"model":"kimi-k3"`, "账号级 model_mapping 必须生效")
	require.Contains(t, calls[0].Authorization, "kimi-key")
	require.Empty(t, h.gwUpstream.snapshot(), "纯 OpenAI 族账号不得走 generic 转换链")

	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.Equal(t, 11, logs[0].InputTokens)
	require.Equal(t, 7, logs[0].OutputTokens)
	require.EqualValues(t, 42401, logs[0].AccountID)
	require.Equal(t, "my-model", logs[0].Model)
	require.NotNil(t, logs[0].UpstreamModel)
	require.Equal(t, "kimi-k3", *logs[0].UpstreamModel)
}

// 池选中 adaptive 多协议账号：CC 入站由 OpenAI CC 链直转供应商原生 CC 端点
// （入站同族优先，零转换；与其在纯 OpenAI 族池中的行为一致），不走 CC→Anthropic
// 既有转换链。
func TestCompositePoolChatCompletionsAdaptiveAccountDelegatesNativeCC(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolAdaptiveKimiAccount(42451, 41011, 0)})
	h.openAIUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return chatCompletionsOKResponse("claude-sonnet-4-5", 9, 4)
	}

	c, rec := h.newRequest(t, "/v1/chat/completions", poolChatCompletionsBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.ChatCompletions(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(9), gjson.GetBytes(rec.Body.Bytes(), "usage.prompt_tokens").Int())

	calls := h.openAIUpstream.snapshot()
	require.Len(t, calls, 1)
	require.True(t, strings.HasSuffix(calls[0].Path, "/chat/completions"), "upstream path=%s", calls[0].Path)
	require.Empty(t, h.gwUpstream.snapshot(), "adaptive 账号 CC 入站走原生 CC，不经 generic 转换链")

	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.Equal(t, 9, logs[0].InputTokens)
	require.Equal(t, 4, logs[0].OutputTokens)
	require.EqualValues(t, 42451, logs[0].AccountID)
}

// 池选中 anthropic 原生账号：CC 入站走既有 generic CC→Anthropic 转换链（假上游
// 收到 {base}/v1/messages），不委派 OpenAI 链。
func TestCompositePoolChatCompletionsAnthropicAccountUsesGenericConversion(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolAnthropicAccount(42501, 41011, 0)})
	h.gwUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return anthropicMessagesSSEStreamResponse()
	}

	c, rec := h.newRequest(t, "/v1/chat/completions", poolChatCompletionsBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.ChatCompletions(c)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, int64(12), gjson.GetBytes(rec.Body.Bytes(), "usage.prompt_tokens").Int())

	calls := h.gwUpstream.snapshot()
	require.Len(t, calls, 1)
	require.True(t, strings.HasSuffix(calls[0].Path, "/v1/messages"), "upstream path=%s", calls[0].Path)
	require.Empty(t, h.openAIUpstream.snapshot(), "anthropic 账号不得委派 OpenAI 链")

	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.Equal(t, 12, logs[0].InputTokens)
	require.Equal(t, 5, logs[0].OutputTokens)
}

// CC 委派链返回 failover 错误：走既有 failover 语义换号重选，改用 anthropic
// 原生账号的 generic 转换链完成请求。
func TestCompositePoolChatCompletionsDelegationFailoverSwitchesAccount(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{
		poolKimiAccount(42601, 41011, 0),
		poolAnthropicAccount(42602, 41011, 5),
	})
	h.gwUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return anthropicMessagesSSEStreamResponse()
	}
	// kimi 假上游默认回 500（respond 为 nil 时的兜底响应即 500）。

	c, rec := h.newRequest(t, "/v1/chat/completions", poolChatCompletionsBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.ChatCompletions(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(12), gjson.GetBytes(rec.Body.Bytes(), "usage.prompt_tokens").Int(),
		"failover 后应由 anthropic 账号的 generic 转换链完成请求")

	require.Len(t, h.openAIUpstream.snapshot(), 1)
	require.Len(t, h.gwUpstream.snapshot(), 1)
	require.True(t, strings.HasSuffix(h.gwUpstream.snapshot()[0].Path, "/v1/messages"))

	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.EqualValues(t, 42602, selected, "ops 记录最终选中的 anthropic 账号")

	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.EqualValues(t, 42602, logs[0].AccountID)
}

// B2 端点委派分发谓词与结果适配器（图片计费字段）的纯函数契约。
func TestCompositePoolTextEndpointDelegationPredicates(t *testing.T) {
	// Responses 入站：纯 OpenAI 族与原生 Responses 账号 → 委派。
	require.True(t, compositePoolAccountDelegatesResponsesToOpenAI(poolKimiAccount(1, 1, 0)))
	require.True(t, compositePoolAccountDelegatesResponsesToOpenAI(&service.Account{Platform: service.PlatformOpenAI}))
	require.True(t, compositePoolAccountDelegatesResponsesToOpenAI(&service.Account{Platform: service.PlatformGrok}))
	require.True(t, compositePoolAccountDelegatesResponsesToOpenAI(poolKimiResponsesAccount(2, 1, 0)))
	// 有原生 Responses 端点的 adaptive 账号同样委派（入站同族零转换）。
	require.True(t, compositePoolAccountDelegatesResponsesToOpenAI(poolAdaptiveKimiAccount(3, 1, 0)))

	// anthropic 原生 / gemini / antigravity / 无原生 Responses 的 adaptive → 既有转换。
	require.False(t, compositePoolAccountDelegatesResponsesToOpenAI(poolAnthropicAccount(4, 1, 0)))
	require.False(t, compositePoolAccountDelegatesResponsesToOpenAI(&service.Account{Platform: service.PlatformGemini}))
	require.False(t, compositePoolAccountDelegatesResponsesToOpenAI(&service.Account{Platform: service.PlatformAntigravity}))
	require.False(t, compositePoolAccountDelegatesResponsesToOpenAI(poolZhipuAdaptiveAccount(5, 1, 0)))
	require.False(t, compositePoolAccountDelegatesResponsesToOpenAI(nil))

	// CC 入站：OpenAI 兼容族（含 adaptive，链内直转原生 CC）→ 委派。
	require.True(t, compositePoolAccountDelegatesChatCompletionsToOpenAI(poolKimiAccount(6, 1, 0)))
	require.True(t, compositePoolAccountDelegatesChatCompletionsToOpenAI(poolAdaptiveKimiAccount(7, 1, 0)))
	require.True(t, compositePoolAccountDelegatesChatCompletionsToOpenAI(poolZhipuAdaptiveAccount(8, 1, 0)))
	require.True(t, compositePoolAccountDelegatesChatCompletionsToOpenAI(&service.Account{Platform: service.PlatformGrok}))

	// anthropic-协议 / gemini / antigravity → 既有转换。
	require.False(t, compositePoolAccountDelegatesChatCompletionsToOpenAI(poolAnthropicAccount(9, 1, 0)))
	require.False(t, compositePoolAccountDelegatesChatCompletionsToOpenAI(&service.Account{Platform: service.PlatformGemini}))
	require.False(t, compositePoolAccountDelegatesChatCompletionsToOpenAI(&service.Account{Platform: service.PlatformAntigravity}))
	require.False(t, compositePoolAccountDelegatesChatCompletionsToOpenAI(nil))

	// 结果适配：图片/搜索计费字段逐字段映射，nil 安全。
	require.Nil(t, adaptOpenAIForwardResultToForwardResult(nil))
	adapted := adaptOpenAIForwardResultToForwardResult(&service.OpenAIForwardResult{
		ImageCount:         2,
		ImageSize:          "2K",
		ImageInputSize:     "1K",
		ImageOutputSize:    "2K",
		ImageOutputSizes:   []string{"2K", "2K"},
		ImageSizeSource:    "response",
		ImageSizeBreakdown: map[string]int{"2K": 2},
		SearchCount:        3,
	})
	require.Equal(t, 2, adapted.ImageCount)
	require.Equal(t, "2K", adapted.ImageSize)
	require.Equal(t, "1K", adapted.ImageInputSize)
	require.Equal(t, "2K", adapted.ImageOutputSize)
	require.Equal(t, []string{"2K", "2K"}, adapted.ImageOutputSizes)
	require.Equal(t, "response", adapted.ImageSizeSource)
	require.Equal(t, map[string]int{"2K": 2}, adapted.ImageSizeBreakdown)
	require.Equal(t, 3, adapted.SearchCount)
}

// openAIResponsesOKResponse 返回一份带 usage 的非流式 Responses 成功响应。
func openAIResponsesOKResponse(model string, inputTokens, outputTokens int) *http.Response {
	body := fmt.Sprintf(
		`{"id":"resp_pool","object":"response","created_at":1,"status":"completed","model":%q,"output":[],"usage":{"input_tokens":%d,"output_tokens":%d}}`,
		model, inputTokens, outputTokens)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"req-resp-pool"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
