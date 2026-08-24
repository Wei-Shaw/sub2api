package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCodexDeviceConversationLeaseRedisOwnershipAndExpiry(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	leases, ok := regular.(service.CodexDeviceConversationLeaseCache)
	require.True(t, ok)
	ctx := context.Background()
	const slotKey = "account:91|profile:windows/desktop/x86_64|slot:0|epoch:4"

	acquired, err := leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-a")
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.False(t, acquired, "a second process cannot acquire the same device slot")

	refreshed, err := leases.RefreshCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.False(t, refreshed, "a non-owner cannot refresh the lease")
	require.NoError(t, leases.ReleaseCodexDeviceConversationLease(ctx, slotKey, "owner-b"))
	acquired, err = leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.False(t, acquired, "a non-owner release cannot delete the lease")

	refreshed, err = leases.RefreshCodexDeviceConversationLease(ctx, slotKey, "owner-a")
	require.NoError(t, err)
	require.True(t, refreshed)
	require.NoError(t, leases.ReleaseCodexDeviceConversationLease(ctx, slotKey, "owner-a"))
	acquired, err = leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.True(t, acquired, "capacity must return immediately after owner release")

	redisServer.FastForward((codexDeviceConversationLeaseTTLSeconds + 1) * time.Second)
	refreshed, err = leases.RefreshCodexDeviceConversationLease(ctx, slotKey, "owner-b")
	require.NoError(t, err)
	require.False(t, refreshed)
	acquired, err = leases.AcquireCodexDeviceConversationLease(ctx, slotKey, "owner-c")
	require.NoError(t, err)
	require.True(t, acquired, "expired leases must not strand a device slot")
}
