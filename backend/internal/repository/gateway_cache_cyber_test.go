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

func TestGatewayCacheCyberBlockWritesScopeAndExactKeysTogether(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()REDACTED)
	t.Cleanup(func() { _ = client.Close() REDACTED)
	store, ok := NewGatewayCache(client).(service.CyberSessionBlockStore)
	require.True(t, ok)

	ctx := context.Background()
	require.NoError(t, store.SetCyberSessionBlocked(ctx, "scope-1", []string{"block-1", "block-2"REDACTED, time.Minute))
	active, err := store.IsCyberSessionScopeActive(ctx, "scope-1")
REDACTED
	require.True(t, active)
	matched, err := store.FindCyberSessionBlocked(ctx, []string{"missing", "block-1", "block-2"REDACTED)
REDACTED
	require.Equal(t, "block-1", matched)
	require.Greater(t, server.TTL(cyberSessionScopePrefix+"scope-1"), time.Duration(0))
	require.Equal(t, server.TTL(cyberSessionBlockPrefix+"block-1"), server.TTL(cyberSessionBlockPrefix+"block-2"))
REDACTED
