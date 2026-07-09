//go:build unit

package pluginkit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeStateStore 是测试内联的最小内存版 StateStore
// （公共版夹具由 kittest 提供，属 TASK-004 范围）。
type fakeStateStore struct {
	mu      sync.Mutex
	enabled map[ID]bool
	subs    []func(StateChange)
}

func newFakeStateStore() *fakeStateStore {
	return &fakeStateStore{enabled: make(map[ID]bool)}
}

func (s *fakeStateStore) Enabled(id ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled[id]
}

func (s *fakeStateStore) Lookup(id ID) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enabled, exists := s.enabled[id]
	return enabled, exists
}

func (s *fakeStateStore) SetEnabled(_ context.Context, id ID, enabled bool, _ string) error {
	s.mu.Lock()
	s.enabled[id] = enabled
	subs := slices.Clone(s.subs)
	s.mu.Unlock()
	// 契约：回调在非持锁上下文调用。
	for _, fn := range subs {
		fn(StateChange{ID: id, Enabled: enabled})
	}
	return nil
}

func (s *fakeStateStore) Subscribe(fn func(StateChange)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, fn)
}

func (s *fakeStateStore) Close() error { return nil }

// testPlugin 覆盖全部扩展面（Initializer + Runner + APIProvider）。
type testPlugin struct {
	id ID

	mu         sync.Mutex
	initCalls  int
	startCalls int
	stopCalls  int
	mountCalls int
	initErr    error
	startErr   error
	initPanic  bool
	startPanic bool
	stopPanic  bool
	mountPanic bool
	stopBlock  chan struct{} // 非 nil 时 Stop 阻塞至 channel 关闭（无视 ctx，模拟卡死插件）
	onStop     func()
	host       *Host
}

func (p *testPlugin) ID() ID { return p.id }

func (p *testPlugin) Init(_ context.Context, host *Host) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.initCalls++
	p.host = host
	if p.initPanic {
		panic("init kaboom")
	}
	return p.initErr
}

func (p *testPlugin) Start(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startCalls++
	if p.startPanic {
		panic("start kaboom")
	}
	return p.startErr
}

func (p *testPlugin) Stop(_ context.Context) error {
	p.mu.Lock()
	p.stopCalls++
	block := p.stopBlock
	onStop := p.onStop
	stopPanic := p.stopPanic
	p.mu.Unlock()
	if onStop != nil {
		onStop()
	}
	if block != nil {
		<-block
	}
	if stopPanic {
		panic("stop kaboom")
	}
	return nil
}

func (p *testPlugin) MountRoutes(r Router) {
	p.mu.Lock()
	p.mountCalls++
	mountPanic := p.mountPanic
	p.mu.Unlock()
	if mountPanic {
		panic("mount kaboom")
	}
	r.Admin().GET("/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"plugin": string(p.id), "path": c.Request.URL.Path})
	})
	r.Admin().GET("/boom", func(_ *gin.Context) {
		panic("handler kaboom")
	})
	r.User().GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"pong": true})
	})
}

func (p *testPlugin) counters() (initCalls, startCalls, stopCalls, mountCalls int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initCalls, p.startCalls, p.stopCalls, p.mountCalls
}

// barePlugin 只实现最小契约（无 Initializer/Runner/APIProvider）。
type barePlugin struct{ id ID }

func (p *barePlugin) ID() ID { return p.id }

// defaultEnabledPlugin 实现 DefaultEnabler + Runner，用于验证自播种。
type defaultEnabledPlugin struct {
	id       ID
	dflt     bool
	started  atomic.Bool
	startCnt atomic.Int32
}

func (p *defaultEnabledPlugin) ID() ID               { return p.id }
func (p *defaultEnabledPlugin) DefaultEnabled() bool { return p.dflt }
func (p *defaultEnabledPlugin) Start(context.Context) error {
	p.started.Store(true)
	p.startCnt.Add(1)
	return nil
}
func (p *defaultEnabledPlugin) Stop(context.Context) error {
	p.started.Store(false)
	return nil
}

