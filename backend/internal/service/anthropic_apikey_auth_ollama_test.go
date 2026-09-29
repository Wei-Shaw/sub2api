package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newOllamaCloudAnthropicAuthAccount 构造挂在 Anthropic 平台分组下的 Ollama Cloud
// API Key 账号（base_url 指向官方 ollama.com Anthropic 兼容端点）。
func newOllamaCloudAnthropicAuthAccount(baseURL string, extra map[string]any) *Account {
	if extra == nil {
		extra = map[string]any{}
	}
	return &Account{
		ID:          901,
		Name:        "ollama-cloud-anthropic-auth",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "ollama-cloud-key",
			"base_url": baseURL,
		},
		Extra:       extra,
		Status:      StatusActive,
		Schedulable: true,
	}
}

// TestSetAnthropicAPIKeyAuthHeader_OllamaCloudForcesBearer：Ollama Cloud 官方
// Anthropic 兼容端点只认 Authorization: Bearer——extra 缺失或显式 x_api_key 均
// 强制 Bearer；判定与实际 base_url 同源并做与出站组装一致的尾斜杠归一化；
// 其它上游保持历史 extra/default 行为，恶意相似 host 不误判。
func TestSetAnthropicAPIKeyAuthHeader_OllamaCloudForcesBearer(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		extra       map[string]any
		accountType string
		wantAuth    string // "" = 不应出现 Authorization 头
		wantXAPIKey string // "" = 不应出现 x-api-key 头
	}{
		{
			name:     "ollama 缺 extra 默认 Bearer",
			baseURL:  "https://ollama.com",
			wantAuth: "Bearer ollama-cloud-key",
		},
		{
			name:     "ollama base 尾斜杠仍 Bearer",
			baseURL:  "https://ollama.com/",
			wantAuth: "Bearer ollama-cloud-key",
		},
		{
			name:     "ollama /v1 路径 Bearer",
			baseURL:  "https://ollama.com/v1",
			wantAuth: "Bearer ollama-cloud-key",
		},
		{
			name:     "ollama /v1 尾斜杠 Bearer",
			baseURL:  "https://www.ollama.com/v1/",
			wantAuth: "Bearer ollama-cloud-key",
		},
		{
			name:     "host 大小写不敏感 Bearer",
			baseURL:  "https://OLLAMA.COM",
			wantAuth: "Bearer ollama-cloud-key",
		},
		{
			name:     "ollama 显式 x_api_key 也强制 Bearer",
			baseURL:  "https://ollama.com",
			extra:    map[string]any{anthropicAPIKeyAuthSchemeExtraKey: AnthropicAPIKeyAuthSchemeXAPIKey},
			wantAuth: "Bearer ollama-cloud-key",
		},
		{
			name:        "非 Ollama 默认 x-api-key",
			baseURL:     "https://api.anthropic.com",
			wantXAPIKey: "ollama-cloud-key",
		},
		{
			name:        "默认官方端点（空 base_url）x-api-key",
			baseURL:     "",
			wantXAPIKey: "ollama-cloud-key",
		},
		{
			name:     "非 Ollama 显式 authorization_bearer 保持 Bearer",
			baseURL:  "https://api.anthropic.com",
			extra:    map[string]any{anthropicAPIKeyAuthSchemeExtraKey: AnthropicAPIKeyAuthSchemeAuthorizationBearer},
			wantAuth: "Bearer ollama-cloud-key",
		},
		{
			name:        "恶意相似 host 子串不命中",
			baseURL:     "https://evil.com/ollama.com",
			wantXAPIKey: "ollama-cloud-key",
		},
		{
			name:        "恶意相似 host 后缀不命中",
			baseURL:     "https://ollama.com.evil.com",
			wantXAPIKey: "ollama-cloud-key",
		},
		{
			name:        "http 明文不命中",
			baseURL:     "http://ollama.com",
			wantXAPIKey: "ollama-cloud-key",
		},
		{
			name:        "非标准端口不命中",
			baseURL:     "https://ollama.com:8443",
			wantXAPIKey: "ollama-cloud-key",
		},
		{
			name:        "带 path 的 messages 地址不命中（判定用 base 而非完整 URL）",
			baseURL:     "https://ollama.com/v1/messages",
			wantXAPIKey: "ollama-cloud-key",
		},
		{
			name:        "非 APIKey 账号类型不强制",
			baseURL:     "https://ollama.com",
			accountType: AccountTypeOAuth,
			wantXAPIKey: "ollama-cloud-key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accountType := AccountTypeAPIKey
			if tt.accountType != "" {
				accountType = tt.accountType
			}
			account := newOllamaCloudAnthropicAuthAccount(tt.baseURL, tt.extra)
			account.Type = accountType

			header := http.Header{}
			setAnthropicAPIKeyAuthHeader(header, account, "ollama-cloud-key", tt.baseURL)
			require.Equal(t, tt.wantAuth, header.Get("Authorization"), "Authorization 头不符")
			require.Equal(t, tt.wantXAPIKey, header.Get("x-api-key"), "x-api-key 头不符")
		})
	}
}

