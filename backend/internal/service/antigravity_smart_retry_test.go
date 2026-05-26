//go:build unit

package service

import (
	"bytes"
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubSmartRetryCache 用于 handleSmartRetry 测试的 GatewayCache mock
// 仅关注 DeleteSessionAccountID 的调用记录
type stubSmartRetryCache struct {
	GatewayCache // 嵌入接口，未实现的方法 panic（确保只调用预期方法）
	deleteCalls  []deleteSessionCall
}

type deleteSessionCall struct {
	groupID     int64
	sessionHash string
}

func (c *stubSmartRetryCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	c.deleteCalls = append(c.deleteCalls, deleteSessionCall{groupID: groupID, sessionHash: sessionHash})
	return nil
}

// mockSmartRetryUpstream 用于 handleSmartRetry 测试的 mock upstream
type mockSmartRetryUpstream struct {
	responses      []*http.Response
	responseBodies [][]byte // 缓存的 response body 字节（用于 repeatLast 重建）
	errors         []error
	callIdx        int
	calls          []string
	requestBodies  [][]byte
	repeatLast     bool // 超出范围时重复最后一个响应
}

func (m *mockSmartRetryUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	idx := m.callIdx
	m.calls = append(m.calls, req.URL.String())
	if req != nil && req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		m.requestBodies = append(m.requestBodies, body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	} else {
		m.requestBodies = append(m.requestBodies, nil)
	}
	m.callIdx++

	// 确定使用哪个索引
	respIdx := idx
	if respIdx >= len(m.responses) {
		if !m.repeatLast || len(m.responses) == 0 {
			return nil, nil
		}
		respIdx = len(m.responses) - 1
	}

	resp := m.responses[respIdx]
	respErr := m.errors[respIdx]
	if resp == nil {
		return nil, respErr
	}

	// 首次调用时缓存 body 字节
	if respIdx >= len(m.responseBodies) {
		for len(m.responseBodies) <= respIdx {
			m.responseBodies = append(m.responseBodies, nil)
		}
	}
	if m.responseBodies[respIdx] == nil && resp.Body != nil {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		m.responseBodies[respIdx] = bodyBytes
	}

	// 用缓存的 body 重建 reader（支持重试场景多次读取）
	cloned := *resp
	if m.responseBodies[respIdx] != nil {
		cloned.Body = io.NopCloser(bytes.NewReader(m.responseBodies[respIdx]))
	}
	return &cloned, respErr
}

func (m *mockSmartRetryUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return m.Do(req, proxyURL, accountID, accountConcurrency)
}

// TestHandleSmartRetry_URLLevelRateLimit 测试 URL 级别限流切换
func TestHandleSmartRetry_URLLevelRateLimit(t *testing.T) {
	account := &Account{
		ID:       1,
		Name:     "acc-1",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	respBody := []byte(`{"error":{"message":"Resource has been exhausted"}}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"}`),
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test", "https://ag-2.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionContinueURL, result.action)
	require.Nil(t, result.resp)
	require.Nil(t, result.err)
	require.Nil(t, result.switchError)
}

// TestHandleSmartRetry_LongDelay_ReturnsSwitchError 测试 retryDelay >= 阈值时返回 switchError
func TestHandleSmartRetry_LongDelay_ReturnsSwitchError(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       1,
		Name:     "acc-1",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 15s >= 7s 阈值，应该返回 switchError
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		accountRepo:     repo,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Nil(t, result.resp, "should not return resp when switchError is set")
	require.Nil(t, result.err)
	require.NotNil(t, result.switchError, "should return switchError for long delay")
	require.Equal(t, account.ID, result.switchError.OriginalAccountID)
	require.Equal(t, "claude-sonnet-4-5", result.switchError.RateLimitedModel)
	require.True(t, result.switchError.IsStickySession)

	// 验证模型限流已设置
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "claude-sonnet-4-5", repo.modelRateLimitCalls[0].modelKey)
}

// TestHandleSmartRetry_ShortDelay_SmartRetrySuccess 测试智能重试成功
func TestHandleSmartRetry_ShortDelay_SmartRetrySuccess(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"result":"ok"}`)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successResp},
		errors:    []error{nil},
	}

	account := &Account{
		ID:       1,
		Name:     "acc-1",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 0.5s < 7s 阈值，应该触发智能重试
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-opus-4"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"}
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
		body:         []byte(`{"input":"test"}`),
		httpUpstream: upstream,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp, "should return successful response")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Nil(t, result.err)
	require.Nil(t, result.switchError, "should not return switchError on success")
	require.Len(t, upstream.calls, 1, "should have made one retry call")
}

// TestHandleSmartRetry_ShortDelay_SmartRetryFailed_ReturnsSwitchError 测试智能重试失败后返回 switchError
func TestHandleSmartRetry_ShortDelay_SmartRetryFailed_ReturnsSwitchError(t *testing.T) {
	// 智能重试后仍然返回 429（需要提供 1 个响应，因为智能重试最多 1 次）
	failRespBody := `{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`
	failResp1 := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{failResp1},
		errors:    []error{nil},
	}

	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       2,
		Name:     "acc-2",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 3s < 7s 阈值，应该触发智能重试（最多 1 次）
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
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
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: false,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Nil(t, result.resp, "should not return resp when switchError is set")
	require.Nil(t, result.err)
	require.NotNil(t, result.switchError, "should return switchError after smart retry failed")
	require.Equal(t, account.ID, result.switchError.OriginalAccountID)
	require.Equal(t, "gemini-3-flash", result.switchError.RateLimitedModel)
	require.False(t, result.switchError.IsStickySession)

	// 验证模型限流已设置
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gemini-3-flash", repo.modelRateLimitCalls[0].modelKey)
	require.Len(t, upstream.calls, 1, "should have made one retry call (max attempts)")
}

// TestHandleSmartRetry_503_ModelCapacityExhausted_RetrySuccess 测试 503 MODEL_CAPACITY_EXHAUSTED 重试成功
// MODEL_CAPACITY_EXHAUSTED 使用固定 1s 间隔重试，不切换账号
func TestHandleSmartRetry_503_ModelCapacityExhausted_RetrySuccess(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       3,
		Name:     "acc-3",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 503 + MODEL_CAPACITY_EXHAUSTED + 39s（上游 retryDelay 应被忽略，使用固定 1s）
	respBody := []byte(`{
		"error": {
			"code": 503,
			"status": "UNAVAILABLE",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro-high"}, "reason": "MODEL_CAPACITY_EXHAUSTED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "39s"}
			],
			"message": "No capacity available for model gemini-3-pro-high on the server"
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	// mock: 第 1 次重试返回 200 成功
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{
			{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))},
		},
		errors: []error{nil},
	}

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		accountRepo:     repo,
		httpUpstream:    upstream,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp, "should return successful response")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Nil(t, result.err)
	require.Nil(t, result.switchError, "MODEL_CAPACITY_EXHAUSTED should not return switchError")

	// 不应设置模型限流
	require.Empty(t, repo.modelRateLimitCalls, "MODEL_CAPACITY_EXHAUSTED should not set model rate limit")
	require.Len(t, upstream.calls, 1, "should have made one retry call before success")
}

