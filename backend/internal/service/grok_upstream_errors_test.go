//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsGrokContentPolicyRejection(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{
			name:   "new sensitive code",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"new_sensitive","message":"image is sensitive"}}`,
			want:   true,
		},
		{
			name:   "content policy violation code",
			status: http.StatusForbidden,
			body:   `{"response":{"error":{"code":"content_policy_violation"}}}`,
			want:   true,
		},
		{
			name:   "cyber policy code",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"cyber_policy","message":"request rejected"}}`,
			want:   true,
		},
		{
			name:   "moderation feature unavailable",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"The moderation feature is not available for this request"}}`,
			want:   true,
		},
		{
			name:   "explicit prompt moderation rejection",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"request rejected by content moderation"}}`,
			want:   true,
		},
		{
			name:   "entitlement forbidden",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"subscription required"}}`,
			want:   false,
		},
		{
			name:   "account policy suspension is not request policy",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"account suspended due to policy violation"}}`,
			want:   false,
		},
		{
			name:   "structured account suspension overrides policy reason",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"account_suspended","reason":"policy_violation","message":"account suspended due to policy violation"}}`,
			want:   false,
		},
		{
			name:   "ambiguous policy violation code is not enough",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"policy_violation","message":"policy violation"}}`,
			want:   false,
		},
		{
			name:   "policy violation with request scoped message",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"policy_violation","message":"request blocked by policy"}}`,
			want:   true,
		},
		{
			name:   "permission-denied usage guidelines is request scoped",
			status: http.StatusForbidden,
			body:   `{"code":"permission-denied","error":"Content violates usage guidelines. "}`,
			want:   true,
		},
		{
			name:   "permission-denied entitlement stays on the account path",
			status: http.StatusForbidden,
			body:   `{"code":"permission-denied","error":"Access to the chat endpoint is denied"}`,
			want:   false,
		},
		{
			name:   "structured account code overrides usage guidelines phrase",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"account_suspended","message":"Content violates usage guidelines."}}`,
			want:   false,
		},
		{
			name:   "wrong status",
			status: http.StatusBadRequest,
			body:   `{"error":{"code":"new_sensitive"}}`,
			want:   false,
		},
		{
			name:   "exact production permission-denied refusal",
			status: http.StatusForbidden,
			body:   `{"code":"permission-denied","error":"I'm sorry, I can't help with that request."}`,
			want:   true,
		},
		{
			name:   "nested error.message can't refusal",
			status: http.StatusForbidden,
			body:   `{"code":"permission-denied","error":{"message":"I'm sorry, I can't help with that request."}}`,
			want:   true,
		},
		{
			name:   "nested error.message cannot refusal",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"I'm sorry, I cannot help with that request."}}`,
			want:   true,
		},
		{
			name:   "uppercase cannot refusal",
			status: http.StatusForbidden,
			body:   `{"message":"I'M SORRY, I CANNOT HELP WITH THAT REQUEST."}`,
			want:   true,
		},
		{
			name:   "plaintext can't refusal",
			status: http.StatusForbidden,
			body:   "I'm sorry, I can't help with that request.",
			want:   true,
		},
		{
			name:   "plaintext cannot refusal",
			status: http.StatusForbidden,
			body:   "I'm sorry, I cannot help with that request.",
			want:   true,
		},
		{
			name:   "curly apostrophe can't refusal",
			status: http.StatusForbidden,
			body:   "{\"error\":\"I\u2019m sorry, I can\u2019t help with that request.\"}",
			want:   true,
		},
		{
			name:   "subscription code keeps account path despite refusal sentence",
			status: http.StatusForbidden,
			body:   `{"code":"subscription_required","error":"I'm sorry, I can't help with that request."}`,
			want:   false,
		},
		{
			name:   "account suspension phrase keeps account path despite refusal sentence",
			status: http.StatusForbidden,
			body:   `{"error":"I'm sorry, I can't help with that request. subscription required"}`,
			want:   false,
		},
		{
			name:   "structured account code overrides nested refusal",
			status: http.StatusForbidden,
			body:   `{"error":{"code":"account_suspended","message":"I'm sorry, I can't help with that request."}}`,
			want:   false,
		},
		{
			name:   "unknown metadata refusal is not a request refusal",
			status: http.StatusForbidden,
			body:   `{"code":"permission-denied","debug_note":"I'm sorry, I can't help with that request.","error":"Access to the chat endpoint is denied"}`,
			want:   false,
		},
		{
			name:   "object message metadata refusal is not a request refusal",
			status: http.StatusForbidden,
			body:   `{"code":"permission-denied","error":"Access to the chat endpoint is denied","message":{"debug_note":"I'm sorry, I can't help with that request."}}`,
			want:   false,
		},
		{
			name:   "object detail metadata refusal is not a request refusal",
			status: http.StatusForbidden,
			body:   `{"code":"permission-denied","error":"Access to the chat endpoint is denied","detail":{"debug_note":"I'm sorry, I can't help with that request."}}`,
			want:   false,
		},
		{
			name:   "nested object error.message metadata refusal is not a request refusal",
			status: http.StatusForbidden,
			body:   `{"code":"permission-denied","error":{"message":{"debug_note":"I'm sorry, I can't help with that request.","text":"Access to the chat endpoint is denied"}}}`,
			want:   false,
		},
		{
			name:   "string detail refusal",
			status: http.StatusForbidden,
			body:   `{"detail":"I'm sorry, I can't help with that request."}`,
			want:   true,
		},
		{
			name:   "string error.error refusal",
			status: http.StatusForbidden,
			body:   `{"error":{"error":"I'm sorry, I can't help with that request."}}`,
			want:   true,
		},
		{
			name:   "permission-denied without refusal sentence",
			status: http.StatusForbidden,
			body:   `{"code":"permission-denied"}`,
			want:   false,
		},
		{
			name:   "unknown 403 message",
			status: http.StatusForbidden,
			body:   `{"error":"something else went wrong"}`,
			want:   false,
		},
		{
			name:   "short can't help is not the refusal sentence",
			status: http.StatusForbidden,
			body:   `{"error":"I can't help"}`,
			want:   false,
		},
		{
			name:   "non-403 exact refusal",
			status: http.StatusBadRequest,
			body:   `{"code":"permission-denied","error":"I'm sorry, I can't help with that request."}`,
			want:   false,
		},
		{
			name:   "non-403 server refusal",
			status: http.StatusInternalServerError,
			body:   `{"message":"I'm sorry, I cannot help with that request."}`,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isGrokContentPolicyRejection(tt.status, []byte(tt.body)))
		})
	}
}

func TestGrokContentPolicy403DoesNotMutateOrFailover(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 4715, Platform: PlatformGrok, Type: AccountTypeOAuth}
	body := []byte(`{"error":{"code":"new_sensitive","message":"text is sensitive"}}`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusForbidden, nil, body)

	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.False(t, svc.shouldFailoverGrokUpstreamError(http.StatusForbidden, body))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
	got := svc.failoverOpenAIUpstreamHTTPError(context.Background(), c, account, resp, body, "text is sensitive", "grok-4.5")
	require.Nil(t, got)
	require.Zero(t, repo.tempUnschedCalls)
}

func TestGrokNonFailoverDoesNotApplyGenericTempUnschedulablePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: NewRateLimitService(repo, nil, nil, nil, nil),
	}
	account := &Account{
		ID:       5099,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{map[string]any{
				"error_code":       float64(http.StatusForbidden),
				"keywords":         []any{"text is sensitive"},
				"duration_minutes": float64(1),
			}},
		},
	}
	body := []byte(`{"error":{"code":"new_sensitive","message":"text is sensitive"}}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}

	got := svc.failoverOpenAIUpstreamHTTPError(
		context.Background(), c, account, resp, body, "text is sensitive", "",
	)

	require.Nil(t, got)
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestGrokContentPolicy403SharedErrorFallbackDoesNotMutate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"error":{"code":"content_filter","message":"prohibited content"}}`)
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{
		ID:       4719,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(http.StatusTooManyRequests)},
		},
	}

	newContext := func() (*gin.Context, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		return c, recorder
	}

	c, recorder := newContext()
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
	_, err := svc.handleErrorResponse(context.Background(), resp, c, account, nil, "grok-4.5")
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid_request_error")

	c, recorder = newContext()
	resp = &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
	_, err = svc.handleCompatErrorResponse(resp, c, account, writeChatCompletionsError, "grok-4.5")
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid_request_error")

	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
}

func TestGrokContentPolicy403MediaResponseBypassesCustomErrorCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"error":{"code":"new_sensitive","message":"image is sensitive"}}`
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{
		ID:       4720,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(http.StatusTooManyRequests)},
		},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	_, err := svc.handleGrokMediaErrorResponse(context.Background(), resp, c, account, "request-id", "grok-imagine")
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid_request_error")
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
}

