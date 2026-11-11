package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func paidDeepseekFallbackInput() CompositeRouteInput {
	return CompositeRouteInput{PublicModel: "deepseek-flash", MatchType: CompositeRouteMatchExact, TargetPlatform: PlatformOpenCodeGo, UpstreamModel: "deepseek-v4.1-flash", Endpoint: CompositeRouteEndpointAny, Enabled: true, FallbackTargets: []CompositeFallbackTarget{{TargetPlatform: PlatformDeepseek, UpstreamModel: "deepseek-flash"}}}
}

func TestCompositeFallbackValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*CompositeRouteInput)
		valid  bool
	}{
		{"paid DeepSeek", func(*CompositeRouteInput) {}, true},
		{"canonical official Pro", func(i *CompositeRouteInput) {
			i.PublicModel, i.UpstreamModel = "deepseek-pro", "deepseek-v4-pro"
			i.FallbackTargets[0].UpstreamModel = "deepseek-v4-pro"
		}, true},
		{"official Pro alias rejected", func(i *CompositeRouteInput) {
			i.PublicModel, i.UpstreamModel = "deepseek-pro", "deepseek-v4-pro"
			i.FallbackTargets[0].UpstreamModel = "deepseek-pro"
		}, false},
		{"legacy empty", func(i *CompositeRouteInput) { i.FallbackTargets = nil; i.MatchType = CompositeRouteMatchPrefix }, true},
		{"prefix", func(i *CompositeRouteInput) { i.MatchType = CompositeRouteMatchPrefix }, false},
		{"free", func(i *CompositeRouteInput) { i.PublicModel = "deepseek-v4-flash-free" }, false},
		{"tier changed", func(i *CompositeRouteInput) { i.FallbackTargets[0].UpstreamModel = "deepseek-pro" }, false},
		{"public tier changed", func(i *CompositeRouteInput) { i.PublicModel = "deepseek-pro" }, false},
		{"cycle", func(i *CompositeRouteInput) { i.FallbackTargets[0].TargetPlatform = PlatformOpenCodeGo }, false},
		{"unbounded", func(i *CompositeRouteInput) { i.FallbackTargets = append(i.FallbackTargets, i.FallbackTargets[0]) }, false},
		{"Kimi", func(i *CompositeRouteInput) { i.TargetPlatform = PlatformKimi; i.UpstreamModel = "kimi-k3" }, false},
		{"GLM", func(i *CompositeRouteInput) { i.TargetPlatform = PlatformZhipu; i.UpstreamModel = "glm-5.3" }, false},
		{"Go Kimi", func(i *CompositeRouteInput) { i.UpstreamModel = "kimi-k3" }, false},
		{"Go GLM", func(i *CompositeRouteInput) { i.UpstreamModel = "glm-5.3" }, false},
		{"Go MiMo", func(i *CompositeRouteInput) { i.UpstreamModel = "mimo-v2.5-pro" }, false},
		{"Go Qwen", func(i *CompositeRouteInput) { i.UpstreamModel = "qwen3.6-plus" }, false},
		{"Go Free", func(i *CompositeRouteInput) { i.UpstreamModel = "deepseek-v4-flash-free" }, false},
		{"media", func(i *CompositeRouteInput) { i.Endpoint = CompositeRouteEndpointImages }, false},
		{"MiniMax", func(i *CompositeRouteInput) {
			i.PublicModel = "MiniMax-M2.7"
			i.TargetPlatform = PlatformMiniMax
			i.UpstreamModel = "MiniMax-M2.7"
			i.FallbackTargets = []CompositeFallbackTarget{{TargetPlatform: PlatformOpenCodeGo, UpstreamModel: "minimax-m2.7"}}
		}, true},
		{"MiniMax different model", func(i *CompositeRouteInput) {
			i.TargetPlatform = PlatformMiniMax
			i.UpstreamModel = "MiniMax-M2.7"
			i.FallbackTargets = []CompositeFallbackTarget{{TargetPlatform: PlatformOpenCodeGo, UpstreamModel: "minimax-m2.5"}}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := paidDeepseekFallbackInput()
			tc.change(&input)
			_, err := compositeRouteFromInput(1, input)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestCompositeFallbackResolverAndFinalMapping(t *testing.T) {
	route, err := compositeRouteFromInput(1, paidDeepseekFallbackInput())
	require.NoError(t, err)
	resolver := NewCompositeRouteResolver(&compositeRouteRepoStub{routes: []CompositeModelRoute{*route}})
	decision, err := resolver.Resolve(context.Background(), 1, route.PublicModel, CompositeRouteEndpointMessages)
	require.NoError(t, err)
	require.Len(t, decision.FallbackTargets, 1)
	require.Equal(t, PlatformOpenCodeGo, decision.TargetPlatform)
	ctx := WithCompositeRouteDecision(context.Background(), decision)
	account := &Account{Platform: PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	require.NoError(t, ValidateCompositeFallbackAccount(ctx, account, "deepseek-v4.1-flash"))
	account.Credentials = map[string]any{"model_mapping": map[string]any{"deepseek-v4.1-flash": "deepseek-v4-pro"}}
	require.Error(t, ValidateCompositeFallbackAccount(ctx, account, "deepseek-v4.1-flash"))
	account.Credentials = map[string]any{"account_mode": AccountModeZen}
	require.Error(t, ValidateCompositeFallbackAccount(ctx, account, "deepseek-v4.1-flash"))
	decision.TargetPlatform, decision.UpstreamModel = PlatformDeepseek, "deepseek-flash"
	ctx = WithCompositeRouteDecision(ctx, decision)
	require.Equal(t, PlatformDeepseek, QuotaPlatform(ctx, nil))
	model, ok := RequestedPublicModelFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "deepseek-flash", model)
	require.NoError(t, ValidateCompositeFallbackAccount(ctx, &Account{Platform: PlatformDeepseek}, "deepseek-flash"))
	require.Error(t, ValidateCompositeFallbackModel(ctx, PlatformDeepseek, "deepseek-pro"))
	decision.UpstreamModel = "deepseek-v4-pro"
	ctx = WithCompositeRouteDecision(ctx, decision)
	pro := &Account{Platform: PlatformDeepseek}
	require.NoError(t, ValidateCompositeFallbackAccount(ctx, pro, "deepseek-v4-pro"))
	require.Error(t, ValidateCompositeFallbackModel(ctx, PlatformDeepseek, "deepseek-pro"))
	pro.Credentials = map[string]any{"model_mapping": map[string]any{"deepseek-v4-pro": "deepseek-pro"}}
	require.Error(t, ValidateCompositeFallbackAccount(ctx, pro, "deepseek-v4-pro"))
}
