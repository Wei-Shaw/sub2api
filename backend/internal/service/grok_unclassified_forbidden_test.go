//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// grokForbiddenAccountRepo counts permanent SetError writes. The shared Grok
// repo stub ignores them, which is exactly how a gateway fixture can pass
// while production permanently disables the account.
type grokForbiddenAccountRepo struct {
	*grokQuotaAccountRepo
	setErrorCalls int
	lastErrorMsg  string
}

func (r *grokForbiddenAccountRepo) SetError(_ context.Context, _ int64, errorMsg string) error {
	r.setErrorCalls++
	r.lastErrorMsg = errorMsg
	return nil
}

// newGrokForbiddenTestService wires the real RateLimitService: the shared
// fallback only runs when it is installed, so a nil dependency would hide the
// account writes these tests assert on.
func newGrokForbiddenTestService(account *Account, upstream *httpUpstreamRecorder) (*OpenAIGatewayService, *grokForbiddenAccountRepo) {
	repo := &grokForbiddenAccountRepo{grokQuotaAccountRepo: &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{account.ID: account},
		},
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
		rateLimitService:  NewRateLimitService(repo, nil, nil, nil, nil),
	}
	return svc, repo
}

func grokForbiddenUpstream(body string, headers http.Header) *httpUpstreamRecorder {
	if headers == nil {
		headers = http.Header{}
	}
	if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "application/json")
	}
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}}
}

func grokExhaustedQuotaHeaders() http.Header {
	return http.Header{
		"Retry-After":                    []string{"120"},
		"X-Ratelimit-Limit-Requests":     []string{"100"},
		"X-Ratelimit-Remaining-Requests": []string{"0"},
	}
}

func newGrokForbiddenContext(path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("api_key", &APIKey{ID: 8935})
	return c, recorder
}

func requireGrokAccountUntouched(t *testing.T, svc *OpenAIGatewayService, repo *grokForbiddenAccountRepo, account *Account) {
	t.Helper()
	require.Zero(t, repo.setErrorCalls, "account must not be permanently disabled: %s", repo.lastErrorMsg)
	require.Zero(t, repo.tempUnschedCalls, "account must not be cooled down: %s", repo.lastTempUnschedReason)
	require.Zero(t, repo.rateLimitedCalls, "account must not be rate limited")
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account), "account must stay schedulable")
	require.Nil(t, account.TempUnschedulableUntil)
}

func requireNotGrokFailover(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "request-scoped 403 must not rotate accounts: %v", err)
}

// Bodies that carry no evidence about the account. xAI returns them for
// requests that healthy accounts reproduce, so they describe the request.
var grokUnclassifiedForbiddenBodies = []struct {
	name string
	body string
}{
	{name: "empty body", body: ""},
	{name: "flat refusal", body: `{"code":"permission-denied","error":"I'm sorry, I can't help with that request."}`},
	{name: "unknown structured body", body: `{"error":{"message":"upstream returned an unknown response"}}`},
	{name: "plain text", body: "Forbidden"},
}

func TestGrokUnclassifiedForbiddenResponsesKeepsAccountAndStopsRotation(t *testing.T) {
	requestBody := []byte(`{"model":"grok","input":[{"role":"user","content":"hi"}],"stream":false}`)
	for _, tt := range grokUnclassifiedForbiddenBodies {
		for _, quotaHeaders := range []bool{false, true} {
			name := tt.name
			if quotaHeaders {
				name += " with exhausted quota headers"
			}
			t.Run(name, func(t *testing.T) {
				c, recorder := newGrokForbiddenContext("/v1/responses", requestBody)
				account := healthyGrokOAuthGatewayTestAccount(8101, "access-token")
				var headers http.Header
				if quotaHeaders {
					headers = grokExhaustedQuotaHeaders()
				}
				upstream := grokForbiddenUpstream(tt.body, headers)
				svc, repo := newGrokForbiddenTestService(account, upstream)

				result, err := svc.forwardGrokResponses(context.Background(), c, account, requestBody, "grok", false, time.Now())

				require.Nil(t, result)
				requireNotGrokFailover(t, err)
				requireGrokAccountUntouched(t, svc, repo, account)
				require.Len(t, upstream.requests, 1)
				require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest, "client must see the failure")
				if quotaHeaders {
					require.Equal(t, 1, repo.updateCalls, "quota snapshot stays available for observation")
				}
			})
		}
	}
}

