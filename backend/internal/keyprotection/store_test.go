package keyprotection

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func testMappingStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisStore(client, strings.Repeat("12", 32)), mr
}

func TestProtectionStoreEncryptionIsolationAndContinuation(t *testing.T) {
	ctx := context.Background()
	store, mr := testMappingStore(t)
	cfg := DefaultConfig()
	cfg.Enabled = true
	scope := Scope{UserID: 1, APIKeyID: 2, GroupID: 3}
	secret := "ghp_" + strings.Repeat("F", 36)
	body := []byte(`{"model":"fake","input":"use ` + secret + `"}`)
	out, state, err := store.Protect(ctx, scope, "shared-session", "", cfg, body, "responses")
	require.NoError(t, err)
	require.NotContains(t, string(out), secret)
	require.NoError(t, store.SaveResponse(ctx, scope, "resp_fake", state))
	for _, key := range mr.Keys() {
		if strings.HasSuffix(key, ":index") {
			continue
		}
		value, err := mr.Get(key)
		require.NoError(t, err)
		require.NotContains(t, value, secret)
		require.NotContains(t, value, "keyx_")
	}
	// Another process with the same platform key resumes encrypted state.
	other := NewRedisStore(store.client, strings.Repeat("12", 32))
	seed1, err := store.Seed(scope, "shared-session")
	require.NoError(t, err)
	seed2, err := other.Seed(scope, "shared-session")
	require.NoError(t, err)
	require.Equal(t, seed1, seed2)
	_, resumed, err := other.Protect(ctx, scope, "", "resp_fake", cfg, []byte(`{"input":"continue"}`), "responses")
	require.NoError(t, err)
	require.Equal(t, state.Entries(), resumed.Entries())
	for token := range state.Entries() {
		require.Equal(t, secret, resumed.RestoreText(token))
	}
	for _, foreign := range []Scope{{2, 2, 3}, {1, 9, 3}, {1, 2, 4}} {
		_, _, err := store.Protect(ctx, foreign, "shared-session", "resp_fake", cfg, body, "responses")
		require.ErrorIs(t, err, ErrExpired)
		_, otherState, err := store.Protect(ctx, foreign, "shared-session", "", cfg, body, "responses")
		require.NoError(t, err)
		require.NotEqual(t, state.Entries(), otherState.Entries())
		for token := range state.Entries() {
			require.Equal(t, token, otherState.RestoreText(token))
		}
	}
	mr.FastForward(time.Duration(cfg.TTLSeconds+1) * time.Second)
	_, _, err = store.Protect(ctx, scope, "", "resp_fake", cfg, body, "responses")
	require.ErrorIs(t, err, ErrExpired)
}

func TestProtectionStoreStableHashConcurrentReuseAndCapacity(t *testing.T) {
	ctx := context.Background()
	store, _ := testMappingStore(t)
	cfg := DefaultConfig()
	cfg.Enabled = true
	scope := Scope{1, 2, 3}
	secret := "ghp_" + strings.Repeat("H", 36)
	body := []byte(`{"messages":[{"role":"user","content":"` + secret + `"}]}`)
	var wg sync.WaitGroup
	results := make(chan string, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _, err := store.Protect(ctx, scope, "same", "", cfg, body, "chat")
			failures <- err
			results <- string(out)
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	first := ""
	for result := range results {
		if first == "" {
			first = result
		}
		require.Equal(t, first, result)
	}
	out, state, err := store.Protect(ctx, scope, "same", "", cfg, body, "chat")
	require.NoError(t, err)
	require.Equal(t, first, string(out))
	require.Len(t, state.Entries(), 1)
	outOther, _, err := store.Protect(ctx, scope, "another", "", cfg, body, "chat")
	require.NoError(t, err)
	require.NotEqual(t, string(out), string(outOther))
	cfg.MaxMappings = 1
	_, _, err = store.Protect(ctx, scope, "same", "", cfg, []byte(`{"messages":[{"role":"user","content":"ghp_`+strings.Repeat("J", 36)+`"}]}`), "chat")
	require.ErrorIs(t, err, ErrCapacity)
	cfg.MaxSessions = 2
	_, _, err = store.Protect(ctx, scope, "third", "", cfg, body, "chat")
	require.ErrorIs(t, err, ErrCapacity)
	require.NoError(t, store.Purge(ctx))
	_, _, err = store.Protect(ctx, scope, "third", "", cfg, body, "chat")
	require.NoError(t, err)
}

func TestProtectionStoreTamperFailureAndCancellation(t *testing.T) {
	ctx := context.Background()
	store, mr := testMappingStore(t)
	cfg := DefaultConfig()
	cfg.Enabled = true
	scope := Scope{1, 2, 3}
	body := []byte(`{"input":"ghp_` + strings.Repeat("K", 36) + `"}`)
	_, state, err := store.Protect(ctx, scope, "", "", cfg, body, "responses")
	require.NoError(t, err)
	require.NoError(t, store.SaveResponse(ctx, scope, "resp_one", state))
	prefix, err := scopePrefix(scope)
	require.NoError(t, err)
	key := mappingKey(prefix, "response", "resp_one")
	value, err := mr.Get(key)
	require.NoError(t, err)
	foreignPrefix, err := scopePrefix(Scope{2, 2, 3})
	require.NoError(t, err)
	require.NoError(t, mr.Set(mappingKey(foreignPrefix, "response", "resp_one"), value))
	_, _, err = store.Protect(ctx, Scope{2, 2, 3}, "", "resp_one", cfg, body, "responses")
	require.ErrorIs(t, err, ErrStore)
	wrongMaster := NewRedisStore(store.client, strings.Repeat("13", 32))
	_, _, err = wrongMaster.Protect(ctx, scope, "", "resp_one", cfg, body, "responses")
	require.ErrorIs(t, err, ErrStore)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, _, err = store.Protect(canceled, scope, "", "", cfg, body, "responses")
	require.ErrorIs(t, err, ErrStore)
	invalid := NewRedisStore(store.client, "")
	_, _, err = invalid.Protect(ctx, scope, "", "", cfg, body, "responses")
	require.ErrorIs(t, err, ErrStore)
}
