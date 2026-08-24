package repository

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type cyberRedisCommandHook struct {
	mu            sync.Mutex
	mgetKeyCounts []int
	setBatchSizes []int
}

func (h *cyberRedisCommandHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *cyberRedisCommandHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "mget" {
			h.mu.Lock()
			h.mgetKeyCounts = append(h.mgetKeyCounts, len(cmd.Args())-1)
			h.mu.Unlock()
		}
		return next(ctx, cmd)
	}
}

func (h *cyberRedisCommandHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		setCount := 0
		for _, cmd := range cmds {
			if cmd.Name() == "set" {
				setCount++
			}
		}
		if setCount > 0 {
			h.mu.Lock()
			h.setBatchSizes = append(h.setBatchSizes, setCount)
			h.mu.Unlock()
		}
		return next(ctx, cmds)
	}
}

func TestGatewayCacheCyberBlockWritesScopeAndExactKeysTogether(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewGatewayCache(client).(service.CyberSessionBlockStore)
	require.True(t, ok)

	ctx := context.Background()
	require.NoError(t, store.SetCyberSessionBlocked(ctx, "scope-1", []string{"block-1", "block-2"}, time.Minute))
	active, err := store.IsCyberSessionScopeActive(ctx, "scope-1")
	require.NoError(t, err)
	require.True(t, active)
	matched, err := store.FindCyberSessionBlocked(ctx, []string{"missing", "block-1", "block-2"})
	require.NoError(t, err)
	require.Equal(t, "block-1", matched)
	require.Greater(t, server.TTL(cyberSessionScopePrefix+"scope-1"), time.Duration(0))
	require.Equal(t, server.TTL(cyberSessionBlockPrefix+"block-1"), server.TTL(cyberSessionBlockPrefix+"block-2"))
}

func TestGatewayCacheCyberBlockCommandsAreBoundedAndLookupShortCircuits(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	hook := &cyberRedisCommandHook{}
	client.AddHook(hook)
	store, ok := NewGatewayCache(client).(service.CyberSessionBlockStore)
	require.True(t, ok)

	keys := make([]string, cyberSessionRedisCommandMaxKeys*2+44)
	for i := range keys {
		keys[i] = "block-" + strconv.Itoa(i)
	}
	ctx := context.Background()
	require.NoError(t, store.SetCyberSessionBlocked(ctx, "large-scope", keys, time.Minute))
	require.Equal(t, []int{cyberSessionRedisCommandMaxKeys, cyberSessionRedisCommandMaxKeys, 44}, hook.setBatchSizes)

	lookup := make([]string, len(keys))
	for i := range lookup {
		lookup[i] = "missing-" + strconv.Itoa(i)
	}
	lookup[cyberSessionRedisCommandMaxKeys+3] = keys[cyberSessionRedisCommandMaxKeys+3]
	matched, err := store.FindCyberSessionBlocked(ctx, lookup)
	require.NoError(t, err)
	require.Equal(t, keys[cyberSessionRedisCommandMaxKeys+3], matched)
	require.Equal(t, []int{cyberSessionRedisCommandMaxKeys, cyberSessionRedisCommandMaxKeys}, hook.mgetKeyCounts)
}