func TestBootstrap_DefaultEnabledSelfSeeds(t *testing.T) {
	store := newFakeStateStore()
	p := &defaultEnabledPlugin{id: "job.seed", dflt: true}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))

	// 自播种：从无记录 → 落 enabled=true → 启动。
	enabled, exists := store.Lookup("job.seed")
	require.True(t, exists, "default-enabled 插件应被播种出显式记录")
	require.True(t, enabled)
	require.True(t, p.started.Load(), "播种后应随 Bootstrap 启动")
}

func TestBootstrap_DefaultEnabledRespectsExplicitDisable(t *testing.T) {
	store := newFakeStateStore()
	// 管理员此前显式停用（记录已存在为 false）。
	require.NoError(t, store.SetEnabled(context.Background(), "job.seed", false, "admin"))
	p := &defaultEnabledPlugin{id: "job.seed", dflt: true}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))

	// 已有显式记录 → 不被 default 翻开 → 不启动。
	enabled, _ := store.Lookup("job.seed")
	require.False(t, enabled, "显式停用不应被 default-enabled 覆盖")
	require.False(t, p.started.Load())
}

// seedFailingStateStore 在 SetEnabled 时注入失败，模拟播种期 DB 瞬时故障。
type seedFailingStateStore struct {
	*fakeStateStore
	setErr error
}

func (s *seedFailingStateStore) SetEnabled(ctx context.Context, id ID, enabled bool, by string) error {
	if s.setErr != nil {
		return s.setErr
	}
	return s.fakeStateStore.SetEnabled(ctx, id, enabled, by)
}

// TestBootstrap_DefaultSeedFailureFailsFast 默认启用插件的播种写库失败必须
// 让 Bootstrap 返回错误（main 侧 fail-fast），不得静默降级为 disabled：
// 否则默认启用的审计类插件会在服务照常对外的同时 fail-open。
func TestBootstrap_DefaultSeedFailureFailsFast(t *testing.T) {
	store := &seedFailingStateStore{
		fakeStateStore: newFakeStateStore(),
		setErr:         errors.New("db transient failure"),
	}
	p := &defaultEnabledPlugin{id: "job.seed", dflt: true}
	m := newTestManager(t, store, nil, func() Plugin { return p })

	err := m.Bootstrap(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "seed default-enabled plugin")
	require.Contains(t, err.Error(), "db transient failure")
	require.False(t, p.started.Load())
}

func TestBootstrap_DefaultDisabledNotSeeded(t *testing.T) {
	store := newFakeStateStore()
	p := &defaultEnabledPlugin{id: "job.off", dflt: false}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))

	_, exists := store.Lookup("job.off")
	require.False(t, exists, "DefaultEnabled=false 不应被播种")
	require.False(t, p.started.Load())
}

func newTestManager(t *testing.T, store StateStore, cfg PluginsConfig, factories ...Factory) *Manager {
	t.Helper()
	deps := HostDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	m, err := NewManager(deps, store, cfg, factories)
	require.NoError(t, err)
	return m
}

func pluginState(m *Manager, id ID) PluginStatus {
	for _, st := range m.Snapshot() {
		if st.ID == id {
			return st
		}
	}
	return PluginStatus{}
}

