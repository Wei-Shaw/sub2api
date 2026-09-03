package service

import (
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestOAuthSessionOwnerStore_BindAssertUnbind(t *testing.T) {
	store := NewOAuthSessionOwnerStore()
	t.Cleanup(store.Stop)

	store.Bind("sess-1", 42)
	require.NoError(t, store.Assert("sess-1", 42))

	store.Unbind("sess-1")
	err := store.Assert("sess-1", 42)
	require.Error(t, err)
	require.Equal(t, "OAUTH_SESSION_NOT_FOUND", infraerrors.Reason(err))
}

func TestOAuthSessionOwnerStore_AssertMismatch(t *testing.T) {
	store := NewOAuthSessionOwnerStore()
	t.Cleanup(store.Stop)

	store.Bind("sess-2", 10)
	err := store.Assert("sess-2", 99)
	require.Error(t, err)
	require.Equal(t, "OAUTH_SESSION_FORBIDDEN", infraerrors.Reason(err))
}

func TestOAuthSessionOwnerStore_AssertMissing(t *testing.T) {
	store := NewOAuthSessionOwnerStore()
	t.Cleanup(store.Stop)

	err := store.Assert("missing", 1)
	require.Error(t, err)
	require.Equal(t, "OAUTH_SESSION_NOT_FOUND", infraerrors.Reason(err))
}

func TestOAuthSessionOwnerStore_Expired(t *testing.T) {
	store := NewOAuthSessionOwnerStore()
	t.Cleanup(store.Stop)
	store.ttl = 20 * time.Millisecond

	store.Bind("sess-exp", 7)
	time.Sleep(40 * time.Millisecond)
	err := store.Assert("sess-exp", 7)
	require.Error(t, err)
	require.Equal(t, "OAUTH_SESSION_NOT_FOUND", infraerrors.Reason(err))
}

func TestRejectProxyID(t *testing.T) {
	require.NoError(t, RejectProxyID(nil))
	id := int64(1)
	err := RejectProxyID(&id)
	require.Error(t, err)
	require.Equal(t, "PROXY_NOT_ALLOWED", infraerrors.Reason(err))
	zero := int64(0)
	require.NoError(t, RejectProxyID(&zero))
}
