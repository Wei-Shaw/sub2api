package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression for the production incident on 2026-05-24 around 9:13 CST:
// user 16 sent /v1/responses with stream:true via Codex CLI; the user-concurrency
// slot wait sent SSE ping comments (flushing HTTP 200 + headers), then the 30s
// timeout fired and the handler emitted `event: error\ndata: {...}`. Codex CLI
// does not recognize that as a Responses terminal event and reports
// "stream closed before response.completed". The fix is to emit a synthetic
// response.failed event when the inbound endpoint is /v1/responses.

func newGinContextForEndpoint(t *testing.T, endpoint string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, endpoint, nil)
	return c, w
}

// parseResponsesFailedSSE 抽出 SSE 中 data 行的 JSON，返回 (response 对象, error 对象)。
func parseResponsesFailedSSE(t *testing.T, body string) (map[string]any, map[string]any) {
	t.Helper()
	require.True(t, strings.HasPrefix(body, "event: response.failed\n"),
		"expect event: response.failed prefix, got: %q", body)
	require.True(t, strings.HasSuffix(body, "\n\n"))

	lines := strings.SplitN(strings.TrimSuffix(body, "\n\n"), "\n", 2)
	require.Len(t, lines, 2)
	require.True(t, strings.HasPrefix(lines[1], "data: "))
	jsonStr := strings.TrimPrefix(lines[1], "data: ")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &parsed), "data must be valid JSON: %s", jsonStr)

	assert.Equal(t, "response.failed", parsed["type"])
	// 故意不发 sequence_number，避免与后续真实事件的序号冲突。
	_, hasSeq := parsed["sequence_number"]
	assert.False(t, hasSeq, "synthetic event must not emit sequence_number")

	resp, ok := parsed["response"].(map[string]any)
	require.True(t, ok, "response object missing")
	assert.Equal(t, "response", resp["object"])
	assert.Equal(t, "failed", resp["status"])

	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok, "error object missing")

	return resp, errObj
}

// OpenAI handler: /v1/responses streaming, after stream started, must emit response.failed.
func TestOpenAIHandleStreamingAwareError_ResponsesStreamingEmitsResponseFailed(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointResponses)
	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error",
		"Concurrency limit exceeded for user, please retry later", true)

	resp, errObj := parseResponsesFailedSSE(t, w.Body.String())

	id, _ := resp["id"].(string)
	assert.True(t, strings.HasPrefix(id, "resp_"), "id should start with resp_, got %q", id)
	assert.Equal(t, "rate_limit_exceeded", errObj["code"])
	assert.Equal(t, "Concurrency limit exceeded for user, please retry later", errObj["message"])
}

// 当 setOpsRequestContext 写过 model，合成事件应回填该字段（与 codebase 已有 makeResponsesCompletedEvent 对齐）。
func TestOpenAIHandleStreamingAwareError_ResponsesStreamingIncludesModel(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointResponses)
	setOpsRequestContext(c, "gpt-5.5", true)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "boom", true)

	resp, _ := parseResponsesFailedSSE(t, w.Body.String())
	assert.Equal(t, "gpt-5.5", resp["model"])
}

// 没有 model 时 model 字段不应出现（避免发空字符串污染下游解析）。
func TestOpenAIHandleStreamingAwareError_ResponsesStreamingOmitsEmptyModel(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointResponses)
	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "boom", true)

	resp, _ := parseResponsesFailedSSE(t, w.Body.String())
	_, hasModel := resp["model"]
	assert.False(t, hasModel, "model field must be omitted when unknown")
}

// 当 request.Context 携带 ctxkey.RequestID 时，合成 id 应与之关联，便于和 server log 串起来。
func TestOpenAIHandleStreamingAwareError_ResponsesStreamingReusesRequestID(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointResponses)
	c.Request = c.Request.WithContext(
		context.WithValue(c.Request.Context(), ctxkey.RequestID, "fd277bc5-ff7e-45d1-8aa9-f54e1df318f1"),
	)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "x", true)

	resp, _ := parseResponsesFailedSSE(t, w.Body.String())
	assert.Equal(t, "resp_fd277bc5ff7e45d18aa9f54e1df318f1", resp["id"])
}

// 与旧分支的 TestOpenAIHandleStreamingAwareError_JSONEscaping 对齐：
// 新的 response.failed payload 也必须正确转义 message 里的特殊字符，
// 否则下游 SDK 解析 JSON 时会失败。
func TestOpenAIHandleStreamingAwareError_ResponsesStreamingJSONEscaping(t *testing.T) {
	cases := []struct {
		name    string
		errType string
		message string
	}{
		{"双引号", "server_error", `upstream returned "invalid" response`},
		{"反斜杠", "server_error", `path C:\Users\test\file.txt not found`},
		{"双引号+反斜杠", "upstream_error", `error parsing "key\value": unexpected token`},
		{"换行与制表", "server_error", "line1\nline2\ttab"},
		{"普通", "upstream_error", "Upstream service temporarily unavailable"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newGinContextForEndpoint(t, EndpointResponses)
			h := &OpenAIGatewayHandler{}
			h.handleStreamingAwareError(c, http.StatusBadGateway, tc.errType, tc.message, true)

			_, errObj := parseResponsesFailedSSE(t, w.Body.String())
			assert.Equal(t, tc.message, errObj["message"], "message 必须被原样还原")
		})
	}
}

