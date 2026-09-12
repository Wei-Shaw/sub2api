package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// composite 跨平台稳定候选池（account_pool）契约测试。
//
// 覆盖必测项：a 双平台显式同别名→有序 pool；b 官方 deepseek 空 mapping +
// opencode 显式 mapping→pool、空 mapping 无关 openai/zhipu 不入；e 显式 route
// 仍 single 且 UpstreamModel 语义不变；f 瞬态限流不改能力池、配置移除声明改变
// 能力；h ctx roundtrip 池不被单 target 污染、切片引用修改不影响 ctx。

func compositePoolOwnershipRepo(accounts []Account) *compositeOwnershipAccountRepo {
	return &compositeOwnershipAccountRepo{accounts: accounts}
}

// a. 双平台对同一公开模型的显式映射 → Matched=true + 去重升序 CandidatePlatforms。
func TestCompositePoolOwnershipDualPlatformExplicitAlias(t *testing.T) {
	groupID := int64(7)
	repo := compositePoolOwnershipRepo([]Account{
		{
			ID:       1,
			Platform: PlatformZhipu,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"router-model": "glm-5.2"},
			},
		},
		{
			ID:       2,
			Platform: PlatformDeepseek,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"router-model": "deepseek-v4-pro"},
			},
		},
		// 同平台重复声明去重：openai 两个账号都声明，不产生重复平台项。
		{
			ID:       3,
			Platform: PlatformOpenAI,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"router-model": "gpt-5"},
			},
		},
		{
			ID:       4,
			Platform: PlatformOpenAI,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"router-model": "gpt-5.1"},
			},
		},
	})
	svc := &GatewayService{accountRepo: repo}

	ownership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "router-model")
	require.NoError(t, err)
	require.True(t, ownership.Matched)
	require.False(t, ownership.Ambiguous)
	require.Empty(t, ownership.TargetPlatform)
	require.Equal(t, []string{PlatformDeepseek, PlatformOpenAI, PlatformZhipu}, ownership.CandidatePlatforms)
}

// b. 官方 deepseek 空 mapping（规则B，强声明）+ opencode 显式 mapping（规则A
// 精确，强声明）→ 双平台 pool；空 mapping 的无关 openai/zhipu 账号不入池。
func TestCompositePoolOwnershipDeepseekNativePlusOpenCodeMapping(t *testing.T) {
	groupID := int64(7)
	repo := compositePoolOwnershipRepo([]Account{
		{ID: 1, Platform: PlatformDeepseek},
		{
			ID:       2,
			Platform: PlatformOpenCodeGo,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"deepseek-chat": "deepseek-chat"},
			},
		},
		{ID: 3, Platform: PlatformOpenAI},
		{ID: 4, Platform: PlatformZhipu},
	})
	svc := &GatewayService{accountRepo: repo}

	ownership, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "deepseek-chat")
	require.NoError(t, err)
	require.True(t, ownership.Matched)
	require.Empty(t, ownership.TargetPlatform)
	require.Equal(t, []string{PlatformDeepseek, PlatformOpenCodeGo}, ownership.CandidatePlatforms)

	// 仅 deepseek 空 mapping 声明时保持 single（既有语义）。
	single, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "deepseek-reasoner")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformDeepseek, Matched: true}, single)
}

// 声明强度分层：有强声明平台时通配 catch-all 平台不混池；完全无强声明才
// 使用通配命中平台集合（可构成 fallback 池）。
func TestCompositePoolOwnershipClaimStrengthTiering(t *testing.T) {
	groupID := int64(7)
	svc := func(accounts []Account) *GatewayService {
		return &GatewayService{accountRepo: compositePoolOwnershipRepo(accounts)}
	}

	// grok 精确（强）+ openai/zhipu "*"（弱）→ 仅 grok（否则 grok 请求被改写为
	// 通配目标模型，回归）。
	grokFirst := svc([]Account{
		{
			ID:       1,
			Platform: PlatformGrok,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"grok-public": "grok-4.3"},
			},
		},
		{
			ID:          2,
			Platform:    PlatformOpenAI,
			Credentials: map[string]any{"model_mapping": map[string]any{"*": "gpt-5"}},
		},
		{
			ID:          3,
			Platform:    PlatformZhipu,
			Credentials: map[string]any{"model_mapping": map[string]any{"*": "glm-5.2"}},
		},
	})
	ownership, err := grokFirst.resolveCompositeModelOwnership(context.Background(), groupID, "grok-public")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformGrok, Matched: true}, ownership)

	// 无任何强声明：deepseek/zhipu 均只有通配命中 → 使用通配 fallback 平台集合。
	wildcardOnly := svc([]Account{
		{
			ID:          1,
			Platform:    PlatformDeepseek,
			Credentials: map[string]any{"model_mapping": map[string]any{"team-*": "deepseek-v4"}},
		},
		{
			ID:          2,
			Platform:    PlatformZhipu,
			Credentials: map[string]any{"model_mapping": map[string]any{"team-*": "glm-5.2", "zhipu-*": "glm-5.2"}},
		},
	})
	fallback, err := wildcardOnly.resolveCompositeModelOwnership(context.Background(), groupID, "team-alpha")
	require.NoError(t, err)
	require.True(t, fallback.Matched)
	require.Empty(t, fallback.TargetPlatform)
	require.Equal(t, []string{PlatformDeepseek, PlatformZhipu}, fallback.CandidatePlatforms)

	// 仅单个平台通配命中 → fallback single。
	singleWildcard, err := wildcardOnly.resolveCompositeModelOwnership(context.Background(), groupID, "zhipu-x")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformZhipu, Matched: true}, singleWildcard)
}

