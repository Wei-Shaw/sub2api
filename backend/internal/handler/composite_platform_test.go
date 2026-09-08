package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type responsesWSCompositeRouteRepoStub struct {
	service.CompositeModelRouteRepository
	routes []service.CompositeModelRoute
}

func (s *responsesWSCompositeRouteRepoStub) ListByGroup(context.Context, int64, bool) ([]service.CompositeModelRoute, error) {
	return append([]service.CompositeModelRoute(nil), s.routes...), nil
}

func TestResolveResponsesWebSocketTargetUsesExplicitCompositeRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/backend-api/codex/responses", nil)
	apiKey := &service.APIKey{Group: &service.Group{ID: 42, Platform: service.PlatformComposite}}
	resolver := service.NewCompositeRouteResolver(&responsesWSCompositeRouteRepoStub{routes: []service.CompositeModelRoute{{
		ID:             1,
		GroupID:        42,
		PublicModel:    "company-coding-model",
		MatchType:      service.CompositeRouteMatchExact,
		TargetPlatform: service.PlatformDeepSeek,
		UpstreamModel:  "deepseek-v4-pro",
		Endpoint:       service.CompositeRouteEndpointResponses,
		Enabled:        true,
	}}})
	AttachResponsesWebSocketCompositeResolver(c, resolver)

	decision, platform, err := resolveResponsesWebSocketTarget(c, apiKey, c.Request.Context(), "company-coding-model")

	require.NoError(t, err)
	require.Equal(t, service.PlatformDeepSeek, platform)
	require.Equal(t, "deepseek-v4-pro", decision.UpstreamModel)
	resolved, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	require.True(t, ok)
	require.Equal(t, service.PlatformDeepSeek, resolved)
}

func TestCompositeTargetPlatformAllowedResolvesKnownAllowedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/embeddings", nil)
	apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}

	require.True(t, compositeTargetPlatformAllowed(c, apiKey, "text-embedding-3-large", service.PlatformOpenAI))
	platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	require.True(t, ok)
	require.Equal(t, service.PlatformOpenAI, platform)
}

func TestEnsureCompositeTargetPlatformBindsInboundEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}

	for _, tc := range []struct {
		path     string
		model    string
		endpoint string
	}{
		{path: "/v1/chat/completions", model: "gpt-deepseek-alias", endpoint: service.CompositeRouteEndpointChatCompletions},
		{path: "/v1/responses", model: "grok-deepseek-alias", endpoint: service.CompositeRouteEndpointResponses},
		{path: "/v1/messages", model: "claude-deepseek-alias", endpoint: service.CompositeRouteEndpointMessages},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", tc.path, nil)

			ensureCompositeTargetPlatform(c, apiKey, tc.model)

			endpoint, ok := service.CompositeRouteEndpointFromContext(c.Request.Context())
			require.True(t, ok)
			require.Equal(t, tc.endpoint, endpoint)
			_, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context())
			require.True(t, resolved, "the handler detector should seed a fallback platform")
		})
	}
}

func TestOpenAICompatibleTextTargetAllowsCompositeProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	providers := []struct {
		model    string
		platform string
	}{
		{model: "grok-4.3", platform: service.PlatformGrok},
		{model: "kimi-k2-thinking", platform: service.PlatformKimi},
		{model: "k3", platform: service.PlatformKimi},
		{model: "glm-5.2", platform: service.PlatformZhipu},
		{model: "deepseek-v3.2", platform: service.PlatformDeepseek},
		{model: "MiniMax-M3", platform: service.PlatformMiniMax},
	}
	for _, path := range []string{"/v1/messages", "/v1/chat/completions", "/v1/responses", "/v1/responses/input_tokens", "/v1/messages/count_tokens"} {
		for _, provider := range providers {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", path, nil)
			apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}

			require.True(t, openAICompatibleTextTargetAllowed(c, apiKey, provider.model), "path=%s model=%s", path, provider.model)
			platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
			require.True(t, ok, "path=%s model=%s", path, provider.model)
			require.Equal(t, provider.platform, platform, "path=%s model=%s", path, provider.model)
		}
	}
}

// DeepSeek 有专用 WS 处理器；Kimi/Zhipu 没有对应 transport/HTTP bridge。
func TestResponsesWebSocketCompositePlatformGuardAllowsSupportedTargets(t *testing.T) {
	require.True(t, isResponsesWebSocketCompositePlatform(service.PlatformOpenAI))
	require.True(t, isResponsesWebSocketCompositePlatform(service.PlatformGrok))
	require.True(t, isResponsesWebSocketCompositePlatform(service.PlatformDeepSeek))
	for _, platform := range []string{
		service.PlatformKimi, service.PlatformZhipu, service.PlatformMiniMax,
		service.PlatformAnthropic, service.PlatformGemini,
	} {
		require.False(t, isResponsesWebSocketCompositePlatform(platform), "platform=%s", platform)
	}
}

