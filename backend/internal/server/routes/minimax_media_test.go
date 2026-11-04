package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMiniMaxMediaCompositeEndpoints(t *testing.T) {
	require.Equal(t, service.CompositeRouteEndpointSpeech, compositeRouteEndpointForPath("/v1/t2a_v2"))
	require.Equal(t, service.CompositeRouteEndpointVideo, compositeRouteEndpointForPath("/v2/video_generation"))
	for _, model := range []string{"speech-2.8-hd", "speech-2.8-turbo", "MiniMax-H3"} {
		platform, ok := service.DetectModelPlatform(model)
		require.True(t, ok)
		require.Equal(t, service.PlatformMiniMax, platform)
	}
}
