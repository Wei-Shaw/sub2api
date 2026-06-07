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

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 辅助函数：构造带 SingleAccountRetry 标记的 context
// ---------------------------------------------------------------------------

func ctxWithSingleAccountRetry() context.Context {
	return context.WithValue(context.Background(), ctxkey.SingleAccountRetry, true)
}

// ---------------------------------------------------------------------------
// 1. isSingleAccountRetry 测试
// ---------------------------------------------------------------------------

func TestIsSingleAccountRetry_True(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.SingleAccountRetry, true)
	require.True(t, isSingleAccountRetry(ctx))
}

func TestIsSingleAccountRetry_False_NoValue(t *testing.T) {
	require.False(t, isSingleAccountRetry(context.Background()))
}

func TestIsSingleAccountRetry_False_ExplicitFalse(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.SingleAccountRetry, false)
	require.False(t, isSingleAccountRetry(ctx))
}

func TestIsSingleAccountRetry_False_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.SingleAccountRetry, "true")
	require.False(t, isSingleAccountRetry(ctx))
}

// ---------------------------------------------------------------------------
// 2. 常量验证
// ---------------------------------------------------------------------------

func TestSingleAccountRetryConstants(t *testing.T) {
	require.Equal(t, 3, antigravitySingleAccountSmartRetryMaxAttempts,
		"单账号原地重试最多 3 次")
	require.Equal(t, 15*time.Second, antigravitySingleAccountSmartRetryMaxWait,
		"单次最大等待 15s")
	require.Equal(t, 30*time.Second, antigravitySingleAccountSmartRetryTotalMaxWait,
		"总累计等待不超过 30s")
}

// ---------------------------------------------------------------------------
// 3. handleSmartRetry + 503 + SingleAccountRetry → 走 handleSingleAccountRetryInPlace
//    （而非设模型限流 + 切换账号）
// ---------------------------------------------------------------------------

