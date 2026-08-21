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
REDACTED

func (h *cyberRedisCommandHook) DialHook(next redis.DialHook) redis.DialHook { return next REDACTED

func (h *cyberRedisCommandHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "mget" {
			h.mu.Lock()
			h.mgetKeyCounts = append(h.mgetKeyCounts, len(cmd.Args())-1)
			h.mu.Unlock()
	REDACTED
		return next(ctx, cmd)
REDACTED
REDACTED

func (h *cyberRedisCommandHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		setCount := 0
		for _, cmd := range cmds {
			if cmd.Name() == "set" {
				setCount++
		REDACTED
	REDACTED
		if setCount > 0 {
			h.mu.Lock()
			h.setBatchSizes = append(h.setBatchSizes, setCount)
			h.mu.Unlock()
	REDACTED
		return next(ctx, cmds)
REDACTED
REDACTED

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

func TestGatewayCacheCyberBlockCommandsAreBoundedAndLookupShortCircuits(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()REDACTED)
	t.Cleanup(func() { _ = client.Close() REDACTED)
	hook := &cyberRedisCommandHook{REDACTED
	client.AddHook(hook)
	store, ok := NewGatewayCache(client).(service.CyberSessionBlockStore)
	require.True(t, ok)

	keys := make([]string, cyberSessionRedisCommandMaxKeys*2+44)
	for i := range keys {
		keys[i] = "block-" + strconv.Itoa(i)
REDACTED
	ctx := context.Background()
	require.NoError(t, store.SetCyberSessionBlocked(ctx, "large-scope", keys, time.Minute))
	require.Equal(t, []int{cyberSessionRedisCommandMaxKeys, cyberSessionRedisCommandMaxKeys, 44REDACTED, hook.setBatchSizes)

	lookup := make([]string, len(keys))
	for i := range lookup {
		lookup[i] = "missing-" + strconv.Itoa(i)
REDACTED
	lookup[cyberSessionRedisCommandMaxKeys+3] = keys[cyberSessionRedisCommandMaxKeys+3]
	matched, err := store.FindCyberSessionBlocked(ctx, lookup)
REDACTED
	require.Equal(t, keys[cyberSessionRedisCommandMaxKeys+3], matched)
	require.Equal(t, []int{cyberSessionRedisCommandMaxKeys, cyberSessionRedisCommandMaxKeysREDACTED, hook.mgetKeyCounts)
REDACTED
