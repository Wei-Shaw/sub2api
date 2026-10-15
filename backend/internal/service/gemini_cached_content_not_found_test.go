//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 上游对不存在、已过期、已删除或不属于当前项目的显式缓存一律返回这个 403。
const geminiCachedContentNotFoundBody = `{"error":{"code":403,"message":"CachedContent not found (or permission denied)","status":"PERMISSION_DENIED"}}`

func newGeminiCachedContentTestService(status int, body string) (*GeminiMessagesCompatService, *geminiCompatHTTPUpstreamStub, *errorPolicyRepoStub) {
	repo := &errorPolicyRepoStub{}
	httpStub := &geminiCompatHTTPUpstreamStub{
		response: &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}
	svc := &GeminiMessagesCompatService{
		httpUpstream:     httpStub,
		cfg:              &config.Config{},
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}
	return svc, httpStub, repo
}

func geminiDefaultPolicyAPIKeyAccount() *Account {
	return &Account{
		ID:       710,
		Platform: PlatformGemini,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "test-key",
		},
	}
}

const geminiCachedContentRequestBody = `{"cachedContent":"cachedContents/doesnotexist","contents":[{"role":"user","parts":[{"text":"hi"}]}]}`

func TestGeminiForwardNative_CachedContentNotFoundPassthroughWithoutAccountPenalty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		action string
		stream bool
	}{
		{name: "generateContent", action: "generateContent"},
		{name: "streamGenerateContent", action: "streamGenerateContent", stream: true},
		{name: "countTokens", action: "countTokens"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, httpStub, repo := newGeminiCachedContentTestService(http.StatusForbidden, geminiCachedContentNotFoundBody)
			c, rec := newGeminiNativeTestContext(t)

			result, err := svc.ForwardNative(context.Background(), c, geminiDefaultPolicyAPIKeyAccount(),
				"gemini-3.8-flash", tc.action, tc.stream, []byte(geminiCachedContentRequestBody))

			require.Nil(t, result)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "缓存不存在是请求问题，不应换号")
			require.Equal(t, 0, repo.setErrCalls, "不应把账号标记为错误")
			require.Equal(t, 0, repo.tempCalls, "不应临时停调度")
			require.Equal(t, 1, httpStub.calls, "不应重试")
			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Equal(t, GeminiCachedContentNotFoundResponse, rec.Body.String(), "应返回标准缓存不存在响应")
			require.True(t, IsOpsRequestScopedError(c), "缓存不存在按客户端请求错误统计")
		})
	}
}

