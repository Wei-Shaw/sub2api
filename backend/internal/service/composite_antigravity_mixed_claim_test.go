package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// antigravity 账号只有管理员显式配置的映射构成 composite 声明；平台默认映射（Claude /
// Gemini 模型）属于混合调度语义，无论是否开启混合调度都不构成声明。
func TestCompositeClaimAntigravityOnlyExplicitMappingClaims(t *testing.T) {
	const model = "claude-sonnet-4-5"
	mixedOff := &Account{ID: 1, Platform: PlatformAntigravity, Type: AccountTypeOAuth}
	require.Equal(t, CompositeClaimNone, CompositeAccountClaimStrength(mixedOff, model))
	require.False(t, CompositeAccountClaimsModel(mixedOff, model))

	mixedOn := &Account{ID: 2, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Extra: map[string]any{"mixed_scheduling": true}}
	require.Equal(t, CompositeClaimNone, CompositeAccountClaimStrength(mixedOn, model),
		"混合调度开启时默认映射也不构成声明，否则与 anthropic 静默成池")

	explicitMixedOff := &Account{ID: 3, Platform: PlatformAntigravity, Type: AccountTypeOAuth,
		Credentials: map[string]any{"model_mapping": map[string]any{"team-alias": "claude-opus-4-8"}}}
	require.Equal(t, CompositeClaimExplicit, CompositeAccountClaimStrength(explicitMixedOff, "team-alias"))
	require.Equal(t, CompositeClaimNone, CompositeAccountClaimStrength(explicitMixedOff, model), "默认映射条目不因存在自定义映射而生效")
}

// 「anthropic 原生账号 + 开启混合调度的 antigravity」分组：归属解析为 anthropic 单目标
// （不成池），antigravity 经混合调度展开服务——配额、渠道定价与分组模型路由仍按
// anthropic，与 main 一致；混合调度展开的账号不受 account_model 归属层级否决。
func TestCompositeAnthropicWithMixedAntigravityStaysSingleTarget(t *testing.T) {
	const (
		group = int64(21)
		model = "claude-sonnet-4-5"
	)
	antigravity := Account{ID: 212, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Extra: map[string]any{"mixed_scheduling": true}}
	resolver, _ := newOllamaOwnershipResolver(
		compositeOwnershipScopedRecord{groupID: group, platform: PlatformAnthropic, schedulable: true,
			account: Account{ID: 211, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}},
		compositeOwnershipScopedRecord{groupID: group, platform: PlatformAntigravity, schedulable: true, account: antigravity},
	)

	decision, err := resolver.Resolve(context.Background(), group, model, CompositeRouteEndpointMessages)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, PlatformAnthropic, decision.TargetPlatform)
	require.Empty(t, decision.CandidatePlatforms, "不得与混合调度 antigravity 构成账号池")

	ctx := WithCompositeRouteDecision(context.Background(), decision)
	require.Equal(t, CompositeRouteSourceAccount, ctx.Value(ctxkey.CompositeRouteSource))
	svc := &GatewayService{}
	require.True(t, svc.isModelSupportedByAccountWithContext(ctx, &antigravity, model),
		"混合调度展开的 antigravity 账号按映射判定可服务，不被归属层级否决")

	mixedOff := antigravity
	mixedOff.Extra = nil
	require.False(t, compositeMixedSchedulingExpandedAccount(ctx, &mixedOff), "未开启混合调度不属于展开账号")
}
