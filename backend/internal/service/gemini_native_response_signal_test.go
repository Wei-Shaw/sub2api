//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Gemini 原生路径带内信号契约：
//   - 上游 2xx 里的 finishReason / promptFeedback.blockReason / 错误信封 / 空流只登记 ops，
//     wire 状态与客户端字节原样不变，用量照常从 usageMetadata 提取（计费不受影响）；
//   - 内容策略类不归因上游账号、不计入 SLA；错误信封与空流按上游失败归因并计入 SLA。
// ---------------------------------------------------------------------------

const geminiSignalTestFinishMessage = "The model output could not be generated. This output contains sensitive words that violate Google's [Generative AI Prohibited Use policy](https://policies.google.com/terms/generative-ai/use-policy). If you think this was an error, [send feedback](https://ai.google.dev/gemini-api/docs/troubleshooting)."

const geminiSignalTestProhibitedSSE = `data: {"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"}}],"modelVersion":"gemini-3.7-flash","responseId":"r1","usageMetadata":{"candidatesTokenCount":26,"promptTokenCount":21775,"thoughtsTokenCount":606,"totalTokenCount":22407}}

data: {"candidates":[{"content":{"parts":[{"text":" world"}],"role":"model"}}],"modelVersion":"gemini-3.7-flash","responseId":"r1","usageMetadata":{"candidatesTokenCount":1289,"promptTokenCount":21775,"thoughtsTokenCount":606,"totalTokenCount":23670}}

data: {"candidates":[{"content":{},"finishMessage":"` + geminiSignalTestFinishMessage + `","finishReason":"PROHIBITED_CONTENT"}],"modelVersion":"gemini-3.7-flash","responseId":"r1","usageMetadata":{"candidatesTokenCount":1289,"promptTokenCount":21775,"thoughtsTokenCount":606,"totalTokenCount":23670}}

`

const geminiSignalTestStopSSE = `data: {"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"}}],"usageMetadata":{"candidatesTokenCount":3,"promptTokenCount":10,"totalTokenCount":13}}

data: {"candidates":[{"content":{"parts":[{"text":"!"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"candidatesTokenCount":4,"promptTokenCount":10,"totalTokenCount":14}}

`

func geminiSignalTestAccount() *Account {
	return &Account{
		ID:       703,
		Name:     "gemini-signal-test",
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "test-key",
		},
	}
}

func newGeminiSignalService(contentType string, body string) *GeminiMessagesCompatService {
	httpStub := &geminiCompatHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{"upstream-req-1"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}
	return &GeminiMessagesCompatService{
		httpUpstream:     httpStub,
		cfg:              &config.Config{},
		rateLimitService: NewRateLimitService(&errorPolicyRepoStub{}, nil, &config.Config{}, nil, nil),
	}
}

func geminiSignalTestRequest() []byte {
	return []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
}

func upstreamErrorEventsFromContext(t *testing.T, c *gin.Context) []*OpsUpstreamErrorEvent {
	t.Helper()
	v, ok := c.Get(OpsUpstreamErrorsKey)
	if !ok {
		return nil
	}
	events, ok := v.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	return events
}

func TestGeminiForwardNative_StreamProhibitedContentMarksInBandErrorAndKeepsBillingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newGeminiSignalService("text/event-stream; charset=utf-8", geminiSignalTestProhibitedSSE)
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
	require.NoError(t, err)
	require.NotNil(t, result)

	// 客户端字节原样透传，wire 状态 200。
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, geminiSignalTestProhibitedSSE, rec.Body.String())

	// 用量照常提取：input=promptTokenCount，output=candidates+thoughts。
	require.Equal(t, 21775, result.Usage.InputTokens)
	require.Equal(t, 1289+606, result.Usage.OutputTokens)
	require.NotNil(t, result.FirstTokenMs)

	// 带内错误登记为请求级内容策略：不计 SLA、不归因上游账号。
	streamErrs := GetOpsStreamErrors(c)
	require.Len(t, streamErrs, 1)
	require.Equal(t, "invalid_request_error", streamErrs[0].ErrType)
	require.Equal(t, "PROHIBITED_CONTENT", streamErrs[0].Code)
	require.Equal(t, http.StatusBadRequest, streamErrs[0].IntendedStatus)
	require.False(t, streamErrs[0].CountTowardsSLA)
	require.True(t, streamErrs[0].RequestScoped)
	require.False(t, streamErrs[0].NonStream)
	require.Contains(t, streamErrs[0].Message, "finishReason=PROHIBITED_CONTENT")
	require.Contains(t, streamErrs[0].Message, "Prohibited Use policy")

	_, hasUpstreamStatus := c.Get(OpsUpstreamStatusCodeKey)
	require.False(t, hasUpstreamStatus, "内容策略不应写上游错误上下文")
	require.Empty(t, upstreamErrorEventsFromContext(t, c))
}

