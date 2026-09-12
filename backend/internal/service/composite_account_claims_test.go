package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 统一能力谓词 CompositeAccountClaimsModel 的契约测试（评审C收敛规则）。

func TestCompositeAccountClaimsModel_NilAndEmptyInputs(t *testing.T) {
	require.False(t, CompositeAccountClaimsModel(nil, "gpt-5"))
	account := &Account{Platform: PlatformOpenAI}
	require.False(t, CompositeAccountClaimsModel(account, ""))
	require.False(t, CompositeAccountClaimsModel(account, "   "))
}

// 规则A：非空 mapping 仅精确/既有通配命中才 claim。
func TestCompositeAccountClaimsModel_ExplicitMappingExactAndWildcard(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"my-gpt":       "gpt-5",
				"team-*":       "gpt-5.1",
				"empty-target": "",
			},
		},
	}

	require.True(t, CompositeAccountClaimsModel(account, "my-gpt"))
	require.True(t, CompositeAccountClaimsModel(account, "team-alpha"))
	// 非空 mapping 未命中 → 不允许 native fallback（测试 d）。
	require.False(t, CompositeAccountClaimsModel(account, "gpt-5"))
	require.False(t, CompositeAccountClaimsModel(account, "unknown-alias"))
	// 映射目标为空的条目不构成可路由声明。
	require.False(t, CompositeAccountClaimsModel(account, "empty-target"))
}

// 规则A（归一化回退）：既有平台归一化后的命中仍算 claim。
func TestCompositeAccountClaimsModel_NormalizedLookupHit(t *testing.T) {
	account := &Account{
		Platform: PlatformGemini,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gemini-3.1-pro-preview": "gemini-3.1-pro-preview"},
		},
	}
	require.True(t, CompositeAccountClaimsModel(account, "gemini-3.1-pro-preview-customtools"))
}

// 规则B：空 mapping 仅当 detector 平台一致且 IsModelSupported 通过。
func TestCompositeAccountClaimsModel_EmptyMappingNativeOnly(t *testing.T) {
	deepseek := &Account{Platform: PlatformDeepseek}
	require.True(t, CompositeAccountClaimsModel(deepseek, "deepseek-chat"))

	// detector 平台不一致的空 mapping 账号不得冒领（测试 b 的谓词面）。
	openAI := &Account{Platform: PlatformOpenAI}
	require.False(t, CompositeAccountClaimsModel(openAI, "deepseek-chat"))
	zhipu := &Account{Platform: PlatformZhipu}
	require.False(t, CompositeAccountClaimsModel(zhipu, "deepseek-chat"))

	// detector 不认识的模型，空 mapping 任何平台都不得 claim。
	require.False(t, CompositeAccountClaimsModel(deepseek, "unknown-alias"))
}

// opencode_go 无 detector 分支：空 mapping 永不入池（评审C修正，测试 c），
// 仅显式 mapping 命中可入池。
func TestCompositeAccountClaimsModel_OpenCodeGoRequiresExplicitMapping(t *testing.T) {
	opencode := &Account{Platform: PlatformOpenCodeGo}
	require.False(t, CompositeAccountClaimsModel(opencode, "deepseek-chat"))
	require.False(t, CompositeAccountClaimsModel(opencode, "gpt-5"))
	require.False(t, CompositeAccountClaimsModel(opencode, "unknown-alias"))

	mapped := &Account{
		Platform: PlatformOpenCodeGo,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"deepseek-chat": "deepseek-chat"},
		},
	}
	require.True(t, CompositeAccountClaimsModel(mapped, "deepseek-chat"))
	require.False(t, CompositeAccountClaimsModel(mapped, "gpt-5"))
}

