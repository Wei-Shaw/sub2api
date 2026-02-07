//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockSmartRetryUpstream 用于 handleSmartRetry 测试的 mock upstream
type mockSmartRetryUpstream struct {
	responses []*http.Response
	errors    []error
	callIdx   int
	calls     []string
REDACTED

func (m *mockSmartRetryUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	idx := m.callIdx
	m.calls = append(m.calls, req.URL.String())
	m.callIdx++
	if idx < len(m.responses) {
		return m.responses[idx], m.errors[idx]
REDACTED
	return nil, nil
REDACTED

func (m *mockSmartRetryUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, enableTLSFingerprint bool) (*http.Response, error) {
	return m.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED

// TestHandleSmartRetry_URLLevelRateLimit 测试 URL 级别限流切换
func TestHandleSmartRetry_URLLevelRateLimit(t *testing.T) {
	account := &Account{
		ID:       1,
		Name:     "acc-1",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
REDACTED

	respBody := []byte(`{"error":{"message":"Resource has been exhausted"REDACTEDREDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"REDACTED`),
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test", "https://ag-2.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionContinueURL, result.action)
	require.Nil(t, result.resp)
	require.Nil(t, result.err)
	require.Nil(t, result.switchError)
REDACTED

// TestHandleSmartRetry_LongDelay_ReturnsSwitchError 测试 retryDelay >= 阈值时返回 switchError
func TestHandleSmartRetry_LongDelay_ReturnsSwitchError(t *testing.T) {
	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:       1,
		Name:     "acc-1",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
REDACTED

	// 15s >= 7s 阈值，应该返回 switchError
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"REDACTED
			]
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"REDACTED`),
		accountRepo:     repo,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

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
REDACTED

// TestHandleSmartRetry_ShortDelay_SmartRetrySuccess 测试智能重试成功
func TestHandleSmartRetry_ShortDelay_SmartRetrySuccess(t *testing.T) {
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"result":"ok"REDACTED`)),
REDACTED
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{successRespREDACTED,
		errors:    []error{nilREDACTED,
REDACTED

	account := &Account{
		ID:       1,
		Name:     "acc-1",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
REDACTED

	// 0.5s < 7s 阈值，应该触发智能重试
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-opus-4"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.5s"REDACTED
			]
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:          context.Background(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"input":"test"REDACTED`),
		httpUpstream: upstream,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp, "should return successful response")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Nil(t, result.err)
	require.Nil(t, result.switchError, "should not return switchError on success")
	require.Len(t, upstream.calls, 1, "should have made one retry call")
REDACTED

// TestHandleSmartRetry_ShortDelay_SmartRetryFailed_ReturnsSwitchError 测试智能重试失败后返回 switchError
func TestHandleSmartRetry_ShortDelay_SmartRetryFailed_ReturnsSwitchError(t *testing.T) {
	// 智能重试后仍然返回 429（需要提供 3 个响应，因为智能重试最多 3 次）
	failRespBody := `{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"REDACTED
			]
	REDACTED
REDACTED`
	failResp1 := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
REDACTED
	failResp2 := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
REDACTED
	failResp3 := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader(failRespBody)),
REDACTED
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{failResp1, failResp2, failResp3REDACTED,
		errors:    []error{nil, nil, nilREDACTED,
REDACTED

	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:       2,
		Name:     "acc-2",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
REDACTED

	// 3s < 7s 阈值，应该触发智能重试（最多 3 次）
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-flash"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"REDACTED
			]
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"REDACTED`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: false,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

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
	require.Len(t, upstream.calls, 3, "should have made three retry calls (max attempts)")
REDACTED

// TestHandleSmartRetry_503_ModelCapacityExhausted_ReturnsSwitchError 测试 503 MODEL_CAPACITY_EXHAUSTED 返回 switchError
func TestHandleSmartRetry_503_ModelCapacityExhausted_ReturnsSwitchError(t *testing.T) {
	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:       3,
		Name:     "acc-3",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
REDACTED

	// 503 + MODEL_CAPACITY_EXHAUSTED + 39s >= 7s 阈值
	respBody := []byte(`{
		"error": {
			"code": 503,
			"status": "UNAVAILABLE",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-3-pro-high"REDACTED, "reason": "MODEL_CAPACITY_EXHAUSTED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "39s"REDACTED
			],
			"message": "No capacity available for model gemini-3-pro-high on the server"
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"REDACTED`),
		accountRepo:     repo,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Nil(t, result.resp)
	require.Nil(t, result.err)
	require.NotNil(t, result.switchError, "should return switchError for 503 model capacity exhausted")
	require.Equal(t, account.ID, result.switchError.OriginalAccountID)
	require.Equal(t, "gemini-3-pro-high", result.switchError.RateLimitedModel)
	require.True(t, result.switchError.IsStickySession)

	// 验证模型限流已设置
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gemini-3-pro-high", repo.modelRateLimitCalls[0].modelKey)
REDACTED

// TestHandleSmartRetry_NonOAuthAccount_ContinuesDefaultLogic 测试非 OAuth 账号走默认逻辑
func TestHandleSmartRetry_NonOAuthAccount_ContinuesDefaultLogic(t *testing.T) {
	account := &Account{
		ID:       4,
		Name:     "acc-4",
		Type:     AccountTypeAPIKey, // 非 OAuth 账号
		Platform: PlatformAntigravity,
REDACTED

	// 即使是模型限流响应，非 OAuth 账号也应该走默认逻辑
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "15s"REDACTED
			]
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"REDACTED`),
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionContinue, result.action, "non-OAuth account should continue default logic")
	require.Nil(t, result.resp)
	require.Nil(t, result.err)
	require.Nil(t, result.switchError)
REDACTED

// TestHandleSmartRetry_NonModelRateLimit_ContinuesDefaultLogic 测试非模型限流响应走默认逻辑
func TestHandleSmartRetry_NonModelRateLimit_ContinuesDefaultLogic(t *testing.T) {
	account := &Account{
		ID:       5,
		Name:     "acc-5",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
REDACTED

	// 429 但没有 RATE_LIMIT_EXCEEDED 或 MODEL_CAPACITY_EXHAUSTED
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "5s"REDACTED
			],
			"message": "Quota exceeded"
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"REDACTED`),
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionContinue, result.action, "non-model rate limit should continue default logic")
	require.Nil(t, result.resp)
	require.Nil(t, result.err)
	require.Nil(t, result.switchError)
REDACTED

// TestHandleSmartRetry_ExactlyAtThreshold_ReturnsSwitchError 测试刚好等于阈值时返回 switchError
func TestHandleSmartRetry_ExactlyAtThreshold_ReturnsSwitchError(t *testing.T) {
	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:       6,
		Name:     "acc-6",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
REDACTED

	// 刚好 7s = 7s 阈值，应该返回 switchError
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "gemini-pro"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "7s"REDACTED
			]
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:         context.Background(),
		prefix:      "[test]",
		account:     account,
		accessToken: "token",
		action:      "generateContent",
		body:        []byte(`{"input":"test"REDACTED`),
		accountRepo: repo,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Nil(t, result.resp)
	require.NotNil(t, result.switchError, "exactly at threshold should return switchError")
	require.Equal(t, "gemini-pro", result.switchError.RateLimitedModel)
REDACTED

// TestAntigravityRetryLoop_HandleSmartRetry_SwitchError_Propagates 测试 switchError 正确传播到上层
func TestAntigravityRetryLoop_HandleSmartRetry_SwitchError_Propagates(t *testing.T) {
	// 模拟 429 + 长延迟的响应
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-opus-4-6"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "30s"REDACTED
			]
	REDACTED
REDACTED`)
	rateLimitResp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{rateLimitRespREDACTED,
		errors:    []error{nilREDACTED,
REDACTED

	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:          7,
		Name:        "acc-7",
		Type:        AccountTypeOAuth,
		Platform:    PlatformAntigravity,
		Schedulable: true,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED

	result, err := antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"REDACTED`),
		httpUpstream:    upstream,
		accountRepo:     repo,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED)

	require.Nil(t, result, "should not return result when switchError")
	require.NotNil(t, err, "should return error")

	var switchErr *AntigravityAccountSwitchError
	require.ErrorAs(t, err, &switchErr, "error should be AntigravityAccountSwitchError")
	require.Equal(t, account.ID, switchErr.OriginalAccountID)
	require.Equal(t, "claude-opus-4-6", switchErr.RateLimitedModel)
	require.True(t, switchErr.IsStickySession)
REDACTED

// TestHandleSmartRetry_NetworkError_ContinuesRetry 测试网络错误时继续重试
func TestHandleSmartRetry_NetworkError_ContinuesRetry(t *testing.T) {
	// 第一次网络错误，第二次成功
	successResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"result":"ok"REDACTED`)),
REDACTED
	upstream := &mockSmartRetryUpstream{
		responses: []*http.Response{nil, successRespREDACTED, // 第一次返回 nil（模拟网络错误）
		errors:    []error{nil, nilREDACTED,                  // mock 不返回 error，靠 nil response 触发
REDACTED

	account := &Account{
		ID:       8,
		Name:     "acc-8",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
REDACTED

	// 0.1s < 7s 阈值，应该触发智能重试
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED,
				{"@type": "type.googleapis.com/google.rpc.RetryInfo", "retryDelay": "0.1s"REDACTED
			]
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:          context.Background(),
		prefix:       "[test]",
		account:      account,
		accessToken:  "token",
		action:       "generateContent",
		body:         []byte(`{"input":"test"REDACTED`),
		httpUpstream: upstream,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.NotNil(t, result.resp, "should return successful response after network error recovery")
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Nil(t, result.switchError, "should not return switchError on success")
	require.Len(t, upstream.calls, 2, "should have made two retry calls")
REDACTED

// TestHandleSmartRetry_NoRetryDelay_UsesDefaultRateLimit 测试无 retryDelay 时使用默认 1 分钟限流
func TestHandleSmartRetry_NoRetryDelay_UsesDefaultRateLimit(t *testing.T) {
	repo := &stubAntigravityAccountRepo{REDACTED
	account := &Account{
		ID:       9,
		Name:     "acc-9",
		Type:     AccountTypeOAuth,
		Platform: PlatformAntigravity,
REDACTED

	// 429 + RATE_LIMIT_EXCEEDED + 无 retryDelay → 使用默认 1 分钟限流
	respBody := []byte(`{
		"error": {
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "metadata": {"model": "claude-sonnet-4-5"REDACTED, "reason": "RATE_LIMIT_EXCEEDED"REDACTED
			],
			"message": "You have exhausted your capacity on this model."
	REDACTED
REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
REDACTED

	params := antigravityRetryLoopParams{
		ctx:             context.Background(),
		prefix:          "[test]",
		account:         account,
		accessToken:     "token",
		action:          "generateContent",
		body:            []byte(`{"input":"test"REDACTED`),
		accountRepo:     repo,
		isStickySession: true,
		handleError: func(ctx context.Context, prefix string, account *Account, statusCode int, headers http.Header, body []byte, quotaScope AntigravityQuotaScope, groupID int64, sessionHash string, isStickySession bool) *handleModelRateLimitResult {
			return nil
	REDACTED,
REDACTED

	availableURLs := []string{"https://ag-1.test"REDACTED

	result := handleSmartRetry(params, resp, respBody, "https://ag-1.test", 0, availableURLs)

	require.NotNil(t, result)
	require.Equal(t, smartRetryActionBreakWithResp, result.action)
	require.Nil(t, result.resp, "should not return resp when switchError is set")
	require.NotNil(t, result.switchError, "should return switchError for no retryDelay")
	require.Equal(t, "claude-sonnet-4-5", result.switchError.RateLimitedModel)
	require.True(t, result.switchError.IsStickySession)

	// 验证模型限流已设置
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "claude-sonnet-4-5", repo.modelRateLimitCalls[0].modelKey)
REDACTED
