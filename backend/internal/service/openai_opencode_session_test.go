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
			name:      "zen endpoint generates a session",
			account:   openCodeSessionTestAccount("https://opencode.ai/zen/v1"),
			targetURL: "https://opencode.ai/zen/v1/responses",
			want:      "<generated>",
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
			switch tt.want {
			case "":
				require.Empty(t, got)
			case "<generated>":
				require.Regexp(t, `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`, got)
			default:
				require.Equal(t, normalizeOpenCodeSessionID(tt.want), got)
			}
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
	require.Regexp(t, `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`, headers.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderOpenCodeZenWithoutCallerSession(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenCodeGo,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode": AccountModeZen,
			"base_url":     "https://opencode.ai/zen/v1",
		},
	}
	for _, targetURL := range []string{
		"https://opencode.ai/zen/v1/chat/completions",
		"https://opencode.ai/zen/v1/responses",
		"https://opencode.ai/zen/v1/messages",
		"https://relay.example.com/v1/chat/completions",
	} {
		t.Run(targetURL, func(t *testing.T) {
			headers := make(http.Header)
			applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, targetURL, headers)
			require.Regexp(t, `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`, headers.Get(openCodeSessionHeader))
		})
	}
}

func TestApplyOpenCodeClientHeadersAddsNativeMetadata(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	headers := make(http.Header)
	applyOpenCodeClientHeaders(newOpenCodeSessionTestContext(t, ""), account, "https://relay.example.com/v1/chat/completions", headers)
	require.Equal(t, "cli", headers.Get(openCodeClientHeader))
	require.Equal(t, "global", headers.Get(openCodeProjectHeader))
	require.Regexp(t, `^msg_[0-9a-f]{12}[0-9A-Za-z]{14}$`, headers.Get(openCodeRequestHeader))
	require.NotEqual(t, headers.Get(openCodeSessionHeader), headers.Get(openCodeRequestHeader))
}

func TestApplyOpenCodeSessionHeaderMapsCallerSessionID(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	const targetURL = "https://opencode.ai/zen/go/v1/chat/completions"

	withCallerID := func() string {
		c := newOpenCodeSessionTestContext(t, "")
		c.Request.Header.Set("session_id", "conv-from-client")
		headers := make(http.Header)
		applyOpenCodeSessionHeader(c, account, targetURL, headers)
		return headers.Get(openCodeSessionHeader)
	}

	first := withCallerID()
	require.Regexp(t, `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`, first)
	require.NotEqual(t, "conv-from-client", first, "客户端不透明会话串不得原样出站")
	require.Equal(t, first, withCallerID(), "同一会话跨轮必须稳定，否则丢失上游 prompt cache")
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
	const targetURL = "https://opencode.ai/zen/go/v1/responses"

	turn := func(input string) string {
		body := []byte(`{"model":"grok-4.6","prompt_cache_key":"kimi-session-42","input":"` + input + `"}`)
		headers := make(http.Header)
		applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, targetURL, headers, body)
		return headers.Get(openCodeSessionHeader)
	}

	first := turn("hello")
	require.Equal(t, normalizeOpenCodeSessionID("kimi-session-42"), first)
	require.Equal(t, first, turn("follow up"), "同一 prompt_cache_key 跨轮不得变化")
}

func TestApplyOpenCodeSessionHeaderUsesAnthropicMetadataUserID(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"minimax-m3","metadata":{"user_id":"coding-agent-session"},"messages":[{"role":"user","content":"hi"}]}`)
	headers := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://opencode.ai/zen/go/v1/messages", headers, body)
	require.Equal(t, normalizeOpenCodeSessionID("coding-agent-session"), headers.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderUnwrapsClaudeCodeMetadataSessionJSON(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"claude-sonnet-4","metadata":{"user_id":"{\"session_id\":\"meta-session-xyz\"}"},"messages":[]}`)
	headers := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://opencode.ai/zen/go/v1/messages", headers, body)
	require.Equal(t, normalizeOpenCodeSessionID("meta-session-xyz"), headers.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderBodyBeatsGeneratedUUIDAndLosesToCallerHeader(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	body := []byte(`{"prompt_cache_key":"from-body"}`)

	headers := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, "from-header"), account, "https://opencode.ai/zen/go/v1/responses", headers, body)
	require.Equal(t, normalizeOpenCodeSessionID("from-header"), headers.Get(openCodeSessionHeader))

	converted := []byte(`{"model":"minimax-m3","messages":[]}`)
	headers2 := make(http.Header)
	applyOpenCodeSessionHeader(newOpenCodeSessionTestContext(t, ""), account, "https://opencode.ai/zen/go/v1/messages", headers2, converted, body)
	require.Equal(t, normalizeOpenCodeSessionID("from-body"), headers2.Get(openCodeSessionHeader))
}

