//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const cloudflareMessagesTestBaseURL = "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai"

func TestAnthropicMessagesURL_EntryPoints(t *testing.T) {
	for _, tt := range []struct {
		name    string
		baseURL string
		query   string
	}{
		{"cloudflare REST", cloudflareMessagesTestBaseURL, ""},
		{"cloudflare REST trailing slash", cloudflareMessagesTestBaseURL + "/", ""},
		{"anthropic default", "", "beta=true"},
		{"anthropic explicit", "https://api.anthropic.com", "beta=true"},
		{"custom upstream", "https://anthropic.example.com", "beta=true"},
		{"cloudflare native gateway", "https://gateway.ai.cloudflare.com/v1/account/gateway/anthropic", "beta=true"},
		{"lookalike host", "https://api.cloudflare.com.example.com/client/v4/accounts/account/ai", "beta=true"},
		{"other cloudflare path", "https://api.cloudflare.com/client/v4/accounts/account/ai-gateway", "beta=true"},
		{"extra cloudflare path segment", cloudflareMessagesTestBaseURL + "/other", "beta=true"},
		{"missing cloudflare account", "https://api.cloudflare.com/client/v4/accounts//ai", "beta=true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := newAnthropicAPIKeyAccountForTest()
			account.Credentials["base_url"] = tt.baseURL
			account.Credentials[credKeyHeaderOverrideEnabled] = true
			account.Credentials["header_overrides"] = map[string]any{"cf-aig-gateway-id": "synthetic-gateway"}
			account.Extra[anthropicAPIKeyAuthSchemeExtraKey] = AnthropicAPIKeyAuthSchemeAuthorizationBearer
			body := []byte(`{"model":"claude-haiku-4-5","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
			baseURL := strings.TrimRight(tt.baseURL, "/")
			if baseURL == "" {
				baseURL = "https://api.anthropic.com"
			}
			wantURL := baseURL + "/v1/messages"
			if tt.query != "" {
				wantURL += "?" + tt.query
			}
			checkRequest := func(t *testing.T, req *http.Request) {
				t.Helper()
				require.NotNil(t, req)
				require.Equal(t, wantURL, req.URL.String())
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "Bearer upstream-anthropic-key", getHeaderRaw(req.Header, "authorization"))
				require.Empty(t, getHeaderRaw(req.Header, "x-api-key"))
				require.Equal(t, "synthetic-gateway", getHeaderRaw(req.Header, "cf-aig-gateway-id"))
				require.Equal(t, "2023-06-01", getHeaderRaw(req.Header, "anthropic-version"))
			}
			svc := &GatewayService{cfg: &config.Config{}}
			t.Run("forward", func(t *testing.T) {
				req, wireBody, err := svc.buildUpstreamRequest(context.Background(), gatewayAnthropicBaseURLContext(t), account, body, "upstream-anthropic-key", "apikey", "claude-haiku-4-5", false, false)
				require.NoError(t, err)
				checkRequest(t, req)
				require.JSONEq(t, string(body), string(wireBody))
			})
			t.Run("passthrough", func(t *testing.T) {
				req, wireBody, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), gatewayAnthropicBaseURLContext(t), account, body, "upstream-anthropic-key")
				require.NoError(t, err)
				checkRequest(t, req)
				require.JSONEq(t, string(body), string(wireBody))
			})
			t.Run("account test", func(t *testing.T) {
				upstream := &anthropicHTTPUpstreamRecorder{resp: adaptiveCNAnthropicTestResponse()}
				testSvc := &AccountTestService{cfg: &config.Config{}, httpUpstream: upstream}
				c, rec := newTestContext()
				require.NoError(t, testSvc.testClaudeAccountConnection(c, account, "claude-haiku-4-5"))
				checkRequest(t, upstream.lastReq)
				require.Equal(t, "claude-haiku-4-5", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Contains(t, rec.Body.String(), `"success":true`)
			})
		})
	}
}

// Simulate the REST endpoint's strict query handling without contacting Cloudflare.
type cloudflareMessagesTestUpstream struct {
	anthropicHTTPUpstreamRecorder
}

func (u *cloudflareMessagesTestUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.resp = adaptiveCNAnthropicTestResponse()
	if req.URL.RawQuery != "" {
		u.resp.StatusCode = http.StatusBadRequest
		u.resp.Body = io.NopCloser(strings.NewReader(`{"errors":[{"code":"unrecognized_keys","keys":["query"]}],"success":false}`))
	}
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestAccountTestService_CloudflareMessagesRejectsBetaQuery(t *testing.T) {
	account := newAnthropicAPIKeyAccountForTest()
	account.Credentials["base_url"] = cloudflareMessagesTestBaseURL
	account.Extra[anthropicAPIKeyAuthSchemeExtraKey] = AnthropicAPIKeyAuthSchemeAuthorizationBearer
	upstream := &cloudflareMessagesTestUpstream{}
	svc := &AccountTestService{cfg: &config.Config{}, httpUpstream: upstream}
	c, rec := newTestContext()
	require.NoError(t, svc.testClaudeAccountConnection(c, account, "claude-haiku-4-5"))
	require.Contains(t, rec.Body.String(), `"success":true`)
	require.NotContains(t, rec.Body.String(), "API returned 400")
}

func TestAnthropicMessagesURL_RespectsURLAllowlist(t *testing.T) {
	account := newAnthropicAPIKeyAccountForTest()
	account.Credentials["base_url"] = cloudflareMessagesTestBaseURL
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = true
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"api.anthropic.com"}
	svc := &GatewayService{cfg: cfg}
	body := []byte(`{"model":"claude-haiku-4-5","messages":[]}`)
	c := gatewayAnthropicBaseURLContext(t)
	req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "test-key", "apikey", "claude-haiku-4-5", false, false)
	require.ErrorContains(t, err, "host is not allowed")
	require.Nil(t, req)
	req, _, err = svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "test-key")
	require.ErrorContains(t, err, "host is not allowed")
	require.Nil(t, req)
	upstream := &anthropicHTTPUpstreamRecorder{}
	testSvc := &AccountTestService{cfg: cfg, httpUpstream: upstream}
	c, _ = newTestContext()
	require.ErrorContains(t, testSvc.testClaudeAccountConnection(c, account, "claude-haiku-4-5"), "host is not allowed")
	require.Nil(t, upstream.lastReq)
}