// TestHandleSmartRetry_503_LongDelay_SingleAccountRetry_RetryInPlace
// 核心场景：503 + retryDelay >= 7s + SingleAccountRetry 标记
// → 不设模型限流、不切换账号，改为原地重试
func TestHandleSmartRetry_503_LongDelay_SingleAccountRetry_RetryInPlace(t *testing.T) {
	// 原地重试成功
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"result":"ok"}`)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successResp},
		errors:    []error{nil},
	}

	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:          1,
		Name:        "acc-single",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Concurrency: 1,
	}

	// 503 + 39s >= 7s 阈值 + MODEL_CAPACITY_EXHAUSTED
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

	params := antigravityRetryLoopParams{
		ctx:          ctxWithSingleAccountRetry(), // 关键：设置单账号标记
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
	// 关键断言：返回 resp（原地重试成功），而非 switchError（切换账号）
	require.NotNil(t, result.resp, "should return successful response from in-place retry")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Nil(t, result.switchError, "should NOT return switchError in single account mode")
	require.Nil(t, result.err)

	// 验证未设模型限流（单账号模式不应设限流）
	require.Len(t, repo.modelRateLimitCalls, 0,
		"should NOT set model rate limit in single account retry mode")

	// 验证确实调用了 upstream（原地重试）
	require.GreaterOrEqual(t, len(upstream.calls), 1, "should have made at least one retry call")
}

// TestHandleSmartRetry_503_LongDelay_NoSingleAccountRetry_StillSwitches
// 对照组：503 + retryDelay >= 7s + 无 SingleAccountRetry 标记
// → 照常设模型限流 + 切换账号
func TestHandleSmartRetry_503_LongDelay_NoSingleAccountRetry_StillSwitches(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       2,
		Name:     "acc-multi",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 503 + 39s >= 7s 阈值（使用 RATE_LIMIT_EXCEEDED 而非 MODEL_CAPACITY_EXHAUSTED，
	// 因为 MODEL_CAPACITY_EXHAUSTED 走独立的重试路径，不触发 shouldRateLimitModel）
	respBody := []byte(`{
		"error": {
			"code": 503,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro-high"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "39s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:         context.Background(), // 关键：无单账号标记
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
	// 对照：多账号模式返回 switchError
	require.NotNil(t, result.switchError, "multi-account mode should return switchError for 503")
	require.Nil(t, result.resp, "should not return resp when switchError is set")

	// 对照：多账号模式应设模型限流
	require.Len(t, repo.modelRateLimitCalls, 1,
		"multi-account mode SHOULD set model rate limit")
}

// TestHandleSmartRetry_429_LongDelay_SingleAccountRetry_StillSwitches
// 边界情况：429（非 503）+ SingleAccountRetry 标记
// → 单账号原地重试仅针对 503，429 依然走切换账号逻辑
func TestHandleSmartRetry_429_LongDelay_SingleAccountRetry_StillSwitches(t *testing.T) {
	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       3,
		Name:     "acc-429",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 429 + 15s >= 7s 阈值
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
		StatusCode: http.StatusTooManyRequests, // 429，不是 503
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:         ctxWithSingleAccountRetry(), // 有单账号标记
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
	// 429 即使有单账号标记，也应走切换账号
	require.NotNil(t, result.switchError, "429 should still return switchError even with SingleAccountRetry")
	require.Len(t, repo.modelRateLimitCalls, 1,
		"429 should still set model rate limit even with SingleAccountRetry")
}

// ---------------------------------------------------------------------------
// 4. handleSmartRetry + 503 + 短延迟 + SingleAccountRetry → 智能重试耗尽后不设限流
// ---------------------------------------------------------------------------

// TestHandleSmartRetry_503_ShortDelay_SingleAccountRetry_NoRateLimit
// 503 + retryDelay < 7s + SingleAccountRetry → 智能重试耗尽后直接返回 503，不设限流
// 使用 RATE_LIMIT_EXCEEDED（走 1 次智能重试），避免 MODEL_CAPACITY_EXHAUSTED 的 60 次重试导致测试超时
func TestHandleSmartRetry_503_ShortDelay_SingleAccountRetry_NoRateLimit(t *testing.T) {
	// 智能重试也返回 503
	failRespBody := `{
		"error": {
			"code": 503,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`
	failResp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
	}
	upstream := &mockSmartRetryUpstream{
		responses:  []*http.Response{failResp},
		errors:     []error{nil},
		repeatLast: true,
	}

	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       4,
		Name:     "acc-short-503",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	// 0.1s < 7s 阈值
	respBody := []byte(`{
		"error": {
			"code": 503,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:          ctxWithSingleAccountRetry(),
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
	// 关键断言：单账号 503 模式下，智能重试耗尽后直接返回 503 响应，不切换
	require.NotNil(t, result.resp, "should return 503 response directly for single account mode")
	require.Equal(t, http.StatusServiceUnavailable, result.resp.StatusCode)
	require.Nil(t, result.switchError, "should NOT switch account in single account mode")

	// 关键断言：不设模型限流
	require.Len(t, repo.modelRateLimitCalls, 0,
		"should NOT set model rate limit for 503 in single account mode")
}

// TestHandleSmartRetry_503_ShortDelay_NoSingleAccountRetry_SetsRateLimit
// 对照组：503 + retryDelay < 7s + 无 SingleAccountRetry → 智能重试耗尽后照常设限流
// 使用 RATE_LIMIT_EXCEEDED 而非 MODEL_CAPACITY_EXHAUSTED，因为后者走独立的 60 次重试路径
func TestHandleSmartRetry_503_ShortDelay_NoSingleAccountRetry_SetsRateLimit(t *testing.T) {
	failRespBody := `{
		"error": {
			"code": 503,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`
	failResp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
	}
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{failResp},
		errors:    []error{nil},
	}

	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:       5,
		Name:     "acc-multi-503",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
	}

	respBody := []byte(`{
		"error": {
			"code": 503,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"}, "reason": "RATE_LIMIT_EXCEEDED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	params := antigravityRetryLoopParams{
		ctx:          context.Background(), // 无单账号标记
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
	// 对照：多账号模式应返回 switchError
	require.NotNil(t, result.switchError, "multi-account mode should return switchError for 503")
	// 对照：多账号模式应设模型限流
	require.Len(t, repo.modelRateLimitCalls, 1,
		"multi-account mode should set model rate limit")
}

// ---------------------------------------------------------------------------
// 5. handleSingleAccountRetryInPlace 直接测试
// ---------------------------------------------------------------------------

// TestHandleSingleAccountRetryInPlace_Success 原地重试成功
func TestHandleSingleAccountRetryInPlace_Success(t *testing.T) {
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
		ID:          10,
		Name:        "acc-inplace-ok",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Concurrency: 1,
	}

	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
	}

	params := antigravityRetryLoopParams{
		ctx:          ctxWithSingleAccountRetry(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"input":"test"}`),
		httpUpstream: upstream,
	}

	svc := &AntigravityGatewayService{}
	result := svc.handleSingleAccountRetryInPlace(params, resp, nil, "https://ag-1.test", 1*time.Second, "gemini-3-pro")

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp, "should return successful response")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Nil(t, result.switchError, "should not switch account on success")
	require.Nil(t, result.err)
}

// TestHandleSingleAccountRetryInPlace_AllRetriesFail 所有重试都失败，返回 503（不设限流）
func TestHandleSingleAccountRetryInPlace_AllRetriesFail(t *testing.T) {
	// 构造 3 个 503 响应（对应 3 次原地重试）
	var responses []*http.Response
	var errors []error
	for i := 0; i < antigravitySingleAccountSmartRetryMaxAttempts; i++ {
		responses = append(responses, &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     http.Header{},
			Body: io.NopCloser(strings.NewReader(`{
				"error": {
					"code": 503,
					"status": "UNAVAILABLE",
					"details": [
						{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro"}, "reason": "MODEL_CAPACITY_EXHAUSTED"},
						{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
					]
				}
			}`)),
		})
		errors = append(errors, nil)
	}
	upstream := &mockSmartRetryUpstream{
		responses: responses,
		errors:    errors,
	}

	account := &Account{
		ID:          11,
		Name:        "acc-inplace-fail",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Concurrency: 1,
	}

	origBody := []byte(`{"error":{"code":503,"status":"UNAVAILABLE"}}`)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"X-Test": {"original"}},
	}

	params := antigravityRetryLoopParams{
		ctx:          ctxWithSingleAccountRetry(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"input":"test"}`),
		httpUpstream: upstream,
	}

	svc := &AntigravityGatewayService{}
	result := svc.handleSingleAccountRetryInPlace(params, resp, origBody, "https://ag-1.test", 1*time.Second, "gemini-3-pro")

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	// 关键：返回 503 resp，不返回 switchError
	require.NotNil(t, result.resp, "should return 503 response directly")
	require.Equal(t, http.StatusServiceUnavailable, result.resp.StatusCode)
	require.Nil(t, result.switchError, "should NOT return switchError - let Handler handle it")
	require.Nil(t, result.err)

	// 验证确实重试了指定次数
	require.Len(t, upstream.calls, antigravitySingleAccountSmartRetryMaxAttempts,
		"should have made exactly maxAttempts retry calls")
}

// TestHandleSingleAccountRetryInPlace_WaitDurationClamped 等待时间被限制在 [min, max] 范围
func TestHandleSingleAccountRetryInPlace_WaitDurationClamped(t *testing.T) {
	// 用短延迟的成功响应，只验证不 panic
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
		ID:          12,
		Name:        "acc-clamp",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Concurrency: 1,
	}

	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
	}

	params := antigravityRetryLoopParams{
		ctx:          ctxWithSingleAccountRetry(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"input":"test"}`),
		httpUpstream: upstream,
	}

	svc := &AntigravityGatewayService{}

	// waitDuration=0 会被 clamp 到 antigravitySmartRetryMinWait=1s。
	// 首次重试即成功（200），总耗时 ~1s。
	result := svc.handleSingleAccountRetryInPlace(params, resp, nil, "https://ag-1.test", 0, "gemini-3-pro")
	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp)
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
}

// TestHandleSingleAccountRetryInPlace_ContextCanceled context 取消时立即返回
func TestHandleSingleAccountRetryInPlace_ContextCanceled(t *testing.T) {
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{nil},
		errors:    []error{nil},
	}

	account := &Account{
		ID:          13,
		Name:        "acc-cancel",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Concurrency: 1,
	}

	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, ctxkey.SingleAccountRetry, true)
	cancel() // 立即取消

	params := antigravityRetryLoopParams{
		ctx:          ctx,
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"input":"test"}`),
		httpUpstream: upstream,
	}

	svc := &AntigravityGatewayService{}
	result := svc.handleSingleAccountRetryInPlace(params, resp, nil, "https://ag-1.test", 1*time.Second, "gemini-3-pro")

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Error(t, result.err, "should return context error")
	// 不应调用 upstream（因为在等待阶段就被取消了）
	require.Len(t, upstream.calls, 0, "should not call upstream when context is canceled")
}

// TestHandleSingleAccountRetryInPlace_NetworkError_ContinuesRetry 网络错误时继续重试
func TestHandleSingleAccountRetryInPlace_NetworkError_ContinuesRetry(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"result":"ok"}`)),
	}
	upstream := &mockSmartRetryUpstream{
		// 第1次网络错误（nil resp），第2次成功
		responses: []*http.Response{nil, successResp},
		errors:    []error{nil, nil},
	}

	account := &Account{
		ID:          14,
		Name:        "acc-net-retry",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Concurrency: 1,
	}

	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
	}

	params := antigravityRetryLoopParams{
		ctx:          ctxWithSingleAccountRetry(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"input":"test"}`),
		httpUpstream: upstream,
	}

	svc := &AntigravityGatewayService{}
	result := svc.handleSingleAccountRetryInPlace(params, resp, nil, "https://ag-1.test", 1*time.Second, "gemini-3-pro")

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp, "should return successful response after network error recovery")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Len(t, upstream.calls, 2, "first call fails (network error), second succeeds")
}