func TestGrokContentPolicySSEErrorDoesNotMutateOrFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &grokQuotaAccountRepo{}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"error\",\"error\":{\"code\":\"new_sensitive\",\"message\":\"text is sensitive\"}}\n\n",
		)),
	}}
	svc := &OpenAIGatewayService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{ID: 4721, Platform: PlatformGrok, Type: AccountTypeOAuth, Concurrency: 1}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	payload := []byte(`{"type":"response.create","model":"grok-4.5","input":"hi"}`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", payload, len(payload),
		"grok-4.5", "", "", "", "cache-id", 1,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			return nil
		},
	)

	require.Error(t, err)
	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Len(t, writes, 1)
	require.Contains(t, string(writes[0]), "new_sensitive")
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestGrokPermissionDeniedContentRefusalDoesNotMutateOrFailover(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 4785, Platform: PlatformGrok, Type: AccountTypeOAuth}
	body := []byte(`{"code":"permission-denied","error":"Content violates usage guidelines. "}`)

	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusForbidden, nil, body)

	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.False(t, svc.shouldFailoverGrokUpstreamError(http.StatusForbidden, body))
}

func TestHandleGrokAccountUpstreamErrorEntitlement403KeepsDefaultCooldown(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 4716, Platform: PlatformGrok, Type: AccountTypeOAuth}
	before := time.Now()

	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusForbidden, nil,
		[]byte(`{"error":{"message":"subscription required"}}`),
	)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok access or entitlement denied", repo.lastTempUnschedReason)
	require.Greater(t, repo.lastTempUnschedUntil, before.Add(29*time.Minute))
	require.Less(t, repo.lastTempUnschedUntil, before.Add(31*time.Minute))
}

