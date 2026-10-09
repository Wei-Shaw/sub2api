package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPluginKVStore(t *testing.T) (*pluginKVStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store, ok := NewPluginKVStore(rdb).(*pluginKVStore)
	require.True(t, ok)
	return store, mr
}

func TestPluginKVStore_SetGetDelete(t *testing.T) {
	store, _ := newTestPluginKVStore(t)
	ctx := context.Background()

	value, found, err := store.Get(ctx, "p1", "state", "k")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, value)

	require.NoError(t, store.Set(ctx, "p1", "state", "k", []byte("hello"), 0))
	value, found, err = store.Get(ctx, "p1", "state", "k")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte("hello"), value)

	require.NoError(t, store.Delete(ctx, "p1", "state", "k"))
	_, found, err = store.Get(ctx, "p1", "state", "k")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestPluginKVStore_TTLExpires(t *testing.T) {
	store, mr := newTestPluginKVStore(t)
	ctx := context.Background()

	require.NoError(t, store.Set(ctx, "p1", "state", "k", []byte("v"), 30*time.Second))
	full := store.fullKey("p1", "state", "k")
	ttl := mr.TTL(full)
	assert.Equal(t, 30*time.Second, ttl)

	mr.FastForward(31 * time.Second)
	_, found, err := store.Get(ctx, "p1", "state", "k")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestPluginKVStore_PluginIsolation(t *testing.T) {
	store, _ := newTestPluginKVStore(t)
	ctx := context.Background()

	require.NoError(t, store.Set(ctx, "p1", "state", "shared", []byte("one"), 0))
	require.NoError(t, store.Set(ctx, "p2", "state", "shared", []byte("two"), 0))

	v1, found, err := store.Get(ctx, "p1", "state", "shared")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []byte("one"), v1)

	v2, found, err := store.Get(ctx, "p2", "state", "shared")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []byte("two"), v2)
}

func TestPluginKVStore_ListStripsPrefixAndHonorsLimit(t *testing.T) {
	store, _ := newTestPluginKVStore(t)
	ctx := context.Background()
	for _, key := range []string{"account-1", "account-2", "account-3", "other"} {
		require.NoError(t, store.Set(ctx, "p1", "state", key, []byte("x"), 0))
	}
	// 另一命名空间/插件的键不应混入结果。
	require.NoError(t, store.Set(ctx, "p1", "other-ns", "account-9", []byte("x"), 0))
	require.NoError(t, store.Set(ctx, "p2", "state", "account-9", []byte("x"), 0))

	keys, err := store.List(ctx, "p1", "state", "account-", 100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"account-1", "account-2", "account-3"}, keys)

	keys, err = store.List(ctx, "p1", "state", "", 100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"account-1", "account-2", "account-3", "other"}, keys)

	keys, err = store.List(ctx, "p1", "state", "account-", 2)
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	keys, err = store.List(ctx, "p1", "state", "account-", 0)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

// namespace 是另一 namespace 的前缀时（state vs state2），List 不能串味——内部键的 ':'
// 分隔符 + namespace 字符集排除 ':' 共同保证消歧。
func TestPluginKVStore_ListNamespacePrefixDisambiguation(t *testing.T) {
	store, _ := newTestPluginKVStore(t)
	ctx := context.Background()
	require.NoError(t, store.Set(ctx, "p1", "state", "k1", []byte("a"), 0))
	require.NoError(t, store.Set(ctx, "p1", "state2", "k2", []byte("b"), 0))

	keys, err := store.List(ctx, "p1", "state", "", 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"k1"}, keys)

	keys, err = store.List(ctx, "p1", "state2", "", 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"k2"}, keys)
}

func TestPluginKVStore_OverwriteResetsValueAndTTL(t *testing.T) {
	store, mr := newTestPluginKVStore(t)
	ctx := context.Background()
	require.NoError(t, store.Set(ctx, "p1", "state", "k", []byte("v1"), 10*time.Second))
	require.NoError(t, store.Set(ctx, "p1", "state", "k", []byte("v2"), 0))

	value, found, err := store.Get(ctx, "p1", "state", "k")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []byte("v2"), value)
	// ttl=0 覆盖后应变为不过期。
	assert.Equal(t, time.Duration(0), mr.TTL(store.fullKey("p1", "state", "k")))
}

func TestPluginKVStore_EmptyValueIsFound(t *testing.T) {
	store, _ := newTestPluginKVStore(t)
	ctx := context.Background()
	require.NoError(t, store.Set(ctx, "p1", "state", "k", []byte{}, 0))
	value, found, err := store.Get(ctx, "p1", "state", "k")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Empty(t, value)
}

func TestPluginKVStore_DeleteMissingIsNoError(t *testing.T) {
	store, _ := newTestPluginKVStore(t)
	require.NoError(t, store.Delete(context.Background(), "p1", "state", "missing"))
}