func TestApplyOpenCodeSessionHeaderUsesRememberedInboundBodyAfterConversion(t *testing.T) {
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}

	c := newOpenCodeSessionTestContext(t, "")
	rememberOpenCodeInboundBody(c, []byte(`{"model":"gpt-5","prompt_cache_key":"inbound-responses-session","input":"hello"}`))
	headers := make(http.Header)
	convertedCC := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hello"}]}`)
	applyOpenCodeSessionHeader(c, account, "https://opencode.ai/zen/go/v1/chat/completions", headers, convertedCC)
	require.Equal(t, normalizeOpenCodeSessionID("inbound-responses-session"), headers.Get(openCodeSessionHeader))

	c2 := newOpenCodeSessionTestContext(t, "")
	rememberOpenCodeInboundBody(c2, []byte(`{"model":"grok-4.6","metadata":{"user_id":"inbound-messages-session"},"messages":[{"role":"user","content":"hi"}]}`))
	headers2 := make(http.Header)
	convertedResponses := []byte(`{"model":"grok-4.6","input":"hi"}`)
	applyOpenCodeSessionHeader(c2, account, "https://opencode.ai/zen/go/v1/responses", headers2, convertedResponses)
	require.Equal(t, normalizeOpenCodeSessionID("inbound-messages-session"), headers2.Get(openCodeSessionHeader))
}

func TestNormalizeOpenCodeSessionID(t *testing.T) {
	// 真实 OpenCode 客户端的合规 session id 原样透出，保留其上游 prompt cache。
	already := "ses_0123456789abAbCdEfGhIjKlMn"
	require.True(t, isOpenCodeSessionID(already))
	require.Equal(t, already, normalizeOpenCodeSessionID(already))

	// 非合规形状（含账号 header_overrides 里的固定串）必须重编码为 ses_ 形状，
	// 否则 Zen 免费档以 403 FreeTierError 拒绝（回归 3ae63e8f8）。
	for _, foreign := range []string{
		"cliproxy-opencode-go-session",
		"fixed-account-value",
		"conversation-123",
		"ses_x",
		"ses_0123456789ABAbCdEfGhIjKl", // 大写 hex 前缀同样被拒
		"550e8400-e29b-41d4-a716-446655440000",
	} {
		got := normalizeOpenCodeSessionID(foreign)
		require.True(t, isOpenCodeSessionID(got), "%q 必须重编码为 ses_ 形状, got %q", foreign, got)
		require.NotEqual(t, foreign, got)
		require.Equal(t, got, normalizeOpenCodeSessionID(foreign), "同一输入必须得到同一 id")
	}
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
			requireSingleOpenCodeSessionHeader(t, req.Header, normalizeOpenCodeSessionID("conversation-456"))
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
	requireSingleOpenCodeSessionHeader(t, req.Header, normalizeOpenCodeSessionID("stable-cache-key"))
}

func TestOpenCodeSessionMissingCallerValueKeepsExistingOverrideBehavior(t *testing.T) {
	// 回归：账号 header_overrides 里配置的固定 session 串（如
	// "cliproxy-opencode-go-session"）不是客户端 ses_ 形状，原样出站会被 Zen
	// 免费档以 403 FreeTierError 拒绝；出站前必须重编码，且保持稳定。
	gin.SetMode(gin.TestMode)
	svc := openCodeSessionTestService()
	account := openCodeSessionTestAccount("https://opencode.ai/zen/v1")
	c := newOpenCodeSessionTestContext(t, "")

	req, err := svc.buildUpstreamRequest(
		context.Background(), c, account,
		[]byte(`{"model":"gpt-5","input":"hello"}`), "token", false, "", false,
	)
	require.NoError(t, err)
	require.Equal(t, normalizeOpenCodeSessionID("fixed-account-value"), getHeaderRaw(req.Header, "x-opencode-session"))
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

func TestApplyOpenCodeUpstreamUserAgent(t *testing.T) {
	tests := []struct {
		name      string
		account   *Account
		targetURL string
		seed      string
		want      string
	}{
		{
			name:      "opencode go account canonicalizes passthrough UA",
			account:   &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey},
			targetURL: "https://opencode.ai/zen/go/v1/chat/completions",
			seed:      "Python-urllib/3.13",
			want:      openCodeUpstreamUserAgent,
		},
		{
			name:      "opencode go account canonicalizes on custom relay too",
			account:   &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey},
			targetURL: "https://relay.example.com/v1/chat/completions",
			seed:      "Python-urllib/3.13",
			want:      openCodeUpstreamUserAgent,
		},
		{
			name:      "non-opencode account targeting official host",
			account:   &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			targetURL: "https://opencode.ai/zen/go/v1/responses",
			seed:      "Python-urllib/3.13",
			want:      openCodeUpstreamUserAgent,
		},
		{
			name:      "official command code host uses canonical Codex UA",
			account:   &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			targetURL: "https://api.commandcode.ai/provider/v1/responses",
			seed:      "Python-urllib/3.13",
			want:      CodexCanonicalUserAgent(),
		},
		{
			name:      "command code lookalike host keeps client UA",
			account:   &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			targetURL: "https://api.commandcode.ai.evil.example/provider/v1/responses",
			seed:      "Python-urllib/3.13",
			want:      "Python-urllib/3.13",
		},
		{
			name:      "command code subdomain keeps client UA",
			account:   &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			targetURL: "https://proxy.api.commandcode.ai/provider/v1/responses",
			seed:      "Python-urllib/3.13",
			want:      "Python-urllib/3.13",
		},
		{
			name:      "insecure command code host keeps client UA",
			account:   &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			targetURL: "http://api.commandcode.ai/provider/v1/responses",
			seed:      "Python-urllib/3.13",
			want:      "Python-urllib/3.13",
		},
		{
			name:      "non-official account and host keeps client UA",
			account:   &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			targetURL: "https://api.openai.com/v1/chat/completions",
			seed:      "Python-urllib/3.13",
			want:      "Python-urllib/3.13",
		},
		{
			name:      "lookalike host does not count as official",
			account:   &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			targetURL: "https://opencode.ai.evil.example/v1/chat/completions",
			seed:      "Python-urllib/3.13",
			want:      "Python-urllib/3.13",
		},
		{
			name:      "missing UA is filled for opencode account",
			account:   &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey},
			targetURL: "https://opencode.ai/zen/go/v1/messages",
			want:      openCodeUpstreamUserAgent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := make(http.Header)
			if tt.seed != "" {
				headers.Set("User-Agent", tt.seed)
			}
			applyOpenCodeUpstreamUserAgent(tt.account, tt.targetURL, headers)
			require.Equal(t, tt.want, headers.Get("User-Agent"))
		})
	}
}

