package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenCodeSessionTestContext(t *testing.T, value string) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if value != "" {
		c.Request.Header.Set(openCodeSessionHeader, value)
	}
	return c
}

func openCodeSessionTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
}

func openCodeSessionTestAccount(baseURL string) *Account {
	return &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url":                   baseURL,
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides:       map[string]any{"x-opencode-session": "fixed-account-value"},
		},
	}
}

func requireSingleOpenCodeSessionHeader(t *testing.T, headers http.Header, want string) {
	t.Helper()
	count := 0
	for key, values := range headers {
		if strings.EqualFold(key, openCodeSessionHeader) {
			count += len(values)
			require.Equal(t, []string{want}, values)
		}
	}
	require.Equal(t, 1, count)
}

func TestApplyOpenCodeSessionHeaderTrustBoundary(t *testing.T) {
	tests := []struct {
		name      string
		account   *Account
		targetURL string
		incoming  string
		want      string
	}{
		{
			name:      "official origin",
			account:   openCodeSessionTestAccount("https://opencode.ai/zen/v1"),
			targetURL: "https://opencode.ai/zen/v1/chat/completions",
			incoming:  " conversation-123 ",
			want:      "conversation-123",
		},
		{
			name:      "lookalike origin",
			account:   openCodeSessionTestAccount("https://opencode.ai.evil.example/v1"),
			targetURL: "https://opencode.ai.evil.example/v1/responses",
			incoming:  "conversation-123",
		},
		{
			name:      "subdomain is not implicitly trusted",
			account:   openCodeSessionTestAccount("https://api.opencode.ai/v1"),
			targetURL: "https://api.opencode.ai/v1/responses",
			incoming:  "conversation-123",
		},
		{
			name:      "insecure official origin",
			account:   openCodeSessionTestAccount("http://opencode.ai/zen/v1"),
			targetURL: "http://opencode.ai/zen/v1/responses",
			incoming:  "conversation-123",
		},
		{
			name:      "missing caller value generates session on go endpoint",
			account:   openCodeSessionTestAccount("https://opencode.ai/zen/go/v1"),
			targetURL: "https://opencode.ai/zen/go/v1/responses",
			want:      "<generated>",
		},
		{
			name:      "zen endpoint does not invent a session",
			account:   openCodeSessionTestAccount("https://opencode.ai/zen/v1"),
			targetURL: "https://opencode.ai/zen/v1/responses",
		},
		{
			name:      "oauth account",
			account:   &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			targetURL: "https://opencode.ai/zen/v1/responses",
			incoming:  "conversation-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := make(http.Header)
			applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, tt.incoming), tt.account, tt.targetURL, headers)
			got := headers.Get(openCodeSessionHeader)
			if tt.want == "<generated>" {
				require.NotEmpty(t, got)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestApplyOpenCodeSessionHeaderOpenCodeGoAlwaysSetsSession(t *testing.T) {
	account := &Account{
		ID:       4,
		Platform: PlatformOpenCodeGo,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://relay.example.com/v1",
		},
	}
	headers := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://relay.example.com/v1/chat/completions", headers)
	require.NotEmpty(t, headers.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderMapsCallerSessionID(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	c := newOpenCodeSessionTestContext(t, "")
	c.Request.Header.Set("session_id", "conv-from-client")
	headers := make(http.Header)
	applyOpenCodeSessionHeader(c, account, "https://opencode.ai/zen/go/v1/chat/completions", headers)
	require.Equal(t, "conv-from-client", headers.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderRejectsControlCharsInPromptCacheKey(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	body := []byte("{\"model\":\"glm-5.3\",\"prompt_cache_key\":\"a\\nb\",\"input\":\"hello\"}")
	headers := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://opencode.ai/zen/go/v1/responses", headers, body)
	got := headers.Get(openCodeSessionHeader)
	require.NotEmpty(t, got)
	require.NotContains(t, got, "\n")
	require.NotEqual(t, "a\nb", got)
}

func TestApplyOpenCodeSessionHeaderUsesPromptCacheKeyInsteadOfRandomUUID(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"grok-4.6","prompt_cache_key":"kimi-session-42","input":"hello"}`)
	headers := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://opencode.ai/zen/go/v1/responses", headers, body)
	require.Equal(t, "kimi-session-42", headers.Get(openCodeSessionHeader))

	headers2 := make(http.Header)
	laterTurn := []byte(`{"model":"grok-4.6","prompt_cache_key":"kimi-session-42","input":"follow up"}`)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://opencode.ai/zen/go/v1/responses", headers2, laterTurn)
	require.Equal(t, headers.Get(openCodeSessionHeader), headers2.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderUsesAnthropicMetadataUserID(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"minimax-m3","metadata":{"user_id":"coding-agent-session"},"messages":[{"role":"user","content":"hi"}]}`)
	headers := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://opencode.ai/zen/go/v1/messages", headers, body)
	require.Equal(t, "coding-agent-session", headers.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderUnwrapsClaudeCodeMetadataSessionJSON(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"claude-sonnet-4","metadata":{"user_id":"{\"session_id\":\"meta-session-xyz\"}"},"messages":[]}`)
	headers := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://opencode.ai/zen/go/v1/messages", headers, body)
	require.Equal(t, "meta-session-xyz", headers.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderBodyBeatsGeneratedUUIDAndLosesToCallerHeader(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	body := []byte(`{"prompt_cache_key":"from-body"}`)

	headers := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, "from-header"), account, "https://opencode.ai/zen/go/v1/responses", headers, body)
	require.Equal(t, "from-header", headers.Get(openCodeSessionHeader))

	converted := []byte(`{"model":"minimax-m3","messages":[]}`)
	headers2 := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://opencode.ai/zen/go/v1/messages", headers2, converted, body)
	require.Equal(t, "from-body", headers2.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderUsesRememberedInboundBodyAfterConversion(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}

	c := newOpenCodeSessionTestContext(t, "")
	rememberOpenCodeInboundBody(c, []byte(`{"model":"gpt-5","prompt_cache_key":"inbound-responses-session","input":"hello"}`))
	headers := make(http.Header)
	convertedCC := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hello"}]}`)
	applyOpenCodeSessionHeader(c, account, "https://opencode.ai/zen/go/v1/chat/completions", headers, convertedCC)
	require.Equal(t, "inbound-responses-session", headers.Get(openCodeSessionHeader))

	c2 := newOpenCodeSessionTestContext(t, "")
	rememberOpenCodeInboundBody(c2, []byte(`{"model":"grok-4.6","metadata":{"user_id":"inbound-messages-session"},"messages":[{"role":"user","content":"hi"}]}`))
	headers2 := make(http.Header)
	convertedResponses := []byte(`{"model":"grok-4.6","input":"hi"}`)
	applyOpenCodeSessionHeader(c2, account, "https://opencode.ai/zen/go/v1/responses", headers2, convertedResponses)
	require.Equal(t, "inbound-messages-session", headers2.Get(openCodeSessionHeader))
}

func TestOpenCodeSessionIDFromPayloadIgnoresEmptyBody(t *testing.T) {
	require.Empty(t, openCodeSessionIDFromPayload(nil))
	require.Empty(t, openCodeSessionIDFromPayload([]byte(`{"model":"gpt-5"}`)))
}

func TestOpenCodeSessionForwardedByResponsesBuildersAfterAccountOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := openCodeSessionTestService()
	account := openCodeSessionTestAccount("https://opencode.ai/zen/v1")
	body := []byte(`{"model":"gpt-5","input":"hello"}`)

	tests := []struct {
		name  string
		build func(*gin.Context) (*http.Request, error)
	}{
		{
			name: "normal responses",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequest(context.Background(), c, account, body, "token", false, "", false)
			},
		},
		{
			name: "passthrough responses",
			build: func(c *gin.Context) (*http.Request, error) {
				return svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "token")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newOpenCodeSessionTestContext(t, "conversation-456")
			req, err := tt.build(c)
			require.NoError(t, err)
			requireSingleOpenCodeSessionHeader(t, req.Header, "conversation-456")
		})
	}
}

