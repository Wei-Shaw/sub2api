//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/pluginkv"
	"github.com/Wei-Shaw/sub2api/internal/pluginhost"

	"github.com/stretchr/testify/require"
)

func TestPluginKVRepo_RoundtripAndUpsert(t *testing.T) {
	client := newPluginStateEntClient(t)
	repo := NewPluginKVRepository(client)
	ctx := context.Background()

	_, err := repo.Get(ctx, "acme.hello", "missing")
	require.ErrorIs(t, err, pluginhost.ErrKVNotFound)

	require.NoError(t, repo.Set(ctx, "acme.hello", "greeting", "hi"))
	got, err := repo.Get(ctx, "acme.hello", "greeting")
	require.NoError(t, err)
	require.Equal(t, "hi", got)

	// 同键覆盖写（(plugin_id, key) upsert）
	require.NoError(t, repo.Set(ctx, "acme.hello", "greeting", "hello again"))
	got, err = repo.Get(ctx, "acme.hello", "greeting")
	require.NoError(t, err)
	require.Equal(t, "hello again", got)
}

// TestPluginKVRepo_UpsertPreservesCreatedAt 覆盖写不得覆盖 created_at
// （Immutable 首写时间），updated_at 正常推进。
func TestPluginKVRepo_UpsertPreservesCreatedAt(t *testing.T) {
	client := newPluginStateEntClient(t)
	repo := NewPluginKVRepository(client)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "acme.hello", "greeting", "hi"))
	first, err := client.PluginKV.Query().
		Where(pluginkv.PluginIDEQ("acme.hello"), pluginkv.KeyEQ("greeting")).
		Only(ctx)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	require.NoError(t, repo.Set(ctx, "acme.hello", "greeting", "hello again"))
	second, err := client.PluginKV.Query().
		Where(pluginkv.PluginIDEQ("acme.hello"), pluginkv.KeyEQ("greeting")).
		Only(ctx)
	require.NoError(t, err)

	require.Equal(t, "hello again", second.Value)
	require.True(t, second.CreatedAt.Equal(first.CreatedAt), "created_at 必须保持首写时间")
	require.True(t, second.UpdatedAt.After(first.UpdatedAt), "updated_at 应随覆盖写推进")
}

func TestPluginKVRepo_NamespaceIsolation(t *testing.T) {
	client := newPluginStateEntClient(t)
	repo := NewPluginKVRepository(client)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "plugin.a", "shared", "value-a"))
	require.NoError(t, repo.Set(ctx, "plugin.b", "shared", "value-b"))

	gotA, err := repo.Get(ctx, "plugin.a", "shared")
	require.NoError(t, err)
	require.Equal(t, "value-a", gotA)
	gotB, err := repo.Get(ctx, "plugin.b", "shared")
	require.NoError(t, err)
	require.Equal(t, "value-b", gotB)

	// 删除 A 的键不影响 B
	require.NoError(t, repo.Delete(ctx, "plugin.a", "shared"))
	_, err = repo.Get(ctx, "plugin.a", "shared")
	require.ErrorIs(t, err, pluginhost.ErrKVNotFound)
	_, err = repo.Get(ctx, "plugin.b", "shared")
	require.NoError(t, err)

	// DeleteAll 只清空指定命名空间
	require.NoError(t, repo.Set(ctx, "plugin.a", "k1", "v1"))
	require.NoError(t, repo.DeleteAll(ctx, "plugin.a"))
	keysA, err := repo.Keys(ctx, "plugin.a", "")
	require.NoError(t, err)
	require.Empty(t, keysA)
	keysB, err := repo.Keys(ctx, "plugin.b", "")
	require.NoError(t, err)
	require.Equal(t, []string{"shared"}, keysB)
}

func TestPluginKVRepo_KeysPrefixAndOrder(t *testing.T) {
	client := newPluginStateEntClient(t)
	repo := NewPluginKVRepository(client)
	ctx := context.Background()

	for key, value := range map[string]string{
		"cfg:beta":  "2",
		"cfg:alpha": "1",
		"other":     "3",
	} {
		require.NoError(t, repo.Set(ctx, "acme.hello", key, value))
	}

	keys, err := repo.Keys(ctx, "acme.hello", "")
	require.NoError(t, err)
	require.Equal(t, []string{"cfg:alpha", "cfg:beta", "other"}, keys, "应按键稳定排序")

	keys, err = repo.Keys(ctx, "acme.hello", "cfg:")
	require.NoError(t, err)
	require.Equal(t, []string{"cfg:alpha", "cfg:beta"}, keys)

	// 删除不存在的键为 no-op
	require.NoError(t, repo.Delete(ctx, "acme.hello", "ghost"))
}
