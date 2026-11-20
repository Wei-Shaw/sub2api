//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAntigravityForwardBaseURLs(t *testing.T) {
	dailyURL := "https://daily-cloudcode-pa.googleapis.com"
	prodURL := "https://cloudcode-pa.googleapis.com"

	origBaseURLs := antigravity.BaseURLs
	antigravity.BaseURLs = []string{prodURL, dailyURL}
	defer func() {
		antigravity.BaseURLs = origBaseURLs
	}()

	t.Run("Paid pro account returns daily only", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "")
		account := &Account{
			ID:       1,
			Platform: PlatformAntigravity,
			Credentials: map[string]any{
				"plan_type": "pro",
			},
		}

		urls := resolveAntigravityForwardBaseURLs(account)
		require.Len(t, urls, 1)
		assert.Equal(t, dailyURL, urls[0])
	})

	t.Run("Paid ultra account returns daily only", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "")
		account := &Account{
			ID:       2,
			Platform: PlatformAntigravity,
			Credentials: map[string]any{
				"plan_type": "ultra",
			},
		}

		urls := resolveAntigravityForwardBaseURLs(account)
		require.Len(t, urls, 1)
		assert.Equal(t, dailyURL, urls[0])
	})

	t.Run("Google One g1-pro-tier account returns daily only", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "")
		account := &Account{
			ID:       3,
			Platform: PlatformAntigravity,
			Credentials: map[string]any{
				"plan_type": "g1-pro-tier",
			},
		}

		urls := resolveAntigravityForwardBaseURLs(account)
		require.Len(t, urls, 1)
		assert.Equal(t, dailyURL, urls[0])
	})

	t.Run("Google One g1-ultra-lite-tier account returns daily only", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "")
		account := &Account{
			ID:       4,
			Platform: PlatformAntigravity,
			Credentials: map[string]any{
				"plan_type": "g1-ultra-lite-tier",
			},
		}

		urls := resolveAntigravityForwardBaseURLs(account)
		require.Len(t, urls, 1)
		assert.Equal(t, dailyURL, urls[0])
	})

	t.Run("Paid account with daily override still returns daily only", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "daily")
		account := &Account{
			ID:       5,
			Platform: PlatformAntigravity,
			Credentials: map[string]any{
				"plan_type": "g1-pro-tier",
			},
		}

		urls := resolveAntigravityForwardBaseURLs(account)
		require.Len(t, urls, 1)
		assert.Equal(t, dailyURL, urls[0])
	})

	t.Run("Free account returns prod only", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "")
		account := &Account{
			ID:       6,
			Platform: PlatformAntigravity,
			Credentials: map[string]any{
				"plan_type": "free",
			},
		}

		urls := resolveAntigravityForwardBaseURLs(account)
		require.Len(t, urls, 1)
		assert.Equal(t, prodURL, urls[0])
	})

	t.Run("Environment override daily for free account", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "daily")

		account := &Account{
			ID:       7,
			Platform: PlatformAntigravity,
			Credentials: map[string]any{
				"plan_type": "free",
			},
		}

		urls := resolveAntigravityForwardBaseURLs(account)
		require.Len(t, urls, 2)
		assert.Equal(t, dailyURL, urls[0])
		assert.Equal(t, prodURL, urls[1])
	})

	t.Run("Environment override prod", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "prod")

		account := &Account{
			ID:       8,
			Platform: PlatformAntigravity,
			Credentials: map[string]any{
				"plan_type": "pro",
			},
		}

		urls := resolveAntigravityForwardBaseURLs(account)
		require.Len(t, urls, 1)
		assert.Equal(t, prodURL, urls[0])
	})
}

