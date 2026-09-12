package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type availabilityAccountRepo struct {
	transientCooldownAccountRepo
	rateLimitedUntil time.Time
	authErrors       int
}

func (r *availabilityAccountRepo) SetRateLimited(_ context.Context, _ int64, until time.Time) error {
	r.rateLimitedUntil = until
	return nil
}

func (r *availabilityAccountRepo) SetError(context.Context, int64, string) error {
	r.authErrors++
	return nil
}

func newAPIKeyAvailabilityTestService(enabled bool) (*OpenAIGatewayService, *Account) {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		OpenAIAPIKeyAvailabilityEnabled: enabled,
		MaxLineSize:                     defaultMaxLineSize,
	}}
	svc := &OpenAIGatewayService{cfg: cfg}
	svc.rateLimitService = NewRateLimitService(&availabilityAccountRepo{}, nil, cfg, nil, nil)
	svc.rateLimitService.SetAccountRuntimeBlocker(svc)
	return svc, &Account{ID: 50301, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
}

func apiKeyAvailabilityTransientEntry(t *testing.T, svc *OpenAIGatewayService, account *Account, model string) openAIAccountModelTransientEntry {
	t.Helper()
	state := svc.getOpenAIAccountModelTransientState()
	key, ok := openAIAccountModelTransientKey(account.ID, model)
	require.True(t, ok)
	state.mu.Lock()
	defer state.mu.Unlock()
	entry, ok := state.entries[key]
	require.True(t, ok, "transient failure must be recorded for this account and model")
	return entry
}

func TestOpenAIAPIKeyAvailability_TransientStatusMatrix(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504, 520, 521, 522, 523, 524} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			body := []byte(`{"error":{"type":"server_error","message":"Upstream unavailable"}}`)
			svc.handleOpenAIAccountUpstreamError(context.Background(), account, status, http.Header{}, body, "gpt-test")
			entry := apiKeyAvailabilityTransientEntry(t, svc, account, "gpt-test")
			require.Equal(t, 1, entry.failureStreak)
			require.Equal(t, 10*time.Second, entry.blockUntil.Sub(entry.lastFailure))
			require.False(t, svc.newOpenAIAccountFailoverError(account, status, nil, body, "", false, true).RetryableOnSameAccount)
			for _, passthrough := range []bool{false, true} {
				svc, account := newAPIKeyAvailabilityTestService(true)
				body := fmt.Sprintf("data: {\"type\":\"error\",\"error\":{\"status_code\":%d,\"type\":\"server_error\",\"message\":\"Upstream unavailable\"}}\n\n", status)
				rec, err := runAPIKeyAvailabilityTestStream(t, svc, account, passthrough, io.NopCloser(strings.NewReader(body)))
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, status, failover.StatusCode)
				require.False(t, failover.RetryableOnSameAccount)
				require.Empty(t, rec.Body.String())
				require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "gpt-test").failureStreak)
			}
		})
	}
}

func TestOpenAIAPIKeyAvailability_StreamClassification(t *testing.T) {
	svc, account := newAPIKeyAvailabilityTestService(true)
	for _, tc := range []struct {
		payload string
		want    int
	}{
		{`{"type":"error","error":{"type":"server_error","message":"Failure"}}`, 500},
		{`{"type":"error","error":{"code":"bad_gateway","message":"Failure"}}`, 502},
		{`{"type":"error","error":{"code":"gateway_timeout","message":"Failure"}}`, 504},
		{`{"type":"error","error":{"code":"unknown","message":"Failure"}}`, 0},
		{`{"type":"error","error":{"type":"invalid_request_error","message":"Invalid input"}}`, 0},
		{`{"type":"error","error":{"status_code":400,"message":"Invalid input"}}`, 0},
		{`{"type":"error","error":{"status_code":400,"type":"server_error","message":"Invalid input"}}`, 0},
		{"", 0},
		{"not json", 0},
	} {
		t.Run(tc.payload, func(t *testing.T) {
			require.Equal(t, tc.want, svc.openAIAPIKeyTransientStreamStatus(account, []byte(tc.payload), extractOpenAISSEErrorMessage([]byte(tc.payload))))
		})
	}
}

