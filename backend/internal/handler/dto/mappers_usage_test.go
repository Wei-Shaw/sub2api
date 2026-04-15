package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogFromService_IncludesOpenAIWSMode(t *testing.T) {
	t.Parallel()

	wsLog := &service.UsageLog{
		RequestID:    "req_1",
		Model:        "gpt-5.3-codex",
		OpenAIWSMode: true,
	}
	httpLog := &service.UsageLog{
		RequestID:    "resp_1",
		Model:        "gpt-5.3-codex",
		OpenAIWSMode: false,
	}

	require.True(t, UsageLogFromService(wsLog).OpenAIWSMode)
	require.False(t, UsageLogFromService(httpLog).OpenAIWSMode)
	require.True(t, UsageLogFromServiceAdmin(wsLog).OpenAIWSMode)
	require.False(t, UsageLogFromServiceAdmin(httpLog).OpenAIWSMode)
}

func TestUsageLogFromService_PrefersRequestTypeForLegacyFields(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		RequestID:    "req_2",
		Model:        "gpt-5.3-codex",
		RequestType:  service.RequestTypeWSV2,
		Stream:       false,
		OpenAIWSMode: false,
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.Equal(t, "ws_v2", userDTO.RequestType)
	require.True(t, userDTO.Stream)
	require.True(t, userDTO.OpenAIWSMode)
	require.Equal(t, "ws_v2", adminDTO.RequestType)
	require.True(t, adminDTO.Stream)
	require.True(t, adminDTO.OpenAIWSMode)
}

func TestUsageCleanupTaskFromService_RequestTypeMapping(t *testing.T) {
	t.Parallel()

	requestType := int16(service.RequestTypeStream)
	task := &service.UsageCleanupTask{
		ID:     1,
		Status: service.UsageCleanupStatusPending,
		Filters: service.UsageCleanupFilters{
			RequestType: &requestType,
		},
	}

	dtoTask := UsageCleanupTaskFromService(task)
	require.NotNil(t, dtoTask)
	require.NotNil(t, dtoTask.Filters.RequestType)
	require.Equal(t, "stream", *dtoTask.Filters.RequestType)
}

func TestRequestTypeStringPtrNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, requestTypeStringPtr(nil))
}

