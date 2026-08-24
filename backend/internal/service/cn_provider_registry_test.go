package service

// CN 供应商注册表的派生语义回归测试：
// 平台集合判定、配额/调度阈值列表、端点矩阵查找、能力位、模型识别
// 均由注册表派生，须与历史硬编码行为完全一致。

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCNProviderRegistry_BuiltinProvidersRegistered(t *testing.T) {
	require.ElementsMatch(t, []string{PlatformKimi, PlatformZhipu, PlatformDeepseek}, CNProviderCodes())

	for _, code := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek} {
		spec, ok := GetCNProviderSpec(code)
		require.True(t, ok, "builtin provider %q must be registered", code)
		require.Equal(t, code, spec.Code)
		require.NotEmpty(t, spec.DisplayName)
	}
}

func TestIsCNProvider_DerivedFromRegistry(t *testing.T) {
	for _, p := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek} {
		require.True(t, IsCNProvider(p), p)
	}
	for _, p := range []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok, PlatformComposite, "kiro", ""} {
		require.False(t, IsCNProvider(p), p)
	}
}

func TestAllowedQuotaPlatforms_DerivedOrder(t *testing.T) {
	// 历史顺序：anthropic/openai/gemini/antigravity/grok + CN 平台（注册序）。
	require.Equal(t, []string{
		PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok,
		PlatformKimi, PlatformZhipu, PlatformDeepseek,
	}, AllowedQuotaPlatforms)
}

func TestAllowedSchedulingThresholdPlatforms_OnlyQuotaProbeProviders(t *testing.T) {
	// openai/anthropic/grok 原生窗口 + 有 QuotaProbe 的 CN 平台（kimi/zhipu）；
	// deepseek 为余额型（无 QuotaProbe），不纳入阈值评估。
	require.Equal(t, []string{
		PlatformOpenAI, PlatformAnthropic, PlatformGrok, PlatformKimi, PlatformZhipu,
	}, AllowedSchedulingThresholdPlatforms)
}

func TestConcretePlatforms_DerivedOrder(t *testing.T) {
	require.Equal(t, []string{
		PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok,
		PlatformKimi, PlatformZhipu, PlatformDeepseek,
	}, ConcretePlatforms())
}

func TestCNProviderSpec_DefaultBaseURL_FallbackChain(t *testing.T) {
	kimi, _ := GetCNProviderSpec(PlatformKimi)
	// 精确模式键命中。
	require.Equal(t, DefaultKimiCodingBaseURL, kimi.DefaultBaseURL(APIProtocolChatCompletions, AccountModeCoding))
	require.Equal(t, DefaultKimiPayGBaseURL, kimi.DefaultBaseURL(APIProtocolChatCompletions, AccountModePayG))
	require.Equal(t, DefaultKimiCodingAnthropicBaseURL, kimi.DefaultBaseURL(APIProtocolAnthropic, AccountModeCoding))
	// 未设置 mode（空串）回退 payg 键。
	require.Equal(t, DefaultKimiPayGBaseURL, kimi.DefaultBaseURL(APIProtocolChatCompletions, ""))

	zhipu, _ := GetCNProviderSpec(PlatformZhipu)
	// anthropic 端点模式无关：coding/payg 均回退到空 Mode 键。
	require.Equal(t, DefaultZhipuAnthropicBaseURL, zhipu.DefaultBaseURL(APIProtocolAnthropic, AccountModeCoding))
	require.Equal(t, DefaultZhipuAnthropicBaseURL, zhipu.DefaultBaseURL(APIProtocolAnthropic, AccountModePayG))
	require.Equal(t, DefaultZhipuCodingBaseURL, zhipu.DefaultBaseURL(APIProtocolChatCompletions, AccountModeCoding))

	deepseek, _ := GetCNProviderSpec(PlatformDeepseek)
	require.Equal(t, DefaultDeepseekBaseURL, deepseek.DefaultBaseURL(APIProtocolChatCompletions, AccountModePayG))
	require.Equal(t, DefaultDeepseekAnthropicBaseURL, deepseek.DefaultBaseURL(APIProtocolAnthropic, ""))

	// 未注册平台 / 未知协议返回空串。
	var nilSpec *CNProviderSpec
	require.Equal(t, "", nilSpec.DefaultBaseURL(APIProtocolChatCompletions, AccountModePayG))
	require.Equal(t, "", kimi.DefaultBaseURL("unknown_protocol", AccountModePayG))
}