func TestApplyOpenCodeUpstreamUserAgentDropsNonCanonicalKeyVariants(t *testing.T) {
	headers := http.Header{"user-agent": []string{"Python-urllib/3.13"}}
	applyOpenCodeUpstreamUserAgent(&Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey},
		"https://opencode.ai/zen/go/v1/chat/completions", headers)

	count := 0
	for key, values := range headers {
		if strings.EqualFold(key, "User-Agent") {
			count += len(values)
			require.Equal(t, []string{openCodeUpstreamUserAgent}, values)
		}
	}
	require.Equal(t, 1, count)
}

func TestApplyOpenCodeUpstreamUserAgentNilSafe(t *testing.T) {
	applyOpenCodeUpstreamUserAgent(&Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey},
		"https://opencode.ai/zen/go/v1/chat/completions", nil)
}

func TestOpenCodeUpstreamUserAgentCanonicalizedAfterPassthrough(t *testing.T) {
	// 回归：客户端透传的编程库 UA（Python-urllib）会被 opencode.ai 前置
	// Cloudflare 以 error code 1010 拦截并计入账号 403 strike，导致健康账号
	// 被自动禁用。出站 UA 必须收敛为规范 opencode 客户端身份。
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
	c.Request.Header.Set("User-Agent", "Python-urllib/3.13")

	req, err := svc.buildUpstreamRequest(
		context.Background(), c, account,
		[]byte(`{"model":"glm-5.3","input":"hello"}`), "token", false, "", false,
	)
	require.NoError(t, err)
	require.Equal(t, openCodeUpstreamUserAgent, req.Header.Get("User-Agent"))
}

func TestCommandCodeUpstreamUserAgentCanonicalizedAfterPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := openCodeSessionTestService()
	account := &Account{
		ID:       2,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://api.commandcode.ai/provider/v1",
		},
	}
	c := newOpenCodeSessionTestContext(t, "")
	c.Request.Header.Set("User-Agent", "Python-urllib/3.13")

	req, err := svc.buildUpstreamRequest(
		context.Background(), c, account,
		[]byte(`{"model":"gpt-5","input":"hello"}`), "token", false, "", false,
	)
	require.NoError(t, err)
	require.Equal(t, CodexCanonicalUserAgent(), req.Header.Get("User-Agent"))
}

func TestOpenCodeUpstreamUserAgentYieldsToExplicitAccountOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := openCodeSessionTestService()
	account := &Account{
		ID:       1,
		Platform: PlatformOpenCodeGo,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url":                   "https://opencode.ai/zen/go/v1",
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides:       map[string]any{"user-agent": "custom-relay-agent/2.0"},
		},
	}
	c := newOpenCodeSessionTestContext(t, "")
	c.Request.Header.Set("User-Agent", "Python-urllib/3.13")

	req, err := svc.buildUpstreamRequest(
		context.Background(), c, account,
		[]byte(`{"model":"glm-5.3","input":"hello"}`), "token", false, "", false,
	)
	require.NoError(t, err)
	require.Equal(t, "custom-relay-agent/2.0", req.Header.Get("User-Agent"))
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
	requireSingleOpenCodeSessionHeader(t, upstream.request.Header, normalizeOpenCodeSessionID("conversation-789"))
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
