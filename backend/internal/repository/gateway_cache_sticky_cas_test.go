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

func TestGatewayCacheCompareAndBindSessionAccountID(t *testing.T) {
	const (
		groupID    = int64(7)
		session    = "cas-session"
		account    = int64(9401)
		other      = int64(9499)
		neither    = int64(8123)
		bindTTL    = 10 * time.Minute
		initialTTL = 2 * time.Minute
	)
	for _, tt := range []struct {
		name        string
		current     int64 // 0 = 键不存在
		replaceable []int64
		write       bool
		// refresh keeps the value and only renews the TTL.
		refresh   bool
		wantValue int64
	}{
		{name: "missing key is written", write: true, wantValue: account},
		{name: "account itself only refreshes", current: account, refresh: true, wantValue: account},
		{name: "replaceable account is replaced", current: other, replaceable: []int64{neither, other}, write: true, wantValue: account},
		{name: "third account is left alone", current: other, replaceable: []int64{neither}, wantValue: other},
		{name: "third account without replaceables is left alone", current: other, wantValue: other},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			cache := NewGatewayCache(client)
			comparable, ok := cache.(service.GatewayStickySessionCompareCache)
			require.True(t, ok, "the Redis cache must expose the compare capability")
			ctx := context.Background()
			if tt.current != 0 {
				require.NoError(t, cache.SetSessionAccountID(ctx, groupID, session, tt.current, initialTTL))
			}

			bound, err := comparable.CompareAndBindSessionAccountID(ctx, groupID, session, account, tt.replaceable, bindTTL)
			require.NoError(t, err)
			require.Equal(t, tt.write || tt.refresh, bound, "the write must be decided on the current value alone")

			got, err := cache.GetSessionAccountID(ctx, groupID, session)
			require.NoError(t, err)
			require.Equal(t, tt.wantValue, got)
			ttl := mr.TTL(buildSessionKey(groupID, session))
			if tt.write || tt.refresh {
				require.Greater(t, ttl, bindTTL-time.Minute, "a write must carry the new TTL")
				require.LessOrEqual(t, ttl, bindTTL)
				return
			}
			require.Greater(t, ttl, initialTTL-time.Minute, "a left-alone key keeps its TTL")
			require.LessOrEqual(t, ttl, initialTTL)
		})
	}
}