// TestSetAnthropicAPIKeyAuthHeader_CNAdaptiveBaseURLResolution：adaptive 账号经
// GetCNProtocolBaseURL(anthropic) / GetAnthropicProtocolBaseURL 选出的分协议地址
// 才是判定依据，Chat Completions 地址（GetOpenAIBaseURL 选到的 CC base）不参与。
func TestSetAnthropicAPIKeyAuthHeader_CNAdaptiveBaseURL(t *testing.T) {
	// adaptive：anthropic 分协议地址指向 ollama.com → Bearer；CC 地址同挂 ollama
	// 不影响（判定只用 anthropic 分协议地址）。
	adaptiveOllamaAnthropic := &Account{
		ID:       902,
		Name:     "adaptive-ollama-anthropic",
		Platform: PlatformKimi,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "kimi-key",
			"api_protocol": APIProtocolAdaptive,
			"api_base_urls": map[string]any{
				APIProtocolAnthropic:       "https://ollama.com",
				APIProtocolChatCompletions: "https://api.moonshot.cn",
			},
		},
	}
	anthropicBase := adaptiveOllamaAnthropic.GetCNProtocolBaseURL(APIProtocolAnthropic)
	require.Equal(t, "https://ollama.com", anthropicBase)
	header := http.Header{}
	setAnthropicAPIKeyAuthHeader(header, adaptiveOllamaAnthropic, "kimi-key", anthropicBase)
	require.Equal(t, "Bearer kimi-key", header.Get("Authorization"))
	require.Empty(t, header.Get("x-api-key"))

	// 反向：CC 地址挂 ollama.com，anthropic 分协议地址非 ollama → 不误判，
	// 保持 x-api-key。
	adaptiveOllamaCC := &Account{
		ID:       903,
		Name:     "adaptive-ollama-cc",
		Platform: PlatformKimi,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "kimi-key",
			"api_protocol": APIProtocolAdaptive,
			"api_base_urls": map[string]any{
				APIProtocolAnthropic:       "https://api.moonshot.cn/anthropic",
				APIProtocolChatCompletions: "https://ollama.com/v1",
			},
		},
	}
	anthropicBase = adaptiveOllamaCC.GetCNProtocolBaseURL(APIProtocolAnthropic)
	require.Equal(t, "https://api.moonshot.cn/anthropic", anthropicBase)
	header = http.Header{}
	setAnthropicAPIKeyAuthHeader(header, adaptiveOllamaCC, "kimi-key", anthropicBase)
	require.Empty(t, header.Get("Authorization"))
	require.Equal(t, "kimi-key", header.Get("x-api-key"))

	// anthropic 协议（非 adaptive）：凭证 base_url 指向 ollama.com → Bearer。
	anthropicProtocolOllama := &Account{
		ID:       904,
		Name:     "anthropic-protocol-ollama",
		Platform: PlatformKimi,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "kimi-key",
			"api_protocol": APIProtocolAnthropic,
			"base_url":     "https://ollama.com",
		},
	}
	anthropicBase = anthropicProtocolOllama.GetAnthropicProtocolBaseURL()
	require.Equal(t, "https://ollama.com", anthropicBase)
	header = http.Header{}
	setAnthropicAPIKeyAuthHeader(header, anthropicProtocolOllama, "kimi-key", anthropicBase)
	require.Equal(t, "Bearer kimi-key", header.Get("Authorization"))
	require.Empty(t, header.Get("x-api-key"))

	// 同协议但 base 为 kimi 官方 → 保持 x-api-key。
	anthropicProtocolKimi := &Account{
		ID:       905,
		Name:     "anthropic-protocol-kimi",
		Platform: PlatformKimi,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "kimi-key",
			"api_protocol": APIProtocolAnthropic,
			"base_url":     "https://api.moonshot.cn/anthropic",
		},
	}
	header = http.Header{}
	setAnthropicAPIKeyAuthHeader(header, anthropicProtocolKimi, "kimi-key", anthropicProtocolKimi.GetAnthropicProtocolBaseURL())
	require.Empty(t, header.Get("Authorization"))
	require.Equal(t, "kimi-key", header.Get("x-api-key"))
}

