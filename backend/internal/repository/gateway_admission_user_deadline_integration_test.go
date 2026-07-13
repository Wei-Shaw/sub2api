//go:build integration

package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayAdmissionStoreExpiredUserRequestCannotRenewItsAbsoluteDeadline(t *testing.T) {
	store := NewGatewayAdmissionStore(testRedis(t), time.Minute)
	const userID int64 = 7401

	activeRequest := service.UserLeaseRequest{
		RequestID:     "deadline-active",
		UserID:        userID,
		StandardLimit: 1,
		WaitTimeout:   time.Second,
	}
	active, err := store.TryAcquireUserLease(t.Context(), activeRequest)
	require.NoError(t, err)
	require.True(t, active.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseUserLease(t.Context(), userID, activeRequest.RequestID)
	})

	waitingRequest := activeRequest
	waitingRequest.RequestID = "deadline-waiter"
	waitingRequest.WaitTimeout = 50 * time.Millisecond
	waiting, err := store.TryAcquireUserLease(t.Context(), waitingRequest)
	require.NoError(t, err)
	require.False(t, waiting.Acquired)
	require.False(t, waiting.Expired)

	time.Sleep(80 * time.Millisecond)
	for range 2 {
		expired, err := store.TryAcquireUserLease(t.Context(), waitingRequest)
		require.NoError(t, err)
		require.False(t, expired.Acquired)
		require.True(t, expired.Expired,
			"retrying the same request ID must not create a fresh absolute deadline")
	}

	require.NoError(t, store.ReleaseUserLease(t.Context(), userID, waitingRequest.RequestID))
	requeued, err := store.TryAcquireUserLease(t.Context(), waitingRequest)
	require.NoError(t, err)
	require.False(t, requeued.Acquired)
	require.False(t, requeued.Expired,
		"explicit release clears the expired request tombstone for a deliberate new lifecycle")
}

func TestGatewayAdmissionStoreCleansCrashedUserMetadataAfterTombstoneGrace(t *testing.T) {
	const (
		userID   int64 = 7402
		leaseTTL       = 500 * time.Millisecond
	)
	rdb := testRedis(t)
	store := NewGatewayAdmissionStore(rdb, leaseTTL)
	crashedRequest := service.UserLeaseRequest{
		RequestID:     "deadline-crashed",
		UserID:        userID,
		StandardLimit: 1,
		WaitTimeout:   50 * time.Millisecond,
	}
	acquired, err := store.TryAcquireUserLease(t.Context(), crashedRequest)
	require.NoError(t, err)
	require.True(t, acquired.Acquired)

	keys := gatewayAdmissionUserLeaseKeys(userID)
	require.NoError(t, rdb.ZScore(t.Context(), keys[5], crashedRequest.RequestID).Err())
	require.NoError(t, rdb.ZScore(t.Context(), keys[6], crashedRequest.RequestID).Err())

	time.Sleep(650 * time.Millisecond)
	triggerRequest := crashedRequest
	triggerRequest.RequestID = "deadline-cleanup-trigger"
	triggerRequest.WaitTimeout = time.Second
	triggered, err := store.TryAcquireUserLease(t.Context(), triggerRequest)
	require.NoError(t, err)
	require.True(t, triggered.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseUserLease(t.Context(), userID, triggerRequest.RequestID)
	})

	require.ErrorIs(t, rdb.ZScore(t.Context(), keys[5], crashedRequest.RequestID).Err(), redis.Nil)
	require.ErrorIs(t, rdb.ZScore(t.Context(), keys[6], crashedRequest.RequestID).Err(), redis.Nil)
}
