//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGPT62IdentityEffortAndSampling(t *testing.T) {
	for _, base := range []string{"gpt-6.2-sol", "gpt-6.2-astra"} {
		require.Contains(t, openai.DefaultModelIDs(), base)
		for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "openai-compact"} {
			model := "openai/" + strings.ToUpper(base) + "-" + effort
			require.Equal(t, base, normalizeKnownOpenAICodexModel(model))
			require.Equal(t, base, normalizeCodexModel(model))
			require.Equal(t, base, NormalizeOpenAICompatRequestedModel(model))
			require.True(t, shouldAutoInjectPromptCacheKeyForCompat(model))
			require.True(t, openai.RequiresResponsesForToolCalls(model))
			require.False(t, openai.IsGPT61SolModelSpelling(model))
			require.False(t, isOpenAIGPT6AstraModel(model))
			require.True(t, configuredCodexSupportsPriorityServiceTier(model))
		}
		for _, effort := range []string{"none", "minimal"} {
			for _, body := range []string{`{"reasoning":{"effort":"` + effort + `"}}`, `{"reasoning_effort":"` + effort + `"}`, `{"output_config":{"effort":"` + effort + `"}}`, `{"model":"` + base + `-` + effort + `"}`} {
				require.ErrorContains(t, validateGPT61CompatRequest([]byte(body), base), "does not support reasoning effort")
			}
			_, err := apicompat.ChatCompletionsToResponses(&apicompat.ChatCompletionsRequest{Model: base, ReasoningEffort: effort})
			require.Error(t, err)
			_, err = apicompat.AnthropicToResponses(&apicompat.AnthropicRequest{Model: base, OutputConfig: &apicompat.AnthropicOutputConfig{Effort: effort}})
			require.Error(t, err)
		}
		sampling := 0.7
		for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
			req := &apicompat.AnthropicRequest{Model: base + "-" + effort, Temperature: &sampling, TopP: &sampling}
			applyOpenAICompatModelNormalization(req)
			converted, err := apicompat.AnthropicToResponses(req)
			require.NoError(t, err)
			require.Equal(t, effort, converted.Reasoning.Effort)
			require.Nil(t, converted.Temperature)
			require.Nil(t, converted.TopP)
			require.NoError(t, validateGPT61CompatRequest([]byte(`{"reasoning":{"effort":"`+effort+`"}}`), base))
		}
		body, changed, err := normalizeGPT6ResponsesSampling([]byte(`{"temperature":0.7,"top_p":0.9,"logprobs":true,"top_logprobs":3,"include":["reasoning.encrypted_content","message.output_text.logprobs"]}`), base)
		require.NoError(t, err)
		require.True(t, changed)
		require.JSONEq(t, `{"include":["reasoning.encrypted_content"]}`, string(body))
		require.Equal(t, "ultrafast", openAIGroupForcedServiceTier(context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIUltrafast: true}), &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, base))
	}
	for _, model := range []string{"gpt-6.2", "gpt-6.2-foo", "gpt-6.2-sol-preview", "gpt-6.2-astra-law"} {
		require.Empty(t, normalizeKnownOpenAICodexModel(model))
		require.False(t, openAIModelSupportsUltrafast(model))
	}
}

func TestGPT62CompatHTTPRejectsDisabledReasoning(t *testing.T) {
	for _, model := range []string{"gpt-6.2-sol", "gpt-6.2-astra"} {
		for _, protocol := range []string{"chat", "responses", "messages"} {
			for _, effort := range []string{"none", "minimal"} {
				t.Run(model+protocol+effort, func(t *testing.T) {
					var body string
					switch protocol {
					case "chat":
						body = fmt.Sprintf(`{"model":"public","reasoning_effort":%q,"messages":[{"role":"user","content":"hi"}]}`, effort)
					case "responses":
						body = fmt.Sprintf(`{"model":"public","reasoning":{"effort":%q},"input":"hi"}`, effort)
					case "messages":
						body = fmt.Sprintf(`{"model":"public","output_config":{"effort":%q},"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`, effort)
					}
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+protocol, strings.NewReader(body))
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"public": model}}}
					svc := &OpenAIGatewayService{cfg: &config.Config{}}
					var err error
					switch protocol {
					case "chat":
						_, err = svc.forwardAsRawChatCompletions(context.Background(), c, account, []byte(body), "")
					case "responses":
						_, err = svc.forwardResponsesViaRawChatCompletions(context.Background(), c, account, []byte(body), time.Now())
					case "messages":
						_, err = svc.forwardAnthropicViaRawChatCompletions(context.Background(), c, account, []byte(body), "")
					}
					require.ErrorContains(t, err, "does not support reasoning effort")
					require.Equal(t, http.StatusBadRequest, recorder.Code)
				})
			}
		}
	}
}

