//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func buildContextLengthFailedSSE() string {
	failed := `{"type":"response.failed","response":{"id":"resp_err","object":"response","status":"failed","error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"Your input exceeds the context window of this model. Please adjust your input and try again."},"output":[],"usage":{"input_tokens":100000,"output_tokens":0,"total_tokens":100000}}}`
	return fmt.Sprintf("data: %s\n\n", failed)
}

func bindPassthroughRule(c *gin.Context, platform string, keywords []string, responseCode int) {
	svc := &ErrorPassthroughService{}
	rules := make([]*cachedPassthroughRule, 0, len(keywords))
	for i, kw := range keywords {
		code := responseCode
		rules = append(rules, &cachedPassthroughRule{
			ErrorPassthroughRule: &model.ErrorPassthroughRule{
				ID:              int64(i + 1),
				Enabled:         true,
				Platforms:       []string{platform},
				MatchMode:       model.MatchModeAny,
				Keywords:        []string{kw},
				ResponseCode:    &code,
				PassthroughBody: true,
			},
			lowerKeywords:  []string{strings.ToLower(kw)},
			lowerPlatforms: []string{strings.ToLower(platform)},
		})
	}
	svc.localCacheMu.Lock()
	svc.localCache = rules
	svc.localCacheMu.Unlock()
	BindErrorPassthroughService(c, svc)
}

func TestForwardAsChatCompletions_ResponseFailed_PassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindPassthroughRule(c, "openai", []string{"context_length_exceeded"}, 400)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "passthrough")
	require.Equal(t, 400, rec.Code, "passthrough rule should override 502 to 400")

	respBody := rec.Body.String()
	errType := gjson.Get(respBody, "error.type").String()
	require.Equal(t, "upstream_error", errType)
	errMsg := gjson.Get(respBody, "error.message").String()
	require.NotEmpty(t, errMsg, "passthrough should preserve error message")
	require.Contains(t, errMsg, "context window")
}

func TestForwardAsAnthropic_ResponseFailed_PassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindPassthroughRule(c, "openai", []string{"context_length_exceeded"}, 400)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "passthrough")
	require.Equal(t, 400, rec.Code, "passthrough rule should override 502 to 400")
	respBody := rec.Body.String()
	errMsg := gjson.Get(respBody, "error.message").String()
	require.NotEmpty(t, errMsg, "passthrough should preserve error message")
}

func TestForwardAsChatCompletions_ResponseFailed_NoRule_Still502(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code, "without passthrough rule should still be 502")
}

// bindStatusCodePassthroughRule 绑定一条按错误码+关键词双条件(MatchModeAll)匹配的规则。
// 此类规则依赖语义状态码推断才能在协议转换路径命中（response.failed 无真实 HTTP 状态码）。
func bindStatusCodePassthroughRule(c *gin.Context, platform string, statusCode int, keyword string, responseCode int) {
	rule := &model.ErrorPassthroughRule{
		ID:              1,
		Name:            "status-code-rule",
		Enabled:         true,
		Priority:        1,
		Platforms:       []string{platform},
		ErrorCodes:      []int{statusCode},
		Keywords:        []string{keyword},
		MatchMode:       model.MatchModeAll,
		ResponseCode:    &responseCode,
		PassthroughBody: true,
	}
	svc := &ErrorPassthroughService{}
	svc.setLocalCache([]*model.ErrorPassthroughRule{rule})
	BindErrorPassthroughService(c, svc)
}

func TestForwardAsChatCompletions_ResponseFailed_ErrorCodeRuleMatchesViaSemanticStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindStatusCodePassthroughRule(c, "openai", http.StatusBadRequest, "context_length_exceeded", http.StatusBadRequest)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code, "error-code-conditioned rule should match via semantic status inference")
	respBody := rec.Body.String()
	require.Equal(t, "upstream_error", gjson.Get(respBody, "error.type").String())
	require.Contains(t, gjson.Get(respBody, "error.message").String(), "context window")
}

func TestForwardAsAnthropic_ResponseFailed_ErrorCodeRuleMatchesViaSemanticStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	bindStatusCodePassthroughRule(c, "openai", http.StatusBadRequest, "context_length_exceeded", http.StatusBadRequest)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(buildContextLengthFailedSSE())),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	account := rawChatCompletionsTestAccount()
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code, "error-code-conditioned rule should match via semantic status inference")
	respBody := rec.Body.String()
	require.NotEmpty(t, gjson.Get(respBody, "error.message").String())
}

