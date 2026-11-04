//go:build unit

package service

import (
	"context"
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

func TestAnthropicAPIVersionRequestBuilders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &GatewayService{cfg: &config.Config{}}
	ctx := context.Background()
	body := []byte(`{"model":"claude-sonnet-4-6","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`)
	type buildFunc func(*gin.Context, *Account) (*http.Request, error)
	builders := []struct {
		name  string
		typ   string
		build buildFunc
	}{
		{"messages/apikey", AccountTypeAPIKey, func(c *gin.Context, a *Account) (*http.Request, error) {
			r, _, err := svc.buildUpstreamRequest(ctx, c, a, body, "test-key", "apikey", "claude-sonnet-4-6", false, false)
			return r, err
		}},
		{"messages/oauth", AccountTypeOAuth, func(c *gin.Context, a *Account) (*http.Request, error) {
			r, _, err := svc.buildUpstreamRequest(ctx, c, a, body, "test-token", "oauth", "claude-sonnet-4-6", false, false)
			return r, err
		}},
		{"messages/mimic", AccountTypeOAuth, func(c *gin.Context, a *Account) (*http.Request, error) {
			r, _, err := svc.buildUpstreamRequest(ctx, c, a, body, "test-token", "oauth", "claude-sonnet-4-6", true, true)
			return r, err
		}},
		{"messages/passthrough", AccountTypeAPIKey, func(c *gin.Context, a *Account) (*http.Request, error) {
			r, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(ctx, c, a, body, "test-key")
			return r, err
		}},
		{"count/apikey", AccountTypeAPIKey, func(c *gin.Context, a *Account) (*http.Request, error) {
			r, _, err := svc.buildCountTokensRequest(ctx, c, a, body, "test-key", "apikey", "claude-sonnet-4-6", false)
			return r, err
		}},
		{"count/oauth", AccountTypeOAuth, func(c *gin.Context, a *Account) (*http.Request, error) {
			r, _, err := svc.buildCountTokensRequest(ctx, c, a, body, "test-token", "oauth", "claude-sonnet-4-6", false)
			return r, err
		}},
		{"count/mimic", AccountTypeOAuth, func(c *gin.Context, a *Account) (*http.Request, error) {
			r, _, err := svc.buildCountTokensRequest(ctx, c, a, body, "test-token", "oauth", "claude-sonnet-4-6", true)
			return r, err
		}},
		{"count/passthrough", AccountTypeAPIKey, func(c *gin.Context, a *Account) (*http.Request, error) {
			return svc.buildCountTokensRequestAnthropicAPIKeyPassthrough(ctx, c, a, body, "test-key")
		}},
	}
	for _, builder := range builders {
		for _, tc := range []struct{ name, client, override, want string }{
			{"default", "", "", "2026-10-21"},
			{"explicit_latest", "2026-10-21", "", "2026-10-21"},
			{"explicit_legacy", "2023-06-01", "", "2023-06-01"},
			{"override_latest", "2023-06-01", "2026-10-21", "2026-10-21"},
			{"override_legacy", "2026-10-21", "2023-06-01", "2023-06-01"},
		} {
			t.Run(builder.name+"/"+tc.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				if tc.client != "" {
					c.Request.Header["anthropic-version"] = []string{tc.client}
				}
				a := &Account{Platform: PlatformAnthropic, Type: builder.typ, Credentials: map[string]any{}}
				want := tc.want
				if tc.override != "" {
					a.Credentials[credKeyHeaderOverrideEnabled] = true
					a.Credentials[credKeyHeaderOverrides] = map[string]any{"anthropic-version": tc.override}
					if builder.typ == AccountTypeOAuth {
						want = tc.client // Account overrides are intentionally disabled for OAuth.
					}
				}
				r, err := builder.build(c, a)
				require.NoError(t, err)
				t.Cleanup(func() { _ = r.Body.Close() })
				require.Equal(t, want, getHeaderRaw(r.Header, "anthropic-version"))
			})
		}
	}

	// Shared Anthropic builders also serve other providers; their default stays unchanged.
	a := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	r, _, err := svc.buildUpstreamRequest(ctx, nil, a, body, "test-key", "apikey", "deepseek-chat", false, false)
	require.NoError(t, err)
	defer r.Body.Close()
	require.Equal(t, "2023-06-01", getHeaderRaw(r.Header, "anthropic-version"))
}

