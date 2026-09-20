//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSHandshakeAccountPolicy(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, tc := range []struct {
			name        string
			status      int
			body        string
			accountType string
			credentials map[string]any
			wantError   int
			wantTemp    int
			wantCounter int
			wantRefresh int
			wantRetry   bool
		}{
			{name: "apikey_401", status: 401, body: `{"error":{"message":"invalid key"}}`, accountType: AccountTypeAPIKey, wantError: 1},
			{name: "oauth_401_refresh_window", status: 401, body: `{"error":{"message":"expired"}}`, accountType: AccountTypeOAuth, credentials: map[string]any{"refresh_token": "refresh-test"}, wantTemp: 1, wantRefresh: 1},
			{name: "oauth_401_revoked", status: 401, body: `{"error":{"code":"token_revoked"}}`, accountType: AccountTypeOAuth, wantError: 1},
			{name: "html_403", status: 403, body: openAI403HTMLBody, accountType: AccountTypeOAuth},
			{name: "structured_403", status: 403, body: `{"error":{"message":"access forbidden"}}`, accountType: AccountTypeOAuth, wantTemp: 1, wantCounter: 1},
			{name: "disabled_workspace", status: 403, body: `{"detail":{"code":"deactivated_workspace"}}`, accountType: AccountTypeOAuth, wantError: 1},
			{name: "verification_required_400", status: 400, body: `{"error":{"message":"identity verification is required"}}`, accountType: AccountTypeAPIKey, wantError: 1},
			{name: "custom_codes_skip_403_penalty", status: 403, body: `{"error":{"message":"access forbidden"}}`, accountType: AccountTypeAPIKey, credentials: map[string]any{"custom_error_codes_enabled": true, "custom_error_codes": []any{float64(401)}}},
			{name: "pool_503", status: 503, body: `{"error":{"message":"temporarily unavailable"}}`, accountType: AccountTypeAPIKey, credentials: map[string]any{"pool_mode": true, "pool_mode_retry_status_codes": []any{503}}, wantRetry: true},
			{name: "pool_default_503_switches", status: 503, body: `{"error":{"message":"temporarily unavailable"}}`, accountType: AccountTypeAPIKey, credentials: map[string]any{"pool_mode": true}},
			{name: "pool_transient_processing_400", status: 400, body: `{"error":{"message":"An error occurred while processing your request"}}`, accountType: AccountTypeAPIKey, credentials: map[string]any{"pool_mode": true}, wantRetry: true},
		} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", tc.name, passthrough), func(t *testing.T) {
				repo := &rateLimitAccountRepoStub{}
				counter := &countingOpenAI403CounterCache{}
				invalidator := &tokenCacheInvalidatorRecorder{}
				cfg := &config.Config{}
				rateLimits := NewRateLimitService(repo, nil, cfg, nil, nil)
				rateLimits.SetOpenAI403CounterCache(counter)
				rateLimits.SetTokenCacheInvalidator(invalidator)
				svc := &OpenAIGatewayService{cfg: cfg, accountRepo: repo, rateLimitService: rateLimits}
				rateLimits.SetAccountRuntimeBlocker(svc)
				account := &Account{ID: 719, Platform: PlatformOpenAI, Type: tc.accountType, Credentials: tc.credentials}
				c, rec := newOpenAITransportErrTestContext()
				dialErr := &openAIWSDialError{StatusCode: tc.status, ResponseHeaders: http.Header{"X-Request-Id": {"handshake-request"}}, ResponseBody: []byte(tc.body), Err: errors.New("handshake rejected")}
				err := svc.handleOpenAIWSHandshakeFailure(context.Background(), c, account, "gpt-5.1", dialErr, passthrough)
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.True(t, IsOpenAIWSHandshakeFailover(fmt.Errorf("wrapped: %w", err)))
				require.Equal(t, tc.status, failover.StatusCode)
				require.True(t, failover.ShouldRetryNextAccount())
				require.Equal(t, tc.wantRetry, failover.RetryableOnSameAccount)
				require.Equal(t, tc.body, string(failover.ResponseBody))
				require.Equal(t, "handshake-request", failover.ResponseHeaders.Get("x-request-id"))
				require.Equal(t, tc.wantError, repo.setErrorCalls)
				require.Equal(t, tc.wantTemp, repo.tempCalls)
				require.Equal(t, tc.wantCounter, counter.increments)
				require.Len(t, invalidator.accounts, tc.wantRefresh)
				rawEvents, exists := c.Get(OpsUpstreamErrorsKey)
				require.True(t, exists)
				events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
				require.True(t, ok)
				require.Len(t, events, 1, "one handshake must produce one failure record")
				require.Equal(t, tc.status, events[0].UpstreamStatusCode)
				require.Equal(t, passthrough, events[0].Passthrough)
				require.Zero(t, rec.Body.Len())
				require.False(t, c.Writer.Written(), "classification must not close or write to the client")
			})
		}
	}
}

