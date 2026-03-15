//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestClassifyAntigravity429(t *testing.T) {
	t.Run("明确配额耗尽", func(t *testing.T) {
		body := []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"QUOTA_EXHAUSTED"}}`)
		require.Equal(t, antigravity429QuotaExhausted, classifyAntigravity429(body))
	})

	t.Run("结构化限流", func(t *testing.T) {
		body := []byte(`{
			"error": {
				"status": "RESOURCE_EXHAUSTED",
				"details": [
					{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
					{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"}
				]
			}
		}`)
		require.Equal(t, antigravity429RateLimited, classifyAntigravity429(body))
	})

	t.Run("未知429", func(t *testing.T) {
		body := []byte(`{"error":{"message":"too many requests"}}`)
		require.Equal(t, antigravity429Unknown, classifyAntigravity429(body))
	})
}

func TestIsCreditsExhausted_UsesAICreditsKey(t *testing.T) {
	t.Run("无 AICredits key 则积分可用", func(t *testing.T) {
		account := &Account{
			ID:       1,
			Platform: PlatformAntigravity,
			Extra: map[string]any{
				"allow_overages": true,
			},
		}
		require.False(t, account.isCreditsExhausted())
	})

	t.Run("AICredits key 生效则积分耗尽", func(t *testing.T) {
		account := &Account{
			ID:       2,
			Platform: PlatformAntigravity,
			Extra: map[string]any{
				"allow_overages": true,
				modelRateLimitsKey: map[string]any{
					creditsExhaustedKey: map[string]any{
						"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
						"rate_limit_reset_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339),
					},
				},
			},
		}
		require.True(t, account.isCreditsExhausted())
	})

	t.Run("AICredits key 过期则积分可用", func(t *testing.T) {
		account := &Account{
			ID:       3,
			Platform: PlatformAntigravity,
			Extra: map[string]any{
				"allow_overages": true,
				modelRateLimitsKey: map[string]any{
					creditsExhaustedKey: map[string]any{
						"rate_limited_at":     time.Now().Add(-6 * time.Hour).UTC().Format(time.RFC3339),
						"rate_limit_reset_at": time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
					},
				},
			},
		}
		require.False(t, account.isCreditsExhausted())
	})
}

func TestHandleSmartRetry_QuotaExhausted_UsesCreditsAndStoresIndependentState(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successResp},
		errors:    []error{nil},
	}
	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       101,
		Name:     "acc-101",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			"allow_overages": true,
		},
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"claude-opus-4-6": "claude-sonnet-4-5",
			},
		},
	}

	respBody := []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"QUOTA_EXHAUSTED"}}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}
	params := antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-opus-4-6","request":{}}`),
		httpUpstream:   upstream,
		accountRepo:    repo,
		requestedModel: "claude-opus-4-6",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, []string{"https://ag-1.test"})

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp)
	require.Nil(t, result.switchError)
	require.Len(t, upstream.requestBodies, 1)
	require.Contains(t, string(upstream.requestBodies[0]), "enabledCreditTypes")
	require.Empty(t, repo.modelRateLimitCalls, "overages 成功后不应写入普通 model_rate_limits")
}

func TestHandleSmartRetry_RateLimited_DoesNotUseCredits(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successResp},
		errors:    []error{nil},
	}
	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       102,
		Name:     "acc-102",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			"allow_overages": true,
		},
	}

	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}
	params := antigravityRetryLoopParams{
		ctx:          context.Background(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"model":"claude-sonnet-4-5","request":{}}`),
		httpUpstream: upstream,
		accountRepo:  repo,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, []string{"https://ag-1.test"})

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp)
	require.Len(t, upstream.requestBodies, 1)
	require.NotContains(t, string(upstream.requestBodies[0]), "enabledCreditTypes")
	require.Empty(t, repo.extraUpdateCalls)
	require.Empty(t, repo.modelRateLimitCalls)
}

func TestAntigravityRetryLoop_ModelRateLimited_InjectsCredits(t *testing.T) {
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	}()

	antigravity.BaseURLs = []string{"https://ag-1.test"}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			},
		},
		errors: []error{nil},
	}
	// 模型已限流 + overages 启用 + 无 AICredits key → 应直接注入积分
	account := &Account{
		ID:          103,
		Name:        "acc-103",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"allow_overages": true,
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
					"rate_limit_reset_at": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
				},
			},
		},
	}

	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-sonnet-4-5","request":{}}`),
		httpUpstream:   upstream,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)
	require.Contains(t, string(upstream.requestBodies[0]), "enabledCreditTypes")
}

func TestAntigravityRetryLoop_CreditsExhausted_DoesNotInject(t *testing.T) {
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	}()

	antigravity.BaseURLs = []string{"https://ag-1.test"}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	// 模型限流 + overages 启用 + AICredits key 生效 → 不应注入积分，应切号
	account := &Account{
		ID:          104,
		Name:        "acc-104",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"allow_overages": true,
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
					"rate_limit_reset_at": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
				},
				creditsExhaustedKey: map[string]any{
					"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
					"rate_limit_reset_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339),
				},
			},
		},
	}

	svc := &AntigravityGatewayService{}
	_, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-sonnet-4-5","request":{}}`),
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})

	// 模型限流 + 积分耗尽 → 应触发切号错误
	require.Error(t, err)
	var switchErr *AntigravityAccountSwitchError
	require.ErrorAs(t, err, &switchErr)
}

func TestAntigravityRetryLoop_CreditErrorMarksExhausted(t *testing.T) {
	oldBaseURLs := append([]string(nil), antigravity.BaseURLs...)
	oldAvailability := antigravity.DefaultURLAvailability
	defer func() {
		antigravity.BaseURLs = oldBaseURLs
		antigravity.DefaultURLAvailability = oldAvailability
	}()

	antigravity.BaseURLs = []string{"https://ag-1.test"}
	antigravity.DefaultURLAvailability = antigravity.NewURLAvailability(time.Minute)

	repo := &stubAntigravityAccountRepo{}
	upstream := &queuedHTTPUpstreamStub{
		responses: []*http.Response{
			{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Insufficient GOOGLE_ONE_AI credits"}}`)),
			},
		},
		errors: []error{nil},
	}
	// 模型限流 + overages 启用 + 积分可用 → 注入积分但上游返回积分不足
	account := &Account{
		ID:          105,
		Name:        "acc-105",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"allow_overages": true,
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limited_at":     time.Now().UTC().Format(time.RFC3339),
					"rate_limit_reset_at": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
				},
			},
		},
	}

	svc := &AntigravityGatewayService{accountRepo: repo}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"claude-sonnet-4-5","request":{}}`),
		httpUpstream:   upstream,
		accountRepo:    repo,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	// 验证 AICredits key 已通过 SetModelRateLimit 写入数据库
	require.Len(t, repo.modelRateLimitCalls, 1, "应通过 SetModelRateLimit 写入 AICredits key")
	require.Equal(t, creditsExhaustedKey, repo.modelRateLimitCalls[0].modelKey)
}
