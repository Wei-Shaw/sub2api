//go:build unit

package service

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChannelServiceTierConfigDefaultsAndValidation(t *testing.T) {
	config := DefaultChannelServiceTierConfig()
	require.NoError(t, config.Validate())
	require.Equal(t, ChannelServiceTierOption{Enabled: true, Multiplier: 1}, config.Standard)
	require.Equal(t, ChannelServiceTierOption{Enabled: true, Multiplier: 2}, config.Priority)
	require.Equal(t, ChannelServiceTierOption{Enabled: true, Multiplier: 0.5}, config.Flex)

	allDisabled := config
	allDisabled.Standard.Enabled = false
	allDisabled.Priority.Enabled = false
	allDisabled.Flex.Enabled = false
	require.ErrorContains(t, allDisabled.Validate(), "at least one")

	for _, multiplier := range []float64{0, 100.01, math.NaN(), math.Inf(1)} {
		invalid := config
		invalid.Priority.Multiplier = multiplier
		require.Error(t, invalid.Validate())
	}
}

func TestNormalizeOpenAICommercialTier(t *testing.T) {
	tests := []struct {
		raw  string
		want OpenAICommercialServiceTier
		ok   bool
	}{
		{"", OpenAICommercialTierStandard, true},
		{"auto", OpenAICommercialTierStandard, true},
		{"default", OpenAICommercialTierStandard, true},
		{"scale", OpenAICommercialTierStandard, true},
		{"standard", OpenAICommercialTierStandard, true},
		{"fast", OpenAICommercialTierPriority, true},
		{" PRIORITY ", OpenAICommercialTierPriority, true},
		{"flex", OpenAICommercialTierFlex, true},
		{"turbo", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, ok := NormalizeOpenAICommercialTier(tt.raw)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestValidateNativeGrokServiceTier(t *testing.T) {
	for _, allowed := range []string{"", "auto", "default", "standard"} {
		require.Nil(t, ValidateNativeGrokServiceTier(allowed))
	}
	for _, rejected := range []string{"fast", "priority", "flex", "scale", "turbo"} {
		blocked := ValidateNativeGrokServiceTier(rejected)
		require.NotNil(t, blocked)
		require.Equal(t, "SERVICE_TIER_UNSUPPORTED_FOR_PLATFORM", blocked.Code)
	}
}

func TestOpenAIChannelAndPlatformPolicyKeepsNativeGrokStandardOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}

	t.Run("channel can disable standard", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		config := DefaultChannelServiceTierConfig()
		config.Standard.Enabled = false
		c.Set(openAIServiceTierSnapshotContextKey, openAIServiceTierSnapshotLookup{
			Found:    true,
			Snapshot: &ChannelServiceTierSnapshot{ChannelID: 1, GroupID: 2, ChannelName: "grok", Config: config},
		})
		svc := &OpenAIGatewayService{channelService: &ChannelService{}}
		err := svc.applyOpenAIChannelAndPlatformPolicyToTier(context.Background(), c, account, "standard")
		var blocked *OpenAIFastBlockedError
		require.ErrorAs(t, err, &blocked)
		require.Equal(t, "CHANNEL_SERVICE_TIER_NOT_ALLOWED", blocked.Code)
	})

	t.Run("native capability rejects priority after channel policy", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(openAIServiceTierSnapshotContextKey, openAIServiceTierSnapshotLookup{
			Found:    true,
			Snapshot: &ChannelServiceTierSnapshot{ChannelID: 1, GroupID: 2, ChannelName: "grok", Config: DefaultChannelServiceTierConfig()},
		})
		svc := &OpenAIGatewayService{channelService: &ChannelService{}}
		err := svc.applyOpenAIChannelAndPlatformPolicyToTier(context.Background(), c, account, "priority")
		var blocked *OpenAIFastBlockedError
		require.ErrorAs(t, err, &blocked)
		require.Equal(t, "SERVICE_TIER_UNSUPPORTED_FOR_PLATFORM", blocked.Code)
	})
}