func TestOpenAIAPIKeyAvailability_RateLimitRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		name, retryAfter string
		quota            bool
		minimum, maximum time.Duration
	}{
		{"seconds", "90", false, 89 * time.Second, 91 * time.Second},
		{"HTTP date", time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat), false, 118 * time.Second, 121 * time.Second},
		{"longer quota reset wins", "10", true, 298 * time.Second, 301 * time.Second},
		{"bounded hint", "172800", false, 23*time.Hour + 59*time.Minute, 24*time.Hour + time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			repo, ok := svc.rateLimitService.accountRepo.(*availabilityAccountRepo)
			require.True(t, ok)
			body := []byte(`{"error":{"code":"rate_limit_exceeded","message":"Rate limited"}}`)
			if tc.quota {
				body = []byte(fmt.Sprintf(`{"error":{"type":"usage_limit_reached","resets_at":%d}}`, time.Now().Add(5*time.Minute).Unix()))
			}
			headers := http.Header{"Retry-After": {tc.retryAfter}}
			svc.handleOpenAIAccountUpstreamError(context.Background(), account, 429, headers, body, "gpt-test")
			remaining := time.Until(repo.rateLimitedUntil)
			require.GreaterOrEqual(t, remaining, tc.minimum)
			require.LessOrEqual(t, remaining, tc.maximum)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.False(t, svc.newOpenAIAccountFailoverError(account, 429, headers, body, "", false, true).RetryableOnSameAccount)
		})
	}
	for _, enabled := range []bool{false, true} {
		svc, account := newAPIKeyAvailabilityTestService(enabled)
		body := []byte(`{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Rate limited"}}}`)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		svc.handleOpenAIStreamTerminalAccountSideEffects(c, account, body, "Rate limited", http.Header{"Retry-After": {"90"}}, "gpt-test")
		repo, ok := svc.rateLimitService.accountRepo.(*availabilityAccountRepo)
		require.True(t, ok)
		if enabled {
			require.Greater(t, time.Until(repo.rateLimitedUntil), 89*time.Second)
		} else {
			require.Less(t, time.Until(repo.rateLimitedUntil), 89*time.Second)
		}
	}
}

func TestOpenAIAPIKeyAvailability_CredentialFailuresStayNarrow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"401 structured authentication", 401, `{"error":{"message":"Invalid credentials"}}`, true},
		{"403 invalid key", 403, `{"error":{"code":"invalid_api_key","message":"Invalid API key"}}`, true},
		{"403 disabled account", 403, `{"error":{"code":"account_disabled","message":"Disabled"}}`, true},
		{"403 permission", 403, `{"error":{"type":"permission_error","message":"Not permitted for this model"}}`, false},
		{"403 HTML", 403, `<html>Forbidden</html>`, false},
		{"400 parameter", 400, `{"error":{"type":"invalid_request_error","message":"Invalid input"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			require.Equal(t, tc.want, svc.openAIAPIKeyCredentialFailure(account, tc.status, []byte(tc.body)))
			if tc.want {
				require.True(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, tc.status, nil, []byte(tc.body), "gpt-test"))
				require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
				require.False(t, svc.newOpenAIAccountFailoverError(account, tc.status, nil, []byte(tc.body), "", true, true).RetryableOnSameAccount)
			}
			svc.cfg.Gateway.OpenAIAPIKeyAvailabilityEnabled = false
			require.False(t, svc.openAIAPIKeyCredentialFailure(account, tc.status, []byte(tc.body)))
		})
	}
}

func TestOpenAIAPIKeyAvailability_HTTPPolicyAndSuccess(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"temporary upstream failure"}}`,
		`{"error":{"code":"server_is_overloaded","message":"Please retry later."}}`,
	} {
		t.Run(body, func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			account.Credentials = map[string]any{"model_mapping": map[string]any{"alias": "upstream-a", "upstream-a": "upstream-b"}}
			for i, cooldown := range []time.Duration{10 * time.Second, 10 * time.Second, 45 * time.Second} {
				disabled := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusServiceUnavailable, http.Header{}, []byte(body), "upstream-a")
				require.False(t, disabled)
				entry := apiKeyAvailabilityTransientEntry(t, svc, account, "upstream-a")
				require.Equal(t, i+1, entry.failureStreak)
				require.Equal(t, cooldown, entry.blockUntil.Sub(entry.lastFailure))
				require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "alias"))
				require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "other-model"))
				require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
				failover := svc.newOpenAIAccountFailoverError(account, http.StatusServiceUnavailable, http.Header{}, []byte(body), "", false, true)
				require.False(t, failover.RetryableOnSameAccount)
			}
			svc.ReportOpenAIAccountScheduleResult(account, "upstream-a", true, nil)
			require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "alias"))
			svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusServiceUnavailable, http.Header{}, []byte(body), "upstream-a")
			require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "upstream-a").failureStreak)
		})
	}
}