func TestGrokUnclassifiedForbiddenChatCompletionsKeepsAccountAndStopsRotation(t *testing.T) {
	requests := []struct {
		name string
		body string
	}{
		{name: "responses bridge", body: `{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":false}`},
		{name: "raw chat completions", body: `{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":false,"stop":"done"}`},
	}
	for _, request := range requests {
		for _, tt := range grokUnclassifiedForbiddenBodies {
			t.Run(request.name+"/"+tt.name, func(t *testing.T) {
				requestBody := []byte(request.body)
				c, recorder := newGrokForbiddenContext(grokChatRawEndpoint, requestBody)
				account := grokChatBridgeTestAccount(8102)
				account.Credentials["expires_at"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
				upstream := grokForbiddenUpstream(tt.body, grokExhaustedQuotaHeaders())
				svc, repo := newGrokForbiddenTestService(account, upstream)

				result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, requestBody, "", "")

				require.Nil(t, result)
				requireNotGrokFailover(t, err)
				requireGrokAccountUntouched(t, svc, repo, account)
				require.Len(t, upstream.requests, 1)
				require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest, "client must see the failure")
			})
		}
	}
}

// The shared fallback is reachable from every Grok path that declines to fail
// over, so it must hold the same decision on its own.
func TestGrokUnclassifiedForbiddenSharedFallbackDoesNotDisableAccount(t *testing.T) {
	for _, tt := range grokUnclassifiedForbiddenBodies {
		t.Run("responses/"+tt.name, func(t *testing.T) {
			c, recorder := newGrokForbiddenContext("/v1/responses", nil)
			account := healthyGrokOAuthGatewayTestAccount(8103, "access-token")
			svc, repo := newGrokForbiddenTestService(account, nil)
			resp := grokForbiddenUpstream(tt.body, nil).resp

			_, err := svc.handleErrorResponse(context.Background(), resp, c, account, nil, "grok-4.5")

			requireNotGrokFailover(t, err)
			requireGrokAccountUntouched(t, svc, repo, account)
			require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest)
		})
		t.Run("chat completions/"+tt.name, func(t *testing.T) {
			c, recorder := newGrokForbiddenContext("/v1/chat/completions", nil)
			account := healthyGrokOAuthGatewayTestAccount(8104, "access-token")
			svc, repo := newGrokForbiddenTestService(account, nil)
			resp := grokForbiddenUpstream(tt.body, nil).resp

			_, err := svc.handleCompatErrorResponse(resp, c, account, writeChatCompletionsError, "grok-4.5")

			requireNotGrokFailover(t, err)
			requireGrokAccountUntouched(t, svc, repo, account)
			require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest)
		})
	}
}

func TestRateLimitServiceGrokUnclassifiedForbiddenDoesNotSetError(t *testing.T) {
	for _, tt := range grokUnclassifiedForbiddenBodies {
		t.Run(tt.name, func(t *testing.T) {
			account := healthyGrokOAuthGatewayTestAccount(8105, "access-token")
			_, repo := newGrokForbiddenTestService(account, nil)
			rateLimitService := NewRateLimitService(repo, nil, nil, nil, nil)

			shouldDisable := rateLimitService.HandleUpstreamError(
				context.Background(), account, http.StatusForbidden, http.Header{}, []byte(tt.body),
			)

			require.False(t, shouldDisable)
			require.Zero(t, repo.setErrorCalls, repo.lastErrorMsg)
			require.Zero(t, repo.tempUnschedCalls)
		})
	}
}