func TestUsageLogFromService_IncludesServiceTierForUserAndAdmin(t *testing.T) {
	t.Parallel()

	serviceTier := "priority"
	inboundEndpoint := "/v1/chat/completions"
	upstreamEndpoint := "/v1/responses"
	log := &service.UsageLog{
		RequestID:             "req_3",
		Model:                 "gpt-5.4",
		ServiceTier:           &serviceTier,
		InboundEndpoint:       &inboundEndpoint,
		UpstreamEndpoint:      &upstreamEndpoint,
		AccountRateMultiplier: f64Ptr(1.5),
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.NotNil(t, userDTO.ServiceTier)
	require.Equal(t, serviceTier, *userDTO.ServiceTier)
	require.NotNil(t, userDTO.InboundEndpoint)
	require.Equal(t, inboundEndpoint, *userDTO.InboundEndpoint)
	require.NotNil(t, userDTO.UpstreamEndpoint)
	require.Equal(t, upstreamEndpoint, *userDTO.UpstreamEndpoint)
	require.NotNil(t, adminDTO.ServiceTier)
	require.Equal(t, serviceTier, *adminDTO.ServiceTier)
	require.NotNil(t, adminDTO.InboundEndpoint)
	require.Equal(t, inboundEndpoint, *adminDTO.InboundEndpoint)
	require.NotNil(t, adminDTO.UpstreamEndpoint)
	require.Equal(t, upstreamEndpoint, *adminDTO.UpstreamEndpoint)
	require.NotNil(t, adminDTO.AccountRateMultiplier)
	require.InDelta(t, 1.5, *adminDTO.AccountRateMultiplier, 1e-12)
}

func TestUsageLogFromService_UsesRequestedModelAndKeepsUpstreamAdminOnly(t *testing.T) {
	t.Parallel()

	upstreamModel := "claude-sonnet-4-20250514"
	log := &service.UsageLog{
		RequestID:      "req_4",
		Model:          upstreamModel,
		RequestedModel: "claude-sonnet-4",
		UpstreamModel:  &upstreamModel,
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.Equal(t, "claude-sonnet-4", userDTO.Model)
	require.Equal(t, "claude-sonnet-4", adminDTO.Model)

	userJSON, err := json.Marshal(userDTO)
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "upstream_model")

	adminJSON, err := json.Marshal(adminDTO)
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"upstream_model":"claude-sonnet-4-20250514"`)
}

func TestUsageLogFromService_FallsBackToLegacyModelWhenRequestedModelMissing(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		RequestID: "req_legacy",
		Model:     "claude-3",
	}

	userDTO := UsageLogFromService(log)
	adminDTO := UsageLogFromServiceAdmin(log)

	require.Equal(t, "claude-3", userDTO.Model)
	require.Equal(t, "claude-3", adminDTO.Model)
}

func TestUsageLogFromServiceAdmin_IncludesRoutingSnapshot(t *testing.T) {
	t.Parallel()

	routingTargetGroup := "exhausted"
	routingSelectedGroup := "reserve"
	routingScheduleLayer := "load_balance"
	routingSelectedAccountID := int64(66)
	routingSelectedAccountName := "acc-66"
	routingEffectiveModel := "gpt-5.4"
	routingFailoverCount := 1
	routingFailoverFinalReason := "upstream_502"

	log := &service.UsageLog{
		RequestID:                  "req-routing-1",
		Model:                      "gpt-5.4-Sys",
		RoutingTargetGroup:         &routingTargetGroup,
		RoutingSelectedGroup:       &routingSelectedGroup,
		RoutingScheduleLayer:       &routingScheduleLayer,
		RoutingSelectedAccountID:   &routingSelectedAccountID,
		RoutingSelectedAccountName: &routingSelectedAccountName,
		RoutingEffectiveModel:      &routingEffectiveModel,
		RoutingFailoverCount:       &routingFailoverCount,
		RoutingFailoverFinalReason: &routingFailoverFinalReason,
	}

	adminDTO := UsageLogFromServiceAdmin(log)

	require.NotNil(t, adminDTO.RoutingTargetGroup)
	require.Equal(t, routingTargetGroup, *adminDTO.RoutingTargetGroup)
	require.NotNil(t, adminDTO.RoutingSelectedGroup)
	require.Equal(t, routingSelectedGroup, *adminDTO.RoutingSelectedGroup)
	require.NotNil(t, adminDTO.RoutingScheduleLayer)
	require.Equal(t, routingScheduleLayer, *adminDTO.RoutingScheduleLayer)
	require.NotNil(t, adminDTO.RoutingSelectedAccountID)
	require.Equal(t, routingSelectedAccountID, *adminDTO.RoutingSelectedAccountID)
	require.NotNil(t, adminDTO.RoutingSelectedAccountName)
	require.Equal(t, routingSelectedAccountName, *adminDTO.RoutingSelectedAccountName)
	require.NotNil(t, adminDTO.RoutingEffectiveModel)
	require.Equal(t, routingEffectiveModel, *adminDTO.RoutingEffectiveModel)
	require.NotNil(t, adminDTO.RoutingFailoverCount)
	require.Equal(t, routingFailoverCount, *adminDTO.RoutingFailoverCount)
	require.NotNil(t, adminDTO.RoutingFailoverFinalReason)
	require.Equal(t, routingFailoverFinalReason, *adminDTO.RoutingFailoverFinalReason)
}

func TestUsageLogFromService_IncludesBillingBreakdownFields(t *testing.T) {
	t.Parallel()

	accountRateMultiplier := 1.5
	priorityAccountMultiplier := 100.0
	effectiveMultiplier := 150.0
	effectiveInputUnitPrice := 5e-6
	effectiveOutputUnitPrice := 30e-6
	effectiveCacheReadUnitPrice := 0.5e-6
	pricingSource := "priority_pricing,priority_account_multiplier"

	log := &service.UsageLog{
		RequestID:                   "req-billing-breakdown",
		Model:                       "gpt-5.4",
		ImageOutputTokens:           4,
		ImageOutputCost:             0.42,
		AccountRateMultiplier:       &accountRateMultiplier,
		PriorityAccountMultiplier:   &priorityAccountMultiplier,
		EffectiveMultiplier:         &effectiveMultiplier,
		EffectiveInputUnitPrice:     &effectiveInputUnitPrice,
		EffectiveOutputUnitPrice:    &effectiveOutputUnitPrice,
		EffectiveCacheReadUnitPrice: &effectiveCacheReadUnitPrice,
		PricingSource:               &pricingSource,
		BillingMode:                 strPtr("image"),
	}

	userDTO := UsageLogFromService(log)

	require.NotNil(t, userDTO.AccountRateMultiplier)
	require.InDelta(t, accountRateMultiplier, *userDTO.AccountRateMultiplier, 1e-12)
	require.NotNil(t, userDTO.PriorityAccountMultiplier)
	require.InDelta(t, priorityAccountMultiplier, *userDTO.PriorityAccountMultiplier, 1e-12)
	require.NotNil(t, userDTO.EffectiveMultiplier)
	require.InDelta(t, effectiveMultiplier, *userDTO.EffectiveMultiplier, 1e-12)
	require.NotNil(t, userDTO.EffectiveInputUnitPrice)
	require.InDelta(t, effectiveInputUnitPrice, *userDTO.EffectiveInputUnitPrice, 1e-12)
	require.NotNil(t, userDTO.EffectiveOutputUnitPrice)
	require.InDelta(t, effectiveOutputUnitPrice, *userDTO.EffectiveOutputUnitPrice, 1e-12)
	require.NotNil(t, userDTO.EffectiveCacheReadUnitPrice)
	require.InDelta(t, effectiveCacheReadUnitPrice, *userDTO.EffectiveCacheReadUnitPrice, 1e-12)
	require.NotNil(t, userDTO.PricingSource)
	require.Equal(t, pricingSource, *userDTO.PricingSource)
	require.Equal(t, 4, userDTO.ImageOutputTokens)
	require.Equal(t, 0.42, userDTO.ImageOutputCost)
	require.NotNil(t, userDTO.BillingMode)
	require.Equal(t, "image", *userDTO.BillingMode)
}

func TestUsageLogFromServiceAdmin_IncludesChannelAndBillingFields(t *testing.T) {
	t.Parallel()

	channelID := int64(7)
	modelMappingChain := "alias->gpt-5.4"
	billingTier := "channel_mapped"
	billingMode := "token"

	log := &service.UsageLog{
		RequestID:         "req-admin-channel",
		Model:             "gpt-5.4",
		ChannelID:         &channelID,
		ModelMappingChain: &modelMappingChain,
		BillingTier:       &billingTier,
		BillingMode:       &billingMode,
	}

	adminDTO := UsageLogFromServiceAdmin(log)

	require.NotNil(t, adminDTO.ChannelID)
	require.Equal(t, channelID, *adminDTO.ChannelID)
	require.NotNil(t, adminDTO.ModelMappingChain)
	require.Equal(t, modelMappingChain, *adminDTO.ModelMappingChain)
	require.NotNil(t, adminDTO.BillingTier)
	require.Equal(t, billingTier, *adminDTO.BillingTier)
	require.NotNil(t, adminDTO.BillingMode)
	require.Equal(t, billingMode, *adminDTO.BillingMode)
}

func f64Ptr(value float64) *float64 {
	return &value
}

func strPtr(value string) *string {
	return &value
}
