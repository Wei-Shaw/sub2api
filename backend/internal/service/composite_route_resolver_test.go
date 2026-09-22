package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type compositeRouteRepoStub struct {
	routes []CompositeModelRoute
}

func (s compositeRouteRepoStub) ListByGroup(ctx context.Context, groupID int64, includeDisabled bool) ([]CompositeModelRoute, error) {
	routes := make([]CompositeModelRoute, 0, len(s.routes))
	for _, route := range s.routes {
		if route.GroupID != groupID {
			continue
		}
		if !includeDisabled && !route.Enabled {
			continue
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func (s compositeRouteRepoStub) Create(ctx context.Context, route *CompositeModelRoute) error {
	return nil
}

func (s compositeRouteRepoStub) Update(ctx context.Context, route *CompositeModelRoute) error {
	return nil
}

func (s compositeRouteRepoStub) Delete(ctx context.Context, id int64) error {
	return nil
}

func (s compositeRouteRepoStub) DeleteByGroup(ctx context.Context, groupID int64) error {
	return nil
}

func TestCompositeRouteResolverExplicitExactRouteRewritesModel(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             10,
				GroupID:        7,
				PublicModel:    "openrouter/gpt-5",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  "gpt-5",
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	decision, err := resolver.Resolve(context.Background(), 7, "openrouter/gpt-5", CompositeRouteEndpointChatCompletions)

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
	require.Equal(t, "gpt-5", decision.UpstreamModel)
	require.NotNil(t, decision.Route)
	require.Equal(t, int64(10), decision.Route.ID)
}

// Scenario: 唯一平台的精确别名可路由
func TestCompositeRouteResolverUsesAccountModelOwnershipForUnprefixedAlias(t *testing.T) {
	resolver := NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(func(_ context.Context, groupID int64, model string) (CompositeModelOwnership, error) {
		require.Equal(t, int64(7), groupID)
		require.Equal(t, "reasoning-alias", model)
		return CompositeModelOwnership{TargetPlatform: PlatformDeepseek, Matched: true}, nil
	})

	decision, err := resolver.Resolve(context.Background(), 7, "reasoning-alias", CompositeRouteEndpointChatCompletions)

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceAccount, decision.Source)
	require.Equal(t, PlatformDeepseek, decision.TargetPlatform)
	require.Equal(t, "reasoning-alias", decision.UpstreamModel)
}

func TestCompositeRouteResolverAccountOwnershipOverridesBuiltInDetector(t *testing.T) {
	resolver := NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (CompositeModelOwnership, error) {
		return CompositeModelOwnership{TargetPlatform: PlatformDeepseek, Matched: true}, nil
	})

	decision, err := resolver.Resolve(context.Background(), 7, "gpt-5", CompositeRouteEndpointResponses)

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceAccount, decision.Source)
	require.Equal(t, PlatformDeepseek, decision.TargetPlatform)
}

// Scenario: 显式路由保持最高优先级
func TestCompositeRouteResolverExplicitRouteBeatsAccountOwnership(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{{
			ID:             10,
			GroupID:        7,
			PublicModel:    "reasoning-alias",
			MatchType:      CompositeRouteMatchExact,
			TargetPlatform: PlatformOpenAI,
			UpstreamModel:  "gpt-5",
			Endpoint:       CompositeRouteEndpointAny,
			Enabled:        true,
		}},
	})
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (CompositeModelOwnership, error) {
		return CompositeModelOwnership{TargetPlatform: PlatformDeepseek, Matched: true}, nil
	})

	decision, err := resolver.Resolve(context.Background(), 7, "reasoning-alias", CompositeRouteEndpointChatCompletions)

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
	require.Equal(t, "gpt-5", decision.UpstreamModel)
}

// Scenario: 跨平台同名别名不被猜测
func TestCompositeRouteResolverDoesNotGuessAmbiguousAccountOwnership(t *testing.T) {
	resolver := NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (CompositeModelOwnership, error) {
		return CompositeModelOwnership{Ambiguous: true}, nil
	})

	decision, err := resolver.Resolve(context.Background(), 7, "shared-alias", CompositeRouteEndpointChatCompletions)

	require.NoError(t, err)
	require.False(t, decision.Matched)
	require.Empty(t, decision.TargetPlatform)
	require.Equal(t, "model is exposed by multiple provider platforms", decision.Reason)
}

func TestCompositeRouteResolverOwnershipLookupErrorFallsBackOnlyForDetectableModels(t *testing.T) {
	lookupErr := errors.New("account catalog unavailable")
	resolver := NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (CompositeModelOwnership, error) {
		return CompositeModelOwnership{}, lookupErr
	})

	detected, err := resolver.Resolve(context.Background(), 7, "gpt-5", CompositeRouteEndpointResponses)
	require.NoError(t, err)
	require.True(t, detected.Matched)
	require.Equal(t, CompositeRouteSourceDetector, detected.Source)
	require.Equal(t, PlatformOpenAI, detected.TargetPlatform)

	unknown, err := resolver.Resolve(context.Background(), 7, "company-model", CompositeRouteEndpointResponses)
	require.ErrorIs(t, err, lookupErr)
	require.False(t, unknown.Matched)
}