func TestCNProviderSpec_ProbeCapabilities(t *testing.T) {
	kimi, _ := GetCNProviderSpec(PlatformKimi)
	require.NotNil(t, kimi.BalanceProbe)
	require.NotNil(t, kimi.QuotaProbe)
	require.False(t, kimi.SupportsNativeResponses)

	zhipu, _ := GetCNProviderSpec(PlatformZhipu)
	require.Nil(t, zhipu.BalanceProbe) // 无公开余额端点
	require.NotNil(t, zhipu.QuotaProbe)

	deepseek, _ := GetCNProviderSpec(PlatformDeepseek)
	require.NotNil(t, deepseek.BalanceProbe)
	require.Nil(t, deepseek.QuotaProbe) // 无 Coding Plan 窗口端点
	require.True(t, deepseek.SupportsNativeResponses)

	require.True(t, cnSupportsNativeResponses(PlatformDeepseek))
	require.False(t, cnSupportsNativeResponses(PlatformKimi))
	require.False(t, cnSupportsNativeResponses(PlatformOpenAI)) // 非 CN 平台
}

func TestAccountDefaultBaseURL_ViaRegistry(t *testing.T) {
	// GetOpenAIBaseURL / GetAnthropicProtocolBaseURL 的默认端点与历史 switch 等价。
	codingKimi := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"account_mode": AccountModeCoding, "api_protocol": APIProtocolAnthropic,
	}}
	require.Equal(t, DefaultKimiCodingAnthropicBaseURL, codingKimi.GetAnthropicProtocolBaseURL())

	paygZhipu := &Account{Platform: PlatformZhipu, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"account_mode": AccountModePayG, "api_protocol": APIProtocolAnthropic,
	}}
	require.Equal(t, DefaultZhipuAnthropicBaseURL, paygZhipu.GetAnthropicProtocolBaseURL())

	// 凭证 base_url 优先于平台默认。
	custom := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": "https://proxy.example.com/anthropic", "api_protocol": APIProtocolAnthropic,
	}}
	require.Equal(t, "https://proxy.example.com/anthropic", custom.GetAnthropicProtocolBaseURL())
}

func TestGetCodingPlanProvider_ViaRegistry(t *testing.T) {
	mk := func(baseURL string) *Account {
		return &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
			"account_mode": AccountModeCoding, "base_url": baseURL,
		}}
	}
	require.Equal(t, PlatformKimi, mk("https://api.kimi.com/coding/v1").GetCodingPlanProvider())
	require.Equal(t, PlatformZhipu, mk("https://open.bigmodel.cn/api/coding/paas/v4").GetCodingPlanProvider())
	require.Equal(t, PlatformZhipu, mk("https://api.z.ai/api/coding/paas/v4").GetCodingPlanProvider())
	require.Equal(t, "", mk("https://api.deepseek.com").GetCodingPlanProvider())

	// 非 coding 模式不反推。
	payg := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"account_mode": AccountModePayG, "base_url": "https://api.kimi.com/coding/v1",
	}}
	require.Equal(t, "", payg.GetCodingPlanProvider())
}

func TestDetectModelPlatform_ViaRegistry(t *testing.T) {
	cases := map[string]string{
		"kimi-k2":                  PlatformKimi,
		"moonshot-v1-8k":           PlatformKimi,
		"kimi/kimi-k2":             PlatformKimi,
		"moonshot/moonshot-v1-8k":  PlatformKimi,
		"glm-4.6":                  PlatformZhipu,
		"zhipu/glm-4.6":            PlatformZhipu,
		"bigmodel/glm-4.6":         PlatformZhipu,
		"deepseek-chat":            PlatformDeepseek,
		"deepseek/deepseek-chat":   PlatformDeepseek,
		"claude-sonnet-4":          PlatformAnthropic,
		"gpt-5":                    PlatformOpenAI,
		"grok-4":                   PlatformGrok,
		"gemini-2.5-pro":           PlatformGemini,
		"totally-unknown-model-42": "",
	}
	for model, want := range cases {
		got, ok := DetectModelPlatform(model)
		require.Equal(t, want, got, "model %q", model)
		require.Equal(t, want != "", ok, "model %q matched flag", model)
	}
}

func TestRegisterCNProvider_DuplicatePanics(t *testing.T) {
	require.Panics(t, func() {
		RegisterCNProvider(CNProviderSpec{Code: PlatformKimi})
	})
	require.Panics(t, func() {
		RegisterCNProvider(CNProviderSpec{Code: ""})
	})
}
