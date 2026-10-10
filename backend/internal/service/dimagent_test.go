//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func dimAgentTestAccount() *Account {
	return &Account{
		ID:          901,
		Name:        "dimagent-subscription",
		Platform:    PlatformDimAgent,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":   "dim-access-token",
			"refresh_token":  "dim-refresh-token",
			"expires_at":     time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"relay_base_url": "https://dimagent.example/v1",
		},
	}
}

func dimAgentTestConfig() *config.Config {
	return &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled: false, AllowInsecureHTTP: true,
	}}}
}

type dimAgentOAuthClientStub struct {
	exchange func(context.Context, string, string, string, string) (*DimAgentOAuthTokenResponse, error)
	refresh  func(context.Context, string, string, string) (*DimAgentOAuthTokenResponse, error)
}

func (s *dimAgentOAuthClientStub) ExchangeCode(ctx context.Context, code, verifier, redirectURI, proxyURL string) (*DimAgentOAuthTokenResponse, error) {
	if s.exchange == nil {
		return nil, nil
	}
	return s.exchange(ctx, code, verifier, redirectURI, proxyURL)
}

func (s *dimAgentOAuthClientStub) RefreshToken(ctx context.Context, refreshToken, tokenEndpoint, proxyURL string) (*DimAgentOAuthTokenResponse, error) {
	if s.refresh == nil {
		return nil, nil
	}
	return s.refresh(ctx, refreshToken, tokenEndpoint, proxyURL)
}

func TestDimAgentOAuthExchangesOnlyMatchingCallbackURL(t *testing.T) {
	stub := &dimAgentOAuthClientStub{}
	service := NewDimAgentOAuthService(nil, stub)
	authorization, err := service.GenerateAuthURL(context.Background(), nil)
	require.NoError(t, err)
	parsedAuthURL, err := url.Parse(authorization.AuthURL)
	require.NoError(t, err)
	require.Equal(t, DimAgentOAuthClientID, parsedAuthURL.Query().Get("client_id"))
	require.Equal(t, DimAgentOAuthRedirectURI, parsedAuthURL.Query().Get("redirect_uri"))
	require.Equal(t, "S256", parsedAuthURL.Query().Get("code_challenge_method"))

	stub.exchange = func(_ context.Context, code, verifier, redirectURI, proxyURL string) (*DimAgentOAuthTokenResponse, error) {
		require.Equal(t, "dim-code", code)
		require.NotEmpty(t, verifier)
		require.Equal(t, DimAgentOAuthRedirectURI, redirectURI)
		require.Empty(t, proxyURL)
		return &DimAgentOAuthTokenResponse{AccessToken: "access-one", RefreshToken: "refresh-one", TokenType: "Bearer", ExpiresIn: 3600}, nil
	}
	callbackURL := DimAgentOAuthRedirectURI + "?code=dim-code&state=" + url.QueryEscape(parsedAuthURL.Query().Get("state"))
	_, err = service.ExchangeCallbackURL(context.Background(), &DimAgentExchangeCodeInput{SessionID: authorization.SessionID, CallbackURL: DimAgentOAuthRedirectURI + "?code=bad&state=wrong"})
	require.Error(t, err, "a mismatched callback must not consume the authorization session")
	_, err = service.ExchangeCallbackURL(context.Background(), &DimAgentExchangeCodeInput{SessionID: authorization.SessionID, CallbackURL: "ftp://localhost:63211/auth/callback?code=dim-code&state=x"})
	require.Error(t, err, "a non-http(s) carrier scheme must be rejected")
	require.Equal(t, "DIMAGENT_OAUTH_CALLBACK_INVALID", infraerrors.Reason(err), "a rejected carrier must not consume the session")
	// The pasted URL only carries code + state; its host and scheme are not the
	// authorization boundary. A deployment may expose the callback page through a
	// TLS reverse proxy, and the exchange must still send the registered redirect
	// URI to the token endpoint.
	carrierCallbackURL := "https://sub2api.example.com/auth/callback?code=dim-code&state=" + url.QueryEscape(parsedAuthURL.Query().Get("state"))
	info, err := service.ExchangeCallbackURL(context.Background(), &DimAgentExchangeCodeInput{SessionID: authorization.SessionID, CallbackURL: carrierCallbackURL})
	require.NoError(t, err)
	require.Equal(t, "access-one", info.AccessToken)
	require.Equal(t, "refresh-one", info.RefreshToken)
	require.Equal(t, DimAgentOAuthIssuer+"/v1", info.RelayBaseURL)

	_, err = service.ExchangeCallbackURL(context.Background(), &DimAgentExchangeCodeInput{SessionID: authorization.SessionID, CallbackURL: callbackURL})
	require.Error(t, err, "OAuth callback session must be single-use")
}