func TestNewManager_InvalidID(t *testing.T) {
	deps := HostDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	_, err := NewManager(deps, newFakeStateStore(), nil, []Factory{
		func() Plugin { return &barePlugin{id: "Bad_ID"} },
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid plugin id")
}

func TestNewManager_DuplicateID(t *testing.T) {
	deps := HostDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	_, err := NewManager(deps, newFakeStateStore(), nil, []Factory{
		func() Plugin { return &barePlugin{id: "demo"} },
		func() Plugin { return &testPlugin{id: "demo"} },
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate plugin id")
}

func TestBootstrap_FailureIsolation(t *testing.T) {
	store := newFakeStateStore()
	bad := &testPlugin{id: "bad", initErr: errors.New("boom")}
	good := &testPlugin{id: "good"}
	m := newTestManager(t, store, nil,
		func() Plugin { return bad },
		func() Plugin { return good },
	)
	require.NoError(t, store.SetEnabled(context.Background(), "bad", true, "t"))
	require.NoError(t, store.SetEnabled(context.Background(), "good", true, "t"))
	require.NoError(t, m.Bootstrap(context.Background()))

	badSt := pluginState(m, "bad")
	require.Equal(t, StateFailed, badSt.State)
	require.Contains(t, badSt.Err, "boom")
	require.Nil(t, badSt.StartedAt)

	goodSt := pluginState(m, "good")
	require.Equal(t, StateRunning, goodSt.State)
	require.Empty(t, goodSt.Err)
	require.NotNil(t, goodSt.StartedAt)

	require.False(t, m.GateAllows("bad"))
	require.True(t, m.GateAllows("good"))
}

// TestBootstrap_MountRoutesPanicIsolation MountRoutes panic 只隔离本插件：
// Bootstrap 正常返回，该插件进 failed 且此后（含 re-enable）永久拒绝启动，
// 其余插件的路由与生命周期不受影响。
func TestBootstrap_MountRoutesPanicIsolation(t *testing.T) {
	store := newFakeStateStore()
	bad := &testPlugin{id: "bad", mountPanic: true}
	good := &testPlugin{id: "good"}
	m := newTestManager(t, store, nil,
		func() Plugin { return bad },
		func() Plugin { return good },
	)
	ctx := context.Background()
	require.NoError(t, store.SetEnabled(ctx, "bad", true, "t"))
	require.NoError(t, store.SetEnabled(ctx, "good", true, "t"))
	require.NoError(t, m.Bootstrap(ctx))

	badSt := pluginState(m, "bad")
	require.Equal(t, StateFailed, badSt.State)
	require.Contains(t, badSt.Err, "mount_routes panicked")
	require.False(t, m.GateAllows("bad"))
	_, badStarts, _, _ := bad.counters()
	require.Zero(t, badStarts, "挂载失败的插件不得被启动")

	goodSt := pluginState(m, "good")
	require.Equal(t, StateRunning, goodSt.State)
	require.True(t, m.GateAllows("good"))

	host := newDispatchHost(m)
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/admin/plugins/bad/api/hello"))
	require.Equal(t, http.StatusOK,
		doRequest(host, http.MethodGet, "/api/v1/admin/plugins/good/api/hello").Code)

	// re-enable 也不会把挂载失败的插件带活（路由只在 Bootstrap 装配一次）。
	require.NoError(t, store.SetEnabled(ctx, "bad", false, "t"))
	require.NoError(t, store.SetEnabled(ctx, "bad", true, "t"))
	badSt = pluginState(m, "bad")
	require.Equal(t, StateFailed, badSt.State)
	require.Contains(t, badSt.Err, "mount routes failed")
	_, badStarts, _, _ = bad.counters()
	require.Zero(t, badStarts)
}

func TestBootstrap_Twice(t *testing.T) {
	m := newTestManager(t, newFakeStateStore(), nil)
	require.NoError(t, m.Bootstrap(context.Background()))
	require.Error(t, m.Bootstrap(context.Background()))
}

func TestToggle_EnableDisableEnable_SameInstance(t *testing.T) {
	store := newFakeStateStore()
	factoryCalls := 0
	p := &testPlugin{id: "demo"}
	m := newTestManager(t, store, nil, func() Plugin {
		factoryCalls++
		return p
	})
	require.NoError(t, m.Bootstrap(context.Background()))
	require.Equal(t, 1, factoryCalls, "factory 只在 NewManager 实例化一次")

	ctx := context.Background()
	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	initCalls, startCalls, stopCalls, _ := p.counters()
	require.Equal(t, [3]int{1, 1, 0}, [3]int{initCalls, startCalls, stopCalls})
	require.Equal(t, StateRunning, pluginState(m, "demo").State)

	require.NoError(t, store.SetEnabled(ctx, "demo", false, "t"))
	initCalls, startCalls, stopCalls, _ = p.counters()
	require.Equal(t, [3]int{1, 1, 1}, [3]int{initCalls, startCalls, stopCalls})
	require.Equal(t, StateStopped, pluginState(m, "demo").State)
	require.Nil(t, pluginState(m, "demo").StartedAt)

	// 再次 enable：同一实例重新 Init→Start。
	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	initCalls, startCalls, stopCalls, _ = p.counters()
	require.Equal(t, [3]int{2, 2, 1}, [3]int{initCalls, startCalls, stopCalls})
	require.Equal(t, StateRunning, pluginState(m, "demo").State)
	require.Equal(t, 1, factoryCalls)
}

func TestToggle_Idempotent(t *testing.T) {
	store := newFakeStateStore()
	p := &testPlugin{id: "demo"}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))
	ctx := context.Background()

	// 未启用时 disable 为 no-op。
	require.NoError(t, store.SetEnabled(ctx, "demo", false, "t"))
	_, _, stopCalls, _ := p.counters()
	require.Zero(t, stopCalls)
	require.Equal(t, StateDisabled, pluginState(m, "demo").State)

	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	initCalls, startCalls, _, _ := p.counters()
	require.Equal(t, 1, initCalls)
	require.Equal(t, 1, startCalls)

	require.NoError(t, store.SetEnabled(ctx, "demo", false, "t"))
	require.NoError(t, store.SetEnabled(ctx, "demo", false, "t"))
	_, _, stopCalls, _ = p.counters()
	require.Equal(t, 1, stopCalls)
	require.Equal(t, StateStopped, pluginState(m, "demo").State)
}

func TestToggle_UnknownID(t *testing.T) {
	store := newFakeStateStore()
	m := newTestManager(t, store, nil)
	require.NoError(t, m.Bootstrap(context.Background()))
	// 未注册插件的 toggle 只打日志，不 panic。
	require.NoError(t, store.SetEnabled(context.Background(), "ghost", true, "t"))
	require.Empty(t, m.Snapshot())
}

func TestToggle_Concurrent(t *testing.T) {
	store := newFakeStateStore()
	a := &testPlugin{id: "a"}
	b := &testPlugin{id: "b"}
	m := newTestManager(t, store, nil,
		func() Plugin { return a },
		func() Plugin { return b },
	)
	require.NoError(t, m.Bootstrap(context.Background()))
	ctx := context.Background()

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			id := ID("a")
			if g%2 == 0 {
				id = "b"
			}
			for i := 0; i < 20; i++ {
				_ = store.SetEnabled(ctx, id, i%2 == 0, "t")
				_ = m.GateAllows(id)
				_ = m.Snapshot()
			}
		}(g)
	}
	wg.Wait()

	require.NoError(t, store.SetEnabled(ctx, "a", false, "t"))
	require.NoError(t, store.SetEnabled(ctx, "b", false, "t"))

	for _, p := range []*testPlugin{a, b} {
		initCalls, startCalls, stopCalls, _ := p.counters()
		st := pluginState(m, p.id).State
		require.NotEqual(t, StateRunning, st)
		require.Equal(t, initCalls, startCalls, "无失败注入时 Init/Start 成对")
		require.Equal(t, startCalls, stopCalls, "终态非 running 时每次 Start 都有配对的 Stop")
	}
}

