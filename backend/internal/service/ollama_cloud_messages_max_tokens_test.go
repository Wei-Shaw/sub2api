//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Anthropic Messages 出站（三个 builder）的 Ollama Cloud DeepSeek max_tokens clamp
// 接线验证。实测背景：POST ollama.com/v1/messages（Bearer）max_tokens=256000 被上游
// 以 "max_tokens (256000) exceeds model's maximum output tokens (65536)" 400 拒绝。
// 用例直接调用各 builder 实际生成 *http.Request，同时检查出站 URL 与 wire body。

func messagesClampTestConfig() *config.Config {
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway = config.GatewayConfig{MaxLineSize: defaultMaxLineSize}
	return cfg
}

func newMessagesClampTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c
}

// messagesClampOllamaAccount 构造挂在实际 ollama.com 上游的 APIKey 账号。
// builder A / B 的 Messages base 取 GetBaseURL()（凭证 base_url），builder C 的
// Anthropic 协议 base 取 GetAnthropicProtocolBaseURL()（anthropic 协议时同为
// 凭证 base_url）。
func messagesClampOllamaAccount(id int64, platform string) *Account {
	return &Account{
		ID:       id,
		Name:     "ollama-cloud-messages",
		Platform: platform,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://ollama.com",
		},
		Extra: map[string]any{},
	}
}

func messagesClampBody(model string, maxTokens int) []byte {
	return []byte(`{"model":"` + model + `","max_tokens":` + strconv.Itoa(maxTokens) +
		`,"stream":false,"messages":[{"role":"user","content":"hi"}]}`)
}

// TestBuildUpstreamRequest_ClampsOllamaCloudDeepSeekMaxTokens 覆盖 GatewayService
// .buildUpstreamRequest（Anthropic 平台原生 Messages + 共享重试路径）。
func TestBuildUpstreamRequest_ClampsOllamaCloudDeepSeekMaxTokens(t *testing.T) {
	svc := &GatewayService{cfg: messagesClampTestConfig()}
	c := newMessagesClampTestContext(t)

	body := messagesClampBody("deepseek-v4-flash", 256000)
	account := messagesClampOllamaAccount(401, PlatformAnthropic)

	req, wireBody, err := svc.buildUpstreamRequest(
		context.Background(), c, account, body, "sk-test", "api_key",
		"deepseek-v4-flash", false, false,
	)
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages?beta=true", req.URL.String())
	require.Equal(t, "deepseek-v4-flash", gjson.GetBytes(wireBody, "model").String())
	require.Equal(t, int64(65535), gjson.GetBytes(wireBody, "max_tokens").Int())

	t.Run("official anthropic base is untouched", func(t *testing.T) {
		official := messagesClampOllamaAccount(402, PlatformAnthropic)
		official.Credentials["base_url"] = "https://api.anthropic.com"
		_, wire, err := svc.buildUpstreamRequest(
			context.Background(), c, official, body, "sk-test", "api_key",
			"deepseek-v4-flash", false, false,
		)
		require.NoError(t, err)
		require.Equal(t, int64(256000), gjson.GetBytes(wire, "max_tokens").Int())
	})

	t.Run("non deepseek model untouched", func(t *testing.T) {
		nonDeepSeek := messagesClampBody("k3-256k", 256000)
		_, wire, err := svc.buildUpstreamRequest(
			context.Background(), c, account, nonDeepSeek, "sk-test", "api_key",
			"k3-256k", false, false,
		)
		require.NoError(t, err)
		require.Equal(t, int64(256000), gjson.GetBytes(wire, "max_tokens").Int())
	})

	t.Run("at cap and extra cap zero untouched", func(t *testing.T) {
		atCap := messagesClampBody("deepseek-v4-flash", 65535)
		_, wire, err := svc.buildUpstreamRequest(
			context.Background(), c, account, atCap, "sk-test", "api_key",
			"deepseek-v4-flash", false, false,
		)
		require.NoError(t, err)
		require.Equal(t, int64(65535), gjson.GetBytes(wire, "max_tokens").Int())

		disabled := messagesClampOllamaAccount(403, PlatformAnthropic)
		disabled.Extra[OllamaCloudMaxTokensCapExtraKey] = 0
		_, wire, err = svc.buildUpstreamRequest(
			context.Background(), c, disabled, body, "sk-test", "api_key",
			"deepseek-v4-flash", false, false,
		)
		require.NoError(t, err)
		require.Equal(t, int64(256000), gjson.GetBytes(wire, "max_tokens").Int())
	})
}