func TestCompositeRouteResolverPrefersEndpointSpecificLongestPrefix(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             1,
				GroupID:        7,
				PublicModel:    "router/",
				MatchType:      CompositeRouteMatchPrefix,
				TargetPlatform: PlatformAnthropic,
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       10,
				Enabled:        true,
			},
			{
				ID:             2,
				GroupID:        7,
				PublicModel:    "router/gpt-",
				MatchType:      CompositeRouteMatchPrefix,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  "gpt-family",
				Endpoint:       CompositeRouteEndpointResponses,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	decision, err := resolver.Resolve(context.Background(), 7, "router/gpt-5", CompositeRouteEndpointResponses)

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
	require.Equal(t, "gpt-family", decision.UpstreamModel)
	require.NotNil(t, decision.Route)
	require.Equal(t, int64(2), decision.Route.ID)
}

// TestCompositeRouteResolverPrefixEmptyUpstreamPassesThroughRequestedModel 验证：
// 前缀匹配路由留空 upstream_model 时，转发的是具体请求模型（各自原样），而不是
// 塌缩成 public_model。这是「留空 = 透传原始模型」语义的核心场景。
func TestCompositeRouteResolverPrefixEmptyUpstreamPassesThroughRequestedModel(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             1,
				GroupID:        7,
				PublicModel:    "deepseek-v4",
				MatchType:      CompositeRouteMatchPrefix,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  "", // 留空 = 透传
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	for _, model := range []string{"deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4"} {
		decision, err := resolver.Resolve(context.Background(), 7, model, CompositeRouteEndpointChatCompletions)
		require.NoError(t, err)
		require.True(t, decision.Matched, "model %q should match prefix route", model)
		require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
		require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
		require.Equal(t, model, decision.UpstreamModel, "model %q should pass through verbatim", model)
	}
}

// TestCompositeRouteResolverPrefixExplicitUpstreamStillFixed 验证：前缀匹配路由显式
// 填写 upstream_model 时，所有命中请求仍转发同一个固定上游模型（行为不变）。
func TestCompositeRouteResolverPrefixExplicitUpstreamStillFixed(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             1,
				GroupID:        7,
				PublicModel:    "deepseek-v4",
				MatchType:      CompositeRouteMatchPrefix,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  "deepseek-chat",
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	for _, model := range []string{"deepseek-v4-flash", "deepseek-v4-pro"} {
		decision, err := resolver.Resolve(context.Background(), 7, model, CompositeRouteEndpointChatCompletions)
		require.NoError(t, err)
		require.True(t, decision.Matched)
		require.Equal(t, "deepseek-chat", decision.UpstreamModel)
	}
}

func TestCompositeRouteResolverIgnoresDisabledRoutesAndFallsBackToDetector(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             1,
				GroupID:        7,
				PublicModel:    "gpt-5",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformAnthropic,
				UpstreamModel:  "claude-sonnet-4-6",
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        false,
			},
		},
	})

	decision, err := resolver.Resolve(context.Background(), 7, "gpt-5", CompositeRouteEndpointAny)

	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceDetector, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)
	require.Equal(t, "gpt-5", decision.UpstreamModel)
	require.Nil(t, decision.Route)
}

func TestCompositeRouteResolverDetectsKimiCodeBareModels(t *testing.T) {
	resolver := NewCompositeRouteResolver(nil)

	for _, model := range []string{"k3", "k3-256k", "kimi-code/k3"} {
		t.Run(model, func(t *testing.T) {
			decision, err := resolver.Resolve(context.Background(), 7, model, CompositeRouteEndpointMessages)

			require.NoError(t, err)
			require.True(t, decision.Matched)
			require.Equal(t, CompositeRouteSourceDetector, decision.Source)
			require.Equal(t, PlatformKimi, decision.TargetPlatform)
			require.Equal(t, model, decision.UpstreamModel)
		})
	}
}