// TestHandleSmartRetry_503_ModelCapacityExhausted_ContextCancel 测试 MODEL_CAPACITY_EXHAUSTED 上下文取消
func TestHandleSmartRetry_503_ModelCapacityExhausted_ContextCancel(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       3,
		Name:     "acc-3",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	respBody := []byte(`{
		"error": {
			"code": 503,
			"status": "UNAVAILABLE",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro-high"}, "reason": "MODEL_CAPACITY_EXHAUSTED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "39s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	// 立即取消上下文，验证重试循环能正确退出
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	params := antigravityRetryLoopParams{
		ctx:         ctx,
		prefix:      "[test]",
		account:     account,
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"}`),
		accountRepo: repo,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, []string{"https://ag-1.test"})

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Error(t, result.err, "should return context error")
	require.Nil(t, result.switchError, "should not return switchError on context cancel")
	require.Empty(t, repo.modelRateLimitCalls, "should not set model rate limit on context cancel")
}

// TestHandleSmartRetry_NonAntigravityAccount_ContinuesDefaultLogic 测试非 Antigravity 平台账号走默认逻辑
func TestHandleSmartRetry_NonAntigravityAccount_ContinuesDefaultLogic(t *testing.T) {
	account := &Account{
		ID:       4,
		Name:     "acc-4",
		Type:     AccountTypeAPIKey, // 非 Antigravity 平台账号
		Platform: PlatformAnthropic,
	}

	// 即使是模型限流响应，非 OAuth 账号也应该走默认逻辑
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"}`),
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionContinue, result.action, "non-Antigravity platform account should continue default logic")
	require.Nil(t, result.resp)
	require.Nil(t, result.err)
	require.Nil(t, result.switchError)
}

