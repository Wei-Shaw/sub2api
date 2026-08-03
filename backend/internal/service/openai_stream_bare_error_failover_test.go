//go:build unit

package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// bareErrorOverloadedSSE 复刻线上样例：response.created / response.in_progress 之后
// 上游先发一个裸 error 帧，再发 response.failed，两者携带同一个 server_is_overloaded。
func bareErrorOverloadedSSE() string {
	return strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_ovl","status":"in_progress"}}`,
		"",
		`data: {"type":"response.in_progress","response":{"id":"resp_ovl","status":"in_progress"}}`,
		"",
		`data: {"type":"error","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`,
		"",
		`data: {"type":"response.failed","response":{"id":"resp_ovl","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`,
		"",
	}, "\n")
}

// largeBareErrorOverloadedSSE 复刻 Codex 线上样例：response.created 会把整段
// instructions 与 tools schema 回显回来，单帧远超下游 4KB 写缓冲。
func largeBareErrorOverloadedSSE() string {
	instructions := strings.Repeat("You are Codex, a coding agent based on GPT-5. ", 1024)
	return strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_ovl","status":"in_progress","instructions":"` + instructions + `"}}`,
		"",
		`data: {"type":"response.in_progress","response":{"id":"resp_ovl","status":"in_progress","instructions":"` + instructions + `"}}`,
		"",
		`data: {"type":"error","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`,
		"",
		`data: {"type":"response.failed","response":{"id":"resp_ovl","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`,
		"",
	}, "\n")
}

func newBareErrorTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize: defaultMaxLineSize,
	}}}
}

func newBareErrorTestContext(t *testing.T, path string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c, rec
}

func TestOpenAINativeStreamBareErrorTriggersFailover(t *testing.T) {
	svc := newBareErrorTestService()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(bareErrorOverloadedSSE())),
	}
	c, rec := newBareErrorTestContext(t, "/v1/responses")

	_, err := svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String(), "裸 error 帧不得写给客户端，否则提交 200 并堵死 failover")
}

// 大 preamble 会撑破下游 4KB 写缓冲直接写穿到 socket，提交 HTTP 200 并让
// openAIStreamClientOutputStarted 变真，从而堵死流内错误的 failover 前置门槛。
func TestOpenAINativeStreamLargePreambleBareErrorTriggersFailover(t *testing.T) {
	svc := newBareErrorTestService()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(largeBareErrorOverloadedSSE())),
	}
	c, rec := newDelayedStatusCodeTestContext(t, &Group{DelayedStatusCode: true})

	_, err := svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String(), "首个语义输出前不得写出任何字节，否则 200 已提交")
}

// 首个语义输出到来时，暂存的 preamble 必须按原顺序一并交付，不能被吞掉。
func TestOpenAINativeStreamLargePreambleReleasedOnFirstOutput(t *testing.T) {
	svc := newBareErrorTestService()
	instructions := strings.Repeat("You are Codex, a coding agent based on GPT-5. ", 1024)
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_ok","status":"in_progress","instructions":"` + instructions + `"}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"hi"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_ok","status":"completed"}}`,
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	c, rec := newDelayedStatusCodeTestContext(t, &Group{DelayedStatusCode: true})

	_, err := svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)

	require.NoError(t, err)
	out := rec.Body.String()
	require.Contains(t, out, "response.created", "暂存的 preamble 必须补发")
	require.Contains(t, out, "response.output_text.delta")
	require.Less(t, strings.Index(out, "response.created"), strings.Index(out, "response.output_text.delta"),
		"preamble 必须排在首个语义输出之前")
}

// 开关关闭时保持现状：大 preamble 写穿提交 200，流内 error 不再换号。
func TestOpenAINativeStreamLargePreambleSwitchOffKeepsCurrentBehavior(t *testing.T) {
	svc := newBareErrorTestService()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(largeBareErrorOverloadedSSE())),
	}
	c, rec := newDelayedStatusCodeTestContext(t, &Group{DelayedStatusCode: false})

	_, err := svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "开关关闭时维持现状，不换号")
	require.NotEmpty(t, rec.Body.String(), "开关关闭时 preamble 照旧写穿")
}

// 裸 error 帧之后没有 response.failed 时，同样要 failover，不能挂到超时。
func TestOpenAINativeStreamBareErrorWithoutFailedTriggersFailover(t *testing.T) {
	svc := newBareErrorTestService()
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_only_err","status":"in_progress"}}`,
		"",
		`data: {"type":"error","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`,
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	c, rec := newBareErrorTestContext(t, "/v1/responses")

	_, err := svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String())
}

// 已经开始输出之后才出现的裸 error 帧必须照常透传，不得改变既有行为。
func TestOpenAINativeStreamBareErrorAfterOutputPassesThrough(t *testing.T) {
	svc := newBareErrorTestService()
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_late","status":"in_progress"}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"hi"}`,
		"",
		`data: {"type":"error","error":{"type":"server_error","code":"server_error","message":"boom"}}`,
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	c, rec := newBareErrorTestContext(t, "/v1/responses")

	_, _ = svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)

	require.Contains(t, rec.Body.String(), "response.output_text.delta")
	require.Contains(t, rec.Body.String(), `"type":"error"`, "输出已开始，error 帧应照常透传")
}

// 不可重试错误（内容策略）经由裸 error 帧到达时不得换号。
func TestOpenAINativeStreamBareErrorContentPolicyDoesNotFailover(t *testing.T) {
	svc := newBareErrorTestService()
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_policy","status":"in_progress"}}`,
		"",
		`data: {"type":"error","error":{"type":"invalid_request_error","code":"content_policy","message":"blocked by content policy"}}`,
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	c, _ := newBareErrorTestContext(t, "/v1/responses")

	_, err := svc.handleStreamingResponse(
		c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model",
	)

	if err != nil {
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr),
			"content_policy 不可重试，不得返回 failover 错误")
	}
}

// compat chat 转换路径同样要在裸 error 帧上换号。签名见
// openai_gateway_chat_completions.go:492 —— 无 ctx 参数。
func TestOpenAIChatCompletionsStreamBareErrorTriggersFailover(t *testing.T) {
	svc := newBareErrorTestService()
	svc.toolCorrector = NewCodexToolCorrector()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(bareErrorOverloadedSSE())),
	}
	c, rec := newBareErrorTestContext(t, "/v1/chat/completions")

	_, err := svc.handleChatStreamingResponse(
		resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI},
		"model", "model", "model",
		time.Now(), 0,
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String(), "裸 error 帧不得写给客户端")
}