func TestHandleGrokAccountUpstreamErrorDefaultCooldownsRespectPoolMode(t *testing.T) {
	for _, statusCode := range []int{
		http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusInternalServerError,
	} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			repo := &grokQuotaAccountRepo{}
			svc := &OpenAIGatewayService{accountRepo: repo}
			account := &Account{
				ID:       int64(4800 + statusCode),
				Platform: PlatformGrok,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					"pool_mode": true,
				},
			}
			body := []byte(`{"error":{"message":"grok access or entitlement denied"}}`)

			svc.handleGrokAccountUpstreamError(
				context.Background(), account, statusCode, nil, body,
			)

			require.Zero(t, repo.tempUnschedCalls)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.Nil(t, account.TempUnschedulableUntil)
			require.Empty(t, account.TempUnschedulableReason)
			require.True(t, svc.shouldFailoverGrokUpstreamError(statusCode, body))
		})
	}

	account := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
	require.True(t, account.IsPoolModeRetryableStatus(http.StatusForbidden))

	t.Run("explicit temporary rule still applies", func(t *testing.T) {
		repo := &grokQuotaAccountRepo{}
		svc := &OpenAIGatewayService{accountRepo: repo}
		account := &Account{
			ID:       4723,
			Platform: PlatformGrok,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"pool_mode":                  true,
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules": []any{
					map[string]any{
						"error_code":       float64(http.StatusForbidden),
						"keywords":         []any{"entitlement denied"},
						"duration_minutes": float64(7),
					},
				},
			},
		}
		before := time.Now()

		svc.handleGrokAccountUpstreamError(
			context.Background(), account, http.StatusForbidden, nil,
			[]byte(`{"error":{"message":"grok access or entitlement denied"}}`),
		)

		require.Equal(t, 1, repo.tempUnschedCalls)
		require.Equal(t, "grok configured forbidden rule", repo.lastTempUnschedReason)
		require.WithinDuration(t, before.Add(7*time.Minute), repo.lastTempUnschedUntil, time.Second)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})
}

func TestHandleGrokAccountUpstreamError403UsesConfiguredRule(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{
		ID:       4717,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusForbidden),
					"keywords":         []any{"subscription"},
					"duration_minutes": float64(7),
				},
			},
		},
	}
	before := time.Now()

	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusForbidden, nil,
		[]byte(`{"error":{"message":"subscription required"}}`),
	)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Greater(t, repo.lastTempUnschedUntil, before.Add(6*time.Minute))
	require.Less(t, repo.lastTempUnschedUntil, before.Add(8*time.Minute))
}

func TestHandleGrokAccountUpstreamError403ConfiguredUnmatchedKeepsDefaultCooldown(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{
		ID:       4718,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusForbidden),
					"keywords":         []any{"different failure"},
					"duration_minutes": float64(7),
				},
			},
		},
	}

	svc.handleGrokAccountUpstreamError(
		context.Background(), account, http.StatusForbidden, nil,
		[]byte(`{"error":{"message":"subscription required"}}`),
	)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Equal(t, "grok access or entitlement denied", repo.lastTempUnschedReason)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

const (
	grokExactProductionRefusalBody    = `{"code":"permission-denied","error":"I'm sorry, I can't help with that request."}`
	grokExactProductionRefusalMessage = "I'm sorry, I can't help with that request."
	grokContentPolicyClientFallback   = "Request blocked by upstream content policy"
)

func TestGrokExactProductionRefusalDoesNotMutateOrFailover(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 88001, Platform: PlatformGrok, Type: AccountTypeOAuth}
	body := []byte(grokExactProductionRefusalBody)

	svc.handleGrokAccountUpstreamError(context.Background(), account, http.StatusForbidden, nil, body)

	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Nil(t, account.TempUnschedulableUntil)
	require.Empty(t, account.TempUnschedulableReason)
	require.False(t, svc.shouldFailoverGrokUpstreamError(http.StatusForbidden, body))
}