func TestOpenCodeSessionForwardedFromPromptCacheKeyWithoutCallerHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := openCodeSessionTestService()
	account := &Account{
		ID:       1,
		Platform: PlatformOpenCodeGo,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://opencode.ai/zen/go/v1",
		},
	}
	c := newOpenCodeSessionTestContext(t, "")
	body := []byte(`{"model":"gpt-5","prompt_cache_key":"stable-cache-key","input":"hello"}`)
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "token", false, "", false)
	require.NoError(t, err)
	requireSingleOpenCodeSessionHeader(t, req.Header, "stable-cache-key")
}

func TestOpenCodeSessionMissingCallerValueKeepsExistingOverrideBehavior(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := openCodeSessionTestService()
	account := openCodeSessionTestAccount("https://opencode.ai/zen/v1")
	c := newOpenCodeSessionTestContext(t, "")

	req, err := svc.buildUpstreamRequest(
		context.Background(), c, account,
		[]byte(`{"model":"gpt-5","input":"hello"}`), "token", false, "", false,
	)
	require.NoError(t, err)
	require.Equal(t, "fixed-account-value", getHeaderRaw(req.Header, "x-opencode-session"))
}

type openCodeSessionHTTPUpstream struct {
	request *http.Request
}

func (u *openCodeSessionHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.request = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{}`)),
	}, nil
}

func (u *openCodeSessionHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestOpenCodeSessionForwardedByRawChatCompletionsAfterAccountOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openCodeSessionHTTPUpstream{}
	svc := openCodeSessionTestService()
	svc.httpUpstream = upstream
	account := openCodeSessionTestAccount("https://opencode.ai/zen/v1")
	c := newOpenCodeSessionTestContext(t, "conversation-789")

	resp, err := svc.sendCCUpstreamRequest(
		context.Background(), c, account,
		"https://opencode.ai/zen/v1/chat/completions", []byte(`{"model":"gpt-5"}`),
		false, "token", "", "",
	)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, upstream.request)
	requireSingleOpenCodeSessionHeader(t, upstream.request.Header, "conversation-789")
}

func TestOpenCodeSessionIsNotForwardedToOtherUpstreams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := openCodeSessionTestService()
	body := []byte(`{"model":"gpt-5","input":"hello"}`)

	for _, baseURL := range []string{
		"https://api.openai.com/v1",
		"https://opencode.ai.evil.example/v1",
		"https://api.opencode.ai/v1",
	} {
		t.Run(baseURL, func(t *testing.T) {
			account := &Account{
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Credentials: map[string]any{"base_url": baseURL},
			}
			c := newOpenCodeSessionTestContext(t, "private-conversation")
			req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "token", false, "", false)
			require.NoError(t, err)
			require.Empty(t, req.Header.Get(openCodeSessionHeader))
		})
	}
}