// TestBuildUpstreamRequestAnthropicAPIKeyPassthrough_ClampsOllamaCloudDeepSeekMaxTokens
// 覆盖 Anthropic 平台 APIKey passthrough builder。
func TestBuildUpstreamRequestAnthropicAPIKeyPassthrough_ClampsOllamaCloudDeepSeekMaxTokens(t *testing.T) {
	svc := &GatewayService{cfg: messagesClampTestConfig()}
	c := newMessagesClampTestContext(t)

	body := messagesClampBody("deepseek-v4-flash", 256000)
	account := messagesClampOllamaAccount(411, PlatformAnthropic)

	req, wireBody, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, account, body, "sk-test",
	)
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages?beta=true", req.URL.String())
	require.Equal(t, int64(65535), gjson.GetBytes(wireBody, "max_tokens").Int())

	// 官方 Anthropic（api.anthropic.com）即使 max_tokens 超过 65535 也字节级不变。
	official := newAnthropicAPIKeyAccountForTest()
	_, wire, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
		context.Background(), c, official, body, "upstream-anthropic-key",
	)
	require.NoError(t, err)
	require.Equal(t, int64(256000), gjson.GetBytes(wire, "max_tokens").Int())
}

// TestBuildNativeAnthropicUpstreamRequest_ClampsOllamaCloudDeepSeekMaxTokens 覆盖
// 国产供应商原生 Anthropic 协议 builder（Messages / Responses / CC 桥接共享）。
func TestBuildNativeAnthropicUpstreamRequest_ClampsOllamaCloudDeepSeekMaxTokens(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: messagesClampTestConfig()}
	c := newMessagesClampTestContext(t)

	body := messagesClampBody("deepseek-v4-flash", 256000)

	t.Run("anthropic protocol ollama base is clamped", func(t *testing.T) {
		account := messagesClampOllamaAccount(421, PlatformDeepseek)
		account.Credentials["api_protocol"] = APIProtocolAnthropic

		targetURL, err := svc.nativeAnthropicTargetURL(account)
		require.NoError(t, err)
		require.Equal(t, "https://ollama.com/v1/messages", targetURL)

		req, wireBody, err := svc.buildNativeAnthropicUpstreamRequest(
			context.Background(), c, account, body, "sk-test", targetURL,
		)
		require.NoError(t, err)
		require.Equal(t, "https://ollama.com/v1/messages", req.URL.String())
		require.Equal(t, int64(65535), gjson.GetBytes(wireBody, "max_tokens").Int())
	})

	// adaptive 账号按 Anthropic 协议地址（api_base_urls[anthropic]）判定，而非 CC
	// 地址：anthropic 指向 ollama.com、chat_completions 指向官方 DeepSeek 时命中。
	t.Run("adaptive uses anthropic base not cc base", func(t *testing.T) {
		account := messagesClampOllamaAccount(422, PlatformDeepseek)
		account.Credentials["api_protocol"] = APIProtocolAdaptive
		account.Credentials["api_base_urls"] = map[string]any{
			APIProtocolAnthropic:       "https://ollama.com",
			APIProtocolChatCompletions: "https://api.deepseek.com",
		}

		targetURL, err := svc.nativeAnthropicTargetURL(account)
		require.NoError(t, err)
		req, wireBody, err := svc.buildNativeAnthropicUpstreamRequest(
			context.Background(), c, account, body, "sk-test", targetURL,
		)
		require.NoError(t, err)
		require.Equal(t, "https://ollama.com/v1/messages", req.URL.String())
		require.Equal(t, int64(65535), gjson.GetBytes(wireBody, "max_tokens").Int())
	})

	// 反向：实际 Anthropic 上游非 ollama.com（官方 anthropic 端点），即使 CC 地址
	// 指向 ollama.com 且残留 usage extra，也不 clamp。
	t.Run("non-ollama anthropic base with cc ollama base and usage extra untouched", func(t *testing.T) {
		account := messagesClampOllamaAccount(423, PlatformDeepseek)
		account.Credentials["api_protocol"] = APIProtocolAdaptive
		account.Credentials["api_base_urls"] = map[string]any{
			APIProtocolAnthropic:       "https://api.deepseek.com/anthropic",
			APIProtocolChatCompletions: "https://ollama.com",
		}
		account.Extra[OllamaCloudUsageSnapshotExtraKey] = map[string]any{"status": "ok"}

		targetURL, err := svc.nativeAnthropicTargetURL(account)
		require.NoError(t, err)
		req, wireBody, err := svc.buildNativeAnthropicUpstreamRequest(
			context.Background(), c, account, body, "sk-test", targetURL,
		)
		require.NoError(t, err)
		require.Equal(t, "https://api.deepseek.com/anthropic/v1/messages", req.URL.String())
		require.Equal(t, int64(256000), gjson.GetBytes(wireBody, "max_tokens").Int())
	})

	// 官方 DeepSeek Anthropic 协议（平台默认端点）字节级不变。
	t.Run("official deepseek anthropic default untouched", func(t *testing.T) {
		account := &Account{
			ID:       424,
			Name:     "official-deepseek-anthropic",
			Platform: PlatformDeepseek,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":      "sk-test",
				"api_protocol": APIProtocolAnthropic,
			},
			Extra: map[string]any{},
		}
		targetURL, err := svc.nativeAnthropicTargetURL(account)
		require.NoError(t, err)
		_, wireBody, err := svc.buildNativeAnthropicUpstreamRequest(
			context.Background(), c, account, body, "sk-test", targetURL,
		)
		require.NoError(t, err)
		require.Equal(t, int64(256000), gjson.GetBytes(wireBody, "max_tokens").Int())
	})

	// 非 DeepSeek 模型（映射后出站 model）不变。
	t.Run("non deepseek model untouched", func(t *testing.T) {
		account := messagesClampOllamaAccount(425, PlatformDeepseek)
		account.Credentials["api_protocol"] = APIProtocolAnthropic
		nonDeepSeek := messagesClampBody("glm-4.7", 256000)
		targetURL, err := svc.nativeAnthropicTargetURL(account)
		require.NoError(t, err)
		_, wireBody, err := svc.buildNativeAnthropicUpstreamRequest(
			context.Background(), c, account, nonDeepSeek, "sk-test", targetURL,
		)
		require.NoError(t, err)
		require.Equal(t, int64(256000), gjson.GetBytes(wireBody, "max_tokens").Int())
	})
}