// OpenAI handler: /v1/chat/completions streaming keeps the legacy event: error format
// (out of scope for this fix; covered to prevent regression of unrelated paths).
func TestOpenAIHandleStreamingAwareError_ChatCompletionsStreamingKeepsLegacy(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointChatCompletions)
	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "boom", true)

	body := w.Body.String()
	assert.True(t, strings.HasPrefix(body, "event: error\n"), "got: %q", body)
}

// Gateway (Anthropic-backed) handler: /v1/responses path also must emit response.failed.
func TestGatewayHandleStreamingAwareError_ResponsesStreamingEmitsResponseFailed(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointResponses)
	h := &GatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "upstream gone", true)

	_, errObj := parseResponsesFailedSSE(t, w.Body.String())
	assert.Equal(t, "upstream_error", errObj["code"])
	assert.Equal(t, "upstream gone", errObj["message"])
}

// Gateway handler: /v1/messages preserves the legacy data:{type:error,...} format
// (Anthropic spec accepts a type:"error" stream event).
func TestGatewayHandleStreamingAwareError_MessagesStreamingKeepsLegacy(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointMessages)
	h := &GatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "boom", true)

	body := w.Body.String()
	assert.True(t, strings.HasPrefix(body, `data: {"type":"error"`), "got: %q", body)
}

// 项目里 /responses 注册在多组路由：/v1/responses（gateway）、裸 /responses（top-level）、
// /backend-api/codex/responses（codex direct）。我们 fix 必须覆盖全部，
// 否则一些客户端走的路径就不会发 response.failed，照样报 stream closed。
// 这是生产 2026-05-24 ~11:05 UTC user 16 实际命中的 bug。
func TestInboundIsResponses_CoversAllRoutes(t *testing.T) {
	cases := []struct {
		route string
		want  bool
	}{
		{"/v1/responses", true},
		{"/v1/responses/compact", true},
		{"/responses", true}, // <-- 用户 16 实际走这条
		{"/responses/compact", true},
		{"/backend-api/codex/responses", true},
		{"/backend-api/codex/responses/compact", true},
		{"/v1/chat/completions", false},
		{"/v1/messages", false},
		{"/", false},
		{"/responses-fake", false},
	}
	for _, tc := range cases {
		t.Run(tc.route, func(t *testing.T) {
			c, _ := newGinContextForEndpoint(t, tc.route)
			assert.Equal(t, tc.want, inboundIsResponses(c), "route=%q", tc.route)
		})
	}
}

// 用 c.Request.URL.Path 作为 fallback（当 c.FullPath() 为空时，例如某些测试 fixture）。
func TestInboundIsResponses_FallsBackToURLPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	// 这种情况下 c.FullPath() 是 ""，必须 fallback 到 URL.Path
	assert.True(t, inboundIsResponses(c), "URL.Path fallback must work when FullPath is empty")
}

// 回归生产事故：用户 16 走 /responses 路径，必须发 response.failed。
func TestOpenAIHandleStreamingAwareError_BareResponsesRouteEmitsResponseFailed(t *testing.T) {
	c, w := newGinContextForEndpoint(t, "/responses")
	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error",
		"Concurrency limit exceeded for user, please retry later", true)

	resp, errObj := parseResponsesFailedSSE(t, w.Body.String())
	id, _ := resp["id"].(string)
	assert.True(t, strings.HasPrefix(id, "resp_"))
	assert.Equal(t, "rate_limit_exceeded", errObj["code"])
}

// Synthesized response.failed id falls back to uuid when no request_id is present.
func TestSynthesizeResponseID_FallbackUUID(t *testing.T) {
	c, _ := newGinContextForEndpoint(t, EndpointResponses)
	id := synthesizeResponseID(c)
	assert.True(t, strings.HasPrefix(id, "resp_"))
	// uuid 去掉短横线后 32 hex 字符；前缀 "resp_" 共 37。
	assert.Len(t, id, 37)
}

func TestMapResponsesErrorCode(t *testing.T) {
	cases := []struct{ in, out string }{
		{"rate_limit_error", "rate_limit_exceeded"},
		{"invalid_request_error", "invalid_request"},
		{"permission_error", "permission_denied"},
		{"authentication_error", "authentication_failed"},
		{"upstream_error", "upstream_error"},
		{"server_error", "server_error"},
		{"api_error", "server_error"},
		{"", "server_error"},
		{"custom_thing", "custom_thing"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.out, mapResponsesErrorCode(tc.in), "in=%q", tc.in)
	}
}
