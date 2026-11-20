//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const geminiAudioBillingModel = "gemini-3-flash-preview"

func loadGeminiAudioPricingForTest(t *testing.T, cfg *config.Config) *PricingService {
	t.Helper()
	catalog, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)
	pricing := &PricingService{cfg: cfg}
	pricing.pricingData, err = pricing.parsePricingData(catalog)
	require.NoError(t, err)
	return pricing
}

func TestGeminiNativeAudioInputBilling(t *testing.T) {
	const model = geminiAudioBillingModel
	const groupRate = 2.2
	pricing := loadGeminiAudioPricingForTest(t, nil)

	usageCases := []struct {
		name      string
		details   string
		inputCost float64
		cacheCost float64
	}{
		{
			name: "mixed",
			details: `,"promptTokensDetails":[{"modality":"TEXT","tokenCount":600},{"modality":"AUDIO","tokenCount":400}]` +
				`,"cacheTokensDetails":[{"modality":"TEXT","tokenCount":300},{"modality":"AUDIO","tokenCount":100}]`,
			inputCost: 0.000450, cacheCost: 0.000025,
		},
		{
			name: "text",
			details: `,"promptTokensDetails":[{"modality":"TEXT","tokenCount":1000}]` +
				`,"cacheTokensDetails":[{"modality":"TEXT","tokenCount":400}]`,
			inputCost: 0.000300, cacheCost: 0.000020,
		},
		{
			name:      "unknown_cached_modalities",
			details:   `,"promptTokensDetails":[{"modality":"TEXT","tokenCount":600},{"modality":"AUDIO","tokenCount":400}]`,
			inputCost: 0.000300, cacheCost: 0.000020,
		},
	}
	modes := []struct {
		name      string
		stream    bool
		bufferSSE bool
	}{
		{name: "json"},
		{name: "sse", stream: true},
		{name: "buffered_sse", bufferSSE: true},
	}

	for _, mode := range modes {
		for _, tc := range usageCases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				request := []byte(`{"contents":[{"role":"user","parts":[{"text":"Summarize the prompt"}]}]}`)
				candidate := `{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP"}]`
				usage := `,"usageMetadata":{"promptTokenCount":1000,"cachedContentTokenCount":400,"candidatesTokenCount":40,"thoughtsTokenCount":10`
				response := candidate + usage + tc.details + "}}"
				early := `{"candidates":[{"content":{"parts":[{"text":"prefix "}]}}]}`
				account := geminiSignalTestAccount()
				action := "generateContent"
				if mode.stream {
					action = "streamGenerateContent"
				}
				contentType := "application/json"
				if mode.bufferSSE {
					account.Type = AccountTypeOAuth
					account.Credentials["project_id"] = "test-project"
					account.Credentials["access_token"] = "test-token"
					early = `{"response":` + early + "}"
					response = `{"response":` + response + "}"
				}
				if mode.stream || mode.bufferSSE {
					contentType = "text/event-stream"
					response = "data: " + early + "\n\ndata: " + response + "\n\n"
				}
				upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{contentType}},
					Body:       io.NopCloser(strings.NewReader(response)),
				}}}
				forwarder := &GeminiMessagesCompatService{
					httpUpstream: upstream, cfg: &config.Config{}, tokenProvider: &GeminiTokenProvider{},
				}
				ctx := context.Background()
				c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1beta/models/"+model+":"+action, request)
				result, err := forwarder.ForwardNative(ctx, c, account, model, action, mode.stream, request)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Len(t, upstream.requestBodies, 1)

				logs := &openAIRecordUsageLogRepoStub{inserted: true}
				users := &openAIRecordUsageUserRepoStub{}
				gateway := newGatewayRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{})
				gateway.billingService = NewBillingService(&config.Config{}, pricing)
				gateway.resolver = NewModelPricingResolver(nil, gateway.billingService)
				group := &Group{ID: 77, Platform: PlatformGemini, Status: StatusActive, Hydrated: true, RateMultiplier: groupRate}
				apiKey := &APIKey{ID: 10, GroupID: &group.ID, Group: group}
				require.NoError(t, gateway.RecordUsage(ctx, &RecordUsageInput{
					Result: result, APIKey: apiKey, User: &User{ID: 20}, Account: account,
				}))

				log := logs.lastLog
				require.NotNil(t, log)
				t.Logf("input=%.9f cache=%.9f total=%.9f deducted=%.9f", log.InputCost, log.CacheReadCost, log.TotalCost, users.lastAmount)
				require.Equal(t, 600, log.InputTokens)
				require.Equal(t, 400, log.CacheReadTokens)
				require.Equal(t, 50, log.OutputTokens)
				require.InDelta(t, tc.inputCost, log.InputCost, 1e-12)
				require.InDelta(t, tc.cacheCost, log.CacheReadCost, 1e-12)
				require.InDelta(t, tc.inputCost+tc.cacheCost+0.000150, log.TotalCost, 1e-12)
				require.InDelta(t, log.TotalCost*groupRate, log.ActualCost, 1e-12)
				require.InDelta(t, log.ActualCost, users.lastAmount, 1e-12)
			})
		}
	}
}