func TestRateLimitServiceGrokAccountForbiddenStillSetsError(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(8106, "access-token")
	_, repo := newGrokForbiddenTestService(account, nil)
	rateLimitService := NewRateLimitService(repo, nil, nil, nil, nil)

	shouldDisable := rateLimitService.HandleUpstreamError(
		context.Background(), account, http.StatusForbidden, http.Header{},
		[]byte(`{"error":{"code":"account_suspended","message":"account suspended"}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
}

// Explicit account, credential, quota and billing evidence keeps the existing
// cooldown and failover on the Grok path.
func TestGrokForbiddenWithAccountEvidenceKeepsCooldownAndFailover(t *testing.T) {
	requestBody := []byte(`{"model":"grok","input":[{"role":"user","content":"hi"}],"stream":false}`)
	tests := []struct {
		name            string
		body            string
		wantTempUnsched int
		wantRateLimited int
	}{
		{name: "subscription required", body: `{"error":{"message":"subscription required"}}`, wantTempUnsched: 1},
		{name: "account suspended", body: `{"error":{"code":"account_suspended","message":"account suspended"}}`, wantTempUnsched: 1},
		{
			name:            "credential denial",
			body:            `{"code":"permission-denied","error":"Access to the chat endpoint is denied. Please ensure you're using the correct credentials."}`,
			wantTempUnsched: 1,
		},
		{name: "insufficient quota", body: `{"error":{"code":"insufficient_quota","message":"insufficient_quota"}}`, wantTempUnsched: 1},
		{
			name:            "spending limit",
			body:            `{"code":"personal-team-blocked:spending-limit","error":"Your team has reached its spending limit."}`,
			wantRateLimited: 1,
		},
		{
			name:            "free usage exhausted",
			body:            `{"code":"subscription:free-usage-exhausted","error":"You've used all the included free usage."}`,
			wantTempUnsched: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newGrokForbiddenContext("/v1/responses", requestBody)
			account := healthyGrokOAuthGatewayTestAccount(8107, "access-token")
			svc, repo := newGrokForbiddenTestService(account, grokForbiddenUpstream(tt.body, nil))

			_, err := svc.forwardGrokResponses(context.Background(), c, account, requestBody, "grok", false, time.Now())

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusForbidden, failoverErr.StatusCode)
			require.Equal(t, tt.wantTempUnsched, repo.tempUnschedCalls)
			require.Equal(t, tt.wantRateLimited, repo.rateLimitedCalls)
			require.Zero(t, repo.setErrorCalls)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
		})
	}
}

// An administrator rule that matches the body declares the 403 account-scoped:
// the configured cooldown and the account switch both still apply.
func TestGrokUnclassifiedForbiddenHonorsTemporaryRule(t *testing.T) {
	requestBody := []byte(`{"model":"grok","input":[{"role":"user","content":"hi"}],"stream":false}`)
	c, _ := newGrokForbiddenContext("/v1/responses", requestBody)
	account := healthyGrokOAuthGatewayTestAccount(8108, "access-token")
	account.Credentials["temp_unschedulable_enabled"] = true
	account.Credentials["temp_unschedulable_rules"] = []any{map[string]any{
		"error_code":       float64(http.StatusForbidden),
		"keywords":         []any{"can't help with that request"},
		"duration_minutes": float64(7),
	}}
	upstream := grokForbiddenUpstream(`{"code":"permission-denied","error":"I'm sorry, I can't help with that request."}`, nil)
	svc, repo := newGrokForbiddenTestService(account, upstream)
	before := time.Now()

	_, err := svc.forwardGrokResponses(context.Background(), c, account, requestBody, "grok", false, time.Now())

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Zero(t, repo.setErrorCalls)
	require.GreaterOrEqual(t, repo.tempUnschedCalls, 1)
	require.WithinDuration(t, before.Add(7*time.Minute), repo.lastTempUnschedUntil, 5*time.Second)
}

// Pool-mode accounts front an upstream pool: a 403 is retried there and never
// touches local account state, with or without account evidence.
func TestGrokUnclassifiedForbiddenPoolModeKeepsRetry(t *testing.T) {
	requestBody := []byte(`{"model":"grok","input":[{"role":"user","content":"hi"}],"stream":false}`)
	c, _ := newGrokForbiddenContext("/v1/responses", requestBody)
	account := &Account{
		ID:          8109,
		Name:        "grok-pool",
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":   "xai-key",
			"pool_mode": true,
		},
	}
	upstream := grokForbiddenUpstream(`{"code":"permission-denied","error":"I'm sorry, I can't help with that request."}`, nil)
	svc, repo := newGrokForbiddenTestService(account, upstream)

	_, err := svc.forwardGrokResponses(context.Background(), c, account, requestBody, "grok", false, time.Now())

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.RetryableOnSameAccount)
	requireGrokAccountUntouched(t, svc, repo, account)
}

// An HTML 403 comes from a proxy or CDN in front of xAI. It is not evidence
// against the account, but another account may reach xAI through another
// route, so rotation continues without a penalty.
func TestGrokHTMLForbiddenRotatesWithoutAccountPenalty(t *testing.T) {
	requestBody := []byte(`{"model":"grok","input":[{"role":"user","content":"hi"}],"stream":false}`)
	c, _ := newGrokForbiddenContext("/v1/responses", requestBody)
	account := healthyGrokOAuthGatewayTestAccount(8110, "access-token")
	upstream := grokForbiddenUpstream("<html><body>403 Forbidden</body></html>", http.Header{"Content-Type": []string{"text/html"}})
	svc, repo := newGrokForbiddenTestService(account, upstream)

	_, err := svc.forwardGrokResponses(context.Background(), c, account, requestBody, "grok", false, time.Now())

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	requireGrokAccountUntouched(t, svc, repo, account)
}

func TestGrokUnclassifiedForbiddenMediaKeepsAccountAndStopsRotation(t *testing.T) {
	for _, tt := range grokUnclassifiedForbiddenBodies {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newGrokForbiddenContext("/v1/images/generations", nil)
			account := healthyGrokOAuthGatewayTestAccount(8111, "access-token")
			svc, repo := newGrokForbiddenTestService(account, nil)
			resp := grokForbiddenUpstream(tt.body, grokExhaustedQuotaHeaders()).resp

			_, err := svc.handleGrokMediaErrorResponse(context.Background(), resp, c, account, "request-id", "grok-imagine")

			requireNotGrokFailover(t, err)
			requireGrokAccountUntouched(t, svc, repo, account)
			require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest)
		})
	}
}

func TestGrokUnclassifiedForbiddenWSHTTPBridgeKeepsAccountAndStopsRotation(t *testing.T) {
	c, _ := newGrokForbiddenContext("/v1/responses", nil)
	account := healthyGrokOAuthGatewayTestAccount(8112, "access-token")
	upstream := grokForbiddenUpstream(`{"code":"permission-denied","error":"I'm sorry, I can't help with that request."}`, nil)
	svc, repo := newGrokForbiddenTestService(account, upstream)
	payload := []byte(`{"type":"response.create","model":"grok-4.5","input":"hi"}`)
	var writes [][]byte

	_, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "access-token", payload, len(payload),
		"grok-4.5", "", "", "", "cache-id", 1,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			return nil
		},
	)

	requireNotGrokFailover(t, err)
	requireGrokAccountUntouched(t, svc, repo, account)
	require.Len(t, writes, 1, "client must receive the terminal error event")
	require.Contains(t, string(writes[0]), `"error"`)
}

func TestGrokUnclassifiedForbiddenRealtimeHandshakeKeepsAccount(t *testing.T) {
	account := healthyGrokOAuthGatewayTestAccount(8113, "access-token")
	svc, repo := newGrokForbiddenTestService(account, nil)

	svc.HandleGrokRealtimeUpstreamError(
		context.Background(), account, http.StatusForbidden,
		[]byte("failed to WebSocket dial: expected handshake response status code 101 but got 403"),
	)

	requireGrokAccountUntouched(t, svc, repo, account)
}

func TestIsGrokUnclassifiedForbidden(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "empty body", status: http.StatusForbidden, want: true},
		{name: "whitespace body", status: http.StatusForbidden, body: " \n", want: true},
		{name: "flat refusal", status: http.StatusForbidden, body: `{"code":"permission-denied","error":"I'm sorry, I can't help with that request."}`, want: true},
		{name: "unknown structured body", status: http.StatusForbidden, body: `{"error":{"message":"upstream returned an unknown response"}}`, want: true},
		{name: "html page", status: http.StatusForbidden, body: "<html><body>403 Forbidden</body></html>", want: true},
		{name: "handshake error text", status: http.StatusForbidden, body: "expected handshake response status code 101 but got 403", want: true},
		{name: "content policy has its own class", status: http.StatusForbidden, body: `{"error":{"code":"new_sensitive","message":"text is sensitive"}}`},
		{name: "account suspended", status: http.StatusForbidden, body: `{"error":{"code":"account_suspended","message":"account suspended"}}`},
		{name: "subscription required", status: http.StatusForbidden, body: `{"error":{"message":"subscription required"}}`},
		{name: "not entitled", status: http.StatusForbidden, body: `{"error":{"code":"not_entitled"}}`},
		{name: "credential denial", status: http.StatusForbidden, body: `{"code":"permission-denied","error":"Access to the chat endpoint is denied. Please ensure you're using the correct credentials."}`},
		{name: "spending limit code", status: http.StatusForbidden, body: `{"code":"personal-team-blocked:spending-limit","error":"blocked"}`},
		{name: "insufficient quota", status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`},
		{name: "free usage exhausted", status: http.StatusForbidden, body: `{"code":"subscription:free-usage-exhausted","error":"You've used all the included free usage."}`},
		{name: "rate limit wording", status: http.StatusForbidden, body: `{"error":{"message":"rate limit exceeded"}}`},
		{name: "unauthorized status", status: http.StatusUnauthorized},
		{name: "payment required status", status: http.StatusPaymentRequired},
		{name: "rate limited status", status: http.StatusTooManyRequests},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isGrokUnclassifiedForbidden(tt.status, []byte(tt.body)))
		})
	}
}