func TestAllowOpenAICompatibleMessagesDispatchUsesCompositeTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	apiKey := &service.APIKey{Group: &service.Group{
		Platform:              service.PlatformComposite,
		AllowMessagesDispatch: false,
	}}

	for _, tc := range []struct {
		name     string
		platform string
		resolved bool
		want     bool
	}{
		{name: "openai target", platform: service.PlatformOpenAI, resolved: true, want: true},
		{name: "grok target", platform: service.PlatformGrok, resolved: true, want: true},
		{name: "anthropic target", platform: service.PlatformAnthropic, resolved: true, want: false},
		{name: "deepseek target", platform: service.PlatformDeepSeek, resolved: true, want: true},
		{name: "unresolved target", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			if tc.resolved {
				c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), tc.platform))
			}

			require.Equal(t, tc.want, allowOpenAICompatibleMessagesDispatch(c, apiKey))
		})
	}
}

func TestOpenAICompatibleRequestPlatformPreservesDeepSeek(t *testing.T) {
	apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformDeepSeek}}
	require.Equal(t, service.PlatformDeepSeek, openAICompatibleRequestPlatform(context.Background(), apiKey))

	ctx := service.WithResolvedTargetPlatform(context.Background(), service.PlatformDeepSeek)
	compositeKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}
	require.Equal(t, service.PlatformDeepSeek, openAICompatibleRequestPlatform(ctx, compositeKey))
}

func TestCompositeTargetPlatformAllowedRejectsWrongOrUnknownModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name  string
		model string
	}{
		{name: "wrong provider", model: "claude-sonnet-4-5"},
		{name: "unknown provider", model: "llama-4-maverick"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/embeddings", nil)
			apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}

			require.False(t, compositeTargetPlatformAllowed(c, apiKey, tc.model, service.PlatformOpenAI))
		})
	}
}

func TestCompositeTargetPlatformResolvedRejectsUnknownModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}

	require.False(t, compositeTargetPlatformResolved(c, apiKey, "llama-4-maverick"))
	_, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	require.False(t, ok)
}

func TestCompositeTargetPlatformResolvedAllowsConcreteGroupWithoutResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	apiKey := &service.APIKey{Group: &service.Group{Platform: service.PlatformAnthropic}}

	require.True(t, compositeTargetPlatformResolved(c, apiKey, "llama-4-maverick"))
}

func TestOpenAIReasoningEffortPolicyForCompositeTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{
		Platform:           service.PlatformComposite,
		MaxReasoningEffort: "medium",
		ReasoningEffortMappings: []service.ReasoningEffortMapping{
			{From: "max", To: "xhigh"},
		},
	}
	apiKey := &service.APIKey{Group: group}
	body := []byte(`{"reasoning":{"effort":"max"}}`)

	openAICtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	openAICtx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	openAICtx.Request = openAICtx.Request.WithContext(service.WithResolvedTargetPlatform(openAICtx.Request.Context(), service.PlatformOpenAI))
	got, changed, err := applyOpenAIReasoningEffortPolicyForRequest(openAICtx, apiKey, body)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"reasoning":{"effort":"medium"}}`, string(got))
	requested := service.RequestedReasoningEffortFromContext(openAICtx.Request.Context())
	require.NotNil(t, requested)
	require.Equal(t, "max", *requested)

	bindOpenAIReasoningEffortPolicyForMessagesRequest(openAICtx, apiKey, []byte(`{"output_config":{"effort":"max"}}`))
	bound, changed, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(openAICtx.Request.Context(), body)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"reasoning":{"effort":"medium"}}`, string(bound))

	omittedCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	omittedCtx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	omittedCtx.Request = omittedCtx.Request.WithContext(service.WithResolvedTargetPlatform(omittedCtx.Request.Context(), service.PlatformOpenAI))
	bindOpenAIReasoningEffortPolicyForMessagesRequest(omittedCtx, apiKey, []byte(`{"model":"gpt-5"}`))
	omitted, changed, err := service.ApplyOpenAIReasoningEffortPolicyFromContext(omittedCtx.Request.Context(), body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, omitted)

	denyGroup := *group
	denyGroup.MaxReasoningEffortOverLimit = service.ReasoningEffortOverLimitDeny
	denyAPIKey := &service.APIKey{Group: &denyGroup}
	denyCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	denyCtx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	denyCtx.Request = denyCtx.Request.WithContext(service.WithResolvedTargetPlatform(denyCtx.Request.Context(), service.PlatformOpenAI))
	_, _, err = applyOpenAIReasoningEffortPolicyForRequest(denyCtx, denyAPIKey, body)
	require.Error(t, err)
	var overLimit *service.ReasoningEffortOverLimitError
	require.ErrorAs(t, err, &overLimit)

	mappingDenyGroup := *group
	mappingDenyGroup.MaxReasoningEffort = ""
	mappingDenyGroup.ReasoningEffortMappings = []service.ReasoningEffortMapping{
		{From: "max", To: service.ReasoningEffortMappingDeny},
	}
	mappingDenyAPIKey := &service.APIKey{Group: &mappingDenyGroup}
	mappingDenyCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	mappingDenyCtx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	mappingDenyCtx.Request = mappingDenyCtx.Request.WithContext(service.WithResolvedTargetPlatform(mappingDenyCtx.Request.Context(), service.PlatformOpenAI))
	_, changed, err = applyOpenAIReasoningEffortPolicyForRequest(mappingDenyCtx, mappingDenyAPIKey, body)
	require.Error(t, err)
	require.False(t, changed)
	var mappingDenied *service.ReasoningEffortMappingDeniedError
	require.ErrorAs(t, err, &mappingDenied)
	require.Equal(t, "max", mappingDenied.Requested)
	require.Contains(t, mappingDenied.Error(), "denied by this group's mapping policy")

	grokCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	grokCtx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	grokCtx.Request = grokCtx.Request.WithContext(service.WithResolvedTargetPlatform(grokCtx.Request.Context(), service.PlatformGrok))
	got, changed, err = applyOpenAIReasoningEffortPolicyForRequest(grokCtx, apiKey, body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, got)
}