func TestAnthropicAPIVersionOAuthBody(t *testing.T) {
	svc := &GatewayService{cfg: &config.Config{}}
	a := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	for _, version := range []string{"", "2026-10-21", "2023-06-01"} {
		t.Run("version_"+version, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("Anthropic-Version", version)
			body := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello"}]}`)
			wire := svc.applyClaudeCodeOAuthMimicryToBody(context.Background(), c, a, body, nil, "claude-sonnet-4-6")
			require.Equal(t, version == "2023-06-01", gjson.GetBytes(wire, "temperature").Exists())
			if version == "2023-06-01" {
				require.Equal(t, int64(1), gjson.GetBytes(wire, "temperature").Int())
			}
		})
	}
	// Do not silently discard client sampling intent. Invalid 2026 requests get
	// the provider's validation error; callers can explicitly select the legacy version.
	body := []byte(`{"model":"claude-sonnet-4-6","temperature":0.2,"top_p":0.9,"top_k":10,"messages":[]}`)
	wire := svc.applyClaudeCodeOAuthMimicryToBody(context.Background(), nil, a, body, nil, "claude-sonnet-4-6")
	for _, field := range []string{"temperature", "top_p", "top_k"} {
		require.Equal(t, gjson.GetBytes(body, field).Raw, gjson.GetBytes(wire, field).Raw)
	}
}

func TestAnthropicAPIVersionProbes(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "latest", true: "legacy_override"}[legacy], func(t *testing.T) {
			a := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
			want := "2026-10-21"
			if legacy {
				want = "2023-06-01"
				a.Credentials[credKeyHeaderOverrideEnabled] = true
				a.Credentials[credKeyHeaderOverrides] = map[string]any{"anthropic-version": want}
			}
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{},
				Body: io.NopCloser(strings.NewReader("data: {\"type\":\"message_stop\"}\n\n")),
			}}
			svc := &AccountTestService{cfg: &config.Config{}, httpUpstream: upstream}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
			require.NoError(t, svc.testClaudeAccountConnection(c, a, "claude-sonnet-4-6"))
			require.Equal(t, want, getHeaderRaw(upstream.lastReq.Header, "anthropic-version"))
			require.Equal(t, legacy, gjson.GetBytes(upstream.lastBody, "temperature").Exists())
			r, err := svc.buildAnthropicUpstreamModelsRequest(context.Background(), a)
			require.NoError(t, err)
			require.Equal(t, want, getHeaderRaw(r.Header, "anthropic-version"))
		})
	}
	require.Equal(t, "2026-10-21", providerAdapters[MonitorProviderAnthropic].buildHeaders("test-key")["anthropic-version"])
}

func TestAnthropicAPIVersionOpenAICompatibility(t *testing.T) {
	endpoints := []struct {
		name string
		body string
		call func(*GatewayService, *gin.Context, *Account, []byte) (*ForwardResult, error)
	}{
		{"responses", `{"model":"claude-sonnet-4-6","input":"hello"}`, func(s *GatewayService, c *gin.Context, a *Account, body []byte) (*ForwardResult, error) {
			return s.ForwardAsResponses(context.Background(), c, a, body, nil)
		}},
		{"chat/completions", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello"}]}`, func(s *GatewayService, c *gin.Context, a *Account, body []byte) (*ForwardResult, error) {
			return s.ForwardAsChatCompletions(context.Background(), c, a, body, nil)
		}},
	}
	for _, endpoint := range endpoints {
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
			for _, version := range []string{"", "2026-10-21", "2023-06-01"} {
				t.Run(endpoint.name+"/"+accountType+"/version_"+version, func(t *testing.T) {
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK, Header: http.Header{},
						Body: io.NopCloser(strings.NewReader(namespaceToolAnthropicStream())),
					}}
					svc := &GatewayService{cfg: &config.Config{}, httpUpstream: upstream}
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+endpoint.name, strings.NewReader(endpoint.body))
					c.Request.Header.Set("Anthropic-Version", version)
					a := &Account{Platform: PlatformAnthropic, Type: accountType,
						Credentials: map[string]any{"api_key": "test-key", "access_token": "test-token"}}
					result, err := endpoint.call(svc, c, a, []byte(endpoint.body))
					require.NoError(t, err)
					require.NotNil(t, result)
					want := version
					if want == "" {
						want = "2026-10-21"
					}
					require.Equal(t, want, getHeaderRaw(upstream.lastReq.Header, "anthropic-version"))
					require.Equal(t, accountType == AccountTypeOAuth && version == "2023-06-01",
						gjson.GetBytes(upstream.lastBody, "temperature").Exists())
				})
			}
		}
	}
}
