package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ollamaCloudRoutingTestService 返回仅配置宽松 URL 白名单的 service，用于只验证
// 出站 URL 组装，不发起真实请求。
func ollamaCloudRoutingTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled:           false,
			AllowInsecureHTTP: true,
		}}},
	}
}

// TestOllamaCloudDefaultProtocolAndBaseURLs 覆盖 A9/A11/A13 + C30/Q2-3 + C33/Q2-4：
// ollama_cloud 属多协议网关、api_protocol 缺省为 adaptive，且 anthropic 默认 base
// 不带 /v1 而 cc/responses 带 /v1；任何情况下都不得回落到 api.openai.com。
func TestOllamaCloudDefaultProtocolAndBaseURLs(t *testing.T) {
	account := &Account{
		ID:          900,
		Name:        "ollama-cloud-default",
		Platform:    PlatformOllamaCloud,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{},
	}

	require.True(t, account.IsOllamaCloud())
	require.True(t, IsOllamaCloud(PlatformOllamaCloud))
	require.True(t, IsMultiProtocolAPIKeyProvider(PlatformOllamaCloud))
	require.True(t, account.IsMultiProtocolAPIKey())

	// A11：api_protocol 缺失时 ollama_cloud 缺省 adaptive（不是 CC）。
	require.Equal(t, APIProtocolAdaptive, account.GetAPIProtocol())
	require.True(t, account.IsAdaptiveAPIProtocol())

	// C30/Q2-3：anthropic 默认 base 不带 /v1；cc/responses 带 /v1。
	require.Equal(t, "https://ollama.com", account.defaultCNProtocolBaseURL(APIProtocolAnthropic))
	require.Equal(t, "https://ollama.com/v1", account.defaultCNProtocolBaseURL(APIProtocolChatCompletions))
	require.Equal(t, "https://ollama.com/v1", account.defaultCNProtocolBaseURL(APIProtocolResponses))

	// 同一结论经 adaptive 解析链（api_base_urls / base_url 均缺失）。
	require.Equal(t, "https://ollama.com", account.GetCNProtocolBaseURL(APIProtocolAnthropic))
	require.Equal(t, "https://ollama.com/v1", account.GetCNProtocolBaseURL(APIProtocolChatCompletions))
	require.Equal(t, "https://ollama.com/v1", account.GetCNProtocolBaseURL(APIProtocolResponses))
	require.Equal(t, "https://ollama.com", account.GetAnthropicProtocolBaseURL())

	// C33/Q2-4 + S6 层 1：OpenAI 协议 base 守卫必须接受 ollama_cloud，
	// 且缺省端点绝不是官方域名。
	require.Equal(t, DefaultOllamaCloudBaseURL, account.GetOpenAIBaseURL())
	require.NotEqual(t, "https://api.openai.com", account.GetOpenAIBaseURL())

	// adaptive 下 anthropic 段没有 credentials.base_url 回退，CC 段有。
	withBase := &Account{
		ID:       901,
		Name:     "ollama-cloud-base-url",
		Platform: PlatformOllamaCloud,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://proxy.example/v1",
		},
		Extra: map[string]any{},
	}
	require.Equal(t, "https://proxy.example/v1", withBase.GetOpenAIBaseURL())
	require.Equal(t, "https://proxy.example/v1", withBase.GetCNProtocolBaseURL(APIProtocolChatCompletions))
	require.Equal(t, "https://ollama.com", withBase.GetCNProtocolBaseURL(APIProtocolAnthropic))
	require.Equal(t, "https://ollama.com/v1", withBase.GetCNProtocolBaseURL(APIProtocolResponses))
}

