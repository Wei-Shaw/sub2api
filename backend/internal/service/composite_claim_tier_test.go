package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 存在强声明平台时，account_model 否决与池模型门只放行强声明账号：同平台的通配
// catch-all 账号不得冒领管理员显式绑定的公开模型（main 上该否决为精确匹配）。
func TestCompositeAccountMeetsClaimTierFollowsOwnershipTier(t *testing.T) {
	const publicModel = "grok-public"
	explicit := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{publicModel: "grok-4.3"}}}
	wildcard := &Account{ID: 2, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{"*": "grok-3-mini"}}}
	none := &Account{ID: 3, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"model_mapping": map[string]any{"other": "grok-3"}}}

	strongCtx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
		Matched: true, Source: CompositeRouteSourceAccount, PublicModel: publicModel,
		TargetPlatform: PlatformGrok, UpstreamModel: publicModel, RequiredClaimStrength: CompositeClaimExplicit,
	})
	require.True(t, CompositeAccountMeetsClaimTier(strongCtx, explicit, publicModel))
	require.False(t, CompositeAccountMeetsClaimTier(strongCtx, wildcard, publicModel))
	require.False(t, CompositeAccountMeetsClaimTier(strongCtx, none, publicModel))

	// 无强声明、按通配回退构成归属时，通配账号可承接。
	wildcardCtx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
		Matched: true, Source: CompositeRouteSourceAccount, PublicModel: publicModel,
		TargetPlatform: PlatformGrok, UpstreamModel: publicModel, RequiredClaimStrength: CompositeClaimWildcard,
	})
	require.True(t, CompositeAccountMeetsClaimTier(wildcardCtx, wildcard, publicModel))
	require.False(t, CompositeAccountMeetsClaimTier(wildcardCtx, none, publicModel))

	// 无层级信息：任意强度均可（等价 CompositeAccountClaimsModel）。
	require.True(t, CompositeAccountMeetsClaimTier(context.Background(), wildcard, publicModel))

	// 池决策同样携带层级。
	poolCtx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
		Matched: true, Source: CompositeRouteSourceAccountPool, PublicModel: publicModel,
		CandidatePlatforms: []string{PlatformGrok, PlatformOpenAI}, RequiredClaimStrength: CompositeClaimExplicit,
	})
	require.False(t, CompositeAccountMeetsClaimTier(poolCtx, wildcard, publicModel))
}

func TestResolveCompositeModelOwnershipRecordsClaimTier(t *testing.T) {
	const group = int64(11)
	strongRepo := &compositeOwnershipScopedRepo{records: []compositeOwnershipScopedRecord{
		{groupID: group, platform: PlatformGrok, schedulable: true, account: mappingAccount(1, PlatformGrok, "alias", "grok-4.3")},
		{groupID: group, platform: PlatformOpenAI, schedulable: true, account: Account{ID: 2, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"*": "gpt-5"}}}},
	}}
	ownership, err := (&GatewayService{accountRepo: strongRepo}).resolveCompositeModelOwnership(context.Background(), group, "alias")
	require.NoError(t, err)
	require.Equal(t, PlatformGrok, ownership.TargetPlatform)
	require.Equal(t, CompositeClaimExplicit, ownership.RequiredClaimStrength)

	wildcardRepo := &compositeOwnershipScopedRepo{records: []compositeOwnershipScopedRecord{
		{groupID: group, platform: PlatformOpenAI, schedulable: true, account: Account{ID: 2, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"*": "gpt-5"}}}},
	}}
	ownership, err = (&GatewayService{accountRepo: wildcardRepo}).resolveCompositeModelOwnership(context.Background(), group, "alias")
	require.NoError(t, err)
	require.Equal(t, PlatformOpenAI, ownership.TargetPlatform)
	require.Equal(t, CompositeClaimWildcard, ownership.RequiredClaimStrength)
}

func TestCompositeRouteResolverPropagatesClaimTier(t *testing.T) {
	const group = int64(12)
	resolver, _ := newOllamaOwnershipResolver(
		compositeOwnershipScopedRecord{groupID: group, platform: PlatformGrok, schedulable: true,
			account: mappingAccount(1, PlatformGrok, "alias", "grok-4.3")},
		compositeOwnershipScopedRecord{groupID: group, platform: PlatformGrok, schedulable: true,
			account: Account{ID: 2, Platform: PlatformGrok, Credentials: map[string]any{"model_mapping": map[string]any{"*": "grok-3-mini"}}}},
	)
	decision, err := resolver.Resolve(context.Background(), group, "alias", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.Equal(t, CompositeRouteSourceAccount, decision.Source)
	require.Equal(t, CompositeClaimExplicit, decision.RequiredClaimStrength)

	ctx := WithCompositeRouteDecision(context.Background(), decision)
	require.True(t, CompositeAccountMeetsClaimTier(ctx, &Account{ID: 1, Platform: PlatformGrok,
		Credentials: map[string]any{"model_mapping": map[string]any{"alias": "grok-4.3"}}}, "alias"))
	require.False(t, CompositeAccountMeetsClaimTier(ctx, &Account{ID: 2, Platform: PlatformGrok,
		Credentials: map[string]any{"model_mapping": map[string]any{"*": "grok-3-mini"}}}, "alias"))
}
