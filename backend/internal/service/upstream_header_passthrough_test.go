//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// httpUpstreamCapture captures the outgoing *http.Request for assertion.
type httpUpstreamCapture struct {
	capturedReq *http.Request
	resp        *http.Response
	err         error
REDACTED

func (s *httpUpstreamCapture) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.capturedReq = req
	return s.resp, s.err
REDACTED

func (s *httpUpstreamCapture) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ bool) (*http.Response, error) {
	s.capturedReq = req
	return s.resp, s.err
REDACTED

func newUpstreamAccount() *Account {
REDACTED
		ID:          100,
		Name:        "upstream-test",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeUpstream,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED
			"base_url": "https://upstream.example.com",
			"api_key":  "sk-upstream-secret",
	REDACTED,
REDACTED
REDACTED

// makeSSEOKResponse builds a minimal SSE response that
// handleClaudeStreamingResponse / handleGeminiStreamingResponse
// can consume without error.
// We return 502 to bypass streaming and hit the error branch instead,
// which is sufficient for testing header passthrough.
func makeUpstreamErrorResponse() *http.Response {
	body := []byte(`{"error":{"message":"test error"REDACTEDREDACTED`)
	return &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(bytes.NewReader(body)),
REDACTED
REDACTED

// --- ForwardUpstream tests ---

func TestForwardUpstream_PassthroughHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(map[string]any{
		"model":      "claude-sonnet-4-5",
		"messages":   []map[string]any{{"role": "user", "content": "hi"REDACTEDREDACTED,
		"max_tokens": 1,
		"stream":     false,
REDACTED)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2024-10-22")
	req.Header.Set("anthropic-beta", "output-128k-2025-02-19")
	req.Header.Set("X-Custom-Header", "custom-value")
	c.Request = req

	stub := &httpUpstreamCapture{resp: makeUpstreamErrorResponse()REDACTED
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  stub,
REDACTED

	_, _ = svc.ForwardUpstream(context.Background(), c, newUpstreamAccount(), body, false)

	captured := stub.capturedReq
	require.NotNil(t, captured, "upstream request should have been made")

	// 客户端 header 应被透传
	require.Equal(t, "application/json", captured.Header.Get("Content-Type"))
	require.Equal(t, "2024-10-22", captured.Header.Get("anthropic-version"))
	require.Equal(t, "output-128k-2025-02-19", captured.Header.Get("anthropic-beta"))
	require.Equal(t, "custom-value", captured.Header.Get("X-Custom-Header"))
REDACTED

func TestForwardUpstream_OverridesAuthHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(map[string]any{
		"model":      "claude-sonnet-4-5",
		"messages":   []map[string]any{{"role": "user", "content": "hi"REDACTEDREDACTED,
		"max_tokens": 1,
		"stream":     false,
REDACTED)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// 客户端发来的认证头应被覆盖
	req.Header.Set("Authorization", "Bearer client-token")
	req.Header.Set("x-api-key", "client-api-key")
	c.Request = req

	stub := &httpUpstreamCapture{resp: makeUpstreamErrorResponse()REDACTED
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  stub,
REDACTED

	_, _ = svc.ForwardUpstream(context.Background(), c, newUpstreamAccount(), body, false)

	captured := stub.capturedReq
	require.NotNil(t, captured)

	// 认证头应使用上游账号的 api_key，而非客户端的
	require.Equal(t, "Bearer sk-upstream-secret", captured.Header.Get("Authorization"))
	require.Equal(t, "sk-upstream-secret", captured.Header.Get("x-api-key"))
REDACTED

func TestForwardUpstream_ExcludesHopByHopHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(map[string]any{
		"model":      "claude-sonnet-4-5",
		"messages":   []map[string]any{{"role": "user", "content": "hi"REDACTEDREDACTED,
		"max_tokens": 1,
		"stream":     false,
REDACTED)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Te", "trailers")
	c.Request = req

	stub := &httpUpstreamCapture{resp: makeUpstreamErrorResponse()REDACTED
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  stub,
REDACTED

	_, _ = svc.ForwardUpstream(context.Background(), c, newUpstreamAccount(), body, false)

	captured := stub.capturedReq
	require.NotNil(t, captured)

	// hop-by-hop header 不应出现
	require.Empty(t, captured.Header.Get("Connection"))
	require.Empty(t, captured.Header.Get("Keep-Alive"))
	require.Empty(t, captured.Header.Get("Transfer-Encoding"))
	require.Empty(t, captured.Header.Get("Upgrade"))
	require.Empty(t, captured.Header.Get("Te"))

	// 但普通 header 应保留
	require.Equal(t, "application/json", captured.Header.Get("Content-Type"))
REDACTED

// --- ForwardUpstreamGemini tests ---

func TestForwardUpstreamGemini_PassthroughHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Gemini", "gemini-value")
	req.Header.Set("X-Request-Id", "req-abc-123")
	c.Request = req

	stub := &httpUpstreamCapture{resp: makeUpstreamErrorResponse()REDACTED
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  stub,
REDACTED

	_, _ = svc.ForwardUpstreamGemini(context.Background(), c, newUpstreamAccount(), "gemini-2.5-flash", "generateContent", false, body, false)

	captured := stub.capturedReq
	require.NotNil(t, captured, "upstream request should have been made")

	// 客户端 header 应被透传
	require.Equal(t, "application/json", captured.Header.Get("Content-Type"))
	require.Equal(t, "gemini-value", captured.Header.Get("X-Custom-Gemini"))
	require.Equal(t, "req-abc-123", captured.Header.Get("X-Request-Id"))
REDACTED

func TestForwardUpstreamGemini_OverridesAuthHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer client-gemini-token")
	c.Request = req

	stub := &httpUpstreamCapture{resp: makeUpstreamErrorResponse()REDACTED
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  stub,
REDACTED

	_, _ = svc.ForwardUpstreamGemini(context.Background(), c, newUpstreamAccount(), "gemini-2.5-flash", "generateContent", false, body, false)

	captured := stub.capturedReq
	require.NotNil(t, captured)

	// 认证头应使用上游账号的 api_key
	require.Equal(t, "Bearer sk-upstream-secret", captured.Header.Get("Authorization"))
REDACTED

func TestForwardUpstreamGemini_ExcludesHopByHopHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Proxy-Authorization", "Basic dXNlcjpwYXNz")
	req.Header.Set("Host", "evil.example.com")
	c.Request = req

	stub := &httpUpstreamCapture{resp: makeUpstreamErrorResponse()REDACTED
	svc := &AntigravityGatewayService{
		tokenProvider: &AntigravityTokenProvider{REDACTED,
		httpUpstream:  stub,
REDACTED

	_, _ = svc.ForwardUpstreamGemini(context.Background(), c, newUpstreamAccount(), "gemini-2.5-flash", "generateContent", false, body, false)

	captured := stub.capturedReq
	require.NotNil(t, captured)

	// hop-by-hop header 不应出现
	require.Empty(t, captured.Header.Get("Connection"))
	require.Empty(t, captured.Header.Get("Proxy-Authorization"))
	// Host header 在 Go http.Request 中特殊处理，但我们的黑名单应阻止透传
	require.Empty(t, captured.Header.Values("Host"))

	// 普通 header 应保留
	require.Equal(t, "application/json", captured.Header.Get("Content-Type"))
REDACTED
