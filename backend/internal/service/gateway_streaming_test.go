//go:build unit

package service

import (
	"context"
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

// --- parseSSEUsage 测试 ---

func newMinimalGatewayService() *GatewayService {
	return &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 0,
				MaxLineSize:               defaultMaxLineSize,
		REDACTED,
	REDACTED,
		rateLimitService: &RateLimitService{REDACTED,
REDACTED
REDACTED

func TestParseSSEUsage_MessageStart(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{REDACTED

	data := `{"type":"message_start","message":{"usage":{"input_tokens":100,"cache_creation_input_tokens":50,"cache_read_input_tokens":200REDACTEDREDACTEDREDACTED`
	svc.parseSSEUsage(data, usage)

	require.Equal(t, 100, usage.InputTokens)
	require.Equal(t, 50, usage.CacheCreationInputTokens)
	require.Equal(t, 200, usage.CacheReadInputTokens)
	require.Equal(t, 0, usage.OutputTokens, "message_start 不应设置 output_tokens")
REDACTED

func TestParseSSEUsage_MessageDelta(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{REDACTED

	data := `{"type":"message_delta","usage":{"output_tokens":42REDACTEDREDACTED`
	svc.parseSSEUsage(data, usage)

	require.Equal(t, 42, usage.OutputTokens)
	require.Equal(t, 0, usage.InputTokens, "message_delta 的 output_tokens 不应影响已有的 input_tokens")
REDACTED

func TestParseSSEUsage_DeltaDoesNotOverwriteStartValues(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{REDACTED

	// 先处理 message_start
	svc.parseSSEUsage(`{"type":"message_start","message":{"usage":{"input_tokens":100REDACTEDREDACTEDREDACTED`, usage)
	require.Equal(t, 100, usage.InputTokens)

	// 再处理 message_delta（output_tokens > 0, input_tokens = 0）
	svc.parseSSEUsage(`{"type":"message_delta","usage":{"output_tokens":50REDACTEDREDACTED`, usage)
	require.Equal(t, 100, usage.InputTokens, "delta 中 input_tokens=0 不应覆盖 start 中的值")
	require.Equal(t, 50, usage.OutputTokens)
REDACTED

func TestParseSSEUsage_DeltaOverwritesWithNonZero(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{REDACTED

	// GLM 等 API 会在 delta 中包含所有 usage 信息
	svc.parseSSEUsage(`{"type":"message_delta","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":30,"cache_read_input_tokens":60REDACTEDREDACTED`, usage)
	require.Equal(t, 200, usage.InputTokens)
	require.Equal(t, 100, usage.OutputTokens)
	require.Equal(t, 30, usage.CacheCreationInputTokens)
	require.Equal(t, 60, usage.CacheReadInputTokens)
REDACTED

func TestParseSSEUsage_DeltaDoesNotResetCacheCreationBreakdown(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{REDACTED

	// 先在 message_start 中写入非零 5m/1h 明细
	svc.parseSSEUsage(`{"type":"message_start","message":{"usage":{"input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":30,"ephemeral_1h_input_tokens":70REDACTEDREDACTEDREDACTEDREDACTED`, usage)
	require.Equal(t, 30, usage.CacheCreation5mTokens)
	require.Equal(t, 70, usage.CacheCreation1hTokens)

	// 后续 delta 带默认 0，不应覆盖已有非零值
	svc.parseSSEUsage(`{"type":"message_delta","usage":{"output_tokens":12,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0REDACTEDREDACTEDREDACTED`, usage)
	require.Equal(t, 30, usage.CacheCreation5mTokens, "delta 的 0 值不应重置 5m 明细")
	require.Equal(t, 70, usage.CacheCreation1hTokens, "delta 的 0 值不应重置 1h 明细")
	require.Equal(t, 12, usage.OutputTokens)
REDACTED

func TestParseSSEUsage_InvalidJSON(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{REDACTED

	// 无效 JSON 不应 panic
	svc.parseSSEUsage("not json", usage)
	require.Equal(t, 0, usage.InputTokens)
	require.Equal(t, 0, usage.OutputTokens)
REDACTED

func TestParseSSEUsage_UnknownType(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{REDACTED

	// 不是 message_start 或 message_delta 的类型
	svc.parseSSEUsage(`{"type":"content_block_delta","delta":{"text":"hello"REDACTEDREDACTED`, usage)
	require.Equal(t, 0, usage.InputTokens)
	require.Equal(t, 0, usage.OutputTokens)
REDACTED

func TestParseSSEUsage_EmptyString(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{REDACTED

	svc.parseSSEUsage("", usage)
	require.Equal(t, 0, usage.InputTokens)
REDACTED

func TestParseSSEUsage_DoneEvent(t *testing.T) {
	svc := newMinimalGatewayService()
	usage := &ClaudeUsage{REDACTED

	// [DONE] 事件不应影响 usage
	svc.parseSSEUsage("[DONE]", usage)
	require.Equal(t, 0, usage.InputTokens)
REDACTED

// --- 流式响应端到端测试 ---

func TestHandleStreamingResponse_CacheTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: prREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"cache_creation_input_tokens\":20,\"cache_read_input_tokens\":30REDACTEDREDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15REDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 10, result.usage.InputTokens)
	require.Equal(t, 15, result.usage.OutputTokens)
	require.Equal(t, 20, result.usage.CacheCreationInputTokens)
	require.Equal(t, 30, result.usage.CacheReadInputTokens)
REDACTED

func TestHandleStreamingResponse_EmptyStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: prREDACTED

	go func() {
		// 直接关闭，不发送任何事件
		_ = pw.Close()
REDACTED()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)
	_ = pr.Close()
REDACTED
	require.Contains(t, err.Error(), "missing terminal event")
	require.NotNil(t, result)
REDACTED

func TestHandleStreamingResponse_SpecialCharactersInJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newMinimalGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: prREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		// 包含特殊字符的 content_block_delta（引号、换行、Unicode）
		_, _ = pw.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \\\"world\\\"\\n你好\"REDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5REDACTEDREDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3REDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 5, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)

	// 验证响应中包含转发的数据
	body := rec.Body.String()
	require.Contains(t, body, "content_block_delta", "响应应包含转发的 SSE 事件")
REDACTED

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
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       &streamReadCloser{err: io.ErrUnexpectedEOFREDACTED,
REDACTED

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)

REDACTED
	require.Nil(t, result, "失败移交场景下不应返回 streamingResult")

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "未输出过字节时 stream read error 必须包成 UpstreamFailoverError，期望: %v", err)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount, "GOAWAY 类错误应允许同账号重试")
	require.Contains(t, string(failoverErr.ResponseBody), "upstream stream disconnected")

	// 客户端应收不到任何 stream_read_error 事件，由 handler 层根据 failover 结果再决定
	require.NotContains(t, rec.Body.String(), "stream_read_error")
REDACTED

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
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: &streamReadCloser{
			payload: []byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5REDACTEDREDACTEDREDACTED\n\n"),
			err:     io.ErrUnexpectedEOF,
	REDACTED,
REDACTED

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)

REDACTED
	require.Contains(t, err.Error(), "stream read error", "已开始流后应透传普通 stream read error")
	require.NotNil(t, result, "透传场景下应返回已收集的 streamingResult")

	// 不应被错误地包成 failover error
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "已经向客户端写过字节时不能再 failover")

	// 客户端必须收到 stream_read_error 事件
	body := rec.Body.String()
	require.True(t,
		strings.Contains(body, "stream_read_error"),
		"已开始流后必须发送 stream_read_error 事件给客户端，实际响应: %q", body)
REDACTED