// ---------------------------------------------------------------------------
// 6. antigravityRetryLoop 预检查：单账号模式跳过限流
// ---------------------------------------------------------------------------

// TestAntigravityRetryLoop_PreCheck_SingleAccountRetry_SkipsRateLimit
// 预检查中，如果有 SingleAccountRetry 标记，即使账号已限流也跳过直接发请求
func TestAntigravityRetryLoop_PreCheck_SingleAccountRetry_SkipsRateLimit(t *testing.T) {
	// 创建一个已设模型限流的账号
	upstream := &recordingOKUpstream{}
	account := &Account{
		ID:          20,
		Name:        "acc-rate-limited",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limit_reset_at": time.Now().Add(30 * time.Second).Format(time.RFC3339),
				},
			},
		},
	}

	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            ctxWithSingleAccountRetry(),
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"input":"test"}`),
		httpUpstream:   upstream,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})

	require.NoError(t, err, "should not return error")
	require.NotNil(t, result, "should return result")
	require.NotNil(t, result.resp, "should have response")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	// 关键：尽管限流了，有 SingleAccountRetry 标记时仍然到达了 upstream
	require.Equal(t, 1, upstream.calls, "should have reached upstream despite rate limit")
}

// TestAntigravityRetryLoop_PreCheck_NoSingleAccountRetry_SwitchesOnRateLimit
// 对照组：无 SingleAccountRetry + 已限流 → 预检查返回 switchError
func TestAntigravityRetryLoop_PreCheck_NoSingleAccountRetry_SwitchesOnRateLimit(t *testing.T) {
	upstream := &recordingOKUpstream{}
	account := &Account{
		ID:          21,
		Name:        "acc-rate-limited-multi",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limit_reset_at": time.Now().Add(30 * time.Second).Format(time.RFC3339),
				},
			},
		},
	}

	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:            context.Background(), // 无单账号标记
		prefix:         "[test]",
		account:        account,
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"input":"test"}`),
		httpUpstream:   upstream,
		requestedModel: "claude-sonnet-4-5",
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, requestedModel string, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
		},
	})

	require.Nil(t, result, "should not return result on rate limit switch")
	require.NotNil(t, err, "should return error")

	var switchErr *AntigravityAccountSwitchError
	require.ErrorAs(t, err, &switchErr, "should return AntigravityAccountSwitchError")
	require.Equal(t, account.ID, switchErr.OriginalAccountID)
	require.Equal(t, "claude-sonnet-4-5", switchErr.RateLimitedModel)

	// upstream 不应被调用（预检查就短路了）
	require.Equal(t, 0, upstream.calls, "upstream should NOT be called when pre-check blocks")
}

