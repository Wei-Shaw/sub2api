//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// messageThreadRedisEnv names an optional real Redis (host:port) to run the
// store tests against as well. They always run on miniredis; the binding is
// written by a Lua script, and a real server is the authority on what that
// script does. Each run uses its own key scope and leaves the server as it was.
const messageThreadRedisEnv = "MESSAGE_THREAD_TEST_REDIS_ADDR"

type messageThreadBackend struct {
	name  string
	store service.MessageThreadStore
	rdb   *redis.Client
	// scope is unique per test, so runs on a shared real server do not meet.
	scope string
	// advance moves time for key expiry; nil where it cannot be moved.
	advance func(time.Duration)
	fail    func(bool)
}

func (b messageThreadBackend) keys(t *testing.T) []string {
	t.Helper()
	keys, err := b.rdb.Keys(context.Background(), messageThreadKeyPrefix+"{"+b.scope+"}:*").Result()
	require.NoError(t, err)
	return keys
}

func forEachMessageThreadBackend(t *testing.T, run func(t *testing.T, b messageThreadBackend)) {
	t.Helper()
	scope := fmt.Sprintf("%d:2:3", time.Now().UnixNano())
	t.Run("miniredis", func(t *testing.T) {
		db := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: db.Addr(), MaxRetries: -1})
		t.Cleanup(func() { _ = client.Close() })
		store, ok := NewGatewayCache(client).(service.MessageThreadStore)
		require.True(t, ok, "the gateway cache is the shared message thread store")
		run(t, messageThreadBackend{name: "miniredis", store: store, rdb: client, scope: scope, advance: db.FastForward, fail: func(on bool) {
			if on {
				db.SetError("ERR unavailable")
			} else {
				db.SetError("")
			}
		}})
	})
	addr := os.Getenv(messageThreadRedisEnv)
	if addr == "" {
		return
	}
	t.Run("redis", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
		require.NoError(t, client.Ping(context.Background()).Err())
		b := messageThreadBackend{name: "redis", store: NewGatewayCache(client).(service.MessageThreadStore), rdb: client, scope: scope + ":real"}
		t.Cleanup(func() {
			if keys := b.keys(t); len(keys) > 0 {
				_ = client.Del(context.Background(), keys...).Err()
			}
			_ = client.Close()
		})
		run(t, b)
	})
}

func messageThreadTestBinding() service.MessageThreadBinding {
	return service.MessageThreadBinding{
		AccountID: 7, Identity: strings.Repeat("a", 64), Model: "claude-sonnet-4-5", ClientModel: "claude-sonnet-4-5",
		Wire: strings.Repeat("b", 32), Beta: "message-threads-2026-08-12", ContextTokens: 1200, Session: "session", Mimic: true, HadTools: true,
		ClientBeta:  strings.Repeat("c", 64),
		Breakpoints: 2, ShortTTL: true, ClientFixed: map[string]string{"tool_choice": "0123456789abcdef"},
	}
}

func TestMessageThreadStoreRoundTrip(t *testing.T) {
	forEachMessageThreadBackend(t, func(t *testing.T, b messageThreadBackend) {
		ctx := context.Background()

		missing, err := b.store.ReadMessageThread(ctx, b.scope, "msg_1")
		require.NoError(t, err)
		require.Nil(t, missing)

		binding := messageThreadTestBinding()
		require.NoError(t, b.store.BindMessageThread(ctx, b.scope, "msg_1", binding))
		got, err := b.store.ReadMessageThread(ctx, b.scope, "msg_1")
		require.NoError(t, err)
		require.Equal(t, binding, *got)
		require.True(t, got.Valid())

		// The response id is not stored, and every key belongs to one scope.
		for _, key := range b.keys(t) {
			require.NotContains(t, key, "msg_1")
			require.Contains(t, key, "{"+b.scope+"}")
			ttl, err := b.rdb.PTTL(ctx, key).Result()
			require.NoError(t, err)
			require.Greater(t, ttl, service.MessageThreadTTL-10*time.Second)
			require.LessOrEqual(t, ttl, service.MessageThreadTTL+5*time.Second)
		}
		other, err := b.store.ReadMessageThread(ctx, "9"+b.scope, "msg_1")
		require.NoError(t, err)
		require.Nil(t, other)

		next := binding
		next.ContextTokens = 2400
		require.NoError(t, b.store.BindMessageThread(ctx, b.scope, "msg_2", next))
		require.Len(t, b.keys(t), 2)

	})
}

