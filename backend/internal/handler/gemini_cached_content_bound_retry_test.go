//go:build unit

package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	boundRetryUpstream429 = `{"error":{"code":429,"message":"Resource exhausted. Please try again later.","status":"RESOURCE_EXHAUSTED"}}`
	boundRetryUpstream502 = `{"error":{"code":502,"message":"Upstream service temporarily unavailable","status":"UNAVAILABLE"}}`
)

// boundRetryUpstream 前 failures 次请求返回 failStatus（默认 429），之后返回成功的生成响应。
type boundRetryUpstream struct {
	service.HTTPUpstream
	mu         sync.Mutex
	failures   int
	failStatus int
	bodies     []string
}

func (u *boundRetryUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	raw, _ := io.ReadAll(req.Body)
	u.mu.Lock()
	u.bodies = append(u.bodies, string(raw))
	n := len(u.bodies)
	u.mu.Unlock()
	header := http.Header{"Content-Type": []string{"application/json"}}
	if n <= u.failures {
		if u.failStatus == http.StatusBadGateway {
			return &http.Response{StatusCode: http.StatusBadGateway, Header: header, Body: io.NopCloser(strings.NewReader(boundRetryUpstream502))}, nil
		}
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: header, Body: io.NopCloser(strings.NewReader(boundRetryUpstream429))}, nil
	}
	body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],` +
		`"usageMetadata":{"promptTokenCount":10,"cachedContentTokenCount":8,"candidatesTokenCount":1,"totalTokenCount":11}}`
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func (u *boundRetryUpstream) requestBodies() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.bodies...)
}

// newBoundRetryEnv 构造绑定在池模式上游网关账号 900 上的显式缓存，账号对 retryCodes 同账号重试 2 次。
func newBoundRetryEnv(t *testing.T, upstream *boundRetryUpstream, retryCodes ...int) *cachedBillingEnv {
	t.Helper()
	env := newCachedBillingEnv(t, 500, 900)
	creds := env.accounts[900].Credentials
	creds["base_url"] = "https://gw.example.com"
	creds[service.GeminiExplicitCacheUpstreamCredentialKey] = true
	creds["pool_mode"] = true
	creds["pool_mode_retry_count"] = float64(2)
	codes := make([]any, 0, len(retryCodes))
	for _, code := range retryCodes {
		codes = append(codes, float64(code))
	}
	creds["pool_mode_retry_status_codes"] = codes
	env.seed(time.Now().Add(time.Hour))

	env.h.geminiCompatService = service.NewGeminiMessagesCompatService(&cachedBillingAccountRepo{accounts: env.accounts},
		nil, nil, nil, nil, nil, upstream, nil, &config.Config{})
	return env
}

func (e *cachedBillingEnv) generateWithCache() *httptest.ResponseRecorder {
	return e.serve(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"cachedContent":"cachedContents/abc","contents":[{"role":"user","parts":[{"text":"hi"}]}]}`,
		gin.Params{{Key: "modelAction", Value: "/gemini-2.5-pro:generateContent"}}, e.h.GeminiV1BetaModels)
}

func TestGeminiV1BetaModels_BoundCachePoolModeRetriesUpstream429(t *testing.T) {
	upstream := &boundRetryUpstream{failures: 1}
	env := newBoundRetryEnv(t, upstream, 429)

	rec := env.generateWithCache()

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	bodies := upstream.requestBodies()
	require.Len(t, bodies, 2, "429 后在持有缓存的账号上重试一次")
	for _, body := range bodies {
		require.Contains(t, body, `"cachedContents/up1"`, "每次重试都引用上游缓存名")
	}
}

func TestGeminiV1BetaModels_BoundCacheRetryExhaustedPassesThroughUpstream429(t *testing.T) {
	upstream := &boundRetryUpstream{failures: 10}
	env := newBoundRetryEnv(t, upstream, 429)

	rec := env.generateWithCache()

	require.Len(t, upstream.requestBodies(), 3, "首发 + pool_mode_retry_count 次同账号重试")
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.JSONEq(t, boundRetryUpstream429, rec.Body.String(), "重试用尽后原样回写上游网关的响应")
}

// 502 在池模式重试状态码内时同样同账号重试；用尽后原样回写，不按空响应临时封禁本账号。
func TestGeminiV1BetaModels_BoundCacheRetryExhaustedOn502KeepsAccountSchedulable(t *testing.T) {
	upstream := &boundRetryUpstream{failures: 10, failStatus: http.StatusBadGateway}
	env := newBoundRetryEnv(t, upstream, 429, 502)

	rec := env.generateWithCache()

	require.Len(t, upstream.requestBodies(), 3)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.JSONEq(t, boundRetryUpstream502, rec.Body.String())
}
