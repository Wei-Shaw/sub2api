package service

import (
	"context"
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

func endpointDefaultOptions() CompositeRouteResolveOptions {
	return CompositeRouteResolveOptions{EndpointDefaultRoutingEnabled: true}
}

// TestCompositeRouteResolverEndpointDefaultsPerEndpoint pins the built-in
// default provider for every endpoint that participates in endpoint-default
// routing. The model name here is deliberately undetectable so only the
// endpoint default can produce a match.
func TestCompositeRouteResolverEndpointDefaultsPerEndpoint(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{})

	tests := []struct {
		endpoint string
		want     string
	}{
		{CompositeRouteEndpointMessages, PlatformAnthropic},
		{CompositeRouteEndpointCountTokens, PlatformAnthropic},
		{CompositeRouteEndpointResponses, PlatformOpenAI},
		{CompositeRouteEndpointChatCompletions, PlatformOpenAI},
		{CompositeRouteEndpointEmbeddings, PlatformOpenAI},
		{CompositeRouteEndpointGemini, PlatformGemini},
	}

	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			decision, err := resolver.ResolveWithOptions(context.Background(), 7, "house-blend-1", tt.endpoint, endpointDefaultOptions())
			require.NoError(t, err)
			require.True(t, decision.Matched)
			require.Equal(t, CompositeRouteSourceEndpointDefault, decision.Source)
			require.Equal(t, tt.want, decision.TargetPlatform)
			require.Equal(t, "house-blend-1", decision.UpstreamModel)
		})
	}
}

// TestCompositeRouteResolverImagesEndpointHasNoDefault documents the images
// boundary: image models are cross-provider, so the images endpoint keeps
// falling through to the detector even with the flag on.
func TestCompositeRouteResolverImagesEndpointHasNoDefault(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{})

	detected, err := resolver.ResolveWithOptions(context.Background(), 7, "gpt-image-1", CompositeRouteEndpointImages, endpointDefaultOptions())
	require.NoError(t, err)
	require.True(t, detected.Matched)
	require.Equal(t, CompositeRouteSourceDetector, detected.Source)
	require.Equal(t, PlatformOpenAI, detected.TargetPlatform)

	undetected, err := resolver.ResolveWithOptions(context.Background(), 7, "house-blend-1", CompositeRouteEndpointImages, endpointDefaultOptions())
	require.NoError(t, err)
	require.False(t, undetected.Matched)
}

// TestCompositeRouteResolverAnyRuleOutranksEndpointDefault is the collision
// case: an operator-configured `any` rule must win over the built-in endpoint
// default, otherwise enabling the flag would silently retarget configured
// traffic.
func TestCompositeRouteResolverAnyRuleOutranksEndpointDefault(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             40,
				GroupID:        7,
				PublicModel:    "claude-",
				MatchType:      CompositeRouteMatchPrefix,
				TargetPlatform: PlatformOpenAI,
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	decision, err := resolver.ResolveWithOptions(context.Background(), 7, "claude-sonnet-4-5", CompositeRouteEndpointMessages, endpointDefaultOptions())
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform, "explicit any rule must outrank the messages endpoint default")
}

// TestCompositeRouteResolverExplicitComparatorUnchangedByEndpointDefault pins
// the pre-existing explicit-rule ranking: match strength is compared before
// endpoint specificity, so an `any`-scope exact rule beats a concrete-endpoint
// prefix rule. Endpoint-default routing must not alter this.
func TestCompositeRouteResolverExplicitComparatorUnchangedByEndpointDefault(t *testing.T) {
	repo := compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             50,
				GroupID:        7,
				PublicModel:    "claude-sonnet-4-5",
				MatchType:      CompositeRouteMatchExact,
				TargetPlatform: PlatformGemini,
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        true,
			},
			{
				ID:             51,
				GroupID:        7,
				PublicModel:    "claude-",
				MatchType:      CompositeRouteMatchPrefix,
				TargetPlatform: PlatformOpenAI,
				Endpoint:       CompositeRouteEndpointMessages,
				Priority:       100,
				Enabled:        true,
			},
		},
	}
	resolver := NewCompositeRouteResolver(repo)

	for _, options := range []CompositeRouteResolveOptions{{}, endpointDefaultOptions()} {
		decision, err := resolver.ResolveWithOptions(context.Background(), 7, "claude-sonnet-4-5", CompositeRouteEndpointMessages, options)
		require.NoError(t, err)
		require.True(t, decision.Matched)
		require.Equal(t, CompositeRouteSourceExplicit, decision.Source)
		require.Equal(t, PlatformGemini, decision.TargetPlatform, "exact match must still outrank concrete-endpoint prefix match")
	}
}