func TestDimAgentOAuthRefreshRetainsRotatedOrOriginalRefreshToken(t *testing.T) {
	stub := &dimAgentOAuthClientStub{refresh: func(_ context.Context, refreshToken, endpoint, proxyURL string) (*DimAgentOAuthTokenResponse, error) {
		require.Equal(t, "dim-refresh-token", refreshToken)
		require.Equal(t, DimAgentOAuthIssuer+"/oauth/token", endpoint)
		require.Empty(t, proxyURL)
		return &DimAgentOAuthTokenResponse{AccessToken: "access-new", TokenType: "Bearer", ExpiresIn: 1800}, nil
	}}
	service := NewDimAgentOAuthService(nil, stub)
	account := dimAgentTestAccount()
	account.Credentials["token_endpoint"] = DimAgentOAuthIssuer + "/oauth/token"
	info, err := service.RefreshAccountToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "access-new", info.AccessToken)
	require.Equal(t, "dim-refresh-token", info.RefreshToken)
	credentials := service.BuildAccountCredentials(info)
	require.Equal(t, "dim-refresh-token", credentials["refresh_token"])
}

func TestDimAgentProviderOnlyOffersChatCompletionsUpstream(t *testing.T) {
	account := dimAgentTestAccount()

	require.True(t, account.IsDimAgent())
	require.True(t, account.IsMultiProtocolAPIKey())
	require.True(t, account.RoutesProtocolByInbound())
	require.Equal(t, DefaultDimAgentBaseURL, (&Account{
		Platform: PlatformDimAgent, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "token"},
	}).GetOpenAIBaseURL())
	require.Equal(t, APIProtocolChatCompletions,
		resolveUpstreamProtocol(account, APIProtocolChatCompletions, "deepseek-v4.1-flash", nil))
	require.Equal(t, APIProtocolChatCompletions,
		resolveUpstreamProtocol(account, APIProtocolResponses, "deepseek-v4.1-flash", nil))
	require.Equal(t, APIProtocolChatCompletions,
		resolveUpstreamProtocol(account, APIProtocolAnthropic, "deepseek-v4.1-flash", nil))
}

