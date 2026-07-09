//go:build unit

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/pluginstate"
	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newPluginStateEntClient(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func newPluginStateTestStore(t *testing.T, client *dbent.Client, mr *miniredis.Miniredis) *PluginStateStore {
	t.Helper()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store, err := NewPluginStateStore(client, rdb)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	return store
}

// pluginStateChangeRecorder 以并发安全的方式记录回调触发历史。
type pluginStateChangeRecorder struct {
	mu      sync.Mutex
	changes []pluginkit.StateChange
}

func (r *pluginStateChangeRecorder) record(change pluginkit.StateChange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changes = append(r.changes, change)
}

func (r *pluginStateChangeRecorder) snapshot() []pluginkit.StateChange {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]pluginkit.StateChange, len(r.changes))
	copy(out, r.changes)
	return out
}

func TestPluginStateStore_SetEnabledUpsertsAndUpdatesSnapshot(t *testing.T) {
	client := newPluginStateEntClient(t)
	store := newPluginStateTestStore(t, client, miniredis.RunT(t))
	ctx := context.Background()

	require.False(t, store.Enabled("demo"), "unknown id must read as disabled")

	rec := &pluginStateChangeRecorder{}
	store.Subscribe(rec.record)

	// 首次 SetEnabled 走 insert 分支，本地立即生效并同步回调。
	require.NoError(t, store.SetEnabled(ctx, "demo", true, "admin-a"))
	require.True(t, store.Enabled("demo"))
	require.Equal(t, []pluginkit.StateChange{{ID: "demo", Enabled: true}}, rec.snapshot())

	// 再次 SetEnabled 走 conflict-update 分支，仍是单行。
	require.NoError(t, store.SetEnabled(ctx, "demo", false, "admin-b"))
	require.False(t, store.Enabled("demo"))

	rows, err := client.PluginState.Query().Where(pluginstate.IDEQ("demo")).All(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1, "upsert must not create duplicate rows")
	require.False(t, rows[0].Enabled)
	require.Equal(t, "admin-b", rows[0].UpdatedBy)

	require.Equal(t, []pluginkit.StateChange{
		{ID: "demo", Enabled: true},
		{ID: "demo", Enabled: false},
	}, rec.snapshot())
}

// TestPluginStateStore_ConcurrentToggleKeepsDBAndSnapshotConsistent 守护并发 toggle
// 下「DB 提交序 = 内存写序」不变量：SetEnabled 将 DB 写 + 内存写 + 广播置于同一临界区，
// 否则并发反向 toggle 会让本实例内存与 DB（唯一事实源）持续漂移到下一个对账周期，
// 期间门控读到过期的 enabled 值（回归防线，勿删）。
func TestPluginStateStore_ConcurrentToggleKeepsDBAndSnapshotConsistent(t *testing.T) {
	client := newPluginStateEntClient(t)
	store := newPluginStateTestStore(t, client, miniredis.RunT(t))
	ctx := context.Background()

	for trial := 0; trial < 30; trial++ {
		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				_ = store.SetEnabled(ctx, "demo", g%2 == 0, "concurrent")
			}(g)
		}
		wg.Wait()

		row, err := client.PluginState.Query().Where(pluginstate.IDEQ("demo")).Only(ctx)
		require.NoError(t, err)
		require.Equal(t, row.Enabled, store.Enabled("demo"),
			"内存快照必须等于 DB 终值（trial %d）", trial)
	}
}

func TestPluginStateStore_SetEnabledRejectsInvalidID(t *testing.T) {
	client := newPluginStateEntClient(t)
	store := newPluginStateTestStore(t, client, miniredis.RunT(t))

	require.Error(t, store.SetEnabled(context.Background(), "Bad_ID", true, "admin"))
	require.False(t, store.Enabled("Bad_ID"))
}

func TestPluginStateStore_InitialLoadFromDB(t *testing.T) {
	client := newPluginStateEntClient(t)
	ctx := context.Background()

	require.NoError(t, client.PluginState.Create().SetID("demo").SetEnabled(true).Exec(ctx))
	require.NoError(t, client.PluginState.Create().SetID("job.backup").SetEnabled(false).Exec(ctx))

	store := newPluginStateTestStore(t, client, miniredis.RunT(t))
	require.True(t, store.Enabled("demo"))
	require.False(t, store.Enabled("job.backup"))
	require.False(t, store.Enabled("unknown"))
}