// ---------------------------------------------------------------------------
// 7. 端到端集成场景测试
// ---------------------------------------------------------------------------

// TestHandleSmartRetry_503_SingleAccount_RetryInPlace_ThenSuccess_E2E
// 端到端场景：503 + 单账号 + 原地重试第2次成功
func TestHandleSmartRetry_503_SingleAccount_RetryInPlace_ThenSuccess_E2E(t *testing.T) {
	// 第1次原地重试仍返回 503，第2次成功
	fail503Body := `{
		"error": {
			"code": 503,
			"status": "UNAVAILABLE",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro"}, "reason": "MODEL_CAPACITY_EXHAUSTED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"}
			]
		}
	}`
	resp503 := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(fail503Body)),
	}
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"result":"ok"}`)),
	}

	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{resp503, successResp},
		errors:    []error{nil, nil},
	}

	account := &Account{
		ID:          30,
		Name:        "acc-e2e",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Concurrency: 1,
	}

	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
	}

	params := antigravityRetryLoopParams{
		ctx:          ctxWithSingleAccountRetry(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"input":"test"}`),
		httpUpstream: upstream,
	}

	svc := &AntigravityGatewayService{}
	result := svc.handleSingleAccountRetryInPlace(params, resp, nil, "https://ag-1.test", 1*time.Second, "gemini-3-pro")

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp, "should return successful response after 2nd attempt")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Nil(t, result.switchError)
	require.Len(t, upstream.calls, 2, "first 503, second OK")
}