func TestGeminiForwardNative_StreamErrorEnvelopeMarksUpstreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `data: {"candidates":[{"content":{"parts":[{"text":"partial"}],"role":"model"}}],"usageMetadata":{"candidatesTokenCount":2,"promptTokenCount":10,"totalTokenCount":12}}

data: {"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}

`
	svc := newGeminiSignalService("text/event-stream", body)
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, body, rec.Body.String(), "错误信封也原样透传，交给 SDK 客户端按错误处理")
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)

	streamErrs := GetOpsStreamErrors(c)
	require.Len(t, streamErrs, 1)
	require.Equal(t, "rate_limit_error", streamErrs[0].ErrType)
	require.Equal(t, "RESOURCE_EXHAUSTED", streamErrs[0].Code)
	require.Equal(t, http.StatusTooManyRequests, streamErrs[0].IntendedStatus)
	require.True(t, streamErrs[0].CountTowardsSLA)
	require.False(t, streamErrs[0].RequestScoped)
	require.False(t, streamErrs[0].NonStream)
	require.Equal(t, "Resource has been exhausted (e.g. check quota).", streamErrs[0].Message)

	status, ok := c.Get(OpsUpstreamStatusCodeKey)
	require.True(t, ok)
	require.Equal(t, http.StatusTooManyRequests, status)

	events := upstreamErrorEventsFromContext(t, c)
	require.Len(t, events, 1)
	require.Equal(t, "stream_failed", events[0].Kind)
	require.Equal(t, int64(703), events[0].AccountID)
	require.Equal(t, PlatformGemini, events[0].Platform)
	require.Equal(t, http.StatusTooManyRequests, events[0].UpstreamStatusCode)
	require.Equal(t, "upstream-req-1", events[0].UpstreamRequestID)
}

func TestGeminiForwardNative_StreamPromptBlockedMarksContentPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `data: {"promptFeedback":{"blockReason":"SAFETY","safetyRatings":[{"category":"HARM_CATEGORY_HARASSMENT","probability":"HIGH","blocked":true}]},"usageMetadata":{"promptTokenCount":10,"totalTokenCount":10}}

`
	svc := newGeminiSignalService("text/event-stream", body)
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
	require.NoError(t, err)
	require.Equal(t, body, rec.Body.String())
	require.Equal(t, 10, result.Usage.InputTokens)

	streamErrs := GetOpsStreamErrors(c)
	require.Len(t, streamErrs, 1)
	require.Equal(t, "invalid_request_error", streamErrs[0].ErrType)
	require.Equal(t, "SAFETY", streamErrs[0].Code)
	require.Contains(t, streamErrs[0].Message, "blockReason=SAFETY")
	require.False(t, streamErrs[0].CountTowardsSLA)
	require.Empty(t, upstreamErrorEventsFromContext(t, c))
}

func TestGeminiForwardNative_EmptyStreamMarksUpstreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newGeminiSignalService("text/event-stream", "")
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Zero(t, rec.Body.Len())
	require.Nil(t, result.FirstTokenMs)

	streamErrs := GetOpsStreamErrors(c)
	require.Len(t, streamErrs, 1)
	require.Equal(t, "upstream_error", streamErrs[0].ErrType)
	require.Equal(t, geminiSignalEmptyStreamReason, streamErrs[0].Code)
	require.Equal(t, http.StatusBadGateway, streamErrs[0].IntendedStatus)
	require.True(t, streamErrs[0].CountTowardsSLA)

	events := upstreamErrorEventsFromContext(t, c)
	require.Len(t, events, 1)
	require.Equal(t, "stream_failed", events[0].Kind)
	require.Equal(t, int64(703), events[0].AccountID)
}