func TestOpenAIShapedReasoningEffortPolicyForAnthropicGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	apiKey := &service.APIKey{Group: &service.Group{
		Platform:           service.PlatformAnthropic,
		MaxReasoningEffort: "xhigh",
	}}

	got, changed, err := applyOpenAIReasoningEffortPolicyForRequest(c, apiKey, []byte(`{"reasoning":{"effort":"max"}}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "xhigh", gjson.GetBytes(got, "reasoning.effort").String())
}

func TestAnthropicReasoningEffortPolicyForConcreteAndCompositeTargets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"claude-fable-5-1","output_config":{"effort":"max"}}`)
	policy := func(platform string) *service.APIKey {
		return &service.APIKey{Group: &service.Group{
			Platform:           platform,
			MaxReasoningEffort: "xhigh",
			ReasoningEffortMappings: []service.ReasoningEffortMapping{
				{From: "max", To: "xhigh"},
			},
		}}
	}

	concrete, _ := gin.CreateTestContext(httptest.NewRecorder())
	concrete.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	got, changed, err := applyAnthropicReasoningEffortPolicyForRequest(concrete, policy(service.PlatformAnthropic), body)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"model":"claude-fable-5-1","output_config":{"effort":"xhigh"}}`, string(got))

	denyMax := policy(service.PlatformAnthropic)
	denyMax.Group.ReasoningEffortMappings = nil
	denyMax.Group.MaxReasoningEffortOverLimit = service.ReasoningEffortOverLimitDeny
	_, changed, err = applyAnthropicReasoningEffortPolicyForRequest(concrete, denyMax, body)
	require.Error(t, err)
	require.False(t, changed)
	var overLimit *service.ReasoningEffortOverLimitError
	require.ErrorAs(t, err, &overLimit)
	require.Equal(t, "max", overLimit.Requested)
	require.Equal(t, "xhigh", overLimit.Max)

	composite, _ := gin.CreateTestContext(httptest.NewRecorder())
	composite.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	composite.Request = composite.Request.WithContext(service.WithResolvedTargetPlatform(composite.Request.Context(), service.PlatformAnthropic))
	got, changed, err = applyAnthropicReasoningEffortPolicyForRequest(composite, policy(service.PlatformComposite), body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "xhigh", gjson.GetBytes(got, "output_config.effort").String())

	openAI, _ := gin.CreateTestContext(httptest.NewRecorder())
	openAI.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	openAI.Request = openAI.Request.WithContext(service.WithResolvedTargetPlatform(openAI.Request.Context(), service.PlatformOpenAI))
	got, changed, err = applyAnthropicReasoningEffortPolicyForRequest(openAI, policy(service.PlatformComposite), body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, got)
}

func TestAnthropicCompositePolicyClampsUnsupportedMinimalToLow(t *testing.T) {
	maxEffort, mappings := anthropicCompatibleReasoningEffortPolicy("minimal", []service.ReasoningEffortMapping{{From: "max", To: "minimal"}})
	require.Equal(t, "low", maxEffort)
	require.Equal(t, []service.ReasoningEffortMapping{{From: "max", To: "low"}}, mappings)
}

func TestClientRequestedModelUsesCompositePublicModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), service.CompositeRouteDecision{
		Matched:        true,
		Source:         service.CompositeRouteSourceExplicit,
		PublicModel:    "public-alias",
		TargetPlatform: service.PlatformOpenAI,
		UpstreamModel:  "gpt-5",
	}))

	input := buildContentModerationInput(c, nil, middleware2.AuthSubject{UserID: 42}, service.ContentModerationProtocolOpenAIChat, "gpt-5", nil)
	require.Equal(t, "public-alias", input.Model)
	require.Equal(t, service.PlatformOpenAI, input.Provider)

	fields := clientRequestedUsageFields(c, service.ChannelMappingResult{MappedModel: "gpt-5"}, "gpt-5", "gpt-5")
	require.Equal(t, "public-alias", fields.OriginalModel)
	require.Equal(t, "public-alias", fields.ChannelMappedModel)
	require.Equal(t, "public-alias\u2192gpt-5", fields.ModelMappingChain)
}
