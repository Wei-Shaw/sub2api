package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMapperExposesForceOpenAIFastOnlyToAdmins(t *testing.T) {
	group := &service.Group{
		ID: 7, Name: "fast", Platform: service.PlatformOpenAI, Status: service.StatusActive, ForceOpenAIFast: true, FreeOpenAIFast: true,
	}

	userJSON, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "force_openai_fast")
	require.NotContains(t, string(userJSON), "free_openai_fast")

	adminJSON, err := json.Marshal(GroupFromServiceAdmin(group))
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"force_openai_fast":true`)
	require.Contains(t, string(adminJSON), `"free_openai_fast":true`)
}

func TestGroupMapperFastMultiplierOnlyExposesUniformTokenPricing(t *testing.T) {
	fast := 2.5
	imageFast := 9.0
	group := &service.Group{ModelPricing: []service.ChannelModelPricing{
		{BillingMode: service.BillingModeToken, FastMultiplier: &fast},
		{BillingMode: service.BillingModeToken, FastMultiplier: &fast},
		{BillingMode: service.BillingModeImage, FastMultiplier: &imageFast},
	}}
	got := GroupFromService(group)
	require.NotNil(t, got.FastMultiplier)
	require.Equal(t, 2.5, *got.FastMultiplier)

	group.ModelPricing[1].FastMultiplier = nil
	require.Nil(t, GroupFromService(group).FastMultiplier)

	group.ModelPricing = []service.ChannelModelPricing{{BillingMode: service.BillingModeImage, FastMultiplier: &fast}}
	require.Nil(t, GroupFromService(group).FastMultiplier)
}