// TestNativeAnthropicTargetURLOllamaCloudVersionAware 覆盖 S5：nativeAnthropicTargetURL
// 改用版本感知拼接后，base 带 /v1 与不带 /v1 都得到同一个 {base}/v1/messages。
func TestNativeAnthropicTargetURLOllamaCloudVersionAware(t *testing.T) {
	svc := ollamaCloudRoutingTestService()

	cases := []struct {
		name          string
		apiProtocol   string
		baseURL       string
		anthropicBase string
		want          string
	}{
		{
			name:        "explicit anthropic base without version suffix",
			apiProtocol: APIProtocolAnthropic,
			baseURL:     "https://ollama.com",
			want:        "https://ollama.com/v1/messages",
		},
		{
			name:        "explicit anthropic base with version suffix",
			apiProtocol: APIProtocolAnthropic,
			baseURL:     "https://ollama.com/v1",
			want:        "https://ollama.com/v1/messages",
		},
		{
			name:        "explicit anthropic base trailing slash",
			apiProtocol: APIProtocolAnthropic,
			baseURL:     "https://ollama.com/",
			want:        "https://ollama.com/v1/messages",
		},
		{
			name:          "adaptive api_base_urls anthropic with version suffix",
			apiProtocol:   APIProtocolAdaptive,
			anthropicBase: "https://ollama.com/v1",
			want:          "https://ollama.com/v1/messages",
		},
		{
			name:          "adaptive api_base_urls anthropic without version suffix",
			apiProtocol:   APIProtocolAdaptive,
			anthropicBase: "https://ollama.com",
			want:          "https://ollama.com/v1/messages",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			credentials := map[string]any{
				"api_key":      "sk-test",
				"api_protocol": tt.apiProtocol,
			}
			if tt.baseURL != "" {
				credentials["base_url"] = tt.baseURL
			}
			if tt.anthropicBase != "" {
				credentials["api_base_urls"] = map[string]any{APIProtocolAnthropic: tt.anthropicBase}
			}
			account := &Account{
				ID:          902,
				Name:        "ollama-cloud-anthropic",
				Platform:    PlatformOllamaCloud,
				Type:        AccountTypeAPIKey,
				Credentials: credentials,
				Extra:       map[string]any{},
			}

			targetURL, err := svc.nativeAnthropicTargetURL(account)
			require.NoError(t, err)
			require.Equal(t, tt.want, targetURL)
		})
	}

	// 双向钉住 buildOpenAIEndpointURL 在 messages 场景的边界行为。
	require.Equal(t, "https://ollama.com/v1/messages", buildOpenAIEndpointURL("https://ollama.com", "/v1/messages"))
	require.Equal(t, "https://ollama.com/v1/messages", buildOpenAIEndpointURL("https://ollama.com/v1", "/v1/messages"))
	require.Equal(t, "https://opencode.ai/zen/go/v1/messages", buildOpenAIEndpointURL("https://opencode.ai/zen/go", "/v1/messages"))
	require.Equal(t, "https://opencode.ai/zen/go/v1/messages", buildOpenAIEndpointURL("https://opencode.ai/zen/go/v1", "/v1/messages"))
}

// TestShouldForwardOpenAIResponsesViaRawChatCompletions_OllamaCloud 覆盖 C32：
// ollama_cloud 不能被 opencode 的「恒 false」复用，显式 CC 必须走 raw CC。
func TestShouldForwardOpenAIResponsesViaRawChatCompletions_OllamaCloud(t *testing.T) {
	account := func(protocol string) *Account {
		return &Account{
			ID:       903,
			Name:     "ollama-cloud-cc",
			Platform: PlatformOllamaCloud,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":      "sk-test",
				"api_protocol": protocol,
			},
			Extra: map[string]any{},
		}
	}

	require.True(t, shouldForwardOpenAIResponsesViaRawChatCompletions(account(APIProtocolChatCompletions)))
	require.False(t, shouldForwardOpenAIResponsesViaRawChatCompletions(account(APIProtocolAnthropic)))
	// C31 落地后 SupportsNativeCNResponses() 对 ollama 为 true：adaptive / responses
	// 不再降级 raw CC，改走平台原生 /v1/responses（连锁验证见
	// TestAdaptiveProtocolRoutesOllamaCloudResponsesToNativeResponses）。
	require.False(t, shouldForwardOpenAIResponsesViaRawChatCompletions(account(APIProtocolAdaptive)))
	require.False(t, shouldForwardOpenAIResponsesViaRawChatCompletions(account(APIProtocolResponses)))
}

// TestForwardCountTokensAsAnthropic_OllamaCloudEstimatesLocally 覆盖 ⑨ findings B1：
// ollama_cloud 账号命中本地估算分支，返回 200 且不产生任何出站请求。
func TestForwardCountTokensAsAnthropic_OllamaCloudEstimatesLocally(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"claude-sonnet-4-5","system":"You are helpful.","messages":[{"role":"user","content":"hello"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{err: errors.New("local estimate must not hit upstream")}
	svc := ollamaCloudRoutingTestService()
	svc.httpUpstream = upstream

	account := &Account{
		ID:          904,
		Name:        "ollama-cloud-count-tokens",
		Platform:    PlatformOllamaCloud,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{},
	}

	err := svc.ForwardCountTokensAsAnthropic(context.Background(), c, account, body, "claude-sonnet-4-5")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Greater(t, gjson.Get(rec.Body.String(), "input_tokens").Int(), int64(0))
	require.Nil(t, upstream.lastReq)
	require.Empty(t, upstream.requests)
}