func TestShouldFailoverGrokUnclassifiedForbidden(t *testing.T) {
	refusal := []byte(`{"code":"permission-denied","error":"I'm sorry, I can't help with that request."}`)
	svc := &OpenAIGatewayService{}
	oauth := &Account{ID: 8201, Platform: PlatformGrok, Type: AccountTypeOAuth}
	pool := &Account{ID: 8202, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
	ruled := &Account{ID: 8203, Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{
		"temp_unschedulable_enabled": true,
		"temp_unschedulable_rules": []any{map[string]any{
			"error_code":       float64(http.StatusForbidden),
			"keywords":         []any{"can't help with that request"},
			"duration_minutes": float64(7),
		}},
	}}
	unmatchedRule := &Account{ID: 8204, Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{
		"temp_unschedulable_enabled": true,
		"temp_unschedulable_rules": []any{map[string]any{
			"error_code":       float64(http.StatusForbidden),
			"keywords":         []any{"different failure"},
			"duration_minutes": float64(7),
		}},
	}}

	require.False(t, svc.shouldFailoverGrokUpstreamError(oauth, http.StatusForbidden, refusal))
	require.False(t, svc.shouldFailoverGrokUpstreamError(oauth, http.StatusForbidden, nil))
	require.False(t, svc.shouldFailoverGrokUpstreamError(unmatchedRule, http.StatusForbidden, refusal))
	require.True(t, svc.shouldFailoverGrokUpstreamError(pool, http.StatusForbidden, refusal))
	require.True(t, svc.shouldFailoverGrokUpstreamError(ruled, http.StatusForbidden, refusal))
	require.True(t, svc.shouldFailoverGrokUpstreamError(oauth, http.StatusForbidden, []byte("<!DOCTYPE html><html></html>")))
	require.True(t, svc.shouldFailoverGrokUpstreamError(oauth, http.StatusForbidden, []byte("error code: 1010")))
	require.True(t, svc.shouldFailoverGrokUpstreamError(oauth, http.StatusForbidden, []byte(`{"error":{"message":"subscription required"}}`)))
	require.True(t, svc.shouldFailoverGrokUpstreamError(oauth, http.StatusUnauthorized, nil))
	require.True(t, svc.shouldFailoverGrokUpstreamError(oauth, http.StatusPaymentRequired, nil))
}