func TestOpenAIWebSocketServiceTierRefreshesChannelSnapshotEachTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(10)
	config := DefaultChannelServiceTierConfig()
	channel := Channel{
		ID:                1,
		Name:              "ws-channel",
		Status:            StatusActive,
		GroupIDs:          []int64{groupID},
		ServiceTierConfig: config,
		UpdatedAt:         time.Now(),
	}
	repo := &mockChannelRepository{
		listAllFn: func(_ context.Context) ([]Channel, error) {
			return []Channel{channel}, nil
		},
		getGroupPlatformsFn: func(_ context.Context, _ []int64) (map[int64]string, error) {
			return map[int64]string{groupID: PlatformOpenAI}, nil
		},
	}
	channelService := newTestChannelService(repo)
	svc := &OpenAIGatewayService{channelService: channelService}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{GroupID: &groupID})
	frame := []byte(`{"type":"response.create","model":"gpt-5.4","service_tier":"priority"}`)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	updated, blocked, err := svc.applyOpenAIFastAndChannelPolicyToWSResponseCreate(context.Background(), c, account, "gpt-5.4", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, "priority", gjson.GetBytes(updated, "service_tier").String())
	firstState := OpenAIServiceTierStateFromContext(c)
	require.NotNil(t, firstState)
	require.True(t, firstState.Snapshot.Config.Priority.Enabled)

	config.Priority.Enabled = false
	channel.ServiceTierConfig = config
	channel.UpdatedAt = channel.UpdatedAt.Add(time.Second)
	channelService.invalidateCache()

	_, blocked, err = svc.applyOpenAIFastAndChannelPolicyToWSResponseCreate(context.Background(), c, account, "gpt-5.4", frame)
	require.NoError(t, err)
	require.NotNil(t, blocked)
	require.Equal(t, "CHANNEL_SERVICE_TIER_NOT_ALLOWED", blocked.Code)
	secondState := OpenAIServiceTierStateFromContext(c)
	require.NotNil(t, secondState)
	require.False(t, secondState.Snapshot.Config.Priority.Enabled)

	cached, ok := channelService.cache.Load().(*channelCache)
	require.True(t, ok)
	cached.loadedAt = time.Now().Add(-(channelCacheMaxTierStale + time.Second))
	channelService.cacheRefreshAfter.Store(time.Now().Add(time.Minute).UnixNano())

	_, blocked, err = svc.applyOpenAIFastAndChannelPolicyToWSResponseCreate(context.Background(), c, account, "gpt-5.4", frame)
	require.ErrorIs(t, err, ErrChannelServiceTierSnapshotStale)
	require.Nil(t, blocked)
	require.Nil(t, OpenAIServiceTierStateFromContext(c), "failed refresh must not retain the previous turn snapshot")
}

func TestClassifyOpenAIServiceTierMismatch(t *testing.T) {
	tests := []struct {
		name     string
		outbound OpenAICommercialServiceTier
		actual   OpenAICommercialServiceTier
		want     string
	}{
		{name: "priority to standard is degraded", outbound: OpenAICommercialTierPriority, actual: OpenAICommercialTierStandard, want: "degraded"},
		{name: "standard to flex is degraded", outbound: OpenAICommercialTierStandard, actual: OpenAICommercialTierFlex, want: "degraded"},
		{name: "standard to priority is upgraded", outbound: OpenAICommercialTierStandard, actual: OpenAICommercialTierPriority, want: "upgraded"},
		{name: "flex to standard is upgraded", outbound: OpenAICommercialTierFlex, actual: OpenAICommercialTierStandard, want: "upgraded"},
		{name: "unknown values are changed", outbound: OpenAICommercialTierStandard, actual: OpenAICommercialServiceTier("scale"), want: "changed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, classifyOpenAIServiceTierMismatch(tt.outbound, tt.actual))
		})
	}
}

func TestServiceTierBlockedErrorsKeepStableCodeAcrossProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	blocked := &OpenAIFastBlockedError{Code: "CHANNEL_SERVICE_TIER_NOT_ALLOWED", Message: "blocked"}

	chatRecorder := httptest.NewRecorder()
	chatContext, _ := gin.CreateTestContext(chatRecorder)
	writeChatCompletionsBlockedError(chatContext, blocked)
	require.Equal(t, http.StatusForbidden, chatRecorder.Code)
	require.Equal(t, blocked.Code, gjson.Get(chatRecorder.Body.String(), "error.code").String())

	messagesRecorder := httptest.NewRecorder()
	messagesContext, _ := gin.CreateTestContext(messagesRecorder)
	writeAnthropicBlockedError(messagesContext, blocked)
	require.Equal(t, http.StatusForbidden, messagesRecorder.Code)
	require.Equal(t, blocked.Code, gjson.Get(messagesRecorder.Body.String(), "error.code").String())

	wsEvent := buildOpenAIFastPolicyBlockedWSEvent(blocked)
	require.Equal(t, blocked.Code, gjson.GetBytes(wsEvent, "error.code").String())
}