// TestGatewayService_AnthropicPassthrough_OllamaCloudBearer：经 passthrough
// builder 真实构造 http.Request 验证最终 header——Ollama Cloud 强制 Bearer 且
// 不泄漏客户端入站认证；非 Ollama 上游保持 x-api-key。
func TestGatewayService_AnthropicPassthrough_OllamaCloudBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	buildReq := func(t *testing.T, baseURL string) *http.Request {
		t.Helper()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		// 入站认证残留：正确上游只发一种认证头，不得泄漏客户端凭证
		c.Request.Header.Set("Authorization", "Bearer inbound-token")
		c.Request.Header.Set("X-Api-Key", "inbound-api-key")

		svc := &GatewayService{cfg: &config.Config{}}
		account := newOllamaCloudAnthropicAuthAccount(baseURL, nil)
		body := []byte(`{"model":"claude-3-7-sonnet-20250219","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
		req, _, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, body, "ollama-cloud-key")
		require.NoError(t, err)
		return req
	}

	ollamaReq := buildReq(t, "https://ollama.com/")
	require.Equal(t, "Bearer ollama-cloud-key", ollamaReq.Header.Get("Authorization"))
	require.Empty(t, ollamaReq.Header.Get("x-api-key"), "Ollama Cloud 上游不得再发 x-api-key")
	require.NotContains(t, ollamaReq.Header.Get("Authorization"), "inbound-token")
	require.Empty(t, ollamaReq.Header.Get("x-inbound"), "sanity")

	anthropicReq := buildReq(t, "https://api.anthropic.com")
	require.Empty(t, anthropicReq.Header.Get("Authorization"))
	require.Equal(t, "ollama-cloud-key", anthropicReq.Header.Get("x-api-key"))
	require.NotContains(t, anthropicReq.Header.Get("x-api-key"), "inbound-api-key")
}

// TestGatewayService_BuildUpstreamRequest_OllamaCloudBearer：经 Anthropic 原生
// GetBaseURL builder 真实构造 http.Request 验证最终 header。
func TestGatewayService_BuildUpstreamRequest_OllamaCloudBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	buildReq := func(t *testing.T, baseURL string) *http.Request {
		t.Helper()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		c.Request.Header.Set("Authorization", "Bearer inbound-token")
		c.Request.Header.Set("X-Api-Key", "inbound-api-key")

		svc := &GatewayService{cfg: &config.Config{}}
		account := newOllamaCloudAnthropicAuthAccount(baseURL, nil)
		body := []byte(`{"model":"claude-3-7-sonnet-20250219","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
		req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "ollama-cloud-key", "apikey", "claude-3-7-sonnet-20250219", false, false)
		require.NoError(t, err)
		return req
	}

	ollamaReq := buildReq(t, "https://ollama.com")
	require.Equal(t, "https://ollama.com/v1/messages?beta=true", ollamaReq.URL.String())
	require.Equal(t, "Bearer ollama-cloud-key", ollamaReq.Header.Get("Authorization"))
	require.Empty(t, ollamaReq.Header.Get("x-api-key"))

	anthropicReq := buildReq(t, "https://api.anthropic.com")
	require.Empty(t, anthropicReq.Header.Get("Authorization"))
	require.Equal(t, "ollama-cloud-key", anthropicReq.Header.Get("x-api-key"))
}

// TestOpenAIGatewayService_NativeAnthropicBridge_OllamaCloudBearer：经 native
// Anthropic 桥接 builder（CC/Responses → /v1/messages）真实构造 http.Request
// 验证最终 header。
func TestOpenAIGatewayService_NativeAnthropicBridge_OllamaCloudBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{
		ID:       906,
		Name:     "anthropic-protocol-ollama-bridge",
		Platform: PlatformKimi,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "ollama-cloud-key",
			"api_protocol": APIProtocolAnthropic,
			"base_url":     "https://ollama.com",
		},
	}
	targetURL, err := svc.nativeAnthropicTargetURL(account)
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages", targetURL)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"model":"kimi-k2-thinking","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
	req, _, err := svc.buildNativeAnthropicUpstreamRequest(context.Background(), c, account, body, "ollama-cloud-key", targetURL)
	require.NoError(t, err)
	require.Equal(t, "Bearer ollama-cloud-key", req.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("x-api-key"))

	// 非 Ollama 上游：保持 x-api-key，无 Authorization。
	account.Credentials["base_url"] = "https://api.moonshot.cn/anthropic"
	targetURL, err = svc.nativeAnthropicTargetURL(account)
	require.NoError(t, err)
	req, _, err = svc.buildNativeAnthropicUpstreamRequest(context.Background(), c, account, body, "ollama-cloud-key", targetURL)
	require.NoError(t, err)
	require.Empty(t, req.Header.Get("Authorization"))
	require.Equal(t, "ollama-cloud-key", req.Header.Get("x-api-key"))
}