func TestGeminiForwardNative_CachedContentNotFoundBypassesTempUnschedulableRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, repo := newGeminiCachedContentTestService(http.StatusForbidden, geminiCachedContentNotFoundBody)
	c, rec := newGeminiNativeTestContext(t)
	account := geminiDefaultPolicyAPIKeyAccount()
	account.Credentials["temp_unschedulable_enabled"] = true
	account.Credentials["temp_unschedulable_rules"] = []any{
		map[string]any{
			"error_code":       float64(403),
			"keywords":         []any{"permission denied"},
			"duration_minutes": float64(10),
			"description":      "403 permission denied",
		},
	}

	_, err := svc.ForwardNative(context.Background(), c, account,
		"gemini-3.8-flash", "generateContent", false, []byte(geminiCachedContentRequestBody))

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, 0, repo.tempCalls, "临时不可调度规则不应命中")
	require.Equal(t, 0, repo.setErrCalls)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestGeminiForwardNative_OtherForbiddenStillPenalizesAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"error":{"code":403,"message":"Your project has been denied access. Please contact support.","status":"PERMISSION_DENIED"}}`
	svc, _, repo := newGeminiCachedContentTestService(http.StatusForbidden, body)
	c, _ := newGeminiNativeTestContext(t)

	_, err := svc.ForwardNative(context.Background(), c, geminiDefaultPolicyAPIKeyAccount(),
		"gemini-3.8-flash", "generateContent", false, []byte(geminiCachedContentRequestBody))

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "其他 403 仍按账号问题换号")
	require.Equal(t, 1, repo.setErrCalls, "其他 403 仍标记账号错误")
}

// Vertex 对已删除 / 已过期缓存的响应（2026-10 实测）：生成请求 400，缓存资源接口 404，文案含上游缓存数字 ID。
const (
	geminiVertexCachedContentInvalidStateBody = `{"error":{"code":400,"message":"Invalid resource state for cache content 8079767076522164224.","status":"FAILED_PRECONDITION"}}`
	geminiVertexCachedContentNotFoundBody     = `{"error":{"code":404,"message":"Cached content 8079767076522164224 is not found.","status":"NOT_FOUND"}}`
)

func TestIsGeminiCachedContentNotFound(t *testing.T) {
	require.True(t, isGeminiCachedContentNotFound(http.StatusForbidden, []byte(geminiCachedContentNotFoundBody)))
	require.True(t, isGeminiCachedContentNotFound(http.StatusForbidden,
		[]byte(`[{"error":{"code":403,"message":"CachedContent not found (or permission denied)","status":"PERMISSION_DENIED"}}]`)))
	require.True(t, isGeminiCachedContentNotFound(http.StatusBadRequest, []byte(geminiVertexCachedContentInvalidStateBody)))
	require.True(t, isGeminiCachedContentNotFound(http.StatusNotFound, []byte(geminiVertexCachedContentNotFoundBody)))

	require.False(t, isGeminiCachedContentNotFound(http.StatusBadRequest, []byte(geminiCachedContentNotFoundBody)))
	require.False(t, isGeminiCachedContentNotFound(http.StatusForbidden,
		[]byte(`{"error":{"code":403,"message":"Permission denied: Consumer has been suspended.","status":"PERMISSION_DENIED"}}`)))
	require.False(t, isGeminiCachedContentNotFound(http.StatusNotFound,
		[]byte(`{"error":{"code":404,"message":"Publisher Model `+"`projects/p/locations/global/publishers/google/models/gemini-9`"+` was not found.","status":"NOT_FOUND"}}`)))
	require.False(t, isGeminiCachedContentNotFound(http.StatusBadRequest,
		[]byte(`{"error":{"code":400,"message":"Tool config, tools and system instruction should not be set in the request when using cached content.","status":"INVALID_ARGUMENT"}}`)))
	require.False(t, isGeminiCachedContentNotFound(http.StatusInternalServerError, []byte(geminiVertexCachedContentNotFoundBody)))
}

func TestGeminiForwardNative_VertexCachedContentNotFoundReturnsStandardForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		status int
		body   string
		action string
		stream bool
	}{
		{name: "generate invalid resource state", status: http.StatusBadRequest, body: geminiVertexCachedContentInvalidStateBody, action: "generateContent"},
		{name: "stream invalid resource state", status: http.StatusBadRequest, body: geminiVertexCachedContentInvalidStateBody, action: "streamGenerateContent", stream: true},
		{name: "generate not found", status: http.StatusNotFound, body: geminiVertexCachedContentNotFoundBody, action: "generateContent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, httpStub, repo := newGeminiCachedContentTestService(tc.status, tc.body)
			svc.tokenProvider = NewGeminiTokenProvider(nil, &fakeGeminiTokenCache{token: "ya29.test-token"}, nil)
			c, rec := newGeminiNativeTestContext(t)
			account := vertexServiceAccount()
			account.ID = 811

			result, err := svc.ForwardNative(context.Background(), c, account,
				"gemini-3.8-flash", tc.action, tc.stream,
				[]byte(`{"cachedContent":"projects/proj-1/locations/global/cachedContents/8079767076522164224","contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))

			require.Nil(t, result)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "缓存不存在是请求问题，不应换号")
			require.Equal(t, 0, repo.setErrCalls)
			require.Equal(t, 0, repo.tempCalls)
			require.Empty(t, repo.modelRateLimitCalls, "404 不应按模型不存在记模型限流")
			require.Equal(t, 1, httpStub.calls, "不应重试")
			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Equal(t, GeminiCachedContentNotFoundResponse, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "8079767076522164224", "不应暴露上游缓存 ID")
			require.True(t, IsOpsRequestScopedError(c), "缓存不存在按客户端请求错误统计")
			errType, _ := OpsLocalErrorType(c)
			require.Equal(t, "permission_error", errType)
		})
	}
}