// e. 显式 route 优先于 pool，route 的 UpstreamModel 改写语义不变。
func TestCompositePoolResolverExplicitRouteBeatsPool(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             10,
				GroupID:        7,
				PublicModel:    "router-model",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformKimi,
				UpstreamModel:  "kimi-k2-thinking",
				Endpoint:       CompositeRouteEndpointAny,
				Enabled:        true,
			},
		},
	})
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (CompositeModelOwnership, error) {
		return CompositeModelOwnership{
			Matched:            true,
			CandidatePlatforms: []string{PlatformKimi, PlatformZhipu},
		}, nil
	})

	decision, err := resolver.Resolve(context.Background(), 7, "router-model", CompositeRouteEndpointMessages)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
	require.Equal(t, PlatformKimi, decision.TargetPlatform)
	require.Equal(t, "kimi-k2-thinking", decision.UpstreamModel)
	require.Empty(t, decision.CandidatePlatforms)
	require.NotNil(t, decision.Route)
}

// ownership 返回多平台候选 → pool decision；Ambiguous=true 但未携带候选的旧
// mock/异常输入仍明确失败。
func TestCompositePoolResolverReturnsPoolForOwnershipCandidates(t *testing.T) {
	resolver := NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (CompositeModelOwnership, error) {
		return CompositeModelOwnership{
			Matched:            true,
			CandidatePlatforms: []string{PlatformZhipu, PlatformDeepseek, PlatformDeepseek},
		}, nil
	})

	decision, err := resolver.Resolve(context.Background(), 7, "shared-alias", CompositeRouteEndpointChatCompletions)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceAccountPool, decision.Source)
	require.Empty(t, decision.TargetPlatform)
	require.Empty(t, decision.UpstreamModel)
	require.Equal(t, []string{PlatformDeepseek, PlatformZhipu}, decision.CandidatePlatforms)
}

func TestCompositePoolResolverAmbiguousWithoutCandidatesStillFails(t *testing.T) {
	resolver := NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (CompositeModelOwnership, error) {
		return CompositeModelOwnership{Ambiguous: true}, nil
	})

	decision, err := resolver.Resolve(context.Background(), 7, "shared-alias", CompositeRouteEndpointChatCompletions)
	require.NoError(t, err)
	require.False(t, decision.Matched)
	require.Empty(t, decision.TargetPlatform)
	require.Empty(t, decision.CandidatePlatforms)
	require.Equal(t, "model is exposed by multiple provider platforms", decision.Reason)
}

// f. 候选账号全部 rate-limited / overloaded 时能力池不变（选择期健康由
// scheduler 后续处理）；配置移除声明则能力立即变化。
func TestCompositePoolOwnershipUnaffectedByTransientAccountState(t *testing.T) {
	groupID := int64(7)
	now := time.Now()
	repo := compositePoolOwnershipRepo([]Account{
		{
			ID:               1,
			Platform:         PlatformDeepseek,
			RateLimitedAt:    &now,
			RateLimitResetAt: &now,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"deepseek-chat": "deepseek-chat"},
			},
		},
		{
			ID:            2,
			Platform:      PlatformOpenCodeGo,
			OverloadUntil: &now,
			Credentials: map[string]any{
				"model_mapping": map[string]any{"deepseek-chat": "deepseek-chat"},
			},
		},
	})
	svc := &GatewayService{accountRepo: repo}

	limited, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "deepseek-chat")
	require.NoError(t, err)
	require.True(t, limited.Matched)
	require.Equal(t, []string{PlatformDeepseek, PlatformOpenCodeGo}, limited.CandidatePlatforms)

	// 配置移除 opencode 声明 → 池坍缩为 deepseek single。
	repo.accounts = []Account{repo.accounts[0]}
	single, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "deepseek-chat")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{TargetPlatform: PlatformDeepseek, Matched: true}, single)

	// 配置移除全部声明 → 能力消失（0 候选，回退 detector/unknown 原逻辑）。
	repo.accounts = nil
	none, err := svc.resolveCompositeModelOwnership(context.Background(), groupID, "deepseek-chat")
	require.NoError(t, err)
	require.Equal(t, CompositeModelOwnership{}, none)
}

