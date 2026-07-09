//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func newOpenAIOAuthAccountForModelTest() *Account {
	return &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}
}

func TestIsModelSupported_OpenAIOAuthEmptyMapping_ServableModels(t *testing.T) {
	account := newOpenAIOAuthAccountForModelTest()

	servable := []string{
		"", // 空模型交由上层必填校验
		"gpt-5.4",
		"gpt-5.4-high", // 推理后缀变体
		"gpt-5.3-codex",
		"gpt-5.1-codex-mini",
		"gpt-5",
		"codex-mini-latest",
		"gpt5.3codexspark",  // 别名拼写
		"gpt-image-1",       // 图像生成模型
		"claude-sonnet-4-6", // /v1/messages 调度默认映射兜底
		"claude-3-opus-20240229",
		"gpt-4o",          // 保守 fail-open：非黑名单模型保持允许
		"my-custom-alias", // 自定义别名可能由渠道级映射在转发前改写，保持允许
	}
	for _, model := range servable {
		require.True(t, account.IsModelSupported(model), "expected %q to be servable by empty-mapping OpenAI OAuth account", model)
	}
}

func TestIsModelSupported_OpenAIOAuthEmptyMapping_RejectsForeignModels(t *testing.T) {
	account := newOpenAIOAuthAccountForModelTest()

	// Codex 上游必然以不可重试的 400 拒绝这些厂商家族；调度阶段就应跳过
	// 该账号，让显式声明支持的 API Key 账号接手（#3662）。
	foreign := []string{
		"deepseek-v4",
		"deepseek-chat",
		"glm-4.7",
		"kimi-k2",
		"moonshot-v1-128k",
		"gemini-3.0-pro",
		"grok-4",
		"qwen3-max",
		"minimax-m2.5",
		"llama-3.3-70b",
		"provider/deepseek-v4", // vendor/model 形式取最后一段判定
	}
	for _, model := range foreign {
		require.False(t, account.IsModelSupported(model), "expected %q to be rejected by empty-mapping OpenAI OAuth account", model)
	}
}

func TestIsModelSupported_OpenAIOAuthExplicitMappingUnchanged(t *testing.T) {
	account := newOpenAIOAuthAccountForModelTest()
	account.Credentials = map[string]any{
		"model_mapping": map[string]any{"deepseek-v4": "gpt-5.4"},
	}

	// 显式映射沿用原有语义：命中映射即支持，未命中即不支持。
	require.True(t, account.IsModelSupported("deepseek-v4"))
	require.False(t, account.IsModelSupported("glm-4.7"))
}

func TestIsModelSupported_OpenAIOAuthPassthroughAllowsAll(t *testing.T) {
	account := newOpenAIOAuthAccountForModelTest()
	account.Extra = map[string]any{"openai_passthrough": true}

	// 透传模式仅替换认证，模型语义由上游决定，保持"允许所有"。
	require.True(t, account.IsModelSupported("deepseek-v4"))
}

func TestIsModelSupported_OpenAIAPIKeyEmptyMappingAllowsAll(t *testing.T) {
	account := &Account{
		ID:       2,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	}

	// API Key 账号（第三方 OpenAI 兼容上游）可服务任意别名，语义不变。
	require.True(t, account.IsModelSupported("deepseek-v4"))
	require.True(t, account.IsModelSupported("gpt-5.4"))
}

func TestIsModelSupported_NonOpenAIPlatformsUnchanged(t *testing.T) {
	anthropic := &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	require.True(t, anthropic.IsModelSupported("claude-sonnet-4-6"))
	require.True(t, anthropic.IsModelSupported("deepseek-v4"))
}

func TestIsOpenAIOAuthServableModel(t *testing.T) {
	require.True(t, isOpenAIOAuthServableModel("gpt-5.4-high"))
	require.True(t, isOpenAIOAuthServableModel("  gpt-5.3-codex  "))
	require.True(t, isOpenAIOAuthServableModel("claude-3-5-haiku-20241022"))
	require.True(t, isOpenAIOAuthServableModel("DeepThink-x"))  // 非黑名单前缀，保持允许
	require.False(t, isOpenAIOAuthServableModel("DeepSeek-V4")) // 大小写不敏感
	require.False(t, isOpenAIOAuthServableModel("qwen3-235b-thinking"))
	require.True(t, isOpenAIOAuthServableModel("deepseekcoder")) // 无连字符 → 非黑名单前缀，保持允许
}