// TestHandleSmartRetry_NonModelRateLimit_ContinuesDefaultLogic 测试非模型限流响应走默认逻辑
func TestHandleSmartRetry_NonModelRateLimit_ContinuesDefaultLogic(t *testing.T) {
	account := &Account{
		ID:       5,
		Name:     "acc-5",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 429 但没有 RATE_LIMIT_EXCEEDED 或 MODEL_CAPACITY_EXHAUSTED
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "5s"}
			],
			"message": "Quota exceeded"
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"}`),
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionContinue, result.action, "non-model rate limit should continue default logic")
	require.Nil(t, result.resp)
	require.Nil(t, result.err)
	require.Nil(t, result.switchError)
}

// TestHandleSmartRetry_ExactlyAtThreshold_ReturnsSwitchError 测试刚好等于阈值时返回 switchError
func TestHandleSmartRetry_ExactlyAtThreshold_ReturnsSwitchError(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       6,
		Name:     "acc-6",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 刚好 7s = 7s 阈值，应该返回 switchError
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-pro"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "7s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"}`),
		accountRepo: repo,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Nil(t, result.resp)
	require.NotNil(t, result.switchError, "exactly at threshold should return switchError")
	require.Equal(t, "gemini-pro", result.switchError.RateLimitedModel)
}