func TestEnforceOpenAIChannelServiceTierUsesOutboundTier(t *testing.T) {
	config := DefaultChannelServiceTierConfig()
	config.Standard.Enabled = false
	config.Priority.Enabled = true
	config.Flex.Enabled = false
	snapshot := &ChannelServiceTierSnapshot{ChannelID: 9, GroupID: 7, ChannelName: "paid", Config: config}

	state, err := enforceOpenAIServiceTierForAccount(snapshot,
		[]byte(`{"service_tier":"fast"}`),
		[]byte(`{"service_tier":"priority"}`),
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, "fast", state.RequestedProtocolTier)
	require.Equal(t, "priority", state.OutboundProtocolTier)
	require.Equal(t, OpenAICommercialTierPriority, state.OutboundTier)

	_, err = enforceOpenAIServiceTierForAccount(snapshot,
		[]byte(`{"service_tier":"flex"}`),
		[]byte(`{"service_tier":"flex"}`),
		nil,
	)
	var blocked *OpenAIFastBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, "CHANNEL_SERVICE_TIER_NOT_ALLOWED", blocked.Code)

	// A global filter can remove priority. Permission is checked against the
	// final outbound Standard tier, not the original client request.
	_, err = enforceOpenAIServiceTierForAccount(snapshot,
		[]byte(`{"service_tier":"priority"}`),
		[]byte(`{}`),
		nil,
	)
	require.ErrorAs(t, err, &blocked)

	// Scale remains a protocol value but belongs to the Standard commercial tier.
	_, err = enforceOpenAIServiceTierForAccount(snapshot,
		[]byte(`{"service_tier":"scale"}`),
		[]byte(`{"service_tier":"scale"}`),
		nil,
	)
	require.ErrorAs(t, err, &blocked)
}

func TestApplyChannelServiceTierRateMultiplierOverridesLegacyPolicyOnce(t *testing.T) {
	snapshot := &ChannelServiceTierSnapshot{Config: ChannelServiceTierConfig{
		Standard: ChannelServiceTierOption{Enabled: true, Multiplier: 1.25},
		Priority: ChannelServiceTierOption{Enabled: true, Multiplier: 3},
		Flex:     ChannelServiceTierOption{Enabled: true, Multiplier: 0.4},
	}}

	rate, legacyTier := applyChannelServiceTierRateMultiplier(2, "priority", snapshot)
	require.InDelta(t, 6, rate, 1e-12)
	require.Empty(t, legacyTier, "legacy priority pricing must be disabled after applying the channel multiplier")

	rate, legacyTier = applyChannelServiceTierRateMultiplier(2, "priority", nil)
	require.InDelta(t, 2, rate, 1e-12)
	require.Equal(t, "priority", legacyTier)
}

func TestResolveOpenAIServiceTierBillingDecisionUsesAPIResponseForAPIKey(t *testing.T) {
	snapshot := &ChannelServiceTierSnapshot{ChannelID: 3, GroupID: 4, Config: DefaultChannelServiceTierConfig()}
	state := &OpenAIServiceTierRequestState{
		Snapshot:              snapshot,
		RequestedProtocolTier: "fast",
		OutboundProtocolTier:  "priority",
		RequestedTier:         OpenAICommercialTierPriority,
		OutboundTier:          OpenAICommercialTierPriority,
	}
	actual := "standard"
	result := &OpenAIForwardResult{ActualServiceTier: &actual}
	decision := resolveOpenAIServiceTierBillingDecision(result, state, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey})

	require.True(t, decision.ActualWasUsed)
	require.Equal(t, OpenAICommercialTierStandard, decision.CommercialTier)
	require.Equal(t, "standard", decision.ProtocolTier)
	require.NotNil(t, result.BillingServiceTier)
	require.Equal(t, "standard", *result.BillingServiceTier)

	billing := newTestBillingServiceForResolver()
	resolver := NewModelPricingResolver(nil, billing)
	snapshot.Config.Standard.Enabled = false
	snapshot.Config.Standard.Multiplier = 1.25
	snapshot.Config.Priority.Multiplier = 3
	cost, err := billing.CalculateCostUnified(CostInput{
		Model:               "claude-sonnet-4",
		Tokens:              UsageTokens{InputTokens: 1000, OutputTokens: 1000},
		RateMultiplier:      1,
		ServiceTier:         decision.ProtocolTier,
		ServiceTierSnapshot: snapshot,
		Resolver:            resolver,
	})
	require.NoError(t, err)
	require.InDelta(t, cost.TotalCost*1.25, cost.ActualCost, 1e-12,
		"actual Standard must be returned and billed with its configured multiplier even when that tier is now disabled")

	unknown := "turbo"
	result = &OpenAIForwardResult{ActualServiceTier: &unknown}
	decision = resolveOpenAIServiceTierBillingDecision(result, state, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
	require.True(t, decision.ActualWasUnknown)
	require.False(t, decision.ActualWasUsed)
	require.Equal(t, OpenAICommercialTierPriority, decision.CommercialTier)
	require.Equal(t, "priority", decision.ProtocolTier)
}

