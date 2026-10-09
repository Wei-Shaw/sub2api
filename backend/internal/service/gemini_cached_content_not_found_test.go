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
			require.Equal(t, geminiCachedContentNotFoundBody, rec.Body.String(), "响应体应原样透传")
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

func TestIsGeminiCachedContentNotFound(t *testing.T) {
	require.True(t, isGeminiCachedContentNotFound(http.StatusForbidden, []byte(geminiCachedContentNotFoundBody)))
	require.True(t, isGeminiCachedContentNotFound(http.StatusForbidden,
		[]byte(`[{"error":{"code":403,"message":"CachedContent not found (or permission denied)","status":"PERMISSION_DENIED"}}]`)))
	require.False(t, isGeminiCachedContentNotFound(http.StatusBadRequest, []byte(geminiCachedContentNotFoundBody)))
	require.False(t, isGeminiCachedContentNotFound(http.StatusForbidden,
		[]byte(`{"error":{"code":403,"message":"Permission denied: Consumer has been suspended.","status":"PERMISSION_DENIED"}}`)))
}
