package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMinimaxMiMoDefaultModelCatalogs(t *testing.T) {
	t.Parallel()

	require.Equal(t, service.DefaultCNProviderModelIDs(service.PlatformMinimax), defaultModelIDsForPlatform(service.PlatformMinimax))
	require.Equal(t, service.DefaultCNProviderModelIDs(service.PlatformMiMo), defaultModelIDsForPlatform(service.PlatformMiMo))
	require.Contains(t, defaultModelIDsForPlatform(service.PlatformMinimax), "MiniMax-M3")
	require.Equal(t, []string{"mimo-v2.5", "mimo-v2.5-pro"}, defaultModelIDsForPlatform(service.PlatformMiMo))
}