func TestPluginStateStore_CrossInstanceConvergence(t *testing.T) {
	client := newPluginStateEntClient(t)
	mr := miniredis.RunT(t)
	storeA := newPluginStateTestStore(t, client, mr)
	storeB := newPluginStateTestStore(t, client, mr)

	recA := &pluginStateChangeRecorder{}
	recB := &pluginStateChangeRecorder{}
	storeA.Subscribe(recA.record)
	storeB.Subscribe(recB.record)

	require.NoError(t, storeA.SetEnabled(context.Background(), "demo", true, "admin"))

	// A 本地同步生效，不依赖广播链路。
	require.True(t, storeA.Enabled("demo"))

	// B 经广播收敛：内存快照与回调均到位。
	require.Eventually(t, func() bool {
		return storeB.Enabled("demo") && len(recB.snapshot()) == 1
	}, 2*time.Second, 10*time.Millisecond, "store B should converge via broadcast")
	require.Equal(t, []pluginkit.StateChange{{ID: "demo", Enabled: true}}, recB.snapshot())

	// A 收到自己发出的广播后不得重复回调（instanceID 去重）。
	// B 已收到同一条广播，说明消息已投递完毕，此时 A 的回调数必须仍为 1。
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, []pluginkit.StateChange{{ID: "demo", Enabled: true}}, recA.snapshot())
}

func TestPluginStateStore_ReconcileConvergesDirectDBChange(t *testing.T) {
	client := newPluginStateEntClient(t)
	store := newPluginStateTestStore(t, client, miniredis.RunT(t))
	ctx := context.Background()

	rec := &pluginStateChangeRecorder{}
	store.Subscribe(rec.record)

	// 绕过 store 直接改 DB（模拟广播丢失/其他渠道写入），对账后收敛。
	require.NoError(t, client.PluginState.Create().SetID("demo").SetEnabled(true).SetUpdatedBy("direct").Exec(ctx))
	require.False(t, store.Enabled("demo"), "direct DB write is invisible before reconcile")

	require.NoError(t, store.reconcile(ctx))
	require.True(t, store.Enabled("demo"))
	require.Equal(t, []pluginkit.StateChange{{ID: "demo", Enabled: true}}, rec.snapshot())

	// DB 行被删除等价 disabled，对账补发 disable 回调。
	_, err := client.PluginState.Delete().Where(pluginstate.IDEQ("demo")).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, store.reconcile(ctx))
	require.False(t, store.Enabled("demo"))
	require.Equal(t, []pluginkit.StateChange{
		{ID: "demo", Enabled: true},
		{ID: "demo", Enabled: false},
	}, rec.snapshot())

	// 无差异对账不产生额外回调。
	require.NoError(t, store.reconcile(ctx))
	require.Len(t, rec.snapshot(), 2)
}

// TestPluginStateStore_BroadcastRejectsInvalidID 共享 Redis 频道上的非法 ID
// 广播（非本包写入者可发布）不得进入内存快照、不得触发回调。
func TestPluginStateStore_BroadcastRejectsInvalidID(t *testing.T) {
	client := newPluginStateEntClient(t)
	store := newPluginStateTestStore(t, client, miniredis.RunT(t))

	rec := &pluginStateChangeRecorder{}
	store.Subscribe(rec.record)

	store.handleBroadcast(`{"origin":"other-instance","id":"Bad_ID","enabled":true}`)

	require.False(t, store.Enabled("Bad_ID"))
	_, exists := store.Lookup("Bad_ID")
	require.False(t, exists, "非法 ID 不得进入内存快照")
	require.Empty(t, rec.snapshot(), "非法广播不得触发回调")

	// 合法广播照常生效（对照组）。
	store.handleBroadcast(`{"origin":"other-instance","id":"demo","enabled":true}`)
	require.True(t, store.Enabled("demo"))
	require.Len(t, rec.snapshot(), 1)
}

func TestPluginStateStore_CloseIsIdempotent(t *testing.T) {
	client := newPluginStateEntClient(t)
	store := newPluginStateTestStore(t, client, miniredis.RunT(t))

	require.NoError(t, store.Close())
	require.NoError(t, store.Close())

	// Close 后内存快照仍可读（只是不再收敛）。
	require.False(t, store.Enabled("demo"))
}
