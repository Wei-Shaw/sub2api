package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInvalidateRuntimeCaches_ExpiresAllProcessLocalSnapshots(t *testing.T) {
	future := time.Now().Add(time.Hour).UnixNano()
	versionBoundsCache.Store(&cachedVersionBounds{min: "1.0.0", max: "2.0.0", expiresAt: future})
	backendModeCache.Store(&cachedBackendMode{value: true, expiresAt: future})
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		fingerprintUnification: true,
		metadataPassthrough:    true,
		cchSigning:             true,
		expiresAt:              future,
	})

	service := &SettingService{}
	service.InvalidateRuntimeCaches()

	versionBounds := versionBoundsCache.Load().(*cachedVersionBounds)
	backendMode := backendModeCache.Load().(*cachedBackendMode)
	gatewayForwarding := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings)
	require.Equal(t, int64(0), versionBounds.expiresAt)
	require.Equal(t, int64(0), backendMode.expiresAt)
	require.Equal(t, int64(0), gatewayForwarding.expiresAt)
}