func TestResolveOpenAIServiceTierBillingDecisionUsesOAuthOutboundForCodexFast(t *testing.T) {
	state := &OpenAIServiceTierRequestState{
		RequestedProtocolTier: "fast",
		OutboundProtocolTier:  "priority",
		RequestedTier:         OpenAICommercialTierPriority,
		OutboundTier:          OpenAICommercialTierPriority,
	}
	actual := "default"
	result := &OpenAIForwardResult{ActualServiceTier: &actual}
	decision := resolveOpenAIServiceTierBillingDecision(result, state, &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	})

	require.True(t, decision.ActualWasObserved)
	require.False(t, decision.ActualWasUsed)
	require.False(t, decision.ActualWasUnknown)
	require.Equal(t, openAIServiceTierEvidenceOAuthOutboundAuthoritative, decision.Evidence)
	require.Equal(t, OpenAICommercialTierPriority, decision.CommercialTier)
	require.Equal(t, "priority", decision.ProtocolTier)
	require.NotNil(t, result.BillingServiceTier)
	require.Equal(t, "priority", *result.BillingServiceTier)
}

func TestResolveOpenAIServiceTierBillingDecisionOAuthUnknownResponseCannotDowngrade(t *testing.T) {
	state := &OpenAIServiceTierRequestState{
		RequestedProtocolTier: "fast",
		OutboundProtocolTier:  "priority",
		RequestedTier:         OpenAICommercialTierPriority,
		OutboundTier:          OpenAICommercialTierPriority,
	}
	actual := "internal_default"
	result := &OpenAIForwardResult{ActualServiceTier: &actual}
	decision := resolveOpenAIServiceTierBillingDecision(result, state, &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	})

	require.True(t, decision.ActualWasObserved)
	require.True(t, decision.ActualWasUnknown)
	require.False(t, decision.ActualWasUsed)
	require.Equal(t, openAIServiceTierEvidenceOAuthOutboundAuthoritative, decision.Evidence)
	require.Equal(t, OpenAICommercialTierPriority, decision.CommercialTier)
}

func TestExtractOpenAIActualServiceTier(t *testing.T) {
	require.Equal(t, "priority", *extractOpenAIActualServiceTierFromJSONBytes([]byte(`{"service_tier":"priority"}`)))
	require.Equal(t, "flex", *extractOpenAIActualServiceTierFromJSONBytes([]byte(`{"response":{"service_tier":"flex"}}`)))
	require.Nil(t, extractOpenAIActualServiceTierFromJSONBytes([]byte(`{"service_tier":3}`)))
	require.Nil(t, extractOpenAIActualServiceTierFromJSONBytes([]byte(`not-json`)))
}

func TestCalculateCostUnifiedAppliesChannelTierMultiplierToFallbackAndPerRequest(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	resolver := NewModelPricingResolver(nil, bs)
	snapshot := &ChannelServiceTierSnapshot{Config: ChannelServiceTierConfig{
		Standard: ChannelServiceTierOption{Enabled: true, Multiplier: 1.25},
		Priority: ChannelServiceTierOption{Enabled: true, Multiplier: 3},
		Flex:     ChannelServiceTierOption{Enabled: true, Multiplier: 0.4},
	}}

	tokenCost, err := bs.CalculateCostUnified(CostInput{
		Model:               "claude-sonnet-4",
		Tokens:              UsageTokens{InputTokens: 1000, OutputTokens: 1000},
		RateMultiplier:      2,
		ServiceTier:         "priority",
		ServiceTierSnapshot: snapshot,
		Resolver:            resolver,
	})
	require.NoError(t, err)
	require.InDelta(t, tokenCost.TotalCost*6, tokenCost.ActualCost, 1e-12)

	perRequestCost, err := bs.CalculateCostUnified(CostInput{
		Model:               "custom-image",
		RequestCount:        2,
		RateMultiplier:      2,
		ServiceTier:         "priority",
		ServiceTierSnapshot: snapshot,
		Resolver:            resolver,
		Resolved: &ResolvedPricing{
			Mode:                   BillingModePerRequest,
			DefaultPerRequestPrice: 0.1,
		},
	})
	require.NoError(t, err)
	require.InDelta(t, 0.2, perRequestCost.TotalCost, 1e-12)
	require.InDelta(t, 1.2, perRequestCost.ActualCost, 1e-12)
}
