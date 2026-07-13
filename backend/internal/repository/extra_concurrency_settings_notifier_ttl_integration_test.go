//go:build integration

package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestExtraConcurrencyAdmissionDrainExpiresWithoutSuccessfulReenablePublish(t *testing.T) {
	rdb := testRedis(t)
	notifier := &extraConcurrencySettingsNotifier{
		rdb:      rdb,
		drainTTL: 100 * time.Millisecond,
	}
	store := NewGatewayAdmissionStore(rdb, time.Minute)
	request := service.UserLeaseRequest{
		RequestID:     "gateway-after-missed-reenable",
		UserID:        1_111,
		StandardLimit: 1,
	}

	require.NoError(t, notifier.PublishExtraConcurrencySettingsState(t.Context(), false))
	blocked, err := store.TryAcquireUserLease(t.Context(), request)
	require.NoError(t, err)
	require.False(t, blocked.Acquired)
	require.True(t, blocked.Draining)

	require.Eventually(t, func() bool {
		admitted, acquireErr := store.TryAcquireUserLease(t.Context(), request)
		return acquireErr == nil && admitted.Acquired && !admitted.Draining
	}, time.Second, 20*time.Millisecond)
}