func TestOpenAIAPIKeyAvailability_LegacyAndAccountScope(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		enabled               bool
		accountType, platform string
		pool                  bool
	}{
		{name: "flag off", accountType: AccountTypeAPIKey, platform: PlatformOpenAI},
		{name: "OAuth", enabled: true, accountType: AccountTypeOAuth, platform: PlatformOpenAI},
		{name: "pool API key", enabled: true, accountType: AccountTypeAPIKey, platform: PlatformOpenAI, pool: true},
		{name: "other platform", enabled: true, accountType: AccountTypeAPIKey, platform: PlatformGrok},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(tc.enabled)
			account.Type, account.Platform = tc.accountType, tc.platform
			account.Credentials = map[string]any{"pool_mode": tc.pool}
			require.False(t, svc.openAIAPIKeyTransientAvailabilityEnabled(account, 503))
			if account.Platform != PlatformOpenAI {
				return
			}
			body := []byte(`{"error":{"code":"server_is_overloaded","message":"Please retry later."}}`)
			for range 3 {
				svc.handleOpenAIAccountUpstreamError(context.Background(), account, 503, http.Header{}, body, "gpt-test")
			}
			require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-test"))
			failover := svc.newOpenAIAccountFailoverError(account, 503, http.Header{}, body, "", false, false)
			require.True(t, failover.RetryableOnSameAccount)
		})
	}
	t.Run("ordinary legacy 503 first failure remains schedulable", func(t *testing.T) {
		svc, account := newAPIKeyAvailabilityTestService(false)
		svc.handleOpenAIAccountUpstreamError(context.Background(), account, 503, http.Header{}, []byte(`{"error":{"message":"upstream unavailable"}}`), "gpt-test")
		require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-test"))
		require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "gpt-test").failureStreak)
	})
	t.Run("parameter 400 stays schedulable", func(t *testing.T) {
		svc, account := newAPIKeyAvailabilityTestService(true)
		for range 3 {
			svc.handleOpenAIAccountUpstreamError(context.Background(), account, 400, http.Header{}, []byte(`{"error":{"type":"invalid_request_error","message":"Invalid type for input[0].arguments"}}`), "gpt-test")
		}
		require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-test"))
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	})
}

func runAPIKeyAvailabilityTestStream(t *testing.T, svc *OpenAIGatewayService, account *Account, passthrough bool, body io.ReadCloser) (*httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
	var err error
	if passthrough {
		// The real passthrough forwarder passes an empty mapped model for ordinary
		// requests. Only compact fallback fills it, unlike the native reader.
		_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-test", "")
	} else {
		_, err = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-test", "gpt-test")
	}
	return rec, err
}

func TestOpenAIAPIKeyAvailability_StreamBeforeOutput(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, payload := range []string{
			`{"type":"response.failed","response":{"error":{"type":"server_error","code":"server_is_overloaded","message":"The server is overloaded"}}}`,
			`{"type":"error","status_code":503,"error":{"message":"Upstream unavailable"}}`,
		} {
			t.Run(payload+map[bool]string{false: "/native", true: "/passthrough"}[passthrough], func(t *testing.T) {
				svc, account := newAPIKeyAvailabilityTestService(true)
				body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_failed\"}}\n\n" + "data: " + payload + "\n\n"
				rec, err := runAPIKeyAvailabilityTestStream(t, svc, account, passthrough, io.NopCloser(strings.NewReader(body)))
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, 503, failover.StatusCode)
				require.False(t, failover.RetryableOnSameAccount)
				require.Empty(t, rec.Body.String(), "failed attempt must not commit preamble or error")
				require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "gpt-test").failureStreak)
				require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-test"))
			})
		}
	}
}

func TestOpenAIAPIKeyAvailability_ExplicitStream503KeepsRequestRejections(t *testing.T) {
	svc, account := newAPIKeyAvailabilityTestService(true)
	for _, payload := range []string{
		`{"type":"error","status_code":503,"error":{"code":"content_policy","message":"Request blocked by policy"}}`,
		`{"type":"error","status_code":503,"error":{"type":"invalid_request_error","message":"Invalid input"}}`,
		`{"type":"response.failed","response":{"error":{"status_code":503,"code":"cyber_policy","message":"flagged for cyber policy"}}}`,
	} {
		t.Run(payload, func(t *testing.T) {
			require.False(t, svc.isOpenAIAPIKeyTransientStreamFailure(account, []byte(payload), extractOpenAISSEErrorMessage([]byte(payload))))
		})
	}
}

