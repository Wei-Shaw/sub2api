//go:build unit

package service

import (
	"context"
	"encoding/json"
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

func openAICapacityShedModelRuleAccount(accountType string) *Account {
	return &Account{
		ID: 101, Platform: PlatformOpenAI, Type: accountType, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{map[string]any{
				"error_code": http.StatusServiceUnavailable, "keywords": []any{"overloaded"},
				"duration_minutes": 5, "scope": TempUnschedScopeModel,
			}},
			"model_mapping": map[string]any{"alias": "gpt-6.1-sol", "gpt-6.1-sol": "must-not-remap"},
		},
	}
}

func TestOpenAICapacityShedModelRule_StreamReaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	readers := []struct {
		name string
		run  func(*OpenAIGatewayService, *gin.Context, *http.Response, *Account, []byte) error
	}{
		{"stream", func(s *OpenAIGatewayService, c *gin.Context, r *http.Response, a *Account, _ []byte) error {
			_, err := s.handleStreamingResponse(c.Request.Context(), r, c, a, time.Now(), "alias", "gpt-6.1-sol")
			return err
		}},
		{"passthrough", func(s *OpenAIGatewayService, c *gin.Context, r *http.Response, a *Account, _ []byte) error {
			_, err := s.handleStreamingResponsePassthrough(c.Request.Context(), r, c, a, time.Now(), "alias", "gpt-6.1-sol")
			return err
		}},
		{"buffered", func(s *OpenAIGatewayService, c *gin.Context, r *http.Response, a *Account, body []byte) error {
			_, err := s.handleSSEToJSON(r, c, a, body, "alias", "gpt-6.1-sol")
			return err
		}},
		{"buffered_passthrough", func(s *OpenAIGatewayService, c *gin.Context, r *http.Response, a *Account, body []byte) error {
			_, err := s.handlePassthroughSSEToJSON(r, c, a, body, "alias", "gpt-6.1-sol")
			return err
		}},
	}
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, reader := range readers {
			for _, frame := range []struct{ name, payload string }{
				{"error", `{"type":"error","error":{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`},
				{"response.failed", `{"type":"response.failed","response":{"error":{"code":"slow_down","message":"overloaded"}}}`},
			} {
				t.Run(accountType+"/"+reader.name+"/"+frame.name, func(t *testing.T) {
					repo := &modelNotFoundAccountRepoStub{}
					svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
					account := openAICapacityShedModelRuleAccount(accountType)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					body := "data: " + frame.payload + "\n\n"
					resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
						Body: io.NopCloser(strings.NewReader(body))}

					err := reader.run(svc, c, resp, account, []byte(body))

					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Len(t, repo.modelRateLimitCalls, 1)
					require.False(t, failover.RetryableOnSameAccount, "a matching model cooldown must stop same-account replay")
					require.True(t, failover.RequestScopedTransient, "do not enable generic account-wide cooldowns")
					require.Equal(t, http.StatusServiceUnavailable, failover.ClientStatusCode)
					call := repo.modelRateLimitCalls[0]
					require.Equal(t, "gpt-6.1-sol", call.scope, "use the already mapped scheduling model")
					require.WithinDuration(t, time.Now().Add(5*time.Minute), call.resetAt, time.Second)
					var state TempUnschedState
					require.NoError(t, json.Unmarshal([]byte(call.reason), &state))
					require.Equal(t, TempUnschedScopeModel, state.Scope)
					require.Equal(t, http.StatusServiceUnavailable, state.StatusCode)
					require.Zero(t, repo.tempCalls)
					require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
					require.False(t, c.Writer.Written())
					require.Empty(t, rec.Body.String())
				})
			}
		}
	}
}

func TestOpenAICapacityShedModelRule_HTTP(t *testing.T) {
	account := openAICapacityShedModelRuleAccount(AccountTypeOAuth)
	repo := &modelNotFoundAccountRepoStub{}
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := []byte(`{"error":{"code":"server_is_overloaded","message":"overloaded"}}`)
	err := svc.handleFailoverErrorResponsePassthrough(context.Background(),
		&http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}}, c, account,
		[]byte(`{"model":"alias"}`), body)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.False(t, failover.RetryableOnSameAccount)
	require.True(t, failover.RequestScopedTransient)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-6.1-sol", repo.modelRateLimitCalls[0].scope)
	require.Zero(t, repo.tempCalls)
	require.False(t, c.Writer.Written())
}

