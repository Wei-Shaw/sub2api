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

type upstreamContextTestKey string

func newStreamingResponseTestGatewayService() *GatewayService {
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

func TestGatewayService_StreamingReusesScannerBufferAndStillParsesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStreamingResponseTestGatewayService()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: prREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		// Minimal SSE event to trigger parseSSEUsage
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3REDACTEDREDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7REDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 3, result.usage.InputTokens)
	require.Equal(t, 7, result.usage.OutputTokens)
REDACTED

func TestGatewayService_StreamingKeepaliveUsesIdleTimer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStreamingResponseTestGatewayService()
	svc.cfg.Gateway.StreamKeepaliveInterval = 1

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: prREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":1REDACTEDREDACTEDREDACTED\n\n"))
		time.Sleep(1100 * time.Millisecond)
		_, _ = pw.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"REDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), "event: ping")
REDACTED

func TestGatewayService_StreamingKeepaliveUsesNoopDeltaForAffectedClaudeCodeVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStreamingResponseTestGatewayService()
	svc.cfg.Gateway.StreamKeepaliveInterval = 1

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.198 (external, cli)")

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: prREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":1REDACTEDREDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"REDACTEDREDACTED\n\n"))
		time.Sleep(1100 * time.Millisecond)
		_, _ = pw.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0REDACTED\n\n"))
		_, _ = pw.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"REDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	body := rec.Body.String()
	require.Contains(t, body, "event: content_block_delta")
	require.Contains(t, body, `"delta":{"type":"text_delta","text":""REDACTED`)
REDACTED

func TestGatewayService_StreamingKeepaliveUsesNoopDeltaDuringToolUseForAffectedClaudeCodeVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStreamingResponseTestGatewayService()
	svc.cfg.Gateway.StreamKeepaliveInterval = 1

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.198 (external, cli)")

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: prREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":1REDACTEDREDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"Edit\",\"input\":{REDACTEDREDACTEDREDACTED\n\n"))
		time.Sleep(1100 * time.Millisecond)
		_, _ = pw.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1REDACTED\n\n"))
		_, _ = pw.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"REDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	body := rec.Body.String()
	require.Contains(t, body, "event: content_block_delta")
	require.Contains(t, body, `"index":1`)
	require.Contains(t, body, `"delta":{"type":"input_json_delta","partial_json":""REDACTED`)
REDACTED

func TestGatewayService_StreamingKeepaliveKeepsPingForOlderClaudeCodeVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStreamingResponseTestGatewayService()
	svc.cfg.Gateway.StreamKeepaliveInterval = 1

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.187 (external, cli)")

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{REDACTED, Body: prREDACTED

	go func() {
		defer func() { _ = pw.Close() REDACTED()
		_, _ = pw.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":1REDACTEDREDACTEDREDACTED\n\n"))
		_, _ = pw.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"REDACTEDREDACTED\n\n"))
		time.Sleep(1100 * time.Millisecond)
		_, _ = pw.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0REDACTED\n\n"))
		_, _ = pw.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"REDACTED\n\n"))
REDACTED()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now(), "model", "model", false)
	_ = pr.Close()
REDACTED
	require.NotNil(t, result)
	body := rec.Body.String()
	require.Contains(t, body, "event: ping")
	require.NotContains(t, body, `"delta":{"type":"text_delta","text":""REDACTED`)
REDACTED

func TestDetachUpstreamContextIgnoresClientCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), upstreamContextTestKey("test-key"), "test-value"))
	upstreamCtx, release := detachUpstreamContext(parent)
	defer release()

	cancel()

	require.NoError(t, upstreamCtx.Err())
	require.Equal(t, "test-value", upstreamCtx.Value(upstreamContextTestKey("test-key")))
REDACTED