func TestCompositeRouteResolverExplicitRoutesCoverBucketTwoProviders(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             1,
				GroupID:        7,
				PublicModel:    "all/gpt-5",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformOpenAI,
				UpstreamModel:  "gpt-5",
				Endpoint:       CompositeRouteEndpointResponses,
				Priority:       100,
				Enabled:        true,
			},
			{
				ID:             2,
				GroupID:        7,
				PublicModel:    "all/claude-sonnet",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformAnthropic,
				UpstreamModel:  "claude-sonnet-4-6",
				Endpoint:       CompositeRouteEndpointMessages,
				Priority:       100,
				Enabled:        true,
			},
			{
				ID:             3,
				GroupID:        7,
				PublicModel:    "all/gemini-pro",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformGemini,
				UpstreamModel:  "gemini-2.5-pro",
				Endpoint:       CompositeRouteEndpointGemini,
				Priority:       100,
				Enabled:        true,
			},
			{
				ID:             4,
				GroupID:        7,
				PublicModel:    "all/grok",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformGrok,
				UpstreamModel:  "grok-4.3",
				Endpoint:       CompositeRouteEndpointResponses,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	tests := []struct {
		model        string
		endpoint     string
		wantPlatform string
		wantUpstream string
	}{
		{"all/gpt-5", CompositeRouteEndpointResponses, PlatformOpenAI, "gpt-5"},
		{"all/claude-sonnet", CompositeRouteEndpointMessages, PlatformAnthropic, "claude-sonnet-4-6"},
		{"all/gemini-pro", CompositeRouteEndpointGemini, PlatformGemini, "gemini-2.5-pro"},
		{"all/grok", CompositeRouteEndpointResponses, PlatformGrok, "grok-4.3"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			decision, err := resolver.Resolve(context.Background(), 7, tt.model, tt.endpoint)

			require.NoError(t, err)
			require.True(t, decision.Matched)
			require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
			require.Equal(t, tt.wantPlatform, decision.TargetPlatform)
			require.Equal(t, tt.wantUpstream, decision.UpstreamModel)
		})
	}
}

// Scenario: Gemini/Adobe 同名生图模型按入口定平台，gemini/any 入口交给账号归属
func TestCompositeRouteResolverSharedGeminiImageUsesEndpoint(t *testing.T) {
	models := []string{
		"gemini-2.5-flash-image", "gemini-2.5-flash-image-preview",
		"gemini-3-pro-image", "gemini-3-pro-image-preview",
		"gemini-3.1-flash-image", "gemini-3.1-flash-image-preview",
	}
	cases := []struct {
		endpoint string
		platform string
		matched  bool
	}{
		{CompositeRouteEndpointImages, PlatformAdobe, true},
		{CompositeRouteEndpointChatCompletions, PlatformGemini, true},
		{CompositeRouteEndpointResponses, PlatformGemini, true},
		{CompositeRouteEndpointMessages, PlatformGemini, true},
		{CompositeRouteEndpointCountTokens, PlatformGemini, true},
		{CompositeRouteEndpointEmbeddings, PlatformGemini, true},
		{CompositeRouteEndpointGemini, "", false},
		{CompositeRouteEndpointAny, "", false},
	}
	resolver := NewCompositeRouteResolver(nil)
	for _, model := range models {
		for _, tc := range cases {
			decision, err := resolver.Resolve(context.Background(), 7, model, tc.endpoint)
			require.NoError(t, err, "%s@%s", model, tc.endpoint)
			require.Equal(t, tc.matched, decision.Matched, "%s@%s", model, tc.endpoint)
			require.Equal(t, tc.platform, decision.TargetPlatform, "%s@%s", model, tc.endpoint)
			if tc.matched {
				require.Equal(t, CompositeRouteSourceDetector, decision.Source)
				require.Equal(t, model, decision.UpstreamModel)
			}
		}
	}
}

func TestCompositeRouteResolverSharedGeminiImageExplicitRouteWins(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{{
			ID:             11,
			GroupID:        7,
			PublicModel:    "gemini-3-pro-image",
			MatchType:      CompositeRouteMatchExact,
			TargetPlatform: PlatformAdobe,
			UpstreamModel:  "gemini-3-pro-image",
			Endpoint:       CompositeRouteEndpointAny,
			Enabled:        true,
		}},
	})

	decision, err := resolver.Resolve(context.Background(), 7, "gemini-3-pro-image", CompositeRouteEndpointGemini)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
	require.Equal(t, PlatformAdobe, decision.TargetPlatform)
}

func TestCompositeRouteResolverSharedGeminiImageUsesOwnershipOnGeminiEndpoint(t *testing.T) {
	resolver := NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (CompositeModelOwnership, error) {
		return CompositeModelOwnership{TargetPlatform: PlatformAdobe, Matched: true}, nil
	})

	decision, err := resolver.Resolve(context.Background(), 7, "gemini-3-pro-image", CompositeRouteEndpointGemini)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceAccount, decision.Source)
	require.Equal(t, PlatformAdobe, decision.TargetPlatform)
}

// 同名模型查归属出错不能 500：/v1beta 由中间件兜底到 Gemini。
func TestCompositeRouteResolverSharedGeminiImageOwnershipErrorDoesNotFail(t *testing.T) {
	resolver := NewCompositeRouteResolver(nil)
	resolver.SetModelOwnershipResolver(func(context.Context, int64, string) (CompositeModelOwnership, error) {
		return CompositeModelOwnership{}, errors.New("account catalog unavailable")
	})

	decision, err := resolver.Resolve(context.Background(), 7, "gemini-3-pro-image", CompositeRouteEndpointGemini)
	require.NoError(t, err)
	require.False(t, decision.Matched)
}
