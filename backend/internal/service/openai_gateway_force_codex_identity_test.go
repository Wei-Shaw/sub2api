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
)

const forceCodexIdentityTUIUserAgent = "codex-tui/0.145.0 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.145.0)"

func newForceCodexIdentityGinContext(path string, headers map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(""))
	for key, value := range headers {
		c.Request.Header.Set(key, value)
	}
	return c
}

func newForceCodexIdentityOAuthAccount(extra map[string]any) *Account {
	return &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
		Extra:       extra,
	}
}

func newForceCodexIdentityPassthroughUpstream() *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid"}},
		Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
	}}
}

func newForceCodexIdentityPassthroughAccount(id int64, enabled bool) *Account {
	extra := map[string]any{"openai_passthrough": true}
	if enabled {
		extra["force_codex_identity"] = true
	}
	return &Account{
		ID:             id,
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
		Extra:          extra,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}
}

func forceCodexIdentityInputHeaders() map[string]string {
	return map[string]string{
		"User-Agent": forceCodexIdentityTUIUserAgent,
		"originator": "opencode",
	}
}

func TestOpenAIBuildUpstreamRequest_ForceCodexIdentity(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		enabled        bool
		wantUserAgent  string
		wantOriginator string
		wantVersion    string
	}{
		{
			name:           "默认关闭时保留最终UA并由终态逻辑自动配对",
			path:           "/v1/responses",
			wantUserAgent:  forceCodexIdentityTUIUserAgent,
			wantOriginator: "codex-tui",
		},
		{
			name:           "标准Responses开启时固定UA并由终态逻辑配对",
			path:           "/v1/responses",
			enabled:        true,
			wantUserAgent:  codexCLIUserAgent,
			wantOriginator: "codex_cli_rs",
		},
		{
			name:           "compact开启时固定Codex CLI UA",
			path:           "/responses/compact",
			enabled:        true,
			wantUserAgent:  codexCLIUserAgent,
			wantOriginator: "codex_cli_rs",
			wantVersion:    codexCLIVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extra := map[string]any{}
			if tt.enabled {
				extra["force_codex_identity"] = true
			}
			c := newForceCodexIdentityGinContext(tt.path, forceCodexIdentityInputHeaders())
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			account := newForceCodexIdentityOAuthAccount(extra)

			req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", false)
			require.NoError(t, err)
			require.Equal(t, tt.wantUserAgent, req.Header.Get("user-agent"))
			require.Equal(t, tt.wantOriginator, req.Header.Get("originator"))
			require.Equal(t, tt.wantVersion, req.Header.Get("version"))
		})
	}
}

func TestOpenAIBuildUpstreamRequest_ForceCodexIdentity_MessagesBridgeIsUnchanged(t *testing.T) {
	c := newForceCodexIdentityGinContext("/v1/responses", map[string]string{
		"User-Agent": "luna/1.0.0",
		"originator": "opencode",
	})
	setOpenAICompatMessagesBridgeContext(c, true)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := newForceCodexIdentityOAuthAccount(map[string]any{"force_codex_identity": true})

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.5"}`), "token", false, "", false)
	require.NoError(t, err)
	require.Equal(t, "luna/1.0.0", req.Header.Get("user-agent"))
	require.Empty(t, req.Header.Get("originator"))
	require.Empty(t, req.Header.Get("OpenAI-Beta"))
}

func TestOpenAIPassthrough_ForceCodexIdentity(t *testing.T) {
	const body = `{"model":"gpt-5.2","stream":true,"store":false,"input":[{"type":"text","text":"hi"}]}`
	tests := []struct {
		name           string
		enabled        bool
		wantUserAgent  string
		wantOriginator string
	}{
		{
			name:           "默认关闭时保留最终UA并由终态逻辑自动配对",
			wantUserAgent:  forceCodexIdentityTUIUserAgent,
			wantOriginator: "codex-tui",
		},
		{
			name:           "开启时固定Codex CLI UA",
			enabled:        true,
			wantUserAgent:  codexCLIUserAgent,
			wantOriginator: "codex_cli_rs",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newForceCodexIdentityGinContext("/v1/responses", forceCodexIdentityInputHeaders())
			upstream := newForceCodexIdentityPassthroughUpstream()
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := newForceCodexIdentityPassthroughAccount(int64(i+1), tt.enabled)

			_, err := svc.Forward(context.Background(), c, account, []byte(body))
			require.NoError(t, err)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, tt.wantUserAgent, upstream.lastReq.Header.Get("user-agent"))
			require.Equal(t, tt.wantOriginator, upstream.lastReq.Header.Get("originator"))
		})
	}
}

func TestBuildOpenAIWSHeaders_ForceCodexIdentity(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		wantUserAgent  string
		wantOriginator string
	}{
		{
			name:           "默认关闭时保留最终UA并由终态逻辑自动配对",
			wantUserAgent:  forceCodexIdentityTUIUserAgent,
			wantOriginator: "codex-tui",
		},
		{
			name:           "开启时固定Codex CLI UA",
			enabled:        true,
			wantUserAgent:  codexCLIUserAgent,
			wantOriginator: "codex_cli_rs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extra := map[string]any{}
			if tt.enabled {
				extra["force_codex_identity"] = true
			}
			c := newForceCodexIdentityGinContext("/v1/responses", forceCodexIdentityInputHeaders())
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			account := newForceCodexIdentityOAuthAccount(extra)

			headers, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "token", OpenAIWSProtocolDecision{}, false, "", "", "")
			require.NoError(t, err)
			require.Equal(t, tt.wantUserAgent, headers.Get("user-agent"))
			require.Equal(t, tt.wantOriginator, headers.Get("originator"))
		})
	}
}