func TestGPT62PricingAtRequestTime(t *testing.T) {
	before := time.Date(2027, 3, 1, 7, 59, 59, 0, time.UTC)
	after := before.Add(time.Second)
	for _, model := range []string{"gpt-6.2-sol", "gpt-6.2-astra"} {
		unit := 2e-6
		if model == "gpt-6.2-astra" {
			unit = 8e-6
		}
		for name, svc := range map[string]*BillingService{"fallback": newTestBillingService(), "catalog fallback": NewBillingService(nil, newStubPricingServiceFromJSON(t, `{"other":{"input_cost_per_token":1}}`))} {
			t.Run(model+name, func(t *testing.T) {
				for _, resolver := range []*ModelPricingResolver{nil, NewModelPricingResolver(nil, svc)} {
					enabled := true
					input := CostInput{Model: model, Tokens: UsageTokens{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 40, CacheReadTokens: 20}, RateMultiplier: 1, PricingAt: before, Resolver: resolver, LongContextBillingEnabled: &enabled}
					for _, tc := range []struct {
						tier  string
						at    time.Time
						ratio float64
					}{{"", before, 1}, {"priority", before, 2}, {"flex", before, 0.5}, {"ultrafast", before, 3}, {"ultrafast", after, 6}, {"ultrafast", before, 3}} {
						input.ServiceTier = tc.tier
						input.PricingAt = tc.at
						cost, err := svc.CalculateCostUnified(input)
						require.NoError(t, err)
						require.InDelta(t, 100*unit*tc.ratio, cost.InputCost, 1e-12)
						require.InDelta(t, 50*unit*5*tc.ratio, cost.OutputCost, 1e-12)
						require.InDelta(t, 40*unit*1.25*tc.ratio, cost.CacheCreationCost, 1e-12)
						require.InDelta(t, 20*unit*0.05*tc.ratio, cost.CacheReadCost, 1e-12)
					}
					input.ServiceTier = ""
					input.Tokens = UsageTokens{InputTokens: 272000, OutputTokens: 50}
					cost, err := svc.CalculateCostUnified(input)
					require.NoError(t, err)
					require.InDelta(t, 272000*unit, cost.InputCost, 1e-12)
					input.Tokens.InputTokens++
					cost, err = svc.CalculateCostUnified(input)
					require.NoError(t, err)
					require.InDelta(t, 272001*unit*2, cost.InputCost, 1e-12)
					require.InDelta(t, 50*unit*5*1.5, cost.OutputCost, 1e-12)
					for _, kind := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
						input.DisableModelLongContextPricing = openAISubscriptionLongContextExempt(&Account{Platform: PlatformOpenAI, Type: kind}, &OpenAIForwardResult{Model: "public", UpstreamModel: model})
						cost, err = svc.CalculateCostUnified(input)
						require.NoError(t, err)
						factor := 2.0
						if kind == AccountTypeOAuth {
							factor = 1
						}
						require.InDelta(t, 272001*unit*factor, cost.InputCost, 1e-12)
					}
				}
				// Request-specific calculation must not mutate shared fallback pricing.
				require.Zero(t, svc.fallbackPrices[model].UltrafastMultiplier)
			})
		}
	}
	for _, model := range []string{"gpt-6-astra", "gpt-6.1-sol"} {
		require.Equal(t, 6.0, openAIUltrafastMultiplier(model, before))
		require.Equal(t, 6.0, openAIUltrafastMultiplier(model, after))
	}
}