func TestOpenAIStreamFailedEventSemanticStatusGrokErrors(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    int
	}{
		{name: "numeric 429", payload: `{"type":"error","error":{"status_code":429,"code":"opaque"}}`, want: http.StatusTooManyRequests},
		{name: "numeric 402", payload: `{"type":"response.failed","response":{"error":{"status":"402","code":"opaque"}}}`, want: http.StatusPaymentRequired},
		{name: "numeric 403", payload: `{"type":"response.failed","response":{"error":{"status_code":403,"code":"opaque"}}}`, want: http.StatusForbidden},
		{name: "numeric 404", payload: `{"type":"response.failed","response":{"error":{"status_code":404,"code":"opaque"}}}`, want: http.StatusNotFound},
		{name: "unauthorized", payload: `{"type":"response.failed","response":{"error":{"code":"invalid_api_key","message":"authentication failed"}}}`, want: http.StatusUnauthorized},
		{name: "rate limit", payload: `{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"rate_limit_exceeded","message":"rate limited"}}}`, want: http.StatusTooManyRequests},
		{name: "bare error rate limit", payload: `{"type":"error","error":{"code":"rate_limit_exceeded","message":"rate limited"}}`, want: http.StatusTooManyRequests},
		{name: "free usage exhausted", payload: `{"type":"response.failed","response":{"error":{"code":"subscription:free-usage-exhausted","message":"free usage exhausted"}}}`, want: http.StatusTooManyRequests},
		{name: "payment required", payload: `{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"payment_required","message":"payment required"}}}`, want: http.StatusPaymentRequired},
		{name: "forbidden", payload: `{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"permission_denied","message":"access denied"}}}`, want: http.StatusForbidden},
		{name: "not found", payload: `{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"not_found","message":"model not found"}}}`, want: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIStreamFailedEventSemanticStatus([]byte(tt.payload), ""))
		})
	}
}

func TestReconcileGrokStreamFailedAccountStateAppliesUnauthorizedPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &Account{ID: 725, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"invalid_api_key","message":"authentication failed"}}}`)

	svc.reconcileGrokStreamFailedAccountState(c, account, payload, "authentication failed")
	svc.reconcileGrokStreamFailedAccountState(c, account, payload, "authentication failed")

	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Zero(t, repo.errorCalls)
	require.Zero(t, repo.rateLimitedCalls)
}

func TestOpenAIStreamingPassthroughWriterDisconnectSkipsLateGrokReconciliation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	writer := &passthroughFlushTestWriter{
		ResponseWriter:  c.Writer,
		recorder:        recorder,
		failAfterWrites: 0,
	}
	c.Writer = writer

	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 726, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	upstream := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"partial"}`,
		"",
		`data: {"type":"response.failed","response":{"error":{"code":"payment_required","message":"payment required"},"usage":{"input_tokens":9,"output_tokens":1,"total_tokens":10}}}`,
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstream)),
	}

	result, err := svc.handleStreamingResponsePassthrough(
		context.Background(), resp, c, account, time.Now(), "", "",
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.Equal(t, 1, writer.failedWrites)
	require.Equal(t, 9, result.usage.InputTokens)
	require.Equal(t, 1, result.usage.OutputTokens)
	require.Zero(t, repo.errorCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestReconcileGrokStreamFailedAccountStateMarks429Once(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &Account{ID: 720, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"subscription:free-usage-exhausted","message":"free usage exhausted"}}}`)

	svc.reconcileGrokStreamFailedAccountState(c, account, payload, "free usage exhausted")
	failoverErr := svc.newOpenAIStreamFailoverError(c, account, false, "req_429", payload, "free usage exhausted")

	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Zero(t, repo.errorCalls)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, "subscription:free-usage-exhausted", gjson.GetBytes(failoverErr.ResponseBody, "error.code").String())
	require.Equal(t, true, repo.updates[account.ID][GrokFreeRecoveryPendingExtraKey])
}

func TestReconcileGrokStreamFailedAccountStateMarksPermanentErrorsOnce(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
		want    int
	}{
		{name: "payment required", code: "payment_required", message: "billing required", want: http.StatusPaymentRequired},
		{name: "forbidden", code: "entitlement_denied", message: "account suspended", want: http.StatusForbidden},
		{name: "not found", code: "account_not_found", message: "account not found", want: http.StatusNotFound},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			account := &Account{ID: int64(730 + i), Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
			repo := &grokQuotaAccountRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo}
			payload := []byte(fmt.Sprintf(`{"type":"response.failed","response":{"error":{"code":%q,"message":%q,"status_code":%d}}}`, tt.code, tt.message, tt.want))

			svc.reconcileGrokStreamFailedAccountState(c, account, payload, tt.message)
			svc.reconcileGrokStreamFailedAccountState(c, account, payload, tt.message)

			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.Equal(t, 1, repo.errorCalls)
			require.Contains(t, repo.lastErrorMessage, fmt.Sprintf("status %d", tt.want))
			require.Zero(t, repo.rateLimitedCalls)
			require.Zero(t, repo.tempUnschedCalls)
		})
	}
}

func TestReconcileGrokStreamFailedAccountStateGeneric403And404DoNotDisableAccount(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		code       string
		message    string
	}{
		{name: "generic permission", statusCode: http.StatusForbidden, code: "permission_denied", message: "permission denied"},
		{name: "endpoint not found", statusCode: http.StatusNotFound, code: "not_found", message: "endpoint not found"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			account := &Account{ID: int64(740 + i), Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
			repo := &grokQuotaAccountRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo}
			payload := []byte(fmt.Sprintf(`{"type":"response.failed","response":{"error":{"code":%q,"message":%q,"status_code":%d}}}`, tt.code, tt.message, tt.statusCode))

			svc.reconcileGrokStreamFailedAccountState(c, account, payload, tt.message)

			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.Zero(t, repo.errorCalls)
			require.Zero(t, repo.rateLimitedCalls)
		})
	}
}

func TestReconcileGrokStreamFailedAccountStateHandlesBareErrorAndSkipsCyberPolicy(t *testing.T) {
	t.Run("bare error rate limit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		account := &Account{ID: 750, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
		repo := &grokQuotaAccountRepo{}
		svc := &OpenAIGatewayService{accountRepo: repo}
		payload := []byte(`{"type":"error","error":{"code":"rate_limit_exceeded","message":"rate limited"}}`)

		svc.reconcileGrokStreamFailedAccountState(c, account, payload, "rate limited")
		failoverErr := svc.newOpenAIStreamFailoverError(c, account, false, "req_bare_429", payload, "rate limited")

		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
		require.Equal(t, 1, repo.rateLimitedCalls)
		require.Zero(t, repo.errorCalls)
		require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
		require.Equal(t, "rate_limit_exceeded", gjson.GetBytes(failoverErr.ResponseBody, "error.code").String())
	})

	t.Run("cyber policy is request scoped", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		account := &Account{ID: 751, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
		repo := &grokQuotaAccountRepo{}
		svc := &OpenAIGatewayService{accountRepo: repo}
		payload := []byte(`{"type":"response.failed","response":{"error":{"code":"cyber_policy","message":"forbidden high-risk cyber request"}}}`)

		svc.reconcileGrokStreamFailedAccountState(c, account, payload, "forbidden high-risk cyber request")

		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		require.Zero(t, repo.errorCalls)
		require.Zero(t, repo.rateLimitedCalls)
		require.Zero(t, repo.tempUnschedCalls)
	})
}

func TestHandleSSEToJSONGrokRateLimitReturnsSemanticFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &Account{ID: 760, Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "Xai-Request-Id": []string{"req_sse_429"}},
	}
	body := []byte(strings.Join([]string{
		`data: {"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"free usage exhausted"}}}`,
		"data: [DONE]",
	}, "\n"))

	result, err := svc.handleSSEToJSON(resp, c, account, body, "grok-4.5", "grok-4.5")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Empty(t, rec.Body.String())
}