// h. ctx roundtrip：池决策不写 ResolvedTargetPlatform / ResolvedUpstreamModel；
// getter 返回副本；显式 pin 不污染池值（但 shortcut 以 pin 为先，保持 single 行为）。
func TestCompositePoolContextRoundTrip(t *testing.T) {
	poolDecision := CompositeRouteDecision{
		Matched:            true,
		Source:             CompositeRouteSourceAccountPool,
		GroupID:            7,
		PublicModel:        "deepseek-chat",
		CandidatePlatforms: []string{PlatformDeepseek, PlatformOpenCodeGo},
		Endpoint:           CompositeRouteEndpointAny,
	}
	ctx := WithCompositeRouteDecision(context.Background(), poolDecision)

	candidates := CompositeCandidatePlatformsFromContext(ctx)
	require.Equal(t, []string{PlatformDeepseek, PlatformOpenCodeGo}, candidates)
	_, hasPlatform := ResolvedTargetPlatformFromContext(ctx)
	require.False(t, hasPlatform)
	_, hasUpstream := ResolvedUpstreamModelFromContext(ctx)
	require.False(t, hasUpstream)
	source, hasSource := CompositeRouteSourceFromContext(ctx)
	require.True(t, hasSource)
	require.Equal(t, CompositeRouteSourceAccountPool, source)

	// 切片引用修改不影响 ctx 内的池。
	candidates[0] = "polluted"
	require.Equal(t, []string{PlatformDeepseek, PlatformOpenCodeGo}, CompositeCandidatePlatformsFromContext(ctx))

	// WithCompositeCandidatePlatforms 归一化：去重、去空白、升序、副本。
	normalizedCtx := WithCompositeCandidatePlatforms(context.Background(),
		[]string{PlatformZhipu, " ", PlatformDeepseek, PlatformZhipu})
	require.Equal(t, []string{PlatformDeepseek, PlatformZhipu}, CompositeCandidatePlatformsFromContext(normalizedCtx))
	require.Nil(t, CompositeCandidatePlatformsFromContext(WithCompositeCandidatePlatforms(context.Background(), nil)))
	require.Nil(t, CompositeCandidatePlatformsFromContext(context.Background()))

	// 单平台决策不携带池。
	singleCtx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
		Matched:        true,
		Source:         CompositeRouteSourceAccount,
		GroupID:        7,
		PublicModel:    "deepseek-chat",
		TargetPlatform: PlatformDeepseek,
		UpstreamModel:  "deepseek-chat",
	})
	require.Nil(t, CompositeCandidatePlatformsFromContext(singleCtx))
}

// resolveCompositeRouteDecision：池在 ctx 时 shortcut 返回完整 pool decision
// （不报 unknown、不折叠 detector）；显式 pin 优先于池（既有 single 行为）。
func TestCompositePoolShortcutDecisionFromContext(t *testing.T) {
	svc := &GatewayService{compositeResolver: NewCompositeRouteResolver(nil)}
	group := &Group{ID: 7, Platform: PlatformComposite}

	poolCtx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
		Matched:            true,
		Source:             CompositeRouteSourceAccountPool,
		GroupID:            7,
		PublicModel:        "shared-alias",
		CandidatePlatforms: []string{PlatformDeepseek, PlatformZhipu},
	})

	decision, ok, err := svc.resolveCompositeRouteDecision(poolCtx, group, "shared-alias", CompositeRouteEndpointMessages)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, decision.Matched)
	require.True(t, isCompositePoolDecision(decision))
	require.Equal(t, CompositeRouteSourceAccountPool, decision.Source)
	require.Empty(t, decision.TargetPlatform)
	require.Empty(t, decision.UpstreamModel)
	require.Equal(t, []string{PlatformDeepseek, PlatformZhipu}, decision.CandidatePlatforms)

	// 显式 pin（如 gemini 端点 fallback）优先：保持既有 single 行为，且池值不被污染。
	pinnedCtx := WithResolvedTargetPlatform(poolCtx, PlatformOpenAI)
	pinned, ok, err := svc.resolveCompositeRouteDecision(pinnedCtx, group, "shared-alias", CompositeRouteEndpointMessages)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, isCompositePoolDecision(pinned))
	require.Equal(t, PlatformOpenAI, pinned.TargetPlatform)
	require.Equal(t, []string{PlatformDeepseek, PlatformZhipu}, CompositeCandidatePlatformsFromContext(pinnedCtx))
}

// isCompositePoolDecision 判定契约。
func TestIsCompositePoolDecision(t *testing.T) {
	require.False(t, isCompositePoolDecision(CompositeRouteDecision{
		Matched:        true,
		TargetPlatform: PlatformOpenAI,
	}))
	require.False(t, isCompositePoolDecision(CompositeRouteDecision{Matched: true}))
	require.True(t, isCompositePoolDecision(CompositeRouteDecision{
		Matched:            true,
		CandidatePlatforms: []string{PlatformOpenAI},
	}))
}
