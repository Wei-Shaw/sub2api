package kiro

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGenerateSessionIDIsUniqueAndURLSafe(t *testing.T) {
	t.Parallel()

	a, err := GenerateSessionID()
	require.NoError(t, err)
	require.NotEmpty(t, a)
	require.NotContains(t, a, "=")
	require.NotContains(t, a, "/")
	require.NotContains(t, a, "+")

	b, err := GenerateSessionID()
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestSessionStoreSetGetDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewSessionStore()
	defer store.Stop()

	sess := &OAuthSession{
		Method:      AuthIdC,
		ClientID:    "cid",
		Verifier:    "ver",
		State:       "st",
		Region:      "us-east-1",
		IssuerURL:   "https://d-90667b4f8e.awsapps.com/start",
		RedirectURI: "https://gw.example.com/cb",
		ExpiresAt:   time.Now().Add(SessionTTL),
	}
	store.Set(ctx, "sid-1", sess)

	got, ok := store.Get(ctx, "sid-1")
	require.True(t, ok)
	require.Equal(t, AuthIdC, got.Method)
	require.Equal(t, "ver", got.Verifier)
	require.Equal(t, "https://d-90667b4f8e.awsapps.com/start", got.IssuerURL)

	store.Delete(ctx, "sid-1")
	_, ok = store.Get(ctx, "sid-1")
	require.False(t, ok)
}

func TestSessionStoreGetMissing(t *testing.T) {
	t.Parallel()

	store := NewSessionStore()
	defer store.Stop()

	_, ok := store.Get(context.Background(), "nope")
	require.False(t, ok)
}

func TestSessionStoreExpiredSessionIsNotReturned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewSessionStore()
	defer store.Stop()

	store.Set(ctx, "old", &OAuthSession{
		Method:    AuthIdC,
		ExpiresAt: time.Now().Add(-time.Minute),
	})

	_, ok := store.Get(ctx, "old")
	require.False(t, ok, "过期会话不得返回")
}

// TestSessionStoreTryConsumeIsSingleUse 保证授权码只能兑换一次，
// 防止回调 URL 被重放。
func TestSessionStoreTryConsumeIsSingleUse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewSessionStore()
	defer store.Stop()

	store.Set(ctx, "sid", &OAuthSession{
		Method:    AuthIdC,
		ExpiresAt: time.Now().Add(SessionTTL),
	})

	require.True(t, store.TryConsume(ctx, "sid"), "首次消费应成功")
	require.False(t, store.TryConsume(ctx, "sid"), "重复消费必须失败")
}

func TestSessionStoreTryConsumeUnknownSession(t *testing.T) {
	t.Parallel()

	store := NewSessionStore()
	defer store.Stop()

	require.False(t, store.TryConsume(context.Background(), "never-existed"))
}

// TestSessionStoreTryConsumeLeavesLocalOnlyUntilDelete documents
// SessionStore's own contract: TryConsume alone does NOT clear the
// localOnly marker; only Delete does. This is a known, previously-ledgered
// gap — tryConsumeMemory only removes the memory entry, not the localOnly
// bookkeeping (see the comment above SessionStore and tryConsumeMemory in
// session.go). Callers that need both cleared (e.g. KiroOAuthService.
// ExchangeCode) must call Delete after a successful TryConsume.
func TestSessionStoreTryConsumeLeavesLocalOnlyUntilDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewSessionStore()
	defer store.Stop()

	// NewSessionStore has no Redis backing, so per Set's own doc comment
	// every session it stores is unconditionally marked localOnly.
	store.Set(ctx, "sid", &OAuthSession{
		Method:    AuthIdC,
		ExpiresAt: time.Now().Add(SessionTTL),
	})
	require.True(t, store.isLocalOnly("sid"), "Set on a Redis-less store must mark the session localOnly")

	require.True(t, store.TryConsume(ctx, "sid"), "first consume should succeed")
	require.True(t, store.isLocalOnly("sid"), "TryConsume alone must not clear the localOnly marker")

	store.Delete(ctx, "sid")
	require.False(t, store.isLocalOnly("sid"), "Delete must clear the localOnly marker")
}

func TestSessionStoreDeviceFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewSessionStore()
	defer store.Stop()

	store.Set(ctx, "dev", &OAuthSession{
		Method:       AuthBuilderID,
		ClientID:     "cid",
		ClientSecret: "csec",
		DeviceCode:   "dc",
		Interval:     5,
		Region:       "us-east-1",
		ExpiresAt:    time.Now().Add(SessionTTL),
	})

	got, ok := store.Get(ctx, "dev")
	require.True(t, ok)
	require.Equal(t, AuthBuilderID, got.Method)
	require.Equal(t, "dc", got.DeviceCode)
	require.Equal(t, 5, got.Interval)
	require.Equal(t, "csec", got.ClientSecret)
}