// TestStart_FailureCompensatesWithStop Start 失败必须触发一次补偿 Stop：
// 回收 Init/半启动已获取的资源，否则 re-enable 再跑 Init 会双重获取。
// failed 现场保留原始 Start 错误，恢复失败原因后 re-enable 照常收敛。
func TestStart_FailureCompensatesWithStop(t *testing.T) {
	store := newFakeStateStore()
	p := &testPlugin{id: "demo", startErr: errors.New("bind: address in use")}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))
	ctx := context.Background()

	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	st := pluginState(m, "demo")
	require.Equal(t, StateFailed, st.State)
	require.Contains(t, st.Err, "address in use")
	initCalls, startCalls, stopCalls, _ := p.counters()
	require.Equal(t, [3]int{1, 1, 1}, [3]int{initCalls, startCalls, stopCalls},
		"Start 失败后应有且仅有一次补偿 Stop")

	// 失败原因消除后 re-enable：同一实例重新 Init→Start 恢复 running。
	p.mu.Lock()
	p.startErr = nil
	p.mu.Unlock()
	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	require.Equal(t, StateRunning, pluginState(m, "demo").State)
}

func TestStop_TimeoutEntersFailed(t *testing.T) {
	store := newFakeStateStore()
	block := make(chan struct{})
	p := &testPlugin{id: "demo", stopBlock: block}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	m.stopTimeout = 30 * time.Millisecond
	require.NoError(t, m.Bootstrap(context.Background()))
	ctx := context.Background()

	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	require.NoError(t, store.SetEnabled(ctx, "demo", false, "t"))

	st := pluginState(m, "demo")
	require.Equal(t, StateFailed, st.State)
	require.Contains(t, st.Err, "timed out")
	require.False(t, m.GateAllows("demo"))
	close(block) // 释放被泄漏的 Stop goroutine

	// failed 态可经 enable 重试恢复。逃逸的 Stop goroutine 退出前 enable 会被
	// 拒绝（防同实例并发），故用重试收敛断言（对应真实管理员"稍后重试"）。
	p.mu.Lock()
	p.stopBlock = nil
	p.mu.Unlock()
	require.Eventually(t, func() bool {
		_ = store.SetEnabled(ctx, "demo", true, "t")
		return pluginState(m, "demo").State == StateRunning
	}, 2*time.Second, 10*time.Millisecond)
}

