//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// --- parseSSEUsage 测试 ---

func newMinimalGatewayService() *GatewayService {
	return &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 0,
				MaxLineSize:               defaultMaxLineSize,
			},
		},
		rateLimitService: &RateLimitService{},
	}
}

func TestParseSSEUsage_MessageStart(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{}

	data := `{"type":"message_start","message":{"usage":{"input_tokens":100,"cache_creation_input_tokens":50,"cache_read_input_tokens":200}}}`
	svc.parseSSEUsage(data, usage)

	require.Equal(t, 100, usage.InputTokens)
	require.Equal(t, 50, usage.CacheCreationInputTokens)
	require.Equal(t, 200, usage.CacheReadInputTokens)
	require.Equal(t, 0, usage.OutputTokens, "message_start 不应设置 output_tokens")
}

func TestParseSSEUsage_MessageDelta(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{}

	data := `{"type":"message_delta","usage":{"output_tokens":42}}`
	svc.parseSSEUsage(data, usage)

	require.Equal(t, 42, usage.OutputTokens)
	require.Equal(t, 0, usage.InputTokens, "message_delta 的 output_tokens 不应影响已有的 input_tokens")
}

func TestParseSSEUsage_DeltaDoesNotOverwriteStartValues(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{}

	// 先处理 message_start
	svc.parseSSEUsage(`{"type":"message_start","message":{"usage":{"input_tokens":100}}}`, usage)
	require.Equal(t, 100, usage.InputTokens)

	// 再处理 message_delta（output_tokens > 0, input_tokens = 0）
	svc.parseSSEUsage(`{"type":"message_delta","usage":{"output_tokens":50}}`, usage)
	require.Equal(t, 100, usage.InputTokens, "delta 中 input_tokens=0 不应覆盖 start 中的值")
	require.Equal(t, 50, usage.OutputTokens)
}

func TestParseSSEUsage_DeltaOverwritesWithNonZero(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{}

	// GLM 等 API 会在 delta 中包含所有 usage 信息
	svc.parseSSEUsage(`{"type":"message_delta","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":30,"cache_read_input_tokens":60}}`, usage)
	require.Equal(t, 200, usage.InputTokens)
	require.Equal(t, 100, usage.OutputTokens)
	require.Equal(t, 30, usage.CacheCreationInputTokens)
	require.Equal(t, 60, usage.CacheReadInputTokens)
}

func TestParseSSEUsage_DeltaDoesNotResetCacheCreationBreakdown(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{}

	// 先在 message_start 中写入非零 5m/1h 明细
	svc.parseSSEUsage(`{"type":"message_start","message":{"usage":{"input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":30,"ephemeral_1h_input_tokens":70}}}}`, usage)
	require.Equal(t, 30, usage.CacheCreation5mTokens)
	require.Equal(t, 70, usage.CacheCreation1hTokens)

	// 后续 delta 带默认 0，不应覆盖已有非零值
	svc.parseSSEUsage(`{"type":"message_delta","usage":{"output_tokens":12,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}}}`, usage)
	require.Equal(t, 30, usage.CacheCreation5mTokens, "delta 的 0 值不应重置 5m 明细")
	require.Equal(t, 70, usage.CacheCreation1hTokens, "delta 的 0 值不应重置 1h 明细")
	require.Equal(t, 12, usage.OutputTokens)
}

func TestParseSSEUsage_InvalidJSON(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{}

	// 无效 JSON 不应 panic
	svc.parseSSEUsage("not json", usage)
	require.Equal(t, 0, usage.InputTokens)
	require.Equal(t, 0, usage.OutputTokens)
}

func TestParseSSEUsage_UnknownType(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{}

	// 不是 message_start 或 message_delta 的类型
	svc.parseSSEUsage(`{"type":"content_block_delta","delta":{"text":"hello"}}`, usage)
	require.Equal(t, 0, usage.InputTokens)
	require.Equal(t, 0, usage.OutputTokens)
}

func TestParseSSEUsage_EmptyString(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{}

	svc.parseSSEUsage("", usage)
	require.Equal(t, 0, usage.InputTokens)
}

func TestParseSSEUsage_DoneEvent(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{}

	// [DONE] 事件不应影响 usage
	svc.parseSSEUsage("[DONE]", usage)
	require.Equal(t, 0, usage.InputTokens)
}

