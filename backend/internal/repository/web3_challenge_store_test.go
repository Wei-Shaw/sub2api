package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestWeb3ChallengeStoreConsumeOnce(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewWeb3ChallengeStore(client)
	ctx := context.Background()
	challenge := service.Web3Challenge{
		Intent:        "login",
		Message:       "message",
		MessageDigest: "digest",
		ExpiresAt:     time.Now().UTC().Add(time.Minute),
	}

	require.NoError(t, store.Save(ctx, "token", challenge, time.Minute))
	loaded, err := store.Get(ctx, "token")
	require.NoError(t, err)
	require.Equal(t, challenge.MessageDigest, loaded.MessageDigest)

	consumed, err := store.Consume(ctx, "token", "digest")
	require.NoError(t, err)
	require.True(t, consumed)

	consumed, err = store.Consume(ctx, "token", "digest")
	require.NoError(t, err)
	require.False(t, consumed)
	_, err = store.Get(ctx, "token")
	require.True(t, errors.Is(err, service.ErrWeb3ChallengeNotFound))
}

func TestWeb3ChallengeStoreDeletesAfterThreeFailures(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewWeb3ChallengeStore(client)
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, "token", service.Web3Challenge{MessageDigest: "digest"}, time.Minute))
	require.NoError(t, store.RecordFailure(ctx, "token"))
	require.NoError(t, store.RecordFailure(ctx, "token"))
	_, err := store.Get(ctx, "token")
	require.NoError(t, err)
	require.NoError(t, store.RecordFailure(ctx, "token"))
	_, err = store.Get(ctx, "token")
	require.True(t, errors.Is(err, service.ErrWeb3ChallengeNotFound))
}
