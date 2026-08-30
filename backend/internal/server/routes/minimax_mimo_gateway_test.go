package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMinimaxMiMoUseOpenAIResponsesGateway(t *testing.T) {
	t.Parallel()

	require.True(t, isOpenAIResponsesCompatiblePlatform(service.PlatformMinimax))
	require.True(t, isOpenAIResponsesCompatiblePlatform(service.PlatformMiMo))
	require.False(t, isOpenAIResponsesCompatiblePlatform(service.PlatformAnthropic))
}
