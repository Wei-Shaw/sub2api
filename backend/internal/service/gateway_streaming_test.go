//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