// --- 流式响应端到端测试 ---

func TestHandleStreamingResponse_CacheTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"cache_creation_input_tokens\":20,\"cache_read_input_tokens\":30}}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15}}\n\n"))
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 10, result.usage.InputTokens)
	require.Equal(t, 15, result.usage.OutputTokens)
	require.Equal(t, 20, result.usage.CacheCreationInputTokens)
	require.Equal(t, 30, result.usage.CacheReadInputTokens)
}

func TestHandleStreamingResponse_KiroCreditsInternalField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"cache_read_input_tokens\":3}}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15,\"_sub2api_kiro_credits\":0.17}}\n\n"))
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 10, result.usage.InputTokens)
	require.Equal(t, 15, result.usage.OutputTokens)
	require.Equal(t, 3, result.usage.CacheReadInputTokens)
	require.InDelta(t, 0.17, result.usage.KiroCredits, 0.000001)

	body := rec.Body.String()
	require.Contains(t, body, `"output_tokens":15`)
	require.NotContains(t, body, "_sub2api_kiro_credits")
}

func TestHandleStreamingResponse_EmptyStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		// 直接关闭，不发送任何事件
		_ = pw.Close()
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing terminal event")
	require.NotNil(t, result)
}

func TestHandleStreamingResponse_SpecialCharactersInJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		// 包含特殊字符的 content_block_delta（引号、换行、Unicode）
		_, _ = pw.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \\\"world\\\"\\n你好\"}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5}}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n"))
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 5, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)

	// 验证响应中包含转发的数据
	body := rec.Body.String()
	require.Contains(t, body, "content_block_delta", "响应应包含转发的 SSE 事件")
}

// 上游中途读错误（如 HTTP/2 GOAWAY 触发的 unexpected EOF）发生在向客户端写入任何字节前：
// 网关应返回 *UpstreamFailoverError 触发账号 failover/重试，而不是把错误事件直接发给客户端。
func TestHandleStreamingResponse_StreamReadErrorBeforeOutput_TriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       &streamReadCloser{err: io.ErrUnexpectedEOF},
	}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)

	require.Error(t, err)
	require.Nil(t, result, "失败移交场景下不应返回 streamingResult")

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "未输出过字节时 stream read error 必须包成 UpstreamFailoverError，期望: %v", err)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount, "GOAWAY 类错误应允许同账号重试")

	// ResponseBody 必须是 Anthropic 标准 error 格式：
	// 1) ExtractUpstreamErrorMessage 能正确从 error.message 提取消息（被 handleFailoverExhausted / ops 日志依赖）
	// 2) error.type 标记为 upstream_disconnected
	extractedMsg := ExtractUpstreamErrorMessage(failoverErr.ResponseBody)
	require.NotEmpty(t, extractedMsg, "ExtractUpstreamErrorMessage 必须从 ResponseBody 取到非空 message，否则 ops 日志会丢失诊断信息")
	require.Contains(t, extractedMsg, "upstream stream disconnected")
	require.Contains(t, string(failoverErr.ResponseBody), `"type":"error"`)
	require.Contains(t, string(failoverErr.ResponseBody), `"upstream_disconnected"`)

	// 客户端应收不到任何 stream_read_error 事件，由 handler 层根据 failover 结果再决定
	require.NotContains(t, rec.Body.String(), "stream_read_error")
}

// 上游已经发送过事件（c.Writer 已写过字节）后再发生读错误：
// SSE 协议无 resume，网关只能透传 stream_read_error 错误事件给客户端，不能 failover。
func TestHandleStreamingResponse_StreamReadErrorAfterOutput_PassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// 第一次 Read 返回完整 SSE 事件让网关向 client 写入字节，第二次 Read 返回 EOF
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &streamReadCloser{
			payload: []byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5}}}\n\n"),
			err:     io.ErrUnexpectedEOF,
		},
	}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)

	require.Error(t, err)
	require.Contains(t, err.Error(), "stream read error", "已开始流后应透传普通 stream read error")
	require.NotNil(t, result, "透传场景下应返回已收集的 streamingResult")

	// 不应被错误地包成 failover error
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "已经向客户端写过字节时不能再 failover")

	// 客户端必须收到 Anthropic 标准格式的 SSE error 事件，error.type=stream_read_error，
	// error.message 含具体根因（让 SDK 能解析、UI 能显示具体错误）
	body := rec.Body.String()
	require.Contains(t, body, "event: error\n", "必须按 Anthropic SSE 标准发送 error 事件帧")
	require.Contains(t, body, `"type":"error"`, "data 必须含 type:error 顶层字段（Anthropic 标准）")
	require.Contains(t, body, `"stream_read_error"`, "error.type 必须为 stream_read_error")
	require.Contains(t, body, "upstream stream disconnected", "error.message 必须包含具体根因，Claude Code 等客户端才能显示有效错误文案")
}