func TestGeminiForwardNative_NormalStreamLeavesNoMark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newGeminiSignalService("text/event-stream", geminiSignalTestStopSSE)
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
	require.NoError(t, err)
	require.Equal(t, geminiSignalTestStopSSE, rec.Body.String())
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Empty(t, GetOpsStreamErrors(c))
	require.Empty(t, upstreamErrorEventsFromContext(t, c))
	_, hasUpstreamStatus := c.Get(OpsUpstreamStatusCodeKey)
	require.False(t, hasUpstreamStatus)
}

func TestGeminiForwardNative_NonStreamProhibitedContentMarksInBandError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"candidates":[{"content":{},"finishMessage":"` + geminiSignalTestFinishMessage + `","finishReason":"PROHIBITED_CONTENT"}],"modelVersion":"gemini-3.7-flash","usageMetadata":{"candidatesTokenCount":120,"promptTokenCount":900,"thoughtsTokenCount":30,"totalTokenCount":1050}}`
	svc := newGeminiSignalService("application/json", body)
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "generateContent", false, geminiSignalTestRequest())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, body, rec.Body.String())
	require.Equal(t, 900, result.Usage.InputTokens)
	require.Equal(t, 150, result.Usage.OutputTokens)

	streamErrs := GetOpsStreamErrors(c)
	require.Len(t, streamErrs, 1)
	require.Equal(t, "invalid_request_error", streamErrs[0].ErrType)
	require.Equal(t, "PROHIBITED_CONTENT", streamErrs[0].Code)
	require.False(t, streamErrs[0].CountTowardsSLA)
	require.True(t, streamErrs[0].RequestScoped)
	require.True(t, streamErrs[0].NonStream)
	require.Empty(t, upstreamErrorEventsFromContext(t, c))
}

func TestGeminiForwardNative_NonStreamErrorEnvelopeOn200MarksUpstreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"error":{"code":500,"message":"Internal error encountered.","status":"INTERNAL"}}`
	svc := newGeminiSignalService("application/json", body)
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "generateContent", false, geminiSignalTestRequest())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, body, rec.Body.String())

	streamErrs := GetOpsStreamErrors(c)
	require.Len(t, streamErrs, 1)
	require.Equal(t, "upstream_error", streamErrs[0].ErrType)
	require.Equal(t, "INTERNAL", streamErrs[0].Code)
	require.Equal(t, http.StatusInternalServerError, streamErrs[0].IntendedStatus)
	require.True(t, streamErrs[0].CountTowardsSLA)

	require.True(t, streamErrs[0].NonStream)

	events := upstreamErrorEventsFromContext(t, c)
	require.Len(t, events, 1)
	require.Equal(t, "http_error", events[0].Kind)
}

func TestGeminiForwardNative_StreamErrorEnvelopeAfterContentFilterWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `data: {"candidates":[{"content":{},"finishReason":"SAFETY"}]}

data: {"error":{"code":503,"message":"The model is overloaded. Please try again later.","status":"UNAVAILABLE"}}

`
	svc := newGeminiSignalService("text/event-stream", body)
	c, rec := newGeminiNativeTestContext(t)

	_, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
	require.NoError(t, err)
	require.Equal(t, body, rec.Body.String())

	streamErrs := GetOpsStreamErrors(c)
	require.Len(t, streamErrs, 1)
	require.Equal(t, "UNAVAILABLE", streamErrs[0].Code, "错误信封优先于内容过滤")
	require.Equal(t, "upstream_error", streamErrs[0].ErrType)
	require.Equal(t, http.StatusServiceUnavailable, streamErrs[0].IntendedStatus)
	require.True(t, streamErrs[0].CountTowardsSLA)
	require.Len(t, upstreamErrorEventsFromContext(t, c), 1)
}