// messagesClampProductionAccount 复刻生产账号 162：platform=deepseek、type=apikey、
// api_protocol=adaptive、base_url 与 api_base_urls 均指向 ollama.com（anthropic 地址
// 带 trailing '/'，CC/Responses 地址带 /v1 后缀）。
func messagesClampProductionAccount(id int64) *Account {
	return &Account{
		ID:       id,
		Name:     "ollama-cloud-prod",
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"api_protocol": APIProtocolAdaptive,
			"base_url":     "https://ollama.com/v1",
			"api_base_urls": map[string]any{
				APIProtocolAnthropic:       "https://ollama.com/",
				APIProtocolResponses:       "https://ollama.com/v1",
				APIProtocolChatCompletions: "https://ollama.com/v1",
			},
		},
		Extra: map[string]any{},
	}
}

// TestBuildNativeAnthropicUpstreamRequest_ClampsTrailingSlashOllamaBase 复刻生产
// 失败：anthropic 协议地址配置为 "https://ollama.com/"（trailing '/'）。真实出站
// URL 组装（ValidateURLFormat 与 nativeAnthropicTargetURL 均做 TrimRight "/"）会
// 正确打到 https://ollama.com/v1/messages，因此 clamp 判定也必须对同一归一化后的
// base 生效，max_tokens=256000 不得原样透传被上游 400。
func TestBuildNativeAnthropicUpstreamRequest_ClampsTrailingSlashOllamaBase(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: messagesClampTestConfig()}
	c := newMessagesClampTestContext(t)
	account := messagesClampProductionAccount(431)
	body := messagesClampBody("deepseek-v4-flash", 256000)

	// 真实出站 URL：trailing '/' 被既有组装归一化掉，请求确实可达 ollama.com。
	targetURL, err := svc.nativeAnthropicTargetURL(account)
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages", targetURL)

	req, wireBody, err := svc.buildNativeAnthropicUpstreamRequest(
		context.Background(), c, account, body, "sk-test", targetURL,
	)
	require.NoError(t, err)
	require.Equal(t, "https://ollama.com/v1/messages", req.URL.String())
	require.Equal(t, "deepseek-v4-flash", gjson.GetBytes(wireBody, "model").String())
	require.Equal(t, int64(65535), gjson.GetBytes(wireBody, "max_tokens").Int(),
		"anthropic base trailing '/' 时仍须命中 clamp（生产 400 回归）")
}