// 默认 (*net.OpError).Error() 会拼接 Source/Addr 字段，泄露内部 IP/端口与上游
// 服务器地址。sanitizeStreamError 必须剥离这些信息，避免基础设施拓扑通过
// failover ResponseBody 或 SSE error 帧返回给客户端。
func TestSanitizeStreamError_StripsNetworkAddresses(t *testing.T) {
	src, err := net.ResolveTCPAddr("tcp", "10.0.0.1:54321")
	require.NoError(t, err)
	dst, err := net.ResolveTCPAddr("tcp", "52.1.2.3:443")
	require.NoError(t, err)

	raw := &net.OpError{
		Op:     "read",
		Net:    "tcp",
		Source: src,
		Addr:   dst,
		Err:    syscall.ECONNRESET,
	}

	// 前置：原始 Error() 确实包含会泄露的字段（避免测试在 Go 行为变化时静默通过）
	require.Contains(t, raw.Error(), "10.0.0.1")
	require.Contains(t, raw.Error(), "52.1.2.3")

	got := sanitizeStreamError(raw)
	require.NotContains(t, got, "10.0.0.1", "不得泄露内部源 IP")
	require.NotContains(t, got, "54321", "不得泄露源端口")
	require.NotContains(t, got, "52.1.2.3", "不得泄露上游目标 IP")
	require.NotContains(t, got, "443", "不得泄露上游端口")
	require.Equal(t, "connection reset by peer", got)
}

func TestSanitizeStreamError_KnownErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"unexpected EOF", io.ErrUnexpectedEOF, "unexpected EOF"},
		{"EOF", io.EOF, "EOF"},
		{"context canceled", context.Canceled, "canceled"},
		{"deadline exceeded", context.DeadlineExceeded, "deadline exceeded"},
		{"ECONNRESET 直接", syscall.ECONNRESET, "connection reset by peer"},
		{"EPIPE", syscall.EPIPE, "broken pipe"},
		{"ETIMEDOUT", syscall.ETIMEDOUT, "connection timed out"},
		{"未识别错误兜底", errors.New("weird internal error"), "upstream connection error"},
		{"nil 返回空串", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, sanitizeStreamError(tc.err))
		})
	}
}

// failover ResponseBody 必须用 sanitize 过的消息，避免泄露给客户端 / 写入 ops 日志
// 时携带内部地址信息。
func TestHandleStreamingResponse_FailoverBodyDoesNotLeakAddresses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	src, _ := net.ResolveTCPAddr("tcp", "10.0.0.1:54321")
	dst, _ := net.ResolveTCPAddr("tcp", "52.1.2.3:443")
	netErr := &net.OpError{
		Op:     "read",
		Net:    "tcp",
		Source: src,
		Addr:   dst,
		Err:    syscall.ECONNRESET,
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       &streamReadCloser{err: netErr},
	}

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	require.Error(t, err)

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))

	body := string(failoverErr.ResponseBody)
	require.NotContains(t, body, "10.0.0.1", "failover ResponseBody 不得泄露内部源 IP")
	require.NotContains(t, body, "54321")
	require.NotContains(t, body, "52.1.2.3", "failover ResponseBody 不得泄露上游 IP")
	require.NotContains(t, body, "443")
	// 仍然包含可诊断的根因
	require.Contains(t, body, "connection reset by peer")
	require.Contains(t, body, "upstream stream disconnected")
}