func TestGeminiAudioUsageModalityDetails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		usage  string
		fresh  int
		cached int
	}{
		{
			name:  "uncached",
			usage: `"promptTokenCount":100,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":75},{"modality":"TEXT","tokenCount":25}]`,
			fresh: 75,
		},
		{
			name: "all_audio_cached",
			usage: `"promptTokenCount":100,"cachedContentTokenCount":100,
				"promptTokensDetails":[{"modality":"AUDIO","tokenCount":100}],
				"cacheTokensDetails":[{"modality":"AUDIO","tokenCount":100}]`,
			cached: 100,
		},
		{
			name: "cached_details_only",
			usage: `"promptTokenCount":100,"cachedContentTokenCount":60,
				"cacheTokensDetails":[{"modality":"AUDIO","tokenCount":60}]`,
			cached: 60,
		},
		{
			name: "cached_audio_exceeds_prompt_audio",
			usage: `"promptTokenCount":100,"cachedContentTokenCount":60,
				"promptTokensDetails":[{"modality":"AUDIO","tokenCount":50},{"modality":"TEXT","tokenCount":50}],
				"cacheTokensDetails":[{"modality":"AUDIO","tokenCount":60}]`,
		},
		{
			name: "cached_text_exceeds_prompt_text",
			usage: `"promptTokenCount":100,"cachedContentTokenCount":60,
				"promptTokensDetails":[{"modality":"AUDIO","tokenCount":50},{"modality":"TEXT","tokenCount":50}],
				"cacheTokensDetails":[{"modality":"TEXT","tokenCount":60}]`,
		},
		{
			name:  "incomplete_prompt_details",
			usage: `"promptTokenCount":100,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":75}]`,
		},
		{
			name: "malformed_cached_details",
			usage: `"promptTokenCount":100,"cachedContentTokenCount":60,
				"promptTokensDetails":[{"modality":"AUDIO","tokenCount":100}],
				"cacheTokensDetails":[{"modality":"AUDIO","tokenCount":59.5},{"modality":"TEXT","tokenCount":0.5}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := extractGeminiUsage([]byte(`{"usageMetadata":{` + tc.usage + "}}"))
			require.NotNil(t, usage)
			require.Equal(t, tc.fresh, usage.audioInputTokens)
			require.Equal(t, tc.cached, usage.audioCacheReadTokens)
		})
	}
}

func TestGeminiAudioPricingRespectsTierAndOverrides(t *testing.T) {
	const model = geminiAudioBillingModel
	tokens := UsageTokens{InputTokens: 100, AudioInputTokens: 50, CacheReadTokens: 50, AudioCacheReadTokens: 20}
	billing := NewBillingService(&config.Config{}, loadGeminiAudioPricingForTest(t, nil))
	resolver := NewModelPricingResolver(nil, billing)
	catalog, err := billing.GetModelPricing(model)
	require.NoError(t, err)

	priority, err := billing.CalculateCostWithServiceTier(model, tokens, 1, "priority")
	require.NoError(t, err)
	require.InDelta(t, 0.000135, priority.InputCost, 1e-12)
	require.InDelta(t, 0.0000063, priority.CacheReadCost, 1e-12)

	longContext := *catalog
	longContext.LongContextInputThreshold = 1
	longContext.LongContextInputMultiplier = 2
	long := billing.computeTokenBreakdown(&longContext, tokens, 1, "", true)
	require.True(t, long.LongContextBillingApplied)
	require.InDelta(t, 0.000150, long.InputCost, 1e-12)
	require.InDelta(t, 0.000007, long.CacheReadCost, 1e-12)

	flat := &ResolvedPricing{Mode: BillingModeToken, BasePricing: catalog}
	resolver.applyTokenOverrides(&ChannelModelPricing{InputPrice: testPtrFloat64(7e-6), CacheReadPrice: testPtrFloat64(0)}, flat)
	cost, err := billing.CalculateCostUnified(CostInput{Model: model, Tokens: tokens, RateMultiplier: 1, Resolver: resolver, Resolved: flat})
	require.NoError(t, err)
	require.InDelta(t, 0.000700, cost.InputCost, 1e-12)
	require.Zero(t, cost.CacheReadCost)

	interval := intervalToModelPricing(&PricingInterval{InputMultiplier: testPtrFloat64(2), CacheReadMultiplier: testPtrFloat64(3)}, catalog, nil)
	cost = billing.computeTokenBreakdown(interval, tokens, 1, "", false)
	require.InDelta(t, 0.000150, cost.InputCost, 1e-12)
	require.InDelta(t, 0.0000105, cost.CacheReadCost, 1e-12)
	interval = intervalToModelPricing(&PricingInterval{InputPrice: testPtrFloat64(0), CacheReadPrice: testPtrFloat64(1e-6)}, catalog, nil)
	cost = billing.computeTokenBreakdown(interval, tokens, 1, "", false)
	require.Zero(t, cost.InputCost)
	require.InDelta(t, 0.000050, cost.CacheReadCost, 1e-12)

	cfg := &config.Config{}
	cfg.Pricing.OverrideFile = filepath.Join(t.TempDir(), "prices.json")
	override := `{"` + model + `":{"input_cost_per_token":0.000002,"cache_read_input_token_cost":0.000001,
		"cache_read_input_audio_token_cost":0}}`
	require.NoError(t, os.WriteFile(cfg.Pricing.OverrideFile, []byte(override), 0o600))
	billing = NewBillingService(cfg, loadGeminiAudioPricingForTest(t, cfg))
	cost, err = billing.CalculateCost(model, tokens, 1)
	require.NoError(t, err)
	require.InDelta(t, 0.000200, cost.InputCost, 1e-12)
	require.InDelta(t, 0.000030, cost.CacheReadCost, 1e-12)
}