func TestAntigravityRetryLoop_URLFallbackOn404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dailyURL := "https://daily-cloudcode-pa.googleapis.com"
	prodURL := "https://cloudcode-pa.googleapis.com"
	origBaseURLs := antigravity.BaseURLs
	antigravity.BaseURLs = []string{prodURL, dailyURL}
	defer func() {
		antigravity.BaseURLs = origBaseURLs
	}()

	t.Run("Free account with daily override falls back from daily to prod on 404", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "daily")
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		mockUpstream := &mockSmartRetryUpstream{
			responses: []*http.Response{
				{
					StatusCode: http.StatusNotFound,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"code":404,"message":"Requested entity was not found."}}`))),
				},
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`))),
				},
			},
			errors: []error{nil, nil},
		}

		svc := &AntigravityGatewayService{}
		account := &Account{
			ID:       10,
			Platform: PlatformAntigravity,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"plan_type": "free",
			},
		}

		params := antigravityRetryLoopParams{
			ctx:            context.Background(),
			prefix:         "[test]",
			account:        account,
			accessToken:    "test-token",
			action:         "v1internal:generateContent",
			body:           []byte(`{"model":"gemini-3.7-flash"}`),
			c:              c,
			httpUpstream:   mockUpstream,
			requestedModel: "gemini-3.7-flash",
			handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
				return nil
			},
		}

		result, err := svc.antigravityRetryLoop(params)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.resp)
		assert.Equal(t, http.StatusOK, result.resp.StatusCode)
		require.Len(t, mockUpstream.calls, 2)
		assert.True(t, strings.HasPrefix(mockUpstream.calls[0], dailyURL))
		assert.True(t, strings.HasPrefix(mockUpstream.calls[1], prodURL))
	})

	t.Run("Paid g1-pro-tier account does not fall back to prod on 404", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "")
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		mockUpstream := &mockSmartRetryUpstream{
			responses: []*http.Response{
				{
					StatusCode: http.StatusNotFound,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"code":404,"message":"Requested entity was not found."}}`))),
				},
				{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"candidates":[{"content":{"parts":[{"text":"unexpected"}]}}]}`))),
				},
			},
			errors: []error{nil, nil},
		}

		svc := &AntigravityGatewayService{}
		account := &Account{
			ID:       11,
			Platform: PlatformAntigravity,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"plan_type": "g1-pro-tier",
			},
		}

		params := antigravityRetryLoopParams{
			ctx:            context.Background(),
			prefix:         "[test]",
			account:        account,
			accessToken:    "test-token",
			action:         "v1internal:generateContent",
			body:           []byte(`{"model":"gemini-3.7-flash"}`),
			c:              c,
			httpUpstream:   mockUpstream,
			requestedModel: "gemini-3.7-flash",
			handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
				return nil
			},
		}

		result, err := svc.antigravityRetryLoop(params)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.resp)
		assert.Equal(t, http.StatusNotFound, result.resp.StatusCode)
		require.Len(t, mockUpstream.calls, 1)
		assert.True(t, strings.HasPrefix(mockUpstream.calls[0], dailyURL))
	})

	t.Run("Paid pro account does not fall back to prod on URL-level 429", func(t *testing.T) {
		t.Setenv("GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL", "")
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		mockUpstream := &mockSmartRetryUpstream{
			responses: []*http.Response{
				{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"code":429,"message":"Resource has been exhausted"}}`))),
				},
			},
			errors:     []error{nil},
			repeatLast: true,
		}

		var handleErrorCalled bool
		svc := &AntigravityGatewayService{}
		account := &Account{
			ID:       12,
			Platform: PlatformAntigravity,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"plan_type": "pro",
			},
		}

		params := antigravityRetryLoopParams{
			ctx:            context.Background(),
			prefix:         "[test]",
			account:        account,
			accessToken:    "test-token",
			action:         "v1internal:generateContent",
			body:           []byte(`{"model":"gemini-3.7-flash"}`),
			c:              c,
			httpUpstream:   mockUpstream,
			requestedModel: "gemini-3.7-flash",
			handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
				handleErrorCalled = true
				return nil
			},
		}

		result, err := svc.antigravityRetryLoop(params)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.resp)
		assert.Equal(t, http.StatusTooManyRequests, result.resp.StatusCode)
		assert.True(t, handleErrorCalled)
		require.Len(t, mockUpstream.calls, antigravityMaxRetries)
		for _, callURL := range mockUpstream.calls {
			assert.True(t, strings.HasPrefix(callURL, dailyURL))
			assert.False(t, strings.HasPrefix(callURL, prodURL))
		}
	})
}
