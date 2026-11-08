//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// 以下为重构前各处手写的平台列表 / switch，作为平台清单派生结果的等价基准；
// 重构后新登记的平台（Command Code、Cline）按同类平台（OpenCode）的位置补入。
var (
	legacyAllPlatforms = []string{
		PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok,
		PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformTypeSafe,
		PlatformCommandCode,
		PlatformCline,
	}
	legacySchedulerSnapshotPlatforms = []string{
		PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformAntigravity, PlatformGrok,
		PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformTypeSafe,
		PlatformCommandCode,
		PlatformCline,
	}
	legacyCompositeMatchingPlatforms = legacySchedulerSnapshotPlatforms
	legacyMultiProtocolProviders     = []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformCommandCode, PlatformCline}
	platformProbeValues              = append(append([]string{}, legacyAllPlatforms...), PlatformComposite, "", "moonshot", "Kimi", "openai ", "glm", "bogus")
	platformProbeAccountTypes        = []string{AccountTypeAPIKey, AccountTypeOAuth, AccountTypeSetupToken, AccountTypeUpstream, ""}
)

func legacyIsCNProvider(platform string) bool {
	switch platform {
	case PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax:
		return true
	}
	return false
}

func legacyIsOpenAICompatible(platform string) bool {
	return platform == PlatformOpenAI || platform == PlatformGrok || legacyIsCNProvider(platform) || platform == PlatformOpenCodeGo || platform == PlatformCommandCode || platform == PlatformCline
}

func legacyNormalizeOpenAICompatiblePlatform(platform string) string {
	switch platform {
	case PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformCommandCode, PlatformCline:
		return platform
	}
	return PlatformOpenAI
}

func legacyIsUpstreamBillingProbeIdentity(platform, accountType string) bool {
	if accountType != AccountTypeAPIKey {
		return false
	}
	switch platform {
	case PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok,
		PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo,
		PlatformTypeSafe, PlatformCommandCode, PlatformCline:
		return true
	}
	return false
}

func legacyIsHeaderOverrideEligible(platform, accountType string) bool {
	switch platform {
	case PlatformAnthropic, PlatformOpenAI, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformCommandCode, PlatformCline:
		return accountType == AccountTypeAPIKey
	case PlatformGrok:
		return accountType == AccountTypeAPIKey || accountType == AccountTypeOAuth
	}
	return false
}

func legacyIsConcreteRequestPlatform(platform string) bool {
	for _, p := range legacyAllPlatforms {
		if p == platform {
			return true
		}
	}
	return false
}

func TestPlatformListDerivedListsMatchLegacy(t *testing.T) {
	require.Equal(t, legacyAllPlatforms, AllowedQuotaPlatforms)
	require.Equal(t, legacySchedulerSnapshotPlatforms, schedulerSnapshotPlatforms())
	require.Equal(t, legacyCompositeMatchingPlatforms, matchingPlatforms(PlatformComposite))
	require.Equal(t, []string{PlatformKimi}, matchingPlatforms(PlatformKimi))

	var multiProtocol []string
	for _, platform := range legacyAllPlatforms {
		if IsMultiProtocolAPIKeyProvider(platform) {
			multiProtocol = append(multiProtocol, platform)
		}
	}
	require.Equal(t, legacyMultiProtocolProviders, multiProtocol)
}

func TestPlatformListDerivedPredicatesMatchLegacy(t *testing.T) {
	for _, platform := range platformProbeValues {
		require.Equal(t, legacyIsCNProvider(platform), IsCNProvider(platform), platform)
		require.Equal(t, legacyIsConcreteRequestPlatform(platform), isConcreteRequestPlatform(platform), platform)
		require.Equal(t, legacyNormalizeOpenAICompatiblePlatform(platform), NormalizeOpenAICompatiblePlatform(platform), platform)
		require.Equal(t, legacyIsOpenAICompatible(platform), (&Account{Platform: platform}).IsOpenAICompatible(), platform)
		for _, accountType := range platformProbeAccountTypes {
			require.Equal(t, legacyIsUpstreamBillingProbeIdentity(platform, accountType),
				IsUpstreamBillingProbeIdentity(platform, accountType), "%s/%s", platform, accountType)
			require.Equal(t, legacyIsHeaderOverrideEligible(platform, accountType),
				(&Account{Platform: platform, Type: accountType}).IsHeaderOverrideEligible(), "%s/%s", platform, accountType)
		}
	}
}