func TestOpenAICapacityShedModelRule_ProtectionBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*Account, map[string]any)
		model  string
	}{
		{"disabled", func(a *Account, _ map[string]any) { a.Credentials["temp_unschedulable_enabled"] = false }, "gpt-6.1-sol"},
		{"no_rules", func(a *Account, _ map[string]any) { delete(a.Credentials, "temp_unschedulable_rules") }, "gpt-6.1-sol"},
		{"unknown_model", func(*Account, map[string]any) {}, ""},
		{"account_scope", func(_ *Account, r map[string]any) { r["scope"] = TempUnschedScopeAccount }, "gpt-6.1-sol"},
		{"legacy_scope", func(_ *Account, r map[string]any) { delete(r, "scope") }, "gpt-6.1-sol"},
		{"keyword_mismatch", func(_ *Account, r map[string]any) { r["keywords"] = []any{"other error"} }, "gpt-6.1-sol"},
		{"status_mismatch", func(_ *Account, r map[string]any) { r["error_code"] = http.StatusBadGateway }, "gpt-6.1-sol"},
		{"pool_mode", func(a *Account, _ map[string]any) { a.Credentials["pool_mode"] = true }, "gpt-6.1-sol"},
		{"excluded_status", func(a *Account, _ map[string]any) {
			a.Credentials["custom_error_codes_enabled"] = true
			a.Credentials["custom_error_codes"] = []any{float64(http.StatusTooManyRequests)}
		}, "gpt-6.1-sol"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := openAICapacityShedModelRuleAccount(AccountTypeAPIKey)
			rule := account.Credentials["temp_unschedulable_rules"].([]any)[0].(map[string]any)
			tt.change(account, rule)
			repo := &modelNotFoundAccountRepoStub{}
			svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
			payload := []byte(`{"type":"error","error":{"code":"server_is_overloaded","message":"overloaded"}}`)
			failover := svc.newOpenAIStreamFailoverErrorWithModel(nil, account, false, "", payload, "overloaded", tt.model)
			require.True(t, failover.RetryableOnSameAccount)
			require.True(t, failover.RequestScopedTransient)
			require.Empty(t, repo.modelRateLimitCalls)
			require.Zero(t, repo.tempCalls)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		})
	}
}

func TestOpenAICapacityShedModelRule_WriteFailureDoesNotBlockAccount(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{modelRateLimitErr: errors.New("write failed")}
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
	account := openAICapacityShedModelRuleAccount(AccountTypeOAuth)
	payload := []byte(`{"type":"error","error":{"code":"server_is_overloaded","message":"overloaded"}}`)
	failover := svc.newOpenAIStreamFailoverErrorWithModel(nil, account, false, "", payload, "overloaded", "gpt-6.1-sol")
	require.False(t, failover.RetryableOnSameAccount)
	require.True(t, failover.RequestScopedTransient)
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Zero(t, repo.tempCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestOpenAICapacityShedModelRule_ErrorFieldsOnly(t *testing.T) {
	for _, tt := range []struct {
		name, payload, keyword string
		matched                bool
	}{
		{"output_is_not_an_error", `{"type":"response.failed","response":{"output":[{"text":"overloaded"}],"error":{"code":"slow_down","message":"try again"}}}`, "", false},
		{"metadata_is_not_an_error", `{"type":"error","metadata":{"prompt":"overloaded"},"error":{"code":"slow_down","message":"try again"}}`, "", false},
		{"model_is_not_an_error", `{"type":"response.failed","response":{"model":"overloaded-model","error":{"code":"slow_down","message":"try again"}}}`, "", false},
		{"flat_metadata_is_not_an_error", `{"type":"error","message":"Our servers are currently overloaded. Please try again later.","metadata":{"prompt":"cooldown-marker"}}`, "cooldown-marker", false},
		{"root_error_code_matches", `{"type":"error","error":{"code":"server_is_overloaded","message":"try again"},"metadata":{"prompt":"private-output-marker"}}`, "", true},
		{"nested_error_code_matches", `{"type":"response.failed","response":{"output":[{"text":"private-output-marker"}],"error":{"code":"server_is_overloaded"}}}`, "", true},
		{"flat_error_message_matches", `{"type":"error","message":"Our servers are currently overloaded. Please try again later.","metadata":{"prompt":"private-output-marker"}}`, "", true},
		{"error_message_matches", `{"type":"response.failed","response":{"error":{"code":"slow_down","message":"overloaded"}}}`, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &modelNotFoundAccountRepoStub{}
			svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
			account := openAICapacityShedModelRuleAccount(AccountTypeOAuth)
			if tt.keyword != "" {
				rule := account.Credentials["temp_unschedulable_rules"].([]any)[0].(map[string]any)
				rule["keywords"] = []any{tt.keyword}
			}
			payload := []byte(tt.payload)
			failover := svc.newOpenAIStreamFailoverErrorWithModel(nil, account, false, "", payload,
				extractOpenAISSEErrorMessage(payload), "gpt-6.1-sol")
			require.Equal(t, !tt.matched, failover.RetryableOnSameAccount)
			require.True(t, failover.RequestScopedTransient)
			if tt.matched {
				require.Len(t, repo.modelRateLimitCalls, 1)
				require.NotContains(t, repo.modelRateLimitCalls[0].reason, "private-output-marker")
			} else {
				require.Empty(t, repo.modelRateLimitCalls)
			}
			require.Zero(t, repo.tempCalls)
		})
	}
}

func TestOpenAICapacityShedModelRule_OtherPlatformKeepsDefault(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
	account := openAICapacityShedModelRuleAccount(AccountTypeOAuth)
	account.Platform = PlatformGrok
	payload := []byte(`{"type":"error","error":{"code":"server_is_overloaded","message":"overloaded"}}`)
	failover := svc.newOpenAIStreamFailoverErrorWithModel(nil, account, false, "", payload, "overloaded", "grok-4")
	require.Empty(t, repo.modelRateLimitCalls)
	require.Zero(t, repo.tempCalls)
	require.True(t, failover.RetryableOnSameAccount)
	require.True(t, failover.RequestScopedTransient)
}