func TestStop_ReenableRejectedWhileStopInFlight(t *testing.T) {
	store := newFakeStateStore()
	block := make(chan struct{})
	p := &testPlugin{id: "demo", stopBlock: block}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	m.stopTimeout = 30 * time.Millisecond
	require.NoError(t, m.Bootstrap(context.Background()))
	ctx := context.Background()

	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	require.NoError(t, store.SetEnabled(ctx, "demo", false, "t"))
	require.Equal(t, StateFailed, pluginState(m, "demo").State)

	// 旧 Stop 仍阻塞在 block 上：re-enable 必须被拒绝，不得与其并发跑 Init/Start。
	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	st := pluginState(m, "demo")
	require.Equal(t, StateFailed, st.State)
	require.Contains(t, st.Err, "previous stop still in progress")
	initCalls, startCalls, _, _ := p.counters()
	require.Equal(t, 1, initCalls, "在途 Stop 未退出前不得再次 Init")
	require.Equal(t, 1, startCalls)

	// 旧 Stop 退出后，重试 enable 恢复 running。
	close(block)
	require.Eventually(t, func() bool {
		_ = store.SetEnabled(ctx, "demo", true, "t")
		return pluginState(m, "demo").State == StateRunning
	}, 2*time.Second, 10*time.Millisecond)
}

// TestLifecyclePanicIsolation 插件 Init/Start/Stop panic 一律被 recover 为
// failed 态：生命周期跑在无兜底的 goroutine（Redis 订阅回调/对账循环）上，
// 未受控的 panic 会直接崩溃宿主进程（多实例下经广播连锁放大）。
func TestLifecyclePanicIsolation(t *testing.T) {
	ctx := context.Background()

	t.Run("init panic", func(t *testing.T) {
		store := newFakeStateStore()
		p := &testPlugin{id: "demo", initPanic: true}
		m := newTestManager(t, store, nil, func() Plugin { return p })
		require.NoError(t, m.Bootstrap(ctx))
		require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))

		st := pluginState(m, "demo")
		require.Equal(t, StateFailed, st.State)
		require.Contains(t, st.Err, "init panicked")
		require.False(t, m.GateAllows("demo"))
	})

	t.Run("start panic", func(t *testing.T) {
		store := newFakeStateStore()
		p := &testPlugin{id: "demo", startPanic: true}
		m := newTestManager(t, store, nil, func() Plugin { return p })
		require.NoError(t, m.Bootstrap(ctx))
		require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))

		st := pluginState(m, "demo")
		require.Equal(t, StateFailed, st.State)
		require.Contains(t, st.Err, "start panicked")
	})

	t.Run("stop panic", func(t *testing.T) {
		store := newFakeStateStore()
		p := &testPlugin{id: "demo", stopPanic: true}
		m := newTestManager(t, store, nil, func() Plugin { return p })
		require.NoError(t, m.Bootstrap(ctx))
		require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
		require.Equal(t, StateRunning, pluginState(m, "demo").State)

		// Stop 跑在 stopRunner 自建 goroutine 里，panic 若不 recover 任何
		// caller 都无法捕获（直接崩进程）；这里断言其被折算为 stop 失败。
		require.NoError(t, store.SetEnabled(ctx, "demo", false, "t"))
		st := pluginState(m, "demo")
		require.Equal(t, StateFailed, st.State)
		require.Contains(t, st.Err, "stop panicked")
	})
}