func TestOpenAIWSHandshakeOAuth429UsesBodyAndHeaders(t *testing.T) {
	for _, tc := range []struct {
		name      string
		headers   http.Header
		body      string
		wantRetry bool
	}{
		{name: "transient", body: `{"error":{"message":"try again"}}`, wantRetry: true},
		{name: "body_quota_reset", body: fmt.Sprintf(`{"error":{"type":"usage_limit_reached","resets_at":%d}}`, time.Now().Add(time.Hour).Unix())},
		{name: "header_quota_reset", headers: http.Header{"X-Codex-Primary-Used-Percent": {"100"}, "X-Codex-Primary-Reset-After-Seconds": {"3600"}, "X-Codex-Primary-Window-Minutes": {"300"}}, body: `{"error":{"message":"quota exceeded"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &oauth429RateLimitRepo{}
			rateLimits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			svc := &OpenAIGatewayService{accountRepo: repo, rateLimitService: rateLimits}
			rateLimits.SetAccountRuntimeBlocker(svc)
			account := &Account{ID: 720, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			c, _ := newOpenAITransportErrTestContext()
			err := svc.handleOpenAIWSHandshakeFailure(context.Background(), c, account, "gpt-5.1", &openAIWSDialError{
				StatusCode: 429, ResponseHeaders: tc.headers, ResponseBody: []byte(tc.body), Err: errors.New("rate limited"),
			}, false)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Equal(t, tc.wantRetry, failover.RetryableOnSameAccount)
			require.Equal(t, !tc.wantRetry, failover.SameAccountRetryDeadline.IsZero())
			require.Equal(t, !tc.wantRetry, svc.isOpenAIAccountRuntimeBlocked(account))
			if tc.wantRetry {
				require.Zero(t, repo.setRateLimitedCalls)
			} else {
				require.Equal(t, 1, repo.setRateLimitedCalls)
			}
		})
	}
}

func TestOpenAIWSHandshakeRejectedRequestDoesNotPenalizeAccount(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{400, `{"error":{"code":"invalid_request_error","message":"bad parameter"}}`},
		{503, `{"error":{"code":"cyber_policy","message":"blocked"}}`},
		{503, `{"error":{"code":"context_length_exceeded","message":"context length exceeded"}}`},
	} {
		c, _ := newOpenAITransportErrTestContext()
		repo := &rateLimitAccountRepoStub{}
		svc := &OpenAIGatewayService{accountRepo: repo, rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
		dialErr := &openAIWSDialError{StatusCode: tc.status, ResponseBody: []byte(tc.body)}
		err := svc.handleOpenAIWSHandshakeFailure(context.Background(), c, &Account{ID: 721, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.1", dialErr, false)
		require.Same(t, dialErr, err)
		require.False(t, IsOpenAIWSHandshakeFailover(err))
		require.Zero(t, repo.tempCalls)
		require.Zero(t, repo.setErrorCalls)
	}
}

func TestOpenAIWSHandshakePolicyScope(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI}
	dialErr := &openAIWSDialError{StatusCode: 503}
	require.True(t, openAIWSHandshakeHTTPPolicyApplies(account, 1, "", dialErr))
	require.False(t, openAIWSHandshakeHTTPPolicyApplies(account, 2, "", dialErr))
	require.False(t, openAIWSHandshakeHTTPPolicyApplies(account, 1, "resp_bound", dialErr))
	require.False(t, openAIWSHandshakeHTTPPolicyApplies(account, 1, "", &openAIWSDialError{}))
	require.False(t, openAIWSHandshakeHTTPPolicyApplies(&Account{Platform: PlatformGrok}, 1, "", dialErr))
	require.False(t, IsOpenAIWSHandshakeFailover(&UpstreamFailoverError{StatusCode: 503, RetryableOnSameAccount: true}))
}

func TestOpenAIWSHandshakeCanceledDoesNotRetryOrPenalize(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(cause)
		c, _ := newOpenAITransportErrTestContext()
		repo := &rateLimitAccountRepoStub{}
		svc := &OpenAIGatewayService{accountRepo: repo, rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil)}
		err := svc.handleOpenAIWSHandshakeFailure(ctx, c, &Account{ID: 722, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "gpt-5.1", &openAIWSDialError{StatusCode: 401, Err: cause}, false)
		require.ErrorIs(t, err, ctx.Err())
		require.Zero(t, repo.tempCalls)
		require.Zero(t, repo.setErrorCalls)
		_, recorded := c.Get(OpsUpstreamErrorsKey)
		require.False(t, recorded)
	}
}