// TestBuildUpstreamRequest_ClampsTrailingSlashOllamaBase 覆盖 builder A / B 的相同
// 场景：GetBaseURL() 原样返回 trailing '/'，而 validateUpstreamBaseURL 归一化后
// 实际出站 URL 可达 ollama.com/v1/messages，clamp 判定须与归一化同源。
func TestBuildUpstreamRequest_ClampsTrailingSlashOllamaBase(t *testing.T) {
	svc := &GatewayService{cfg: messagesClampTestConfig()}
	c := newMessagesClampTestContext(t)
	body := messagesClampBody("deepseek-v4-flash", 256000)

	newAccount := func(id int64) *Account {
		account := messagesClampOllamaAccount(id, PlatformAnthropic)
		account.Credentials["base_url"] = "https://ollama.com/"
		return account
	}

	t.Run("builder A trailing slash base is clamped", func(t *testing.T) {
		req, wireBody, err := svc.buildUpstreamRequest(
			context.Background(), c, newAccount(441), body, "sk-test", "api_key",
			"deepseek-v4-flash", false, false,
		)
		require.NoError(t, err)
		require.Equal(t, "https://ollama.com/v1/messages?beta=true", req.URL.String())
		require.Equal(t, int64(65535), gjson.GetBytes(wireBody, "max_tokens").Int())
	})

	t.Run("builder B trailing slash base is clamped", func(t *testing.T) {
		req, wireBody, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(
			context.Background(), c, newAccount(442), body, "sk-test",
		)
		require.NoError(t, err)
		require.Equal(t, "https://ollama.com/v1/messages?beta=true", req.URL.String())
		require.Equal(t, int64(65535), gjson.GetBytes(wireBody, "max_tokens").Int())
	})

	t.Run("evil suffix and custom path never match", func(t *testing.T) {
		for _, base := range []string{"https://ollama.com.evil.com/", "https://ollama.com/anthropic/"} {
			account := newAccount(443)
			account.Credentials["base_url"] = base
			_, wireBody, err := svc.buildUpstreamRequest(
				context.Background(), c, account, body, "sk-test", "api_key",
				"deepseek-v4-flash", false, false,
			)
			require.NoError(t, err)
			require.Equal(t, int64(256000), gjson.GetBytes(wireBody, "max_tokens").Int(),
				"base %q 不得被识别为 Ollama Cloud", base)
		}
	})
}