func TestGeminiForwardNative_StreamNonSSEJSONBodyIsInspected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("json array with content filter", func(t *testing.T) {
		body := `[{
  "candidates": [{"content": {"parts": [{"text": "hello"}], "role": "model"}}],
  "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 1}
}
,
{
  "candidates": [{"content": {}, "finishReason": "PROHIBITED_CONTENT"}]
}
]
`
		svc := newGeminiSignalService("application/json", body)
		c, rec := newGeminiNativeTestContext(t)
		_, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
			"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
		require.NoError(t, err)
		require.Equal(t, body, rec.Body.String())
		streamErrs := GetOpsStreamErrors(c)
		require.Len(t, streamErrs, 1)
		require.Equal(t, "PROHIBITED_CONTENT", streamErrs[0].Code)
		require.True(t, streamErrs[0].RequestScoped)
	})
	t.Run("json object error envelope", func(t *testing.T) {
		body := `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}` + "\n"
		svc := newGeminiSignalService("application/json", body)
		c, _ := newGeminiNativeTestContext(t)
		_, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
			"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
		require.NoError(t, err)
		streamErrs := GetOpsStreamErrors(c)
		require.Len(t, streamErrs, 1)
		require.Equal(t, "RESOURCE_EXHAUSTED", streamErrs[0].Code)
		require.True(t, streamErrs[0].CountTowardsSLA)
	})
	t.Run("healthy json object leaves no mark", func(t *testing.T) {
		body := `{"candidates":[{"content":{"parts":[{"text":"hello"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1}}` + "\n"
		svc := newGeminiSignalService("application/json", body)
		c, _ := newGeminiNativeTestContext(t)
		_, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
			"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
		require.NoError(t, err)
		require.Empty(t, GetOpsStreamErrors(c))
		require.Empty(t, upstreamErrorEventsFromContext(t, c))
	})
}

func TestGeminiForwardNative_StreamOversizedNonSSEBodyLeavesNoMark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	filler := strings.Repeat("x", geminiSSEFallbackBodyLimit)
	body := `{"candidates":[{"content":{"parts":[{"text":"` + filler + `"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1}}` + "\n"
	svc := newGeminiSignalService("application/json", body)
	c, rec := newGeminiNativeTestContext(t)
	_, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
	require.NoError(t, err)
	require.Equal(t, len(body), rec.Body.Len())
	require.Empty(t, GetOpsStreamErrors(c), "超限的兜底体放弃判定，不得记成空流")
	require.Empty(t, upstreamErrorEventsFromContext(t, c))
}

func TestGeminiForwardNative_StreamNonSSEEmptyBodyIsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, body := range map[string]string{
		"empty object":     "{}\n",
		"empty candidates": `{"candidates":[],"usageMetadata":{"promptTokenCount":3}}` + "\n",
		"empty array":      "[]\n",
		"array of empties": "[{\"candidates\":[]}\n,\n{}]\n",
	} {
		t.Run(name, func(t *testing.T) {
			svc := newGeminiSignalService("application/json", body)
			c, rec := newGeminiNativeTestContext(t)
			_, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
				"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
			require.NoError(t, err)
			require.Equal(t, body, rec.Body.String())
			streamErrs := GetOpsStreamErrors(c)
			require.Len(t, streamErrs, 1)
			require.Equal(t, geminiSignalEmptyStreamReason, streamErrs[0].Code)
			require.True(t, streamErrs[0].CountTowardsSLA)
		})
	}
}

func TestGeminiForwardNative_StreamWithoutDataEventsIsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, body := range map[string]string{
		"keepalive comments only": ": keepalive\n\n: keepalive\n\n",
		"done marker only":        "data: [DONE]\n\n",
		"blank data only":         "data: \n\n",
		"whitespace only":         "\n\n   \n",
	} {
		t.Run(name, func(t *testing.T) {
			svc := newGeminiSignalService("text/event-stream", body)
			c, rec := newGeminiNativeTestContext(t)
			_, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
				"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
			require.NoError(t, err)
			require.Equal(t, body, rec.Body.String())
			streamErrs := GetOpsStreamErrors(c)
			require.Len(t, streamErrs, 1)
			require.Equal(t, geminiSignalEmptyStreamReason, streamErrs[0].Code)
			require.True(t, streamErrs[0].CountTowardsSLA)
			require.False(t, streamErrs[0].NonStream)
		})
	}
}