func TestStopAll_ReverseOrderAndAggregatesErrors(t *testing.T) {
	store := newFakeStateStore()
	var orderMu sync.Mutex
	var stopped []ID
	record := func(id ID) func() {
		return func() {
			orderMu.Lock()
			stopped = append(stopped, id)
			orderMu.Unlock()
		}
	}
	a := &testPlugin{id: "a", onStop: record("a")}
	b := &testPlugin{id: "b", onStop: record("b")}
	c := &testPlugin{id: "c", onStop: record("c")}
	m := newTestManager(t, store, nil,
		func() Plugin { return b },
		func() Plugin { return c },
		func() Plugin { return a },
	)
	require.NoError(t, m.Bootstrap(context.Background()))
	ctx := context.Background()
	for _, id := range []ID{"a", "b", "c"} {
		require.NoError(t, store.SetEnabled(ctx, id, true, "t"))
	}

	require.NoError(t, m.StopAll(ctx))
	require.Equal(t, []ID{"c", "b", "a"}, stopped, "逆 ID 序停止")
	for _, id := range []ID{"a", "b", "c"} {
		require.Equal(t, StateStopped, pluginState(m, id).State)
	}
}

func TestStopAll_TimeoutIsolation(t *testing.T) {
	store := newFakeStateStore()
	block := make(chan struct{})
	stuck := &testPlugin{id: "stuck", stopBlock: block}
	ok := &testPlugin{id: "ok"}
	m := newTestManager(t, store, nil,
		func() Plugin { return stuck },
		func() Plugin { return ok },
	)
	m.stopTimeout = 30 * time.Millisecond
	require.NoError(t, m.Bootstrap(context.Background()))
	ctx := context.Background()
	require.NoError(t, store.SetEnabled(ctx, "stuck", true, "t"))
	require.NoError(t, store.SetEnabled(ctx, "ok", true, "t"))

	err := m.StopAll(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "stuck")
	require.Equal(t, StateFailed, pluginState(m, "stuck").State)
	require.Equal(t, StateStopped, pluginState(m, "ok").State)
	close(block)
}

// newDispatchHost 模拟宿主侧分发器路由（真实挂载在 TASK-005 完成）。
func newDispatchHost(m *Manager) *gin.Engine {
	host := gin.New()
	host.Any("/api/v1/admin/plugins/:id/api/*path", func(c *gin.Context) {
		m.Dispatch(SideAdmin, c.Param("id"), c)
	})
	host.Any("/api/v1/plugins/:id/api/*path", func(c *gin.Context) {
		m.Dispatch(SideUser, c.Param("id"), c)
	})
	return host
}

func doRequest(host *gin.Engine, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	host.ServeHTTP(w, req)
	return w
}

func requireEnvelope404(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusNotFound, w.Code)
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, http.StatusNotFound, body.Code)
	require.NotEmpty(t, body.Message)
}

func TestDispatch_GateAndPathRewrite(t *testing.T) {
	store := newFakeStateStore()
	p := &testPlugin{id: "demo"}
	bare := &barePlugin{id: "bare"}
	m := newTestManager(t, store, nil,
		func() Plugin { return p },
		func() Plugin { return bare },
	)
	require.NoError(t, m.Bootstrap(context.Background()))
	host := newDispatchHost(m)
	ctx := context.Background()

	// 未启用 → 404（项目标准错误体）。
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/admin/plugins/demo/api/hello"))

	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))

	// admin 面命中，且插件看到的是私有子路由器内的重写路径。
	w := doRequest(host, http.MethodGet, "/api/v1/admin/plugins/demo/api/hello")
	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "demo", body["plugin"])
	require.Equal(t, "/admin/hello", body["path"])

	// user 面命中；admin 路由不可从 user 面访问。
	require.Equal(t, http.StatusOK, doRequest(host, http.MethodGet, "/api/v1/plugins/demo/api/ping").Code)
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/plugins/demo/api/hello"))

	// 插件内部未注册路径 → 子路由器 404（同样的标准错误体）。
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/admin/plugins/demo/api/nope"))

	// 未知插件 ID → 404。
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/admin/plugins/ghost/api/hello"))

	// 已启用但无 API 面 → 404。
	require.NoError(t, store.SetEnabled(ctx, "bare", true, "t"))
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/admin/plugins/bare/api/hello"))

	// disable 后门控关闭。
	require.NoError(t, store.SetEnabled(ctx, "demo", false, "t"))
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/admin/plugins/demo/api/hello"))
}

