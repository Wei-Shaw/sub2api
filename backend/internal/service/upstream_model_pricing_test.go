package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type upstreamPricingGroupAccounts struct {
	AccountRepository
	accounts []Account
}

func (r upstreamPricingGroupAccounts) ListByGroup(_ context.Context, _ int64) ([]Account, error) {
	return r.accounts, nil
}

func TestUpstreamModelPricingRefreshGuardsMarginAndRecordsAccountCost(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	model, alias := "gpt-5.1", "chat-alias"
	providerGroup := &Group{ID: 31, Platform: PlatformOpenAI, RateMultiplier: 0.4}
	providerBilling := NewBillingService(&config.Config{}, nil)
	provider := &OpenAIGatewayService{
		accountRepo:    upstreamPricingGroupAccounts{accounts: []Account{{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}}},
		billingService: providerBilling,
		resolver:       newOpenAITokenImageChannelPricingResolverForTest(t, providerGroup.ID, model),
	}
	response := func() *http.Response {
		catalog := provider.ResolveKeyModelPricing(ctx, providerGroup, []string{model}, now)
		require.NotNil(t, catalog)
		require.Contains(t, catalog.Models, model)
		body, err := json.Marshal(map[string]any{
			"object": "sub2api.key_billing", "schema_version": 1, "billing_scope": "token",
			"group_rate_multiplier": 0.4, "resolved_rate_multiplier": 0.4, "effective_rate_multiplier": 0.4,
			"peak_rate_enabled": false, "observed_at": now, "model_pricing": catalog,
		})
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}
	}
	low := response()
	cache, _ := provider.resolver.channelService.cache.Load().(*channelCache)
	cache.pricingByGroupModel[channelModelKey{groupID: providerGroup.ID, model: model}].InputPrice = f64p(12e-6)
	high := response()

	pool := &Account{
		ID: 41, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, RateMultiplier: f64p(0.4),
		Credentials: map[string]any{
			"pool_mode": true, "base_url": "https://upstream.example/v1", "api_key": "example-key",
			"model_mapping": map[string]any{alias: model},
		},
		Extra: map[string]any{UpstreamBillingProbeEnabledExtraKey: true, UpstreamModelPricingSyncExtraKey: true, "quota_limit": 100.0},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{pool.ID: pool}}
	httpUpstream := &httpUpstreamRecorder{responses: []*http.Response{low, high}}
	probe := newUpstreamBillingProbeTestService(repo, httpUpstream, &upstreamBillingProbeSettingRepo{})
	probe.now = func() time.Time { return now }
	group := &Group{ID: 32, Platform: PlatformOpenAI, RateMultiplier: 1, SubscriptionType: SubscriptionTypeStandard}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	usageBilling := &openAIRecordUsageBillingRepoStub{}
	downstream := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	downstream.resolver = newOpenAITokenImageChannelPricingResolverForTest(t, group.ID, model)
	downstream.usageBillingRepo = usageBilling
	gateCtx := context.WithValue(ctx, openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{
		billingGroup: group, groupID: group.ID, platform: group.Platform, pricingAt: now, threshold: 0.8,
	})
	gateCtx = downstream.WithUpstreamPricingModel(gateCtx, alias, false)

	vetoed, reason := openAIProfitControlVetoReason(gateCtx, pool)
	require.True(t, vetoed)
	require.Equal(t, openAIProfitFilterReasonUpstreamPriceUnavailable, reason)
	first, err := probe.ProbeAccount(ctx, pool.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusOK, first.Status)
	require.Equal(t, "model_pricing", httpUpstream.lastReq.URL.Query().Get("include"))
	require.Equal(t, model, httpUpstream.lastReq.URL.Query().Get("models"))
	pool, err = repo.GetByID(ctx, pool.ID)
	require.NoError(t, err)
	vetoed, reason = openAIProfitControlVetoReason(gateCtx, pool)
	require.False(t, vetoed, "%s", reason)

	second, err := probe.ProbeAccount(ctx, pool.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusOK, second.Status)
	require.Equal(t, first.Data["effective_rate_multiplier"], second.Data["effective_rate_multiplier"])
	pool, err = repo.GetByID(ctx, pool.ID)
	require.NoError(t, err)
	vetoed, reason = openAIProfitControlVetoReason(gateCtx, pool)
	require.True(t, vetoed)
	require.Equal(t, openAIProfitFilterReasonThreshold, reason)

	// A request allowed by a group without profit control still uses the
	// synchronized purchase price for account quota and cost statistics.
	err = downstream.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "price-sync-usage", Model: model, Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 500}},
		APIKey: &APIKey{ID: 33, GroupID: &group.ID, Group: group}, User: &User{ID: 34}, Account: pool, PricingAt: now,
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.AccountStatsCost)
	require.InDelta(t, 1000*12e-6+500*15e-6, *usageRepo.lastLog.AccountStatsCost, 1e-12)
	require.InDelta(t, 0.4, *usageRepo.lastLog.AccountRateMultiplier, 1e-12)
	require.NotNil(t, usageBilling.lastCmd)
	require.InDelta(t, (1000*12e-6+500*15e-6)*0.4, usageBilling.lastCmd.AccountQuotaCost, 1e-12)
	require.InDelta(t, 1000*3e-6+500*15e-6, usageBilling.lastCmd.BalanceCost, 1e-12)

	_, _, fresh := upstreamModelPriceAt(pool, model, now, second.FreshUntil.Add(time.Second))
	require.False(t, fresh)
	delete(pool.Extra, UpstreamModelPricingSyncExtraKey)
	vetoed, _ = openAIProfitControlVetoReason(gateCtx, pool)
	require.False(t, vetoed)
}