func TestGrokExactProductionRefusalPathsReturn403WithoutFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		streamLabel := fmt.Sprintf("stream=%v", stream)
		t.Run("raw/"+streamLabel, func(t *testing.T) {
			svc, account, repo, recorder, c := newGrokRefusalPathFixture(t, 88010+boolAccountID(stream))
			body := grokRefusalChatBody(stream)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			_, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
			assertGrokRefusalUpstreamPath(t, svc, "/v1/chat/completions")
			assertGrokRefusalClient403(t, err, recorder, repo, svc, account)
		})
		t.Run("bridge/"+streamLabel, func(t *testing.T) {
			svc, account, repo, recorder, c := newGrokRefusalPathFixture(t, 88020+boolAccountID(stream))
			body := grokRefusalChatBodyForModel("grok-4.7", stream)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			_, err := svc.forwardGrokChatCompletionsViaResponses(context.Background(), c, account, body, "refusal-cache-key", "")
			assertGrokRefusalUpstreamPath(t, svc, "/v1/responses")
			assertGrokRefusalClient403(t, err, recorder, repo, svc, account)
		})
		t.Run("responses/"+streamLabel, func(t *testing.T) {
			svc, account, repo, recorder, c := newGrokRefusalPathFixture(t, 88030+boolAccountID(stream))
			body := grokRefusalResponsesBody(stream)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			_, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", stream, time.Now())
			assertGrokRefusalUpstreamPath(t, svc, "/v1/responses")
			assertGrokRefusalClient403(t, err, recorder, repo, svc, account)
		})
	}
}

func boolAccountID(stream bool) int64 {
	if stream {
		return 1
	}
	return 0
}

func grokRefusalChatBody(stream bool) []byte {
	return grokRefusalChatBodyForModel("grok", stream)
}

func grokRefusalChatBodyForModel(model string, stream bool) []byte {
	return []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, model, stream))
}

func grokRefusalResponsesBody(stream bool) []byte {
	return []byte(fmt.Sprintf(`{"model":"grok","input":"hi","stream":%t}`, stream))
}

func newGrokRefusalPathFixture(t *testing.T, id int64) (*OpenAIGatewayService, *Account, *grokQuotaAccountRepo, *httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	account := grokChatBridgeTestAccount(id)
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(grokExactProductionRefusalBody)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("api_key", &APIKey{ID: id})
	return svc, account, repo, recorder, c
}

func assertGrokRefusalUpstreamPath(t *testing.T, svc *OpenAIGatewayService, path string) {
	t.Helper()
	upstream, ok := svc.httpUpstream.(*httpUpstreamRecorder)
	require.True(t, ok)
	require.Len(t, upstream.requests, 1)
	require.NotNil(t, upstream.requests[0])
	require.NotNil(t, upstream.requests[0].URL)
	require.Equal(t, path, upstream.requests[0].URL.Path)
}

func assertGrokRefusalClient403(t *testing.T, err error, recorder *httptest.ResponseRecorder, repo *grokQuotaAccountRepo, svc *OpenAIGatewayService, account *Account) {
	t.Helper()
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "request refusal must not failover: %v", err)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "data: ")
	require.NotContains(t, recorder.Body.String(), "permission-denied")
	assertGrokStandardRefusalJSON(t, recorder.Body.Bytes(), grokExactProductionRefusalMessage)
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.rateLimitedCalls)
	require.Zero(t, repo.updateCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.Nil(t, account.TempUnschedulableUntil)
	require.Empty(t, account.TempUnschedulableReason)
	require.False(t, svc.shouldFailoverGrokUpstreamError(http.StatusForbidden, []byte(grokExactProductionRefusalBody)))
}

func assertGrokStandardRefusalJSON(t *testing.T, body []byte, message string) {
	t.Helper()
	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "invalid_request_error", payload.Error.Type)
	require.Equal(t, message, payload.Error.Message)
}

func TestGrokContentPolicyClientMessageContracts(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "string error",
			body: grokExactProductionRefusalBody,
			want: grokExactProductionRefusalMessage,
		},
		{
			name: "nested error.message",
			body: `{"code":"permission-denied","error":{"message":"I'm sorry, I can't help with that request."}}`,
			want: grokExactProductionRefusalMessage,
		},
		{
			name: "plaintext",
			body: "I'm sorry, I can't help with that request.",
			want: grokExactProductionRefusalMessage,
		},
		{
			name: "empty fallback",
			body: "",
			want: grokContentPolicyClientFallback,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, grokContentPolicyClientMessage([]byte(tt.body)))
		})
	}
}
