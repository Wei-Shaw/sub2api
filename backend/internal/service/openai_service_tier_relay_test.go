package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func forwardServiceTierRelayFixture(t *testing.T, accountType string, passthrough, stream bool, requested string, upstreamResponse *http.Response) (*Account, *OpenAIForwardResult, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	body := []byte(fmt.Sprintf(`{"model":"gpt-5.5","stream":%t,"service_tier":%q,"input":"hello"}`, stream, requested))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: upstreamResponse}
	// Buffered SSE is emitted as JSON. Do not forward the upstream SSE content
	// type into the outer hop; header filtering is independent of tier billing.
	cfg := &config.Config{Security: config.SecurityConfig{ResponseHeaders: config.ResponseHeaderConfig{
		Enabled: true, ForceRemove: []string{"content-type"},
	}}}
	svc := &OpenAIGatewayService{
		cfg: cfg, responseHeaderFilter: compileResponseHeaderFilter(cfg), httpUpstream: upstream, cache: &stubGatewayCache{},
		settingService: NewSettingService(&openAIFastPolicyRepoStub{values: map[string]string{}}, &config.Config{}),
	}
	account := &Account{
		ID: 1, Name: "relay-fixture", Platform: PlatformOpenAI, Type: accountType,
		Concurrency: 1, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "fixture-key", "access_token": "fixture-token", "chatgpt_account_id": "fixture-account"},
		Extra:       map[string]any{"openai_responses_supported": true, "openai_passthrough": passthrough},
	}
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	return account, result, rec
}

func TestOpenAIServiceTierRelayHTTP(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, passthrough := range []bool{false, true} {
			for _, tc := range []struct{ name, accountType, requested, observed, billed string }{
				{"oauth_fast_default_echo", AccountTypeOAuth, "fast", "default", "priority"},
				{"oauth_priority_default_echo", AccountTypeOAuth, "priority", "default", "priority"},
				{"oauth_ultrafast_default_echo", AccountTypeOAuth, "ultrafast", "default", "ultrafast"},
				{"oauth_explicit_flex_downgrade", AccountTypeOAuth, "priority", "flex", "flex"},
				{"public_api_real_downgrade", AccountTypeAPIKey, "priority", "default", "default"},
				{"standard", AccountTypeOAuth, "default", "default", "default"},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/passthrough=%t", tc.name, stream, passthrough), func(t *testing.T) {
					terminal := fmt.Sprintf(`{"id":"resp_relay","object":"response","model":"gpt-5.5","status":"completed","service_tier":%q,"output":[],"usage":{"input_tokens":82497,"output_tokens":2176,"input_tokens_details":{"cached_tokens":81152}}}`, tc.observed)
					wire := "data: {\"type\":\"response.completed\",\"response\":" + terminal + "}\n\ndata: [DONE]\n\n"
					native := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
					innerAccount, inner, innerRec := forwardServiceTierRelayFixture(t, tc.accountType, passthrough, stream, tc.requested, native)
					require.Equal(t, tc.observed, inner.UpstreamResponseServiceTier, "raw upstream observation must survive client-facing normalization")
					innerBilling := ApplyOpenAIServiceTierBillingResolution(innerAccount, inner)
					outerAccount, outer, _ := forwardServiceTierRelayFixture(t, AccountTypeAPIKey, false, stream, tc.requested, innerRec.Result())
					outerBilling := ApplyOpenAIServiceTierBillingResolution(outerAccount, outer)
					require.Equal(t, tc.billed, innerBilling.Billing)
					require.Equal(t, innerBilling.Billing, outerBilling.Billing, "a downstream relay must not bill a different tier solely because the inner credential is OAuth")
					require.Equal(t, inner.Usage, outer.Usage, "tier normalization must preserve usage")
				})
			}
		}
	}
}

func TestOpenAIServiceTierRelayChatCompatibility(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-5.5","service_tier":"priority","stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			wire := "data: " + `{"type":"response.created","response":{"id":"resp_chat","model":"gpt-5.5","service_tier":"default"}}` + "\n\n" +
				"data: " + `{"type":"response.completed","response":{"id":"resp_chat","model":"gpt-5.5","status":"completed","service_tier":"default","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}` + "\n\ndata: [DONE]\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: &stubGatewayCache{}}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "fixture-token", "chatgpt_account_id": "fixture-account"}}
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "default", result.UpstreamResponseServiceTier)
			require.Contains(t, rec.Body.String(), `"service_tier":"priority"`)
			require.NotContains(t, rec.Body.String(), `"service_tier":"default"`)
		})
	}
}

func TestOpenAIServiceTierRelayUsesFilteredOutboundTier(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprint(passthrough), func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			body := []byte(`{"model":"gpt-5.5","service_tier":"priority","stream":true,"input":"hello"}`)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			policy, _ := json.Marshal(OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{ServiceTier: OpenAIFastTierPriority, Action: BetaPolicyActionFilter, Scope: BetaPolicyScopeAll}}})
			repo := &openAIFastPolicyRepoStub{values: map[string]string{SettingKeyOpenAIFastPolicySettings: string(policy)}}
			wire := "data: " + `{"type":"response.completed","response":{"model":"gpt-5.5","status":"completed","service_tier":"default","output":[],"usage":{"input_tokens":1,"output_tokens":2}}}` + "\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, settingService: NewSettingService(repo, &config.Config{})}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "fixture-token", "chatgpt_account_id": "fixture-account"}, Extra: map[string]any{"openai_passthrough": passthrough}}
			previous := "priority"
			setOpenAIResponseServiceTier(c, &previous)
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, gjson.GetBytes(upstream.lastBody, "service_tier").Exists())
			require.Nil(t, result.ServiceTier)
			require.Empty(t, openAIResponseServiceTier(c))
			require.Equal(t, "default", result.UpstreamResponseServiceTier)
		})
	}
}