// TestAntigravityRetryLoop_HandleSmartRetry_SwitchError_Propagates 测试 switchError 正确传播到上层
func TestAntigravityRetryLoop_HandleSmartRetry_SwitchError_Propagates(t *testing.T) {
	// 模拟 429 + 长延迟的响应
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-opus-4-6"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "30s"}
			]
		}
	}`)
	rateLimitResp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{rateLimitResp},
		errors:    []error{nil},
	}

	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:          7,
		Name:        "acc-7",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
	}

	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})

	require.Nil(t, result, "should not return result when switchError")
	require.NotNil(t, err, "should return error")

	var switchErr *AntigravityAccountSwitchError
	require.ErrorAs(t, err, &switchErr, "error should be AntigravityAccountSwitchError")
	require.Equal(t, account.ID, switchErr.OriginalAccountID)
	require.Equal(t, "claude-opus-4-6", switchErr.RateLimitedModel)
	require.True(t, switchErr.IsStickySession)
}

// TestHandleSmartRetry_NetworkError_ExhaustsRetry 测试网络错误时（maxAttempts=1）直接耗尽重试并切换账号
func TestHandleSmartRetry_NetworkError_ExhaustsRetry(t *testing.T) {
	// 唯一一次重试遇到网络错误（nil response）
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{nil}, // 返回 nil（模拟网络错误）
		errors:    []error{nil},          // mock 不返回 error，靠 nil response 触发
	}

	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       8,
		Name:     "acc-8",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 0.1s < 7s 阈值，应该触发智能重试
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
		body:         []byte(`{"input":"test"}`),
		httpUpstream: upstream,
		accountRepo:  repo,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Nil(t, result.resp, "should not return resp when switchError is set")
	require.NotNil(t, result.switchError, "should return switchError after network error exhausted retry")
	require.Equal(t, account.ID, result.switchError.OriginalAccountID)
	require.Equal(t, "claude-sonnet-4-5", result.switchError.RateLimitedModel)
	require.Len(t, upstream.calls, 1, "should have made one retry call")

	// 验证模型限流已设置
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "claude-sonnet-4-5", repo.modelRateLimitCalls[0].modelKey)
}

// TestHandleSmartRetry_NoRetryDelay_UsesDefaultRateLimit 测试无 retryDelay 时使用默认 1 分钟限流
func TestHandleSmartRetry_NoRetryDelay_UsesDefaultRateLimit(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       9,
		Name:     "acc-9",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 429 + RATE_LIMIT_EXCEEDED + 无 retryDelay → 使用默认 1 分钟限流
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"}
			],
			"message": "You have exhausted your capacity on this model."
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		accountRepo:     repo,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Nil(t, result.resp, "should not return resp when switchError is set")
	require.NotNil(t, result.switchError, "should return switchError for no retryDelay")
	require.Equal(t, "claude-sonnet-4-5", result.switchError.RateLimitedModel)
	require.True(t, result.switchError.IsStickySession)

	// 验证模型限流已设置
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "claude-sonnet-4-5", repo.modelRateLimitCalls[0].modelKey)
}

// ---------------------------------------------------------------------------
// 以下测试覆盖本次改动：
// 1. antigravitySmartRetryMaxAttempts = 1（仅重试 1 次）
// 2. 智能重试失败后清除粘性会话绑定（DeleteSessionAccountID）
// ---------------------------------------------------------------------------

// TestSmartRetryMaxAttempts_VerifyConstant 验证常量值为 1
func TestSmartRetryMaxAttempts_VerifyConstant(t *testing.T) {
	require.Equal(t, 1, antigravitySmartRetryMaxAttempts,
		"antigravitySmartRetryMaxAttempts should be 1 to prevent repeated rate limiting")
}

// TestHandleSmartRetry_ShortDelay_StickySession_FailedRetry_ClearsSession
// 核心场景：粘性会话 + 短延迟重试失败 → 必须清除粘性绑定
func TestHandleSmartRetry_ShortDelay_StickySession_FailedRetry_ClearsSession(t *testing.T) {
	failRespBody := `{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`
	failResp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{failResp},
		errors:    []error{nil},
	}

	repo := &stubAntigravityAccountRepo{}
	cache := &stubSmartRetryCache{}
	account := &Account{
		ID:       10,
		Name:     "acc-10",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
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
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: true,
		groupID:         42,
		sessionHash:     "sticky-hash-abc",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{cache: cache}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	// 验证返回 switchError
	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.switchError)
	require.True(t, result.switchError.IsStickySession, "switchError should carry IsStickySession=true")
	require.Equal(t, account.ID, result.switchError.OriginalAccountID)

	// 核心断言：DeleteSessionAccountID 被调用，且参数正确
	require.Len(t, cache.deleteCalls, 1, "should call DeleteSessionAccountID exactly once")
	require.Equal(t, int64(42), cache.deleteCalls[0].groupID)
	require.Equal(t, "sticky-hash-abc", cache.deleteCalls[0].sessionHash)

	// 验证仅重试 1 次
	require.Len(t, upstream.calls, 1, "should make exactly 1 retry call (maxAttempts=1)")

	// 验证模型限流已设置
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "claude-sonnet-4-5", repo.modelRateLimitCalls[0].modelKey)
}

// TestHandleSmartRetry_ShortDelay_NonStickySession_FailedRetry_NoDeleteSession
// 非粘性会话 + 短延迟重试失败 → 不应调用 DeleteSessionAccountID（sessionHash 为空）
func TestHandleSmartRetry_ShortDelay_NonStickySession_FailedRetry_NoDeleteSession(t *testing.T) {
	failRespBody := `{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`
	failResp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{failResp},
		errors:    []error{nil},
	}

	repo := &stubAntigravityAccountRepo{}
	cache := &stubSmartRetryCache{}
	account := &Account{
		ID:       11,
		Name:     "acc-11",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
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
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: false,
		groupID:         42,
		sessionHash:     "", // 非粘性会话，sessionHash 为空
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{cache: cache}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.switchError)
	require.False(t, result.switchError.IsStickySession)

	// 核心断言：sessionHash 为空时不应调用 DeleteSessionAccountID
	require.Len(t, cache.deleteCalls, 0, "should NOT call DeleteSessionAccountID when sessionHash is empty")
}

// TestHandleSmartRetry_ShortDelay_StickySession_FailedRetry_NilCache_NoPanic
// 边界：cache 为 nil 时不应 panic
func TestHandleSmartRetry_ShortDelay_StickySession_FailedRetry_NilCache_NoPanic(t *testing.T) {
	failRespBody := `{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`
	failResp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{failResp},
		errors:    []error{nil},
	}

	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       12,
		Name:     "acc-12",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
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
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: true,
		groupID:         42,
		sessionHash:     "sticky-hash-nil-cache",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	// cache 为 nil，不应 panic
	svc := &AntigravityGatewayService{cache: nil}
	require.NotPanics(t, func() {
		result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)
		require.NotNil(t, result)
		require.Equal(t, smartRetryActionBreakWithResp, result.action)
		require.NotNil(t, result.switchError)
		require.True(t, result.switchError.IsStickySession)
	})
}

// TestHandleSmartRetry_ShortDelay_StickySession_SuccessRetry_NoDeleteSession
// 重试成功时不应清除粘性会话（只有失败才清除）
func TestHandleSmartRetry_ShortDelay_StickySession_SuccessRetry_NoDeleteSession(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"result":"ok"}`)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successResp},
		errors:    []error{nil},
	}

	cache := &stubSmartRetryCache{}
	account := &Account{
		ID:       13,
		Name:     "acc-13",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-opus-4"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		httpUpstream:    upstream,
		isStickySession: true,
		groupID:         42,
		sessionHash:     "sticky-hash-success",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{cache: cache}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp, "should return successful response")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Nil(t, result.switchError, "should not return switchError on success")

	// 核心断言：重试成功时不应清除粘性会话
	require.Len(t, cache.deleteCalls, 0, "should NOT call DeleteSessionAccountID on successful retry")
}