// TestAntigravityRetryLoop_503_SingleAccount_InPlaceRetryUsed_E2E
// 通过 antigravityRetryLoop → handleSmartRetry → handleSingleAccountRetryInPlace 完整链路
func TestAntigravityRetryLoop_503_SingleAccount_InPlaceRetryUsed_E2E(t *testing.T) {
	// 初始请求返回 503 + 长延迟
	initial503Body := []byte(`{
		"error": {
			"code": 503,
			"status": "UNAVAILABLE",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro"}, "reason": "MODEL_CAPACITY_EXHAUSTED"},
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "10s"}
			],
			"message": "No capacity available"
		}
	}`)
	initial503Resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(initial503Body)),
	}

	// 原地重试成功
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"result":"ok"}`)),
	}

	upstream := &mockSmartRetryUpstream{
		// 第1次调用（retryLoop 主循环）返回 503
		// 第2次调用（handleSingleAccountRetryInPlace 原地重试）返回 200
		responses: []*http.Response{initial503Resp, successResp},
		errors:    []error{nil, nil},
	}

	repo := &stubAntigravityAccountRepo{}
	account := &Account{
		ID:          31,
		Name:        "acc-e2e-loop",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
	}

	svc := &AntigravityGatewayService{}
	result, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:          ctxWithSingleAccountRetry(),
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
	})

	require.NoError(t, err, "should not return error on successful retry")
	require.NotNil(t, result, "should return result")
	require.NotNil(t, result.resp, "should return response")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)

	// 验证未设模型限流
	require.Len(t, repo.modelRateLimitCalls, 0,
		"should NOT set model rate limit in single account retry mode")
}
