//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListStorefrontModels_DisabledFallsBack(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10},
		RestrictModels: false,
		ModelPricing: []ChannelModelPricing{
			{Platform: "antigravity", Models: []string{"gemini-3.7-flash"}},
		},
	}
	svc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "antigravity"}))

	models, enabled := svc.ListStorefrontModels(context.Background(), 10, "antigravity")
	require.False(t, enabled)
	require.Nil(t, models)
}

func TestListStorefrontModels_RestrictOnReturnsPricedModelsOnly(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10},
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "antigravity", Models: []string{"gemini-3.7-flash", "gemini-3.7-flash-high"}},
			{Platform: "openai", Models: []string{"gpt-5.4"}},
		},
	}
	svc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "antigravity"}))

	models, enabled := svc.ListStorefrontModels(context.Background(), 10, "antigravity")
	require.True(t, enabled)
	require.Equal(t, []string{"gemini-3.7-flash", "gemini-3.7-flash-high"}, models)

	models, enabled = svc.ListStorefrontModels(context.Background(), 10, "openai")
	require.True(t, enabled)
	require.Equal(t, []string{"gpt-5.4"}, models)
}

func TestListStorefrontModels_EmptyShelfStaysEmpty(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10},
		RestrictModels: true,
	}
	svc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "antigravity"}))

	models, enabled := svc.ListStorefrontModels(context.Background(), 10, "antigravity")
	require.True(t, enabled)
	require.Empty(t, models)
}

func TestGetAvailableModels_UsesChannelStorefrontWhenRestricted(t *testing.T) {
	ch := Channel{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{10},
		RestrictModels: true,
		ModelPricing: []ChannelModelPricing{
			{Platform: "antigravity", Models: []string{"gemini-3.7-flash-tiered"}},
		},
	}
	channelSvc := newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "antigravity"}))
	gw := &GatewayService{channelService: channelSvc}
	groupID := int64(10)

	got := gw.GetAvailableModels(context.Background(), &groupID, "antigravity")
	require.Equal(t, []string{"gemini-3.7-flash-tiered"}, got)
}