func TestReadOpenAICompatBufferedTerminalGrokBareAccountErrors(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		event       string
		suffix      string
		wantLimited bool
	}{
		{
			name:        "rate limit",
			statusCode:  http.StatusTooManyRequests,
			event:       `{"type":"error","error":{"code":"rate_limit_exceeded","message":"free usage exhausted"}}`,
			suffix:      "\n\n",
			wantLimited: true,
		},
		{
			name:       "payment required",
			statusCode: http.StatusPaymentRequired,
			event:      `{"type":"error","error":{"code":"payment_required","message":"payment required"}}`,
			suffix:     "\n\n",
		},
		{
			name:       "forbidden",
			statusCode: http.StatusForbidden,
			event:      `{"type":"error","error":{"code":"entitlement_denied","message":"account suspended","status_code":403}}`,
			suffix:     "\n\n",
		},
		{
			name:       "not found at eof",
			statusCode: http.StatusNotFound,
			event:      `{"type":"error","error":{"code":"account_not_found","message":"account not found"}}`,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			account := &Account{ID: int64(780 + i), Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
			repo := &grokQuotaAccountRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo}
			resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: " + tt.event + tt.suffix))}

			_, _, _, err := svc.readOpenAICompatBufferedTerminal(resp, c, account, "grok buffered test", "req_buffered")

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, tt.statusCode, failoverErr.StatusCode)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			if tt.wantLimited {
				require.Equal(t, 1, repo.rateLimitedCalls)
				require.Zero(t, repo.errorCalls)
			} else {
				require.Zero(t, repo.rateLimitedCalls)
				require.Equal(t, 1, repo.errorCalls)
			}
		})
	}
}