// 上游 HTTP 200 + SSE 流体内 event:error 帧应被识别为 *sseStreamErrorEventError，
// 且 RawData 等于上游 data: 行的原始 JSON。这是 Forward 主流程后续把 dataLine
// 透传到 UpstreamFailoverError.ResponseBody 与 ops_error_logs 的前提。
func TestHandleStreamingResponse_SSEErrorEvent_ReturnsTypedErrorWithRawData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	const errorJSON = `{"type":"error","error":{"type":"overloaded_error","message":"Anthropic upstream is overloaded"}}`

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("event: error\ndata: " + errorJSON + "\n\n"))
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()

	require.Error(t, err)
	require.Nil(t, result)

	// typed error 必须可被 errors.As 匹配，RawData 必须保留上游 dataLine 原文
	var sseErr *sseStreamErrorEventError
	require.True(t, errors.As(err, &sseErr), "SSE event:error 必须包成 *sseStreamErrorEventError，期望: %v", err)
	require.Equal(t, errorJSON, sseErr.RawData)

	// 字符串兼容：保留与旧实现一致的 "have error in stream"，避免破坏依赖该字符串的日志检索
	require.Equal(t, "have error in stream", err.Error())

	// 在 Forward 主流程中调用方依赖 ExtractUpstreamErrorMessage 从 RawData 解析出 message
	extracted := ExtractUpstreamErrorMessage([]byte(sseErr.RawData))
	require.Equal(t, "Anthropic upstream is overloaded", extracted)
}

// 边界用例：上游只发了 event: error 而没有 data 行。RawData 为空，
// 调用方不得 panic，UpstreamFailoverError.ResponseBody 应回退为空切片。
func TestHandleStreamingResponse_SSEErrorEvent_EmptyDataLine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("event: error\n\n"))
	}()

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()

	require.Error(t, err)
	var sseErr *sseStreamErrorEventError
	require.True(t, errors.As(err, &sseErr), "即使 data 行为空，也必须返回 typed error 让上层走 stream_error 分支")
	require.Equal(t, "", sseErr.RawData)
}

// 对抗用例：上游先发 message_start 再发 event:error，模拟"流已开始写客户端"+SSE error 帧。
// 这是 ping 放大场景的服务侧近似（c.Writer 已被写后才出现 error）。
// 必须仍然返回 *sseStreamErrorEventError 且 RawData 包含真实错误体，
// 让 Forward 调用方能正确补全 ResponseBody 与 ops 事件。
func TestHandleStreamingResponse_SSEErrorEvent_AfterPartialStreamOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	const errorJSON = `{"type":"error","error":{"type":"rate_limit_error","message":"Rate limited"}}`

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		// 先发 message_start，让 handleStreamingResponse 把它转发到客户端 → c.Writer 已被写
		_, _ = pw.Write([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}` + "\n\n"))
		// 紧接着发 event:error
		_, _ = pw.Write([]byte("event: error\ndata: " + errorJSON + "\n\n"))
	}()

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()

	require.Error(t, err)
	var sseErr *sseStreamErrorEventError
	require.True(t, errors.As(err, &sseErr), "已发数据后再来的 SSE event:error 必须仍包成 typed error，期望: %v", err)
	require.Equal(t, errorJSON, sseErr.RawData)

	// c.Writer 必定已被写过（message_start 已转发）— 这是 handler 838 行 streamStarted 守卫触发的条件，
	// 修复前/后均会让 handler 直接走 handleFailoverExhausted 而非切账号；不变。
	require.Greater(t, rec.Body.Len(), 0, "message_start 应被转发到客户端")
	require.Contains(t, rec.Body.String(), "message_start")
}

// 对抗用例：上游发 event:error 但 data 行不是合法 JSON。
// RawData 必须保留原始字节，ExtractUpstreamErrorMessage 不得 panic，
// upstreamMsg 回退为空字符串（不再丢失原始诊断线索 — Detail 字段仍保留原始 body）。
func TestHandleStreamingResponse_SSEErrorEvent_NonJSONDataLine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("event: error\ndata: not-a-json-payload\n\n"))
	}()

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()

	require.Error(t, err)
	var sseErr *sseStreamErrorEventError
	require.True(t, errors.As(err, &sseErr))
	require.Equal(t, "not-a-json-payload", sseErr.RawData)

	// gjson 对非 JSON 输入返回空字符串，不 panic — Forward 主流程靠这个 invariant 安全地走下去
	require.NotPanics(t, func() {
		_ = ExtractUpstreamErrorMessage([]byte(sseErr.RawData))
	})
	require.Equal(t, "", ExtractUpstreamErrorMessage([]byte(sseErr.RawData)))
}
