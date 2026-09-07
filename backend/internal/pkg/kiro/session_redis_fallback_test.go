//go:build unit

package kiro

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// TestSessionStoreLocalOnlySurvivesRedisRecovery reproduces the task-11
// review finding: Redis is unreachable exactly while Set() runs, so the
// session falls back to memory (fine, by design). Redis then recovers
// before the browser-based OAuth callback arrives — very plausible, since a
// human has to complete an SSO login in between (tens of seconds to
// minutes). Because that session was never actually written to Redis, a
// naive query against the now-reachable Redis truthfully answers "not
// found". Get/TryConsume must still resolve the session from memory via the
// localOnly marker, or a valid callback gets rejected as if the session
// never existed.
func TestSessionStoreLocalOnlySurvivesRedisRecovery(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	store := NewRedisSessionStore(client)
	defer store.Stop()

	ctx := context.Background()
	sess := &OAuthSession{
		Method:    AuthIdC,
		State:     "st",
		ExpiresAt: time.Now().Add(SessionTTL),
	}

	// Redis is unreachable exactly while Set() runs.
	mr.Close()
	store.Set(ctx, "sid", sess)
	require.True(t, store.isLocalOnly("sid"), "a failed Redis write must mark the session localOnly")

	// Redis recovers before the callback arrives. It never received "sid",
	// so it will truthfully report "not found" for it.
	require.NoError(t, mr.Restart())

	got, ok := store.Get(ctx, "sid")
	require.True(t, ok, "Get must resolve a localOnly session from memory, not from a reachable-but-empty Redis")
	require.Equal(t, "st", got.State)

	require.True(t, store.TryConsume(ctx, "sid"), "TryConsume must resolve a localOnly session from memory, not ask a now-reachable-but-empty Redis")
	require.False(t, store.TryConsume(ctx, "sid"), "a consumed localOnly session must not be consumable twice")
}

// TestSessionStoreDeleteClearsLocalOnlyMarker confirms Delete() cleans up
// the localOnly bookkeeping map alongside the session, mirroring pkg/xai's
// SessionStore.Delete.
func TestSessionStoreDeleteClearsLocalOnlyMarker(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	store := NewRedisSessionStore(client)
	defer store.Stop()

	ctx := context.Background()
	mr.Close()
	store.Set(ctx, "sid", &OAuthSession{Method: AuthIdC, ExpiresAt: time.Now().Add(SessionTTL)})
	require.True(t, store.isLocalOnly("sid"))

	require.NoError(t, mr.Restart())
	store.Delete(ctx, "sid")
	require.False(t, store.isLocalOnly("sid"), "Delete must clear the localOnly marker along with the session")

	_, ok := store.Get(ctx, "sid")
	require.False(t, ok)
}

// TestSessionStoreSuccessfulRedisWriteIsNotLocalOnly confirms a session
// whose Redis write succeeded is not marked localOnly, so Get/TryConsume
// keep trusting Redis as authoritative for it, same as before this fix.
func TestSessionStoreSuccessfulRedisWriteIsNotLocalOnly(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	store := NewRedisSessionStore(client)
	defer store.Stop()

	ctx := context.Background()
	store.Set(ctx, "sid", &OAuthSession{Method: AuthIdC, State: "remote", ExpiresAt: time.Now().Add(SessionTTL)})
	require.False(t, store.isLocalOnly("sid"), "a successful Redis write must not be marked localOnly")

	got, ok := store.Get(ctx, "sid")
	require.True(t, ok)
	require.Equal(t, "remote", got.State)
}