// 多协议 API Key 供应商经 OpenAI 网关转发（NormalizeOpenAICompatiblePlatform、
// countTokens 路由等依赖此约束），ProviderProfile 登记的平台必须在平台清单中声明 openai 网关。
func TestProviderProfilesUseOpenAIGateway(t *testing.T) {
	for platform := range providerProfiles {
		spec, ok := domain.LookupPlatform(platform)
		require.True(t, ok, platform)
		require.Equal(t, domain.PlatformGatewayOpenAI, spec.Gateway, platform)
	}
}

// 转发、探测与计费中"多协议供应商"类判断改为查 profile：现有平台上与改动前的
// 国产厂商 / 国产厂商 ∪ OpenCode 判断逐一相同。
func TestProviderProfileAccountPredicatesMatchLegacy(t *testing.T) {
	for _, platform := range platformProbeValues {
		account := &Account{Platform: platform, Type: AccountTypeAPIKey}
		// Cline 只有 Chat Completions 一个端点，按入站协议分流（其余入站转换）。
		require.Equal(t, legacyIsCNProvider(platform) || platform == PlatformCline, account.RoutesProtocolByInbound(), platform)
		require.Equal(t, legacyIsCNProvider(platform) || platform == PlatformOpenCodeGo || platform == PlatformCommandCode || platform == PlatformCline, account.IsMultiProtocolAPIKey(), platform)
	}
	var nilAccount *Account
	require.False(t, nilAccount.RoutesProtocolByInbound())
}

// 仅在 providerProfiles 中登记的新供应商按分流方式自动获得对应行为：按入站协议
// 分流的与国产厂商一致，按模型分流的与 OpenCode 一致。
func TestNewlyRegisteredProviderInheritsRoutingBehaviour(t *testing.T) {
	const byInbound, byModel = "test_inbound_provider", "test_model_provider"
	providerProfiles[byInbound] = &ProviderProfile{
		Platform: byInbound, DefaultMode: AccountModePayG, Routing: ProviderRoutingByInbound,
		Modes: map[string]ProviderEndpoints{AccountModePayG: {BaseURLs: map[string]string{
			APIProtocolChatCompletions: "https://inbound.example.com/v1",
			APIProtocolAnthropic:       "https://inbound.example.com/anthropic",
		}}},
	}
	providerProfiles[byModel] = &ProviderProfile{
		Platform: byModel, DefaultMode: AccountModePayG, Routing: ProviderRoutingByModel,
		Modes: map[string]ProviderEndpoints{AccountModePayG: {BaseURLs: map[string]string{
			APIProtocolChatCompletions: "https://model.example.com/v1",
		}}},
	}
	t.Cleanup(func() {
		delete(providerProfiles, byInbound)
		delete(providerProfiles, byModel)
	})

	inbound := &Account{Platform: byInbound, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "sk-test", "api_protocol": APIProtocolAdaptive,
		"base_url": "https://inbound.example.com/v1",
	}}
	require.True(t, inbound.IsMultiProtocolAPIKey())
	require.True(t, inbound.RoutesProtocolByInbound())
	// 无原生 Responses 端点的 adaptive 账号回退 Chat Completions（与 GLM 相同）。
	require.True(t, shouldForwardOpenAIResponsesViaRawChatCompletions(inbound))
	require.True(t, shouldEstimateOpenAIInputTokensLocally(inbound))
	require.Equal(t, "https://inbound.example.com/v1", upstreamModelRegistryBaseURL(inbound))

	model := &Account{Platform: byModel, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "sk-test", "api_protocol": APIProtocolAdaptive,
		"base_url": "https://model.example.com/v1",
	}}
	require.True(t, model.IsMultiProtocolAPIKey())
	require.False(t, model.RoutesProtocolByInbound())
	// 按模型分流：protocol_rules 决定协议，不按 Responses 能力标记回退（与 OpenCode 相同）。
	require.False(t, shouldForwardOpenAIResponsesViaRawChatCompletions(model))
	require.Equal(t, "https://model.example.com/v1", upstreamModelRegistryBaseURL(model))
}
