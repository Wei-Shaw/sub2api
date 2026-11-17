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

func TestGatewayCacheCyberBlockWritesScopeLengthsAndExactKeysTogether(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewGatewayCache(client).(service.CyberSessionBlockStore)
	require.True(t, ok)

	ctx := context.Background()
	lengths, err := store.CyberSessionTranscriptLengths(ctx, 7, "scope-1")
	require.NoError(t, err)
	require.Empty(t, lengths)

	require.NoError(t, store.SetCyberSessionBlocked(ctx, service.CyberSessionBlock{
		APIKeyID: 7, ScopeKey: "scope-1", Keys: []string{"block-1", "block-2"}, TranscriptLengths: []int{3, 2},
	}, time.Minute))
	require.NoError(t, store.SetCyberSessionBlocked(ctx, service.CyberSessionBlock{
		APIKeyID: 7, ScopeKey: "scope-2", Keys: []string{"block-3"}, TranscriptLengths: []int{3, 40},
	}, time.Minute))
	lengths, err = store.CyberSessionTranscriptLengths(ctx, 7, "scope-1")
	require.NoError(t, err)
	require.ElementsMatch(t, []int{2, 3, 40}, lengths, "lengths are indexed per API key, like the block keys")
	lengths, err = store.CyberSessionTranscriptLengths(ctx, 7, "inactive-scope")
	require.NoError(t, err)
	require.Empty(t, lengths)
	lengths, err = store.CyberSessionTranscriptLengths(ctx, 8, "scope-1")
	require.NoError(t, err)
	require.Empty(t, lengths)

	matched, err := store.FindCyberSessionBlocked(ctx, []string{"missing", "block-1", "block-2"})
	require.NoError(t, err)
	require.Equal(t, "block-1", matched)
	require.Greater(t, server.TTL(cyberSessionScopePrefix+"scope-1"), time.Duration(0))
	require.Greater(t, server.TTL(cyberSessionTranscriptLengthsKey(7)), time.Duration(0))
	require.Equal(t, server.TTL(cyberSessionBlockPrefix+"block-1"), server.TTL(cyberSessionBlockPrefix+"block-2"))
}

func TestGatewayCacheCyberBlockWithoutScopeSkipsTranscriptIndex(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, ok := NewGatewayCache(client).(service.CyberSessionBlockStore)
	require.True(t, ok)

	ctx := context.Background()
	require.NoError(t, store.SetCyberSessionBlocked(ctx, service.CyberSessionBlock{APIKeyID: 7, Keys: []string{"explicit"}, TranscriptLengths: []int{3}}, time.Minute))
	require.False(t, server.Exists(cyberSessionTranscriptLengthsKey(7)))
	matched, err := store.FindCyberSessionBlocked(ctx, []string{"explicit"})
	require.NoError(t, err)
	require.Equal(t, "explicit", matched)
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
	require.NoError(t, store.SetCyberSessionBlocked(ctx, service.CyberSessionBlock{APIKeyID: 7, ScopeKey: "large-scope", Keys: keys}, time.Minute))
	// Exact blocks in bounded batches, then the scope activation transaction.
	require.Equal(t, []int{cyberSessionRedisCommandMaxKeys, cyberSessionRedisCommandMaxKeys, 44, 1}, hook.setBatchSizes)

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