func TestUpstreamModelPriceMatchesTokenBilling(t *testing.T) {
	const model = "gpt-5.1"
	groupID := int64(22)
	at := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	resolver := newOpenAITokenImageChannelPricingResolverWithTimeForTest(t, groupID, model, &ChannelTimePricing{
		Timezone: "UTC", Periods: []ChannelTimePricingPeriod{{StartTime: "01:00", EndTime: "03:00", Multiplier: 2}},
	})
	billing := NewBillingService(&config.Config{}, nil)
	input := CostInput{Model: model, GroupID: &groupID, Resolver: resolver, RateMultiplier: 1, PricingAt: at}
	input.Resolved = resolver.Resolve(context.Background(), PricingInput{Model: model, GroupID: &groupID})
	quote, ok := billing.buildUpstreamModelPrice(input, false)
	require.True(t, ok)
	for _, tier := range upstreamPriceTiers {
		input.ServiceTier = tier
		for _, tokens := range []UsageTokens{
			{OutputTokens: 3},
			{InputTokens: 350, OutputTokens: 100, CacheReadTokens: 90, CacheCreationTokens: 40, CacheCreation1hTokens: 10},
			{InputTokens: 520_000, OutputTokens: 100, ImageInputTokens: 50, ImageOutputTokens: 30, CacheReadTokens: 90, ImageCacheReadTokens: 10, CacheCreationTokens: 40, CacheCreation12hTokens: 10},
		} {
			input.Tokens = tokens
			actual, err := billing.CalculateCostUnified(input)
			require.NoError(t, err)
			got, ok := quote.baseCost(tokens, tier, "", at)
			require.True(t, ok)
			require.InDelta(t, actual.TotalCost, got, 1e-10, "%s %+v", tier, tokens)
		}
	}
}

func TestUpstreamModelPriceMatchesContextTieredRequestBilling(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	bound := 10
	input := CostInput{
		Model: "request-priced", Resolver: NewModelPricingResolver(nil, billing),
		RateMultiplier: 1, PricingAt: time.Now(),
		Resolved: &ResolvedPricing{
			Mode: BillingModePerRequest, DefaultPerRequestPrice: 0.02,
			RequestTiers: []PricingInterval{{MaxTokens: &bound, PerRequestPrice: f64p(0.03)}},
		},
	}
	quote, ok := billing.buildUpstreamModelPrice(input, false)
	require.True(t, ok)
	for _, count := range []int{0, 1, 10, 11} {
		input.Tokens.InputTokens = count
		native, err := billing.CalculateCostUnified(input)
		require.NoError(t, err)
		got, valid := quote.baseCost(input.Tokens, "", "", input.PricingAt)
		require.True(t, valid)
		require.InDelta(t, native.TotalCost, got, 1e-12, "context tokens %d", count)
	}
}

func TestParseUpstreamModelPricingDiscardsInvalidPrices(t *testing.T) {
	raw := json.RawMessage(`{
		"version": 1, "currency": "USD", "models": {
			"good": {"mode":"per_request", "bands":[{"through_context":10,"request_price":0.2},{"through_context":null,"request_price":0.5}]},
			"null-price": {"mode":"per_request", "bands":[{"through_context":null,"request_price":null}]},
			"missing-tail": {"mode":"per_request", "bands":[{"through_context":10,"request_price":0.2}]},
			"wildcard-*": {"mode":"per_request", "bands":[{"through_context":null,"request_price":0.2}]}
		}
	}`)
	models := parseUpstreamModelPricing(raw)
	require.Len(t, models, 1)
	require.Contains(t, models, "good")
}