func TestFetchDimAgentModelsUsesOAuthTokenAndDimQuery(t *testing.T) {
	account := dimAgentTestAccount()
	upstream := &httpUpstreamRecorder{resp: ordinaryModelsUpstreamResponse(`{"object":"list","data":[{"id":"deepseek-v4.1-flash","display_name":"DeepSeek V4.1 Flash"},{"id":"glm-5.3","display_name":"GLM 5.3"}]}`)}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	svc.SetDimAgentTokenProvider(&DimAgentTokenProvider{})

	response, err := svc.FetchOpenAIModelsList(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "/v1/models", upstream.lastReq.URL.Path)
	require.Equal(t, "dim", upstream.lastReq.URL.Query().Get("type"))
	require.Equal(t, "Bearer dim-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, DimAgentUserAgent, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, DimAgentTitle, upstream.lastReq.Header.Get("X-Title"))
	require.Equal(t, DimAgentReferer, upstream.lastReq.Header.Get("HTTP-Referer"))
	require.Contains(t, string(response.Body), `"id":"deepseek-v4.1-flash"`)
	require.Contains(t, string(response.Body), `"id":"glm-5.3"`)
}

func TestDimAgentRawForwardUsesManagedIdentityAndRecordsStreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"public-dim","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	// These must not reach the subscription relay unchanged.
	c.Request.Header.Set("User-Agent", "untrusted-client/1.0")
	c.Request.Header.Set("X-Title", "untrusted-title")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_dim","model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		``,
		`data: {"id":"chatcmpl_dim","model":"deepseek-v4.1-flash","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: dimAgentTestConfig(), httpUpstream: upstream}
	account := dimAgentTestAccount()
	svc.SetDimAgentTokenProvider(&DimAgentTokenProvider{})
	account.Credentials["model_mapping"] = map[string]any{"public-dim": "deepseek-v4.1-flash"}

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://dimagent.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer dim-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, DimAgentUserAgent, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, DimAgentTitle, upstream.lastReq.Header.Get("X-Title"))
	require.Equal(t, DimAgentReferer, upstream.lastReq.Header.Get("HTTP-Referer"))
	require.Equal(t, "deepseek-v4.1-flash", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
}

func TestDimAgentRawForwardRefreshesOnceAfterUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"expired"}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"chatcmpl_dim","model":"deepseek-v4.1-flash","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))},
	}}
	stub := &dimAgentOAuthClientStub{refresh: func(_ context.Context, refreshToken, endpoint, proxyURL string) (*DimAgentOAuthTokenResponse, error) {
		require.Equal(t, "dim-refresh-token", refreshToken)
		require.Equal(t, DimAgentOAuthIssuer+"/oauth/token", endpoint)
		require.Empty(t, proxyURL)
		return &DimAgentOAuthTokenResponse{AccessToken: "dim-refreshed-access", TokenType: "Bearer", ExpiresIn: 3600}, nil
	}}
	account := dimAgentTestAccount()
	account.Credentials["token_endpoint"] = DimAgentOAuthIssuer + "/oauth/token"
	svc := &OpenAIGatewayService{cfg: dimAgentTestConfig(), httpUpstream: upstream}
	svc.SetDimAgentTokenProvider(NewDimAgentTokenProvider(nil, NewDimAgentOAuthService(nil, stub), nil))

	_, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "Bearer dim-access-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer dim-refreshed-access", upstream.requests[1].Header.Get("Authorization"))
}

func TestDimAgentSubscriptionHeadersCannotBeOverridden(t *testing.T) {
	account := dimAgentTestAccount()
	account.Credentials["header_override_enabled"] = true
	account.Credentials["header_overrides"] = map[string]any{
		"user-agent": "operator-custom-client",
		"x-title":    "operator-custom-title",
	}
	headers := http.Header{"User-Agent": []string{"caller-client"}, "X-Title": []string{"caller-title"}, "HTTP-Referer": []string{"https://untrusted.example/"}}

	require.False(t, account.IsHeaderOverrideEligible())
	account.ApplyHeaderOverrides(headers)

	// Neither the caller nor an operator header override may replace the
	// official client identity the relay gates on.
	account.ApplyDimAgentSubscriptionHeaders(headers)
	require.Equal(t, DimAgentUserAgent, headers.Get("User-Agent"))
	require.Equal(t, DimAgentTitle, headers.Get("X-Title"))
	require.Equal(t, DimAgentReferer, headers.Get("HTTP-Referer"))
}

func TestDimAgentUserAgentOverrideKeepsOfficialProductToken(t *testing.T) {
	account := dimAgentTestAccount()

	const newer = "deepseek-harness/0.3.0 (+https://github.com/deepseek-ai/deepseek-harness)"
	account.Credentials["user_agent"] = newer
	require.Equal(t, newer, account.DimAgentUserAgentForAccount())

	// Any identity without the official product token would be rejected by the
	// relay, so it must not be able to replace the pinned official client.
	account.Credentials["user_agent"] = "DimAgent/0.5.8"
	require.Equal(t, DimAgentUserAgent, account.DimAgentUserAgentForAccount())
}

func TestDimAgentDoesNotUseUnverifiedBillingProbe(t *testing.T) {
	require.False(t, IsUpstreamBillingProbeIdentity(PlatformDimAgent, AccountTypeAPIKey))
}