// A response id names one route. Once two different bindings were offered for
// it, it names none, for as long as either of them could have been alive.
func TestMessageThreadStoreConflict(t *testing.T) {
	forEachMessageThreadBackend(t, func(t *testing.T, b messageThreadBackend) {
		ctx := context.Background()
		first := messageThreadTestBinding()
		second := first
		second.AccountID = 8
		third := first
		third.AccountID = 9

		require.NoError(t, b.store.BindMessageThread(ctx, b.scope, "msg_1", first))
		// The same binding again is harmless and does not extend its life.
		key := b.keys(t)[0]
		if b.advance != nil {
			b.advance(time.Minute)
		}
		require.NoError(t, b.store.BindMessageThread(ctx, b.scope, "msg_1", first))
		ttl, err := b.rdb.PTTL(ctx, key).Result()
		require.NoError(t, err)
		if b.advance != nil {
			require.LessOrEqual(t, ttl, service.MessageThreadTTL-time.Minute)
		}

		require.ErrorIs(t, b.store.BindMessageThread(ctx, b.scope, "msg_1", second), errMessageThreadConflict)
		unusable := func() {
			t.Helper()
			got, err := b.store.ReadMessageThread(ctx, b.scope, "msg_1")
			require.NoError(t, err)
			require.Nil(t, got, "a conflicted response id is not routed anywhere")
			require.Len(t, b.keys(t), 1, "the id stays taken")
		}
		unusable()
		ttl, err = b.rdb.PTTL(ctx, key).Result()
		require.NoError(t, err)
		require.Greater(t, ttl, service.MessageThreadTTL-10*time.Second, "for a full lifetime from the conflict")

		// Neither a third writer nor the first one gets the id back.
		for _, binding := range []service.MessageThreadBinding{third, first, second} {
			require.ErrorIs(t, b.store.BindMessageThread(ctx, b.scope, "msg_1", binding), errMessageThreadConflict)
			unusable()
		}
		if b.advance != nil {
			b.advance(service.MessageThreadTTL - time.Second)
			unusable()
			b.advance(2 * time.Second)
			require.Empty(t, b.keys(t))
			require.NoError(t, b.store.BindMessageThread(ctx, b.scope, "msg_1", third), "after that the id is free again")
		}
	})
}