func TestGPT62OfflineCodexCatalogAndAccountTiers(t *testing.T) {
	for _, tc := range []struct {
		model            string
		window, priority int
	}{{"gpt-6.2-sol", 500000, 1}, {"gpt-6.2-astra", 272000, 2}} {
		var raw map[string]any
		require.NoError(t, json.Unmarshal(newConfiguredCodexModelDescriptor(tc.model).officialMetadata, &raw))
		require.EqualValues(t, tc.priority, raw["priority"])
		require.EqualValues(t, tc.window, raw["context_window"])
		require.EqualValues(t, 872000, raw["max_context_window"])
		require.Equal(t, "low", raw["default_reasoning_level"])
		require.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"}, effortsFromManifestModel(t, raw))
		require.Equal(t, "xhigh", raw["multi_agent_reasoning_effort"])
		require.Equal(t, "v2", raw["multi_agent_version"])
		require.Equal(t, true, raw["use_responses_lite"])
		require.Equal(t, "code_mode_only", raw["tool_mode"])
		require.NotEmpty(t, openai.CodexBaseInstructionsForModel(tc.model))
		descriptor := newConfiguredCodexModelDescriptor(tc.model)
		require.EqualValues(t, tc.window, descriptor.ContextWindow)
		require.Equal(t, "low", *descriptor.DefaultReasoningLevel)
		for _, kind := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
			account := &Account{Platform: PlatformOpenAI, Type: kind}
			caps := accountCodexToolCapabilities(account, tc.model)
			if kind == AccountTypeAPIKey {
				require.Contains(t, string(caps["service_tiers"]), "ultrafast")
				require.Contains(t, string(caps["service_tiers"]), fmt.Sprintf("%gx", openAIUltrafastMultiplier(tc.model, time.Now())))
			} else {
				require.NotContains(t, string(caps["service_tiers"]), "ultrafast")
				group := &Group{ID: 1, Status: StatusActive, Hydrated: true, Platform: PlatformOpenAI, ForceOpenAIUltrafast: true}
				ctx := context.WithValue(context.Background(), ctxkey.Group, group)
				require.Empty(t, openAIGroupForcedServiceTier(ctx, account, tc.model))
				group.ForceOpenAIFast = true
				account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{tc.model: {ID: tc.model, CodexToolCapabilities: map[string]json.RawMessage{"service_tiers": json.RawMessage(`[{"id":"priority"}]`)}}}})
				require.Equal(t, "priority", openAIGroupForcedServiceTier(ctx, account, tc.model))
				// An account which actually advertises Ultrafast keeps that capability.
				account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{tc.model: {ID: tc.model, CodexToolCapabilities: map[string]json.RawMessage{"service_tiers": json.RawMessage(`[{"id":"ultrafast","name":"Ultrafast"}]`)}}}})
				require.Equal(t, "ultrafast", openAIGroupForcedServiceTier(ctx, account, tc.model+"-max"))
				require.Contains(t, string(accountCodexToolCapabilities(account, tc.model)["service_tiers"]), "ultrafast")
			}
		}
	}
}

func TestGPT62RecordUsageUsesRequestPricingAt(t *testing.T) {
	for _, withResolver := range []bool{false, true} {
		for _, at := range []time.Time{time.Date(2027, 3, 1, 7, 59, 59, 0, time.UTC), time.Date(2027, 3, 1, 8, 0, 0, 0, time.UTC)} {
			repo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(repo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			if withResolver {
				svc.resolver = NewModelPricingResolver(nil, svc.billingService)
			} else {
				svc.resolver = nil
			}
			tier := "ultrafast"
			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{RequestID: fmt.Sprint(at.Unix(), withResolver), Model: "gpt-6.2-sol", UpstreamModel: "gpt-6.2-sol", ServiceTier: &tier, Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 500}},
				APIKey: &APIKey{ID: 1007, GroupID: i64p(17), Group: &Group{ID: 17, RateMultiplier: 1, SubscriptionType: SubscriptionTypeSubscription}},
				User:   &User{ID: 2007}, Account: &Account{ID: 3007, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, PricingAt: at,
			})
			require.NoError(t, err)
			require.NotNil(t, repo.lastLog)
			require.InDelta(t, (1000*2e-6+500*10e-6)*openAIUltrafastMultiplier("gpt-6.2-sol", at), repo.lastLog.TotalCost, 1e-12)
			require.WithinDuration(t, time.Now(), repo.lastLog.CreatedAt, time.Minute)
		}
	}
}