// TestHandleSmartRetry_LongDelay_StickySession_NoDeleteInHandleSmartRetry
// 长延迟路径（情况1）在 handleSmartRetry 中不直接调用 DeleteSessionAccountID
// （清除由 handler 层的 shouldClearStickySession 在下次请求时处理）
func TestHandleSmartRetry_LongDelay_StickySession_NoDeleteInHandleSmartRetry(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	cache := &stubSmartRetryCache{}
	account := &Account{
		ID:       14,
		Name:     "acc-14",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 15s >= 7s 阈值 → 走长延迟路径
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		accountRepo:     repo,
		isStickySession: true,
		groupID:         42,
		sessionHash:     "sticky-hash-long-delay",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{cache: cache}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.switchError)
	require.True(t, result.switchError.IsStickySession)

	// 长延迟路径不在 handleSmartRetry 中调用 DeleteSessionAccountID
	// （由上游 handler 的 shouldClearStickySession 处理）
	require.Len(t, cache.deleteCalls, 0,
		"long delay path should NOT call DeleteSessionAccountID in handleSmartRetry (handled by handler layer)")
}

// TestHandleSmartRetry_ShortDelay_NetworkError_StickySession_ClearsSession
// 网络错误耗尽重试 + 粘性会话 → 也应清除粘性绑定
func TestHandleSmartRetry_ShortDelay_NetworkError_StickySession_ClearsSession(t *testing.T) {
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{nil}, // 网络错误
		errors:    []error{nil},
	}

	repo := &stubAntigravityAccountRepo{}
	cache := &stubSmartRetryCache{}
	account := &Account{
		ID:       15,
		Name:     "acc-15",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
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
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: true,
		groupID:         99,
		sessionHash:     "sticky-net-error",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{cache: cache}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.NotNil(t, result.switchError)
	require.True(t, result.switchError.IsStickySession)

	// 核心断言：网络错误耗尽重试后也应清除粘性绑定
	require.Len(t, cache.deleteCalls, 1, "should call DeleteSessionAccountID after network error exhausts retry")
	require.Equal(t, int64(99), cache.deleteCalls[0].groupID)
	require.Equal(t, "sticky-net-error", cache.deleteCalls[0].sessionHash)
}