func TestGeminiForwardNative_StreamOtherFinishReasonLeavesNoMark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `data: {"candidates":[{"content":{"parts":[{"text":"full answer"}],"role":"model"},"finishReason":"OTHER"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2}}

`
	svc := newGeminiSignalService("text/event-stream", body)
	c, _ := newGeminiNativeTestContext(t)
	result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
	require.NoError(t, err)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Empty(t, GetOpsStreamErrors(c))
	require.Empty(t, upstreamErrorEventsFromContext(t, c))
}

func TestGeminiForwardNative_StreamMalformedFunctionCallLeavesNoMark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `data: {"candidates":[{"content":{"parts":[{"text":"partial"}],"role":"model"},"finishReason":"MALFORMED_FUNCTION_CALL"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2}}

`
	svc := newGeminiSignalService("text/event-stream", body)
	c, _ := newGeminiNativeTestContext(t)
	result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "streamGenerateContent", true, geminiSignalTestRequest())
	require.NoError(t, err)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Empty(t, GetOpsStreamErrors(c))
	require.Empty(t, upstreamErrorEventsFromContext(t, c))
}

func TestGeminiForwardNative_NonStreamEmptyBodyMarksEmptyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, body := range map[string]string{
		"blank":            "",
		"empty object":     "{}",
		"empty candidates": `{"candidates":[],"usageMetadata":{"promptTokenCount":3,"totalTokenCount":3}}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc := newGeminiSignalService("application/json", body)
			c, rec := newGeminiNativeTestContext(t)
			result, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
				"gemini-3.7-flash", "generateContent", false, geminiSignalTestRequest())
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, body, rec.Body.String())
			streamErrs := GetOpsStreamErrors(c)
			require.Len(t, streamErrs, 1)
			require.Equal(t, geminiSignalEmptyResponseReason, streamErrs[0].Code)
			require.Equal(t, "upstream_error", streamErrs[0].ErrType)
			require.True(t, streamErrs[0].CountTowardsSLA)
			require.True(t, streamErrs[0].NonStream)
			events := upstreamErrorEventsFromContext(t, c)
			require.Len(t, events, 1)
			require.Equal(t, "http_error", events[0].Kind)
		})
	}
}

func TestGeminiForwardNative_CountTokensBodyLeavesNoMark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newGeminiSignalService("application/json", `{"totalTokens":12}`)
	c, rec := newGeminiNativeTestContext(t)
	_, err := svc.ForwardNative(context.Background(), c, geminiSignalTestAccount(),
		"gemini-3.7-flash", "countTokens", false, geminiSignalTestRequest())
	require.NoError(t, err)
	require.Equal(t, `{"totalTokens":12}`, rec.Body.String())
	require.Empty(t, GetOpsStreamErrors(c))
	require.Empty(t, upstreamErrorEventsFromContext(t, c))
}

func TestCollectGeminiSSEObserved_SeesTerminalChunkEvenWhenAggregateDropsIt(t *testing.T) {
	var observed []string
	collected, usage, stats, err := collectGeminiSSEObserved(strings.NewReader(geminiSignalTestProhibitedSSE), false, func(raw []byte) {
		observed = append(observed, string(raw))
	})
	require.NoError(t, err)
	require.Len(t, observed, 3)
	require.Equal(t, 3, stats.dataEvents)
	require.Empty(t, stats.fallback.Bytes())
	require.Equal(t, 21775, usage.InputTokens)

	var hit bool
	for _, raw := range observed {
		if sig, ok := detectGeminiResponseSignal([]byte(raw)); ok {
			hit = true
			require.Equal(t, geminiSignalContentFilter, sig.Kind)
		}
	}
	require.True(t, hit)
	// 聚合结果沿用"最后一个带 parts 的块"，本身不携带末块的 finishReason；检测必须逐事件进行。
	aggregated, err := json.Marshal(collected)
	require.NoError(t, err)
	require.NotContains(t, string(aggregated), "PROHIBITED_CONTENT")
}
