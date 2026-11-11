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

func TestGatewayCacheLiveCallIdentityAndController(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	otherInstance, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	record := &service.LiveCallRecord{
		CallID:                "call_secret",
		CallHash:              HashLiveCallID("call_secret"),
		AccountID:             11,
		APIKeyID:              22,
		UserID:                33,
		GroupID:               44,
		LeaseID:               "lease",
		Model:                 "gpt-live-test",
		AttestationCiphertext: "encrypted-attestation",
		CreatedAt:             time.Now(),
		ExpiresAt:             time.Now().Add(time.Hour),
		Controller:            service.LiveControllerPending,
	}
	require.NoError(t, cache.SaveLiveCall(context.Background(), record, time.Hour))

	loaded, err := otherInstance.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, record.CallID, loaded.CallID)
	require.Equal(t, record.AccountID, loaded.AccountID)
	require.Equal(t, record.AttestationCiphertext, loaded.AttestationCiphertext)

	claimed, err := cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerObserver, "observer-1")
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerProxy, "proxy-1")
	require.NoError(t, err)
	require.True(t, claimed)
	controller, err := cache.GetLiveController(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, service.LiveControllerProxy, controller)

	released, err := cache.ReleaseLiveController(context.Background(), record.CallHash, "proxy-1")
	require.NoError(t, err)
	require.True(t, released)
	closed, err := cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.True(t, closed)
	closed, err = cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.False(t, closed)
}

func TestGatewayCacheLiveCallUpstreamRoutingAcrossInstances(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	writer, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	reader, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	ctx := context.Background()

	for _, mode := range []string{"device_check", "codex_oauth", "future_unknown_mode"} {
		t.Run(mode, func(t *testing.T) {
			record := &service.LiveCallRecord{
				CallID:            "call_" + mode,
				CallHash:          HashLiveCallID("call_" + mode),
				UpstreamAuthMode:  mode,
				UpstreamSessionID: "session_for_" + mode,
				Controller:        service.LiveControllerPending,
				CreatedAt:         time.Now(),
				ExpiresAt:         time.Now().Add(time.Hour),
			}
			require.NoError(t, writer.SaveLiveCall(ctx, record, time.Hour))

			loaded, err := reader.GetLiveCall(ctx, record.CallHash)
			require.NoError(t, err)
			// The store preserves the creation-time route; the service decides
			// whether a mode is supported instead of silently selecting a default.
			require.Equal(t, record.UpstreamAuthMode, loaded.UpstreamAuthMode)
			require.Equal(t, record.UpstreamSessionID, loaded.UpstreamSessionID)
		})
	}
}

func TestGatewayCacheLiveCallReadsLegacyRecordWithoutRoutingFields(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	cache, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	ctx := context.Background()
	callHash := HashLiveCallID("legacy_call")
	key := liveCallKey(callHash)
	// Write the pre-upgrade Redis shape directly so new serializer defaults
	// cannot accidentally hide a compatibility regression.
	require.NoError(t, client.HSet(ctx, key, map[string]any{
		"call_id":     "legacy_call",
		"account_id":  11,
		"api_key_id":  22,
		"user_id":     33,
		"controller":  service.LiveControllerPending,
		"attestation": "legacy-encrypted-attestation",
	}).Err())
	require.NoError(t, client.Expire(ctx, key, time.Hour).Err())

	loaded, err := cache.GetLiveCall(ctx, callHash)
	require.NoError(t, err)
	require.Equal(t, "legacy_call", loaded.CallID)
	require.Equal(t, int64(11), loaded.AccountID)
	require.Equal(t, "legacy-encrypted-attestation", loaded.AttestationCiphertext)
	require.Empty(t, loaded.UpstreamAuthMode)
	require.Empty(t, loaded.UpstreamSessionID)
	// A read must not turn an old record into a new OAuth record or mutate it.
	hasMode, err := client.HExists(ctx, key, "upstream_auth_mode").Result()
	require.NoError(t, err)
	require.False(t, hasMode)
	hasSession, err := client.HExists(ctx, key, "upstream_session_id").Result()
	require.NoError(t, err)
	require.False(t, hasSession)
}

func TestGatewayCacheLiveCallReplacementClearsUpstreamRouting(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	cache, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	ctx := context.Background()
	record := &service.LiveCallRecord{
		CallID:            "replacement_call",
		CallHash:          HashLiveCallID("replacement_call"),
		UpstreamAuthMode:  "codex_oauth",
		UpstreamSessionID: "session_to_clear",
		Controller:        service.LiveControllerPending,
	}
	require.NoError(t, cache.SaveLiveCall(ctx, record, time.Hour))

	record.UpstreamAuthMode = ""
	record.UpstreamSessionID = ""
	record.AttestationCiphertext = "legacy-encrypted-attestation"
	require.NoError(t, cache.SaveLiveCall(ctx, record, time.Hour))
	loaded, err := cache.GetLiveCall(ctx, record.CallHash)
	require.NoError(t, err)
	require.Empty(t, loaded.UpstreamAuthMode)
	require.Empty(t, loaded.UpstreamSessionID)
	require.Equal(t, record.AttestationCiphertext, loaded.AttestationCiphertext)
}
