//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ErrorPolicySkipped 的客户端写出契约（与 OpenAI 网关路径对齐）：
//   - 池模式：不可 failover 的 4xx 按上游原始状态码/响应体保真写出，不改写成 5xx；
//   - 自定义错误码未命中：统一 500 + 固定文案，上游细节只进 ops 错误日志；
//   - 可 failover 的状态码（两种账号）一律换号，不透传。
// ---------------------------------------------------------------------------

const geminiSkippedTestUpstreamMsg = "antigravity executor: invalid Gemini function call history"

func geminiSkippedTestUpstreamBody() string {
	return `{"error":{"code":null,"message":"` + geminiSkippedTestUpstreamMsg + `","param":"","type":"invalid_request_error"REDACTEDREDACTED`
REDACTED

func newGeminiSkippedWriteService(status int, body string) (*GeminiMessagesCompatService, *geminiCompatHTTPUpstreamStub) {
	httpStub := &geminiCompatHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(body)),
	REDACTED,
REDACTED
	svc := &GeminiMessagesCompatService{
		httpUpstream:     httpStub,
		cfg:              &config.Config{REDACTED,
		rateLimitService: NewRateLimitService(&errorPolicyRepoStub{REDACTED, nil, &config.Config{REDACTED, nil, nil),
REDACTED
	return svc, httpStub
REDACTED

func geminiPoolModeAPIKeyAccount() *Account {
REDACTED
		ID:       700,
REDACTED
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":   "test-key",
			"pool_mode": true,
	REDACTED,
REDACTED
REDACTED

func geminiCustomCodesAPIKeyAccount() *Account {
REDACTED
		ID:       701,
REDACTED
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":                    "test-key",
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(429)REDACTED,
	REDACTED,
REDACTED
REDACTED

func newGeminiNativeTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
REDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", strings.NewReader("{REDACTED"))
	return c, rec
REDACTED

func TestGeminiForwardNative_PoolModeSkipped400PassthroughRealStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := geminiSkippedTestUpstreamBody()
	svc, _ := newGeminiSkippedWriteService(http.StatusBadRequest, upstreamBody)
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiPoolModeAPIKeyAccount(),
		"gemini-2.5-flash", "generateContent", false, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"REDACTED]REDACTED]REDACTED`))

	require.Nil(t, result)
REDACTED
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "池模式 400 不应换号")
	require.Contains(t, err.Error(), "gemini upstream error: 400")
	require.Equal(t, http.StatusBadRequest, rec.Code, "状态码应保真为上游 400")
	require.Equal(t, upstreamBody, rec.Body.String(), "响应体应原样透传")
REDACTED

func TestGeminiForwardNative_PoolModeSkipped503Failover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newGeminiSkippedWriteService(http.StatusServiceUnavailable, `{"error":{"message":"Upstream service temporarily unavailable"REDACTEDREDACTED`)
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiPoolModeAPIKeyAccount(),
		"gemini-2.5-flash", "generateContent", false, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"REDACTED]REDACTED]REDACTED`))

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "池模式 503 应换号")
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.Zero(t, rec.Body.Len(), "换号场景不应写客户端响应")
REDACTED

func TestGeminiForwardNative_CustomCodesMiss400HiddenAs500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newGeminiSkippedWriteService(http.StatusBadRequest, geminiSkippedTestUpstreamBody())
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiCustomCodesAPIKeyAccount(),
		"gemini-2.5-flash", "generateContent", false, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"REDACTED]REDACTED]REDACTED`))

	require.Nil(t, result)
REDACTED
	require.Contains(t, err.Error(), "not in custom error codes")
	require.Equal(t, http.StatusInternalServerError, rec.Code)

REDACTED
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	errObj, ok := got["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, geminiCustomCodeSkippedClientMessage, errObj["message"])
	require.NotContains(t, rec.Body.String(), geminiSkippedTestUpstreamMsg, "上游细节不应透传给客户端")
REDACTED

func TestGeminiForwardNative_CustomCodesMiss500Failover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newGeminiSkippedWriteService(http.StatusInternalServerError, `{"error":{"message":"internal"REDACTEDREDACTED`)
	c, rec := newGeminiNativeTestContext(t)

	result, err := svc.ForwardNative(context.Background(), c, geminiCustomCodesAPIKeyAccount(),
		"gemini-2.5-flash", "generateContent", false, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"REDACTED]REDACTED]REDACTED`))

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "自定义错误码未命中的 500 应换号")
	require.Equal(t, http.StatusInternalServerError, failoverErr.StatusCode)
	require.False(t, failoverErr.RetryableOnSameAccount, "非池模式不应同账号重试")
	require.Zero(t, rec.Body.Len())
REDACTED

func TestGeminiForwardAsChatCompletions_CustomCodesMiss400HiddenAs500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newGeminiSkippedWriteService(http.StatusBadRequest, geminiSkippedTestUpstreamBody())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiCustomCodesAPIKeyAccount(), body)

	require.Nil(t, result)
REDACTED
	require.Contains(t, err.Error(), "not in custom error codes")
	require.Equal(t, http.StatusInternalServerError, rec.Code)

REDACTED
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	errObj, ok := got["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "api_error", errObj["type"])
	require.Equal(t, geminiCustomCodeSkippedClientMessage, errObj["message"])
REDACTED

func TestGeminiForwardAsChatCompletions_PoolMode400KeepsUpstreamMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newGeminiSkippedWriteService(http.StatusBadRequest, geminiSkippedTestUpstreamBody())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"REDACTED]REDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiPoolModeAPIKeyAccount(), body)

	require.Nil(t, result)
REDACTED
	require.Equal(t, http.StatusBadRequest, rec.Code, "状态码应保真为上游 400")

REDACTED
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	errObj, ok := got["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", errObj["type"])
	require.Equal(t, geminiSkippedTestUpstreamMsg, errObj["message"], "应回传上游 message")
REDACTED

func TestWriteGeminiMappedError_400KeepsUpstreamMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &GeminiMessagesCompatService{cfg: &config.Config{REDACTEDREDACTED
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	err := svc.writeGeminiMappedError(c, &Account{ID: 702, Platform: PlatformGeminiREDACTED, http.StatusBadRequest, "req-1", []byte(geminiSkippedTestUpstreamBody()))

REDACTED
	require.Equal(t, http.StatusBadRequest, rec.Code)
REDACTED
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	errObj, ok := got["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, geminiSkippedTestUpstreamMsg, errObj["message"], "应回传上游 message")
REDACTED