func TestGPT62ResponsesToChatDropsSamplingAfterMapping(t *testing.T) {
	account := passthroughArgsFallbackAccount()
	account.Credentials["model_mapping"] = map[string]any{"public": "gpt-6.2-astra"}
	body := []byte(`{"model":"public","input":"hi","temperature":0.3,"top_p":0.8,"reasoning":{"effort":"max"}}`)
	captured := &httpUpstreamRecorder{err: errors.New("capture outbound request")}
	svc := &OpenAIGatewayService{cfg: passthroughArgsTestConfig(), httpUpstream: captured}
	_, err := svc.forwardResponsesViaRawChatCompletions(context.Background(), adaptiveProtocolTestContext("/v1/responses", body), account, body, time.Now())
	require.Error(t, err)
	var outbound map[string]any
	require.NoError(t, json.Unmarshal(captured.lastBody, &outbound))
	require.Equal(t, "gpt-6.2-astra", outbound["model"])
	require.Equal(t, "max", outbound["reasoning_effort"])
	require.NotContains(t, outbound, "temperature")
	require.NotContains(t, outbound, "top_p")
}

func TestGPT62SubscriptionUltrafastInManifestAndForcedRequests(t *testing.T) {
	const fastOnly = `[{"id":"priority","name":"Fast","description":"account Fast"}]`
	const both = `[{"id":"priority","name":"Fast"},{"id":"ultrafast","name":"Ultrafast","description":"workspace grant"}]`
	for _, tc := range []struct {
		plan, model, snapshot string
		want                  bool
	}{
		{"pro", "gpt-6.2-sol", fastOnly, true},
		{"pro", "gpt-6.2-astra", fastOnly, false},
		{"chatgpt_pro", "gpt-6.2-sol", "", true},
		{"promax", "gpt-6.2-astra", fastOnly, true},
		{"promax", "gpt-6.2-sol", "", true},
		{"prolite", "gpt-6.2-sol", fastOnly, false},
		{"enterprise", "gpt-6.2-astra", both, true},
		{"edu", "gpt-6.2-sol", fastOnly, false},
		{"pro", "gpt-6.2-sol", both, true},
	} {
		t.Run(tc.plan+"/"+tc.model+"/"+tc.snapshot, func(t *testing.T) {
			account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": tc.plan, "model_mapping": map[string]any{"public": tc.model}}}
			if tc.snapshot != "" {
				account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{tc.model: {ID: tc.model, CodexToolCapabilities: map[string]json.RawMessage{"service_tiers": json.RawMessage(tc.snapshot)}}}})
			}
			group := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, ForceOpenAIFast: true, ForceOpenAIUltrafast: true}
			gateway := &GatewayService{accountRepo: codexModelsVisibilityAccountRepo{byGroup: map[int64][]Account{1: {account}}}}
			body, err := gateway.BuildCodexModelsManifestForGroup(context.Background(), group, "", []string{"public"})
			require.NoError(t, err)
			models := decodeCodexManifestModels(t, body)
			require.Len(t, models, 1)
			tiers := models[0]["service_tiers"].([]any)
			require.Equal(t, "priority", tiers[0].(map[string]any)["id"])
			if tc.snapshot == fastOnly {
				require.Equal(t, "account Fast", tiers[0].(map[string]any)["description"])
			}
			ctx := context.WithValue(context.Background(), ctxkey.Group, group)
			forced := openAIGroupForcedServiceTier(ctx, &account, account.GetMappedModel("public")+"-max")
			if tc.want {
				require.Len(t, tiers, 2)
				require.Equal(t, "ultrafast", tiers[1].(map[string]any)["id"])
				require.Equal(t, OpenAIFastTierUltrafast, forced)
				if tc.snapshot == both {
					require.Equal(t, "workspace grant", tiers[1].(map[string]any)["description"])
				}
			} else {
				require.Len(t, tiers, 1)
				require.Equal(t, OpenAIFastTierPriority, forced)
			}
		})
	}
}