// TestSanitizeDispatchPath 归一化拼接契约：归一化后逃出鉴权面前缀的路径
// 一律拒绝，前缀内的相对段（a/../b）允许归一化通过。
func TestSanitizeDispatchPath(t *testing.T) {
	cases := []struct {
		prefix, rel string
		want        string
		ok          bool
	}{
		{"/user", "/hello", "/user/hello", true},
		{"/user", "hello", "/user/hello", true}, // 无前导 / 时补齐
		{"/user", "", "/user", true},
		{"/user", "/", "/user", true},
		{"/user", "/a/../b", "/user/b", true},
		{"/user", "/a//b/./c", "/user/a/b/c", true},
		{"/user", "/../admin/secret", "", false},  // 越权到 admin 面
		{"/user", "/..", "", false},               // 逃到根
		{"/user", "/../../etc/passwd", "", false}, // 逃出鉴权面
		{"/user", "/a/../../admin/x", "", false},  // 深层回退逃逸
		{"/user", "/../userland/x", "", false},    // 字符串前缀相同但非路径前缀
		{"/admin", "/../user/ping", "", false},
	}
	for _, tc := range cases {
		got, ok := SanitizeDispatchPath(tc.prefix, tc.rel)
		require.Equal(t, tc.ok, ok, "prefix=%q rel=%q", tc.prefix, tc.rel)
		if tc.ok {
			require.Equal(t, tc.want, got, "prefix=%q rel=%q", tc.prefix, tc.rel)
		}
	}
}

// TestDispatch_PathTraversalRejected user 面经 ".." 归一化蹦到 admin 前缀的
// 请求在分发器收口拒绝（与未知插件同一个 404），/user↔/admin 权限断言
// 不外包给插件子路由器的归一化行为。
func TestDispatch_PathTraversalRejected(t *testing.T) {
	store := newFakeStateStore()
	p := &testPlugin{id: "demo"}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))
	require.NoError(t, store.SetEnabled(context.Background(), "demo", true, "t"))
	host := newDispatchHost(m)

	// 对照组：admin 面直达 /hello、user 面直达 /ping 均正常。
	require.Equal(t, http.StatusOK, doRequest(host, http.MethodGet, "/api/v1/admin/plugins/demo/api/hello").Code)
	require.Equal(t, http.StatusOK, doRequest(host, http.MethodGet, "/api/v1/plugins/demo/api/ping").Code)

	// user 面经 ".." 穿越到 admin 路由必须 404。
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/plugins/demo/api/../admin/hello"))
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/plugins/demo/api/x/../../admin/hello"))
	requireEnvelope404(t, doRequest(host, http.MethodGet, "/api/v1/plugins/demo/api/.."))
}

// TestDispatch_HandlerPanicRecovered 插件 handler 的 panic 在子路由器边界内
// 兜住：返回标准 500 错误体，且同一插件后续请求不受影响。
func TestDispatch_HandlerPanicRecovered(t *testing.T) {
	store := newFakeStateStore()
	p := &testPlugin{id: "demo"}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))
	require.NoError(t, store.SetEnabled(context.Background(), "demo", true, "t"))
	host := newDispatchHost(m)

	w := doRequest(host, http.MethodGet, "/api/v1/admin/plugins/demo/api/boom")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, http.StatusInternalServerError, body.Code)
	require.NotEmpty(t, body.Message)

	// panic 后子路由器仍然存活，正常路由照常命中。
	require.Equal(t, http.StatusOK, doRequest(host, http.MethodGet, "/api/v1/admin/plugins/demo/api/hello").Code)
}