// OpenAI OAuth 空 mapping 保持既有 foreign-model / servable 限制。
func TestCompositeAccountClaimsModel_OpenAIOAuthKeepsServableRestriction(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.True(t, CompositeAccountClaimsModel(oauth, "gpt-5"))
	require.False(t, CompositeAccountClaimsModel(oauth, "deepseek-v4-pro"))

	// 显式映射命中仍按规则A claim。
	mapped := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"deepseek-v4-pro": "deepseek-v4-pro"},
		},
	}
	require.True(t, CompositeAccountClaimsModel(mapped, "deepseek-v4-pro"))
}

// Grok 默认映射经 GetModelMapping 生效，归入规则A（与转发阶段映射一致）。
func TestCompositeAccountClaimsModel_GrokDefaultMappingUsesRuleA(t *testing.T) {
	grok := &Account{Platform: PlatformGrok}
	require.True(t, CompositeAccountClaimsModel(grok, "grok-4.3"))
	// detector 识别为 grok 但不在有效映射中的模型不 claim。
	require.False(t, CompositeAccountClaimsModel(grok, "grok-future-unknown"))
}

// 声明强度分级：精确/受控 native 为强声明，通配为弱声明，空目标显式条目
// 无声明且不回落通配。
func TestCompositeAccountClaimStrength(t *testing.T) {
	exact := &Account{
		Platform: PlatformDeepseek,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"deepseek-chat": "deepseek-chat"},
		},
	}
	require.Equal(t, CompositeClaimExplicit, CompositeAccountClaimStrength(exact, "deepseek-chat"))
	require.Equal(t, CompositeClaimNone, CompositeAccountClaimStrength(exact, "deepseek-v4"))

	wildcard := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"*": "gpt-5", "gpt-*": "gpt-5.1"},
		},
	}
	require.Equal(t, CompositeClaimWildcard, CompositeAccountClaimStrength(wildcard, "any-alias"))
	// 精确键优先于通配（同为 mapping 命中时精确是强声明）。
	withExact := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"*": "gpt-5", "gpt-5": "gpt-5"},
		},
	}
	require.Equal(t, CompositeClaimExplicit, CompositeAccountClaimStrength(withExact, "gpt-5"))

	// 空目标显式条目遮蔽通配：不构成声明也不回落（与转发匹配顺序一致）。
	emptyTarget := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"empty-alias": "", "*": "gpt-5"},
		},
	}
	require.Equal(t, CompositeClaimNone, CompositeAccountClaimStrength(emptyTarget, "empty-alias"))

	// 受控 native 空 mapping 与精确同强等级。
	native := &Account{Platform: PlatformDeepseek}
	require.Equal(t, CompositeClaimExplicit, CompositeAccountClaimStrength(native, "deepseek-chat"))
	// detector 不认识的模型无声明。
	require.Equal(t, CompositeClaimNone, CompositeAccountClaimStrength(native, "unknown-alias"))
	require.Equal(t, CompositeClaimNone, CompositeAccountClaimStrength(nil, "gpt-5"))
}

// 通配一致性（测试 g）：同一通配映射在 claim 与既有 IsModelSupported 语义一致。
func TestCompositeAccountClaimsModel_WildcardConsistentWithSupport(t *testing.T) {
	account := &Account{
		Platform: PlatformDeepseek,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"deepseek-v4*": "deepseek-v4"},
		},
	}
	for _, model := range []string{"deepseek-v4", "deepseek-v4-flash", "deepseek-v4-pro"} {
		require.True(t, CompositeAccountClaimsModel(account, model), "model=%s", model)
		require.True(t, account.IsModelSupported(model), "model=%s", model)
	}
	require.False(t, CompositeAccountClaimsModel(account, "deepseek-v3"))
}

// 瞬态字段不参与能力判定（测试 f 的谓词面）：限流/过载中的账号能力不变。
func TestCompositeAccountClaimsModel_IgnoresTransientState(t *testing.T) {
	now := time.Now()
	account := &Account{
		Platform:         PlatformDeepseek,
		RateLimitedAt:    &now,
		RateLimitResetAt: &now,
		OverloadUntil:    &now,
	}
	require.True(t, CompositeAccountClaimsModel(account, "deepseek-chat"))
}
