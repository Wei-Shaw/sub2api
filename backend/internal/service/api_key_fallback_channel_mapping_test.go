//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayService_SelectAccountWithScheduler_FallbackUsesCandidateChannelMapping(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	primary := &Group{ID: 10, Platform: PlatformOpenAI, Status: StatusActive}
	fallback := &Group{ID: 20, Platform: PlatformOpenAI, Status: StatusActive}
	channelRepo := makeStandardRepo(Channel{
		ID:       1,
		Status:   StatusActive,
		GroupIDs: []int64{primary.ID, fallback.ID},
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"public-model": "fallback-upstream"},
		},
	}, map[int64]string{primary.ID: PlatformAnthropic, fallback.ID: PlatformOpenAI})
	account := &Account{ID: 200, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true}
	scheduler := &fallbackTrackingOpenAIAccountScheduler{account: account, succeed: fallback.ID}
	apiKey := &APIKey{GroupID: &primary.ID, Group: primary}
	routing := NewAPIKeyRoutingState(apiKey, []APIKeyRoutingCandidate{{Group: primary}, {Group: fallback}})
	ctx := WithAPIKeyRoutingState(context.Background(), routing)
	svc := &OpenAIGatewayService{
		channelService:   newTestChannelService(channelRepo),
		openaiScheduler:  scheduler,
		rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"),
	}

	_, _, err := svc.SelectAccountWithScheduler(ctx, apiKey.GroupID, "", "", "public-model", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Len(t, scheduler.requests, 2)
	require.Equal(t, "public-model", scheduler.requests[0].RequestedModel)
	require.Equal(t, "fallback-upstream", scheduler.requests[1].RequestedModel)
}
