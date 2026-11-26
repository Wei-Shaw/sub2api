//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelIsResponseModelExposed(t *testing.T) {
	t.Parallel()

	require.False(t, (*Channel)(nil).IsResponseModelExposed())
	require.False(t, (&Channel{}).IsResponseModelExposed())
	require.False(t, (&Channel{FeaturesConfig: map[string]any{featureKeyExposeResponseModel: false}}).IsResponseModelExposed())
	require.False(t, (&Channel{FeaturesConfig: map[string]any{featureKeyExposeResponseModel: "true"}}).IsResponseModelExposed())
	require.True(t, (&Channel{FeaturesConfig: map[string]any{featureKeyExposeResponseModel: true}}).IsResponseModelExposed())
}

func TestChannelService_ResponseModelExposedChannels(t *testing.T) {
	t.Parallel()

	channels := []Channel{
		{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{featureKeyExposeResponseModel: true}},
		{ID: 2, Status: StatusActive, FeaturesConfig: map[string]any{featureKeyExposeResponseModel: false}},
		{ID: 3, Status: StatusActive},
	}
	svc := &ChannelService{}
	svc.cache.Store(populateChannelCache(channels, map[int64]string{}))

	got := svc.ResponseModelExposedChannels(context.Background(), []int64{1, 2, 3, 404})
	require.Equal(t, map[int64]bool{1: true}, got)

	require.Nil(t, svc.ResponseModelExposedChannels(context.Background(), nil))
	require.Nil(t, (*ChannelService)(nil).ResponseModelExposedChannels(context.Background(), []int64{1}))
}
