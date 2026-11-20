//go:build unit

package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type livenessTrackingCache struct {
	stubConcurrencyCacheForTest
	calls         []string
	heartbeatErr  error
	heartbeatID   string
	heartbeatInst string
	cleanupPrefix string
}

func (c *livenessTrackingCache) HeartbeatProcess(_ context.Context, requestPrefix, instanceID string) error {
	c.calls = append(c.calls, "heartbeat")
	c.heartbeatID = requestPrefix
	c.heartbeatInst = instanceID
	return c.heartbeatErr
}

func (c *livenessTrackingCache) CleanupStaleProcessSlots(_ context.Context, prefix string) error {
	c.calls = append(c.calls, "cleanup")
	c.cleanupPrefix = prefix
	return nil
}

func TestCleanupStaleProcessSlots_RegistersHeartbeatBeforeCleanup(t *testing.T) {
	cache := &livenessTrackingCache{}
	svc := NewConcurrencyService(cache)

	require.NoError(t, svc.CleanupStaleProcessSlots(context.Background()))

	require.Equal(t, []string{"heartbeat", "cleanup"}, cache.calls)
	require.Equal(t, RequestIDPrefix(), cache.heartbeatID)
	require.Equal(t, RequestIDPrefix(), cache.cleanupPrefix)
	host, err := os.Hostname()
	require.NoError(t, err)
	require.Equal(t, host, cache.heartbeatInst)
}

func TestCleanupStaleProcessSlots_SkipsCleanupWhenHeartbeatFails(t *testing.T) {
	heartbeatErr := errors.New("redis unavailable")
	cache := &livenessTrackingCache{heartbeatErr: heartbeatErr}
	svc := NewConcurrencyService(cache)

	require.ErrorIs(t, svc.CleanupStaleProcessSlots(context.Background()), heartbeatErr)
	require.Equal(t, []string{"heartbeat"}, cache.calls, "never clean without first proving this process is registered")
}
