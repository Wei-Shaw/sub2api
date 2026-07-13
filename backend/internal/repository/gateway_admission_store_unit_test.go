//go:build unit

package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGatewayAdmissionStoreExposesConfiguredLeaseTTL(t *testing.T) {
	store := NewGatewayAdmissionStore(nil, 9*time.Second)
	provider, ok := store.(interface {
		GatewayAdmissionLeaseTTL() time.Duration
	})

	require.True(t, ok)
	require.Equal(t, 9*time.Second, provider.GatewayAdmissionLeaseTTL())
}