func TestDispatch_RestoresHostPath(t *testing.T) {
	store := newFakeStateStore()
	p := &testPlugin{id: "demo"}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))
	require.NoError(t, store.SetEnabled(context.Background(), "demo", true, "t"))

	host := gin.New()
	var afterPath string
	host.Use(func(c *gin.Context) {
		c.Next()
		afterPath = c.Request.URL.Path
	})
	host.Any("/api/v1/admin/plugins/:id/api/*path", func(c *gin.Context) {
		m.Dispatch(SideAdmin, c.Param("id"), c)
	})

	w := doRequest(host, http.MethodGet, "/api/v1/admin/plugins/demo/api/hello")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "/api/v1/admin/plugins/demo/api/hello", afterPath,
		"转发后宿主侧后置中间件应看到原始路径")
}

func TestDispatch_MountRoutesOnceAcrossRestarts(t *testing.T) {
	store := newFakeStateStore()
	p := &testPlugin{id: "demo"}
	m := newTestManager(t, store, nil, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))
	ctx := context.Background()

	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))
	require.NoError(t, store.SetEnabled(ctx, "demo", false, "t"))
	require.NoError(t, store.SetEnabled(ctx, "demo", true, "t"))

	_, _, _, mountCalls := p.counters()
	require.Equal(t, 1, mountCalls, "MountRoutes 仅在 Bootstrap 时调用一次")

	// 重启后路由仍可命中（同一子路由器）。
	host := newDispatchHost(m)
	require.Equal(t, http.StatusOK, doRequest(host, http.MethodGet, "/api/v1/admin/plugins/demo/api/hello").Code)
}

func TestSnapshot_SortedByID(t *testing.T) {
	store := newFakeStateStore()
	m := newTestManager(t, store, nil,
		func() Plugin { return &barePlugin{id: "b"} },
		func() Plugin { return &barePlugin{id: "a.x"} },
		func() Plugin { return &barePlugin{id: "c"} },
	)
	require.NoError(t, m.Bootstrap(context.Background()))
	require.NoError(t, store.SetEnabled(context.Background(), "b", true, "t"))

	snap := m.Snapshot()
	ids := make([]ID, 0, len(snap))
	for _, st := range snap {
		ids = append(ids, st.ID)
	}
	require.Equal(t, []ID{"a.x", "b", "c"}, ids)

	require.True(t, snap[1].Enabled)
	require.Equal(t, StateRunning, snap[1].State, "无生命周期扩展面的插件 enable 后即 running")
	require.False(t, snap[0].Enabled)
	require.Equal(t, StateDisabled, snap[0].State)
}

func TestGateAllows(t *testing.T) {
	store := newFakeStateStore()
	failing := &testPlugin{id: "failing", initErr: errors.New("nope")}
	ok := &testPlugin{id: "ok"}
	m := newTestManager(t, store, nil,
		func() Plugin { return failing },
		func() Plugin { return ok },
	)
	require.NoError(t, m.Bootstrap(context.Background()))
	ctx := context.Background()

	require.False(t, m.GateAllows("ghost"), "未注册")
	require.False(t, m.GateAllows("ok"), "未启用")

	require.NoError(t, store.SetEnabled(ctx, "ok", true, "t"))
	require.True(t, m.GateAllows("ok"))

	require.NoError(t, store.SetEnabled(ctx, "failing", true, "t"))
	require.False(t, m.GateAllows("failing"), "enabled 但 failed 不放行")
}

func TestHostConfigDecoding(t *testing.T) {
	store := newFakeStateStore()
	p := &testPlugin{id: "demo"}
	cfg, err := ParsePluginsConfig(map[string]any{
		"demo": map[string]any{"greeting": "hola", "interval": "3s"},
	})
	require.NoError(t, err)
	m := newTestManager(t, store, cfg, func() Plugin { return p })
	require.NoError(t, m.Bootstrap(context.Background()))
	require.NoError(t, store.SetEnabled(context.Background(), "demo", true, "t"))

	p.mu.Lock()
	host := p.host
	p.mu.Unlock()
	require.NotNil(t, host)
	require.NotNil(t, host.Logger)

	var out demoPluginConfig
	require.NoError(t, host.Config(&out))
	require.Equal(t, "hola", out.Greeting)
	require.Equal(t, 3*time.Second, out.Interval)
}