func TestMessageThreadStoreConcurrentBinds(t *testing.T) {
	forEachMessageThreadBackend(t, func(t *testing.T, b messageThreadBackend) {
		ctx := context.Background()
		bind := func(id string, accounts []int64) []error {
			errs := make([]error, len(accounts))
			var wg sync.WaitGroup
			start := make(chan struct{})
			for i, account := range accounts {
				wg.Add(1)
				go func() {
					defer wg.Done()
					binding := messageThreadTestBinding()
					binding.AccountID = account
					<-start
					errs[i] = b.store.BindMessageThread(ctx, b.scope, id, binding)
				}()
			}
			close(start)
			wg.Wait()
			return errs
		}

		// The same binding from many writers: all succeed, one binding.
		for _, err := range bind("msg_same", []int64{7, 7, 7, 7, 7, 7, 7, 7}) {
			require.NoError(t, err)
		}
		got, err := b.store.ReadMessageThread(ctx, b.scope, "msg_same")
		require.NoError(t, err)
		require.EqualValues(t, 7, got.AccountID)

		// Different bindings: exactly the first writer succeeds, and the id is
		// unusable whichever order the server ran them in.
		succeeded := 0
		for _, err := range bind("msg_different", []int64{1, 2, 3, 4, 5, 6, 7, 8}) {
			if err == nil {
				succeeded++
			} else {
				require.ErrorIs(t, err, errMessageThreadConflict)
			}
		}
		require.Equal(t, 1, succeeded)
		got, err = b.store.ReadMessageThread(ctx, b.scope, "msg_different")
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

func TestMessageThreadStoreExpiry(t *testing.T) {
	forEachMessageThreadBackend(t, func(t *testing.T, b messageThreadBackend) {
		if b.advance == nil {
			t.Skip("needs a clock that can be moved")
		}
		ctx := context.Background()
		require.NoError(t, b.store.BindMessageThread(ctx, b.scope, "msg_1", messageThreadTestBinding()))

		b.advance(service.MessageThreadTTL - time.Second)
		require.NoError(t, b.store.BindMessageThread(ctx, b.scope, "msg_2", messageThreadTestBinding()))
		b.advance(2 * time.Second)
		got, err := b.store.ReadMessageThread(ctx, b.scope, "msg_1")
		require.NoError(t, err)
		require.Nil(t, got, "the first response expired")
		got, err = b.store.ReadMessageThread(ctx, b.scope, "msg_2")
		require.NoError(t, err)
		require.Equal(t, messageThreadTestBinding(), *got)

		b.advance(service.MessageThreadTTL - 4*time.Second)
		got, err = b.store.ReadMessageThread(ctx, b.scope, "msg_2")
		require.NoError(t, err)
		require.NotNil(t, got)
		b.advance(3 * time.Second)
		got, err = b.store.ReadMessageThread(ctx, b.scope, "msg_2")
		require.NoError(t, err)
		require.Nil(t, got)
		b.advance(10 * time.Second)
		require.Empty(t, b.keys(t), "nothing outlives the thread")
	})
}

// What cannot be read back whole is an error, never a miss, and no error path
// deletes anything.
func TestMessageThreadStoreFailsClosed(t *testing.T) {
	forEachMessageThreadBackend(t, func(t *testing.T, b messageThreadBackend) {
		ctx := context.Background()
		binding := messageThreadTestBinding()
		require.NoError(t, b.store.BindMessageThread(ctx, b.scope, "msg_1", binding))

		bindingKey := messageThreadBindingKey(b.scope, "msg_1")
		intactBinding := b.rdb.Get(ctx, bindingKey).Val()
		set := func(key, value string) {
			t.Helper()
			require.NoError(t, b.rdb.Set(ctx, key, value, service.MessageThreadTTL).Err())
		}
		failsToRead := func(why string) {
			t.Helper()
			got, err := b.store.ReadMessageThread(ctx, b.scope, "msg_1")
			require.Error(t, err, why)
			require.Nil(t, got)
		}

		set(bindingKey, `not json`)
		failsToRead("a binding that cannot be decoded")

		// A record is the version this code writes, with every field it
		// writes. One field less, a null, one field more, another version, or
		// a value of a shape this version never writes: none of them is a
		// binding, and none of them is a miss.
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(intactBinding), &fields))
		require.ElementsMatch(t, []string{"v", "a", "i", "m", "b", "w", "cm", "cb", "cf", "t", "s", "mi", "cc", "ht", "bp", "st"}, keysOf(fields),
			"every field is written on every record")
		require.Equal(t, "1", string(fields["v"]))
		rewrite := func(change func(map[string]json.RawMessage)) string {
			copied := map[string]json.RawMessage{}
			for name, value := range fields {
				copied[name] = value
			}
			change(copied)
			encoded, err := json.Marshal(copied)
			require.NoError(t, err)
			return string(encoded)
		}
		for name := range fields {
			set(bindingKey, rewrite(func(f map[string]json.RawMessage) { delete(f, name) }))
			failsToRead("without " + name)
			set(bindingKey, rewrite(func(f map[string]json.RawMessage) { f[name] = json.RawMessage(`null`) }))
			failsToRead(name + " null")
		}
		for why, change := range map[string]func(map[string]json.RawMessage){
			"an unknown field":           func(f map[string]json.RawMessage) { f["zz"] = json.RawMessage(`1`) },
			"an earlier version":         func(f map[string]json.RawMessage) { f["v"] = json.RawMessage(`0`) },
			"a later version":            func(f map[string]json.RawMessage) { f["v"] = json.RawMessage(`2`) },
			"a version that is text":     func(f map[string]json.RawMessage) { f["v"] = json.RawMessage(`"1"`) },
			"a flag that is text":        func(f map[string]json.RawMessage) { f["mi"] = json.RawMessage(`"true"`) },
			"a client beta digest cut":   func(f map[string]json.RawMessage) { f["cb"] = json.RawMessage(`"abc"`) },
			"an empty client beta":       func(f map[string]json.RawMessage) { f["cb"] = json.RawMessage(`""`) },
			"client fields as a list":    func(f map[string]json.RawMessage) { f["cf"] = json.RawMessage(`[]`) },
			"a wire digest cut":          func(f map[string]json.RawMessage) { f["w"] = json.RawMessage(`"abc"`) },
			"no account":                 func(f map[string]json.RawMessage) { f["a"] = json.RawMessage(`0`) },
			"no context size":            func(f map[string]json.RawMessage) { f["t"] = json.RawMessage(`0`) },
			"a beta set without threads": func(f map[string]json.RawMessage) { f["b"] = json.RawMessage(`"other-beta"`) },
		} {
			set(bindingKey, rewrite(change))
			failsToRead(why)
		}
		set(bindingKey, intactBinding)
		got, err := b.store.ReadMessageThread(ctx, b.scope, "msg_1")
		require.NoError(t, err)
		require.Equal(t, binding, *got)

		if b.fail == nil {
			return
		}
		b.fail(true)
		failsToRead("the server")
		require.Error(t, b.store.BindMessageThread(ctx, b.scope, "msg_4", binding))
		conflicting := binding
		conflicting.AccountID = 8
		require.Error(t, b.store.BindMessageThread(ctx, b.scope, "msg_1", conflicting))
		b.fail(false)
		got, err = b.store.ReadMessageThread(ctx, b.scope, "msg_1")
		require.NoError(t, err)
		require.Equal(t, binding, *got, "a failed write changed nothing")
	})
}

func keysOf(fields map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	return keys
}