// TestCompositeRouteResolverFlagOffMatchesLegacyBehavior asserts the opt-in
// promise: with the flag off, resolution is identical to Resolve.
func TestCompositeRouteResolverFlagOffMatchesLegacyBehavior(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{})

	legacy, err := resolver.Resolve(context.Background(), 7, "house-blend-1", CompositeRouteEndpointMessages)
	require.NoError(t, err)
	require.False(t, legacy.Matched)

	flagOff, err := resolver.ResolveWithOptions(context.Background(), 7, "house-blend-1", CompositeRouteEndpointMessages, CompositeRouteResolveOptions{})
	require.NoError(t, err)
	require.Equal(t, legacy, flagOff)

	detected, err := resolver.ResolveWithOptions(context.Background(), 7, "claude-sonnet-4-5", CompositeRouteEndpointResponses, CompositeRouteResolveOptions{})
	require.NoError(t, err)
	require.True(t, detected.Matched)
	require.Equal(t, CompositeRouteSourceDetector, detected.Source)
	require.Equal(t, PlatformAnthropic, detected.TargetPlatform)
}

// TestCompositeRouteResolverDetectorOutranksEndpointDefault is the
// no-configuration promise: a gpt-* model arriving on /v1/messages must still
// resolve to OpenAI by model name, not be pulled to the messages endpoint
// default. The endpoint default is a last resort for names the detector cannot
// identify.
func TestCompositeRouteResolverDetectorOutranksEndpointDefault(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{})

	decision, err := resolver.ResolveWithOptions(context.Background(), 7, "gpt-5.6-sol", CompositeRouteEndpointMessages, endpointDefaultOptions())
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceDetector, decision.Source)
	require.Equal(t, PlatformOpenAI, decision.TargetPlatform)

	claude, err := resolver.ResolveWithOptions(context.Background(), 7, "claude-opus-5", CompositeRouteEndpointChatCompletions, endpointDefaultOptions())
	require.NoError(t, err)
	require.Equal(t, CompositeRouteSourceDetector, claude.Source)
	require.Equal(t, PlatformAnthropic, claude.TargetPlatform)
}

// TestCompositeRouteResolverEndpointDefaultIsPurelyAdditive asserts the flag
// only ever rescues a request that previously had no route at all.
func TestCompositeRouteResolverEndpointDefaultIsPurelyAdditive(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{
		routes: []CompositeModelRoute{
			{
				ID:             60,
				GroupID:        7,
				PublicModel:    "house-blend-",
				MatchType:      CompositeRouteMatchPrefix,
				TargetPlatform: PlatformGemini,
				Endpoint:       CompositeRouteEndpointAny,
				Priority:       100,
				Enabled:        true,
			},
		},
	})

	for _, model := range []string{"house-blend-1", "gpt-5.6-sol", "claude-opus-5"} {
		off, err := resolver.ResolveWithOptions(context.Background(), 7, model, CompositeRouteEndpointMessages, CompositeRouteResolveOptions{})
		require.NoError(t, err)
		require.True(t, off.Matched, model)

		on, err := resolver.ResolveWithOptions(context.Background(), 7, model, CompositeRouteEndpointMessages, endpointDefaultOptions())
		require.NoError(t, err)
		require.Equal(t, off, on, "flag must not change an already-resolvable request: %s", model)
	}

	unresolvable, err := resolver.ResolveWithOptions(context.Background(), 7, "mystery-model-1", CompositeRouteEndpointMessages, CompositeRouteResolveOptions{})
	require.NoError(t, err)
	require.False(t, unresolvable.Matched)

	rescued, err := resolver.ResolveWithOptions(context.Background(), 7, "mystery-model-1", CompositeRouteEndpointMessages, endpointDefaultOptions())
	require.NoError(t, err)
	require.True(t, rescued.Matched)
	require.Equal(t, CompositeRouteSourceEndpointDefault, rescued.Source)
	require.Equal(t, PlatformAnthropic, rescued.TargetPlatform)
}

// TestCompositeRouteResolverUnknownEndpointFallsThroughToDetector guards the
// `any` normalization path: an unidentified endpoint has no default.
func TestCompositeRouteResolverUnknownEndpointFallsThroughToDetector(t *testing.T) {
	resolver := NewCompositeRouteResolver(compositeRouteRepoStub{})

	decision, err := resolver.ResolveWithOptions(context.Background(), 7, "claude-sonnet-4-5", "totally-unknown", endpointDefaultOptions())
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.Equal(t, CompositeRouteSourceDetector, decision.Source)
	require.Equal(t, PlatformAnthropic, decision.TargetPlatform)
	require.Equal(t, CompositeRouteEndpointAny, decision.Endpoint)
}