func TestOpenAIAPIKeyAvailability_NonStreamingPassthroughTerminal(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "enabled"}[enabled], func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(enabled)
			account.Credentials = map[string]any{"model_mapping": map[string]any{"alias": "gpt-test", "gpt-test": "must-not-map-twice"}}
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			body := []byte("data: {\"type\":\"error\",\"status_code\":503,\"error\":{\"type\":\"server_error\",\"message\":\"Upstream unavailable\"}}\n\n")
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}}
			_, err := svc.handlePassthroughSSEToJSON(resp, c, account, body, "alias", "")
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.Equal(t, enabled, errors.As(err, &failover))
			if enabled {
				require.Equal(t, 503, failover.StatusCode)
				require.False(t, failover.RetryableOnSameAccount)
				require.Empty(t, rec.Body.String())
				require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "alias"))
				require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "gpt-test").failureStreak)
			} else {
				require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "alias"))
			}
		})
	}
}

func TestOpenAIAPIKeyAvailability_StreamLegacyAndCancellation(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		path := map[bool]string{false: "native", true: "passthrough"}[passthrough]
		for _, tc := range []struct {
			name          string
			enabled, pool bool
			accountType   string
		}{
			{name: "flag off", accountType: AccountTypeAPIKey},
			{name: "OAuth", enabled: true, accountType: AccountTypeOAuth},
			{name: "pool", enabled: true, pool: true, accountType: AccountTypeAPIKey},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				svc, account := newAPIKeyAvailabilityTestService(tc.enabled)
				account.Type = tc.accountType
				account.Credentials = map[string]any{"pool_mode": tc.pool}
				body := "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"Please retry later.\"}}}\n\n"
				rec, err := runAPIKeyAvailabilityTestStream(t, svc, account, passthrough, io.NopCloser(strings.NewReader(body)))
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.True(t, failover.RetryableOnSameAccount)
				require.Empty(t, rec.Body.String())
				require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-test"))
			})
		}
		t.Run(path+"/client cancellation", func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			body := &passthroughFlushTestErrorBody{payload: []byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"cancelled\"}}\n\n"), err: context.Canceled}
			_, err := runAPIKeyAvailabilityTestStream(t, svc, account, passthrough, body)
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-test"))
		})
	}
}

func TestOpenAIAPIKeyAvailability_StreamAfterOutputDeduplicatesAndNeverReplays(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "passthrough"}[passthrough], func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"real answer\"}\n\n" +
				"data: {\"type\":\"error\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"Please retry later.\"}}\n\n" +
				"data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"Please retry later.\"}}}\n\n"
			rec, err := runAPIKeyAvailabilityTestStream(t, svc, account, passthrough, io.NopCloser(strings.NewReader(body)))
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover), "already committed content must not be replayed")
			require.Equal(t, 1, strings.Count(rec.Body.String(), "real answer"))
			require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "gpt-test").failureStreak, "error plus failed is one failed attempt")
			require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "gpt-test"))
		})
	}
}

func TestOpenAIAPIKeyAvailability_ToolArgumentsPreventReplay(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "passthrough"}[passthrough], func(t *testing.T) {
			svc, account := newAPIKeyAvailabilityTestService(true)
			body := "data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"fc_test\",\"type\":\"function_call\",\"call_id\":\"call_test\",\"name\":\"search\",\"arguments\":\"\"}}\n\n" +
				"data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_test\",\"output_index\":0,\"delta\":\"{\\\"query\\\":\\\"hello\\\"}\"}\n\n" +
				"data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"Please retry later.\"}}}\n\n"
			rec, err := runAPIKeyAvailabilityTestStream(t, svc, account, passthrough, io.NopCloser(strings.NewReader(body)))
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover), "committed tool arguments must not be replayed")
			require.Equal(t, 1, strings.Count(rec.Body.String(), "response.function_call_arguments.delta"))
			require.Equal(t, 1, apiKeyAvailabilityTransientEntry(t, svc, account, "gpt-test").failureStreak)
		})
	}
}