// TestHandleSmartRetry_ShortDelay_503_StickySession_FailedRetry_ClearsSession
// 429 + 短延迟 + 粘性会话 + 重试失败 → 清除粘性绑定
func TestHandleSmartRetry_ShortDelay_503_StickySession_FailedRetry_ClearsSession(t *testing.T) {
	failRespBody := `{
		"error": {
			"code": 429,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"}
			]
		}
	}`
	failResp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{failResp},
		errors:    []error{nil},
	}

	repo := &stubAntigravityAccountRepo{}
	cache := &stubSmartRetryCache{}
	account := &Account{
		ID:       16,
		Name:     "acc-16",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	respBody := []byte(`{
		"error": {
			"code": 429,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: true,
		groupID:         77,
		sessionHash:     "sticky-503-short",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	}

	availableURLs := []string{"https://ag-1.test"}

	svc := &AntigravityGatewayService{cache: cache}
	result := svc.handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.NotNil(t, result.switchError)
	require.True(t, result.switchError.IsStickySession)

	// 验证粘性绑定被清除
	require.Len(t, cache.deleteCalls, 1)
	require.Equal(t, int64(77), cache.deleteCalls[0].groupID)
	require.Equal(t, "sticky-503-short", cache.deleteCalls[0].sessionHash)

	// 验证模型限流已设置
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gemini-3-pro", repo.modelRateLimitCalls[0].modelKey)
}

// TestAntigravityRetryLoop_SmartRetryFailed_StickySession_SwitchErrorPropagates
// 集成测试：antigravityRetryLoop → handleSmartRetry → switchError 传播
// 验证 IsStickySession 正确传递到上层，且粘性绑定被清除
func TestAntigravityRetryLoop_SmartRetryFailed_StickySession_SwitchErrorPropagates(t *testing.T) {
	// 初始 429 响应
	initialRespBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-opus-4-6"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`)
	initialResp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(initialRespBody)),
	}

	// 智能重试也返回 429
	retryRespBody := `{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-opus-4-6"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`
	retryResp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(retryRespBody)),
	}

	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{initialResp, retryResp},
		errors:    []error{nil, nil},
	}

	repo := &stubAntigravityAccountRepo{}
	cache := &stubSmartRetryCache{}
	account := &Account{
		ID:          17,
		Name:        "acc-17",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
	}

	svc := &AntigravityGatewayService{cache: cache}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"}`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: true,
		groupID:         55,
		sessionHash:     "sticky-loop-test",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})

	require.Nil(t, result, "should not return result when switchError")
	require.NotNil(t, err, "should return error")

	var switchErr *AntigravityAccountSwitchError
	require.ErrorAs(t, err, &switchErr, "error should be AntigravityAccountSwitchError")
	require.Equal(t, account.ID, switchErr.OriginalAccountID)
	require.Equal(t, "claude-opus-4-6", switchErr.RateLimitedModel)
	require.True(t, switchErr.IsStickySession, "IsStickySession must propagate through retryLoop")

	// 验证粘性绑定被清除
	require.Len(t, cache.deleteCalls, 1, "should clear sticky session in handleSmartRetry")
	require.Equal(t, int64(55), cache.deleteCalls[0].groupID)
	require.Equal(t, "sticky-loop-test", cache.deleteCalls[0].sessionHash)
}
