//go:build unit

package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// =============================================================
// T38 — manager_spawn pipeline stage 单元测试
//
// 本文件只覆盖两层语义,不依赖 gRPC/真实进程:
//
//   Layer A: spawn pipeline 编排算法
//     - 顺序执行 stage,中途失败按已成功 stage 倒序触发 rollback
//     - 失败 stage 自身的 rollback 不被调用
//     - nil rollback 不 panic
//     - spawnStages() 返回的 13 个 stage 的顺序与 rollback 是否存在
//       满足 manager_spawn.go 中的设计契约
//
//   Layer B: 关键 rollback 函数实际副作用
//     - spawnRollbackCapabilities: SDKServer.knownPlugins / capabilities 被清掉
//     - spawnRollbackEvents:       eventsAllowList 条目被 Forget
//     - spawnRollbackSettings:     未注册时不动 service; 已注册时 unsub + UnregisterSchema
//     - spawnRollbackRoutes:       路由表移除该 plugin 的 entries
//     - spawnRollbackDial:         conn 为 nil 时不 panic
//
// stage do 函数本身需要的 mock 太重(gRPC client / lifecycle / migrations / 等),
// 且与全链路集成测试有重叠,本文件不覆盖。
// =============================================================

// -------------------------------------------------------------
// Layer A — pipeline 编排算法
// -------------------------------------------------------------

// runSpawnPipelineForTest 复刻 spawnAndConnect 主循环的算法语义:顺序执行
// stages,任一 do 返回 error 时按 executed 倒序触发 rollback,失败 stage 自
// 身不被回滚。任何对该算法的偏离都应让本文件的测试失败 — 因此重构 spawn-
// AndConnect 时 *必须* 同步更新这里的复刻,作为"算法不变量"的回归网。
//
// 返回包装格式与 spawnAndConnect 保持一致("spawn stage <name>: <err>")。
func runSpawnPipelineForTest(stages []spawnStage, sc *spawnCtx) error {
	executed := make([]spawnStage, 0, len(stages))
	failed := false
	defer func() {
		if !failed {
			return
		}
		for i := len(executed) - 1; i >= 0; i-- {
			if rb := executed[i].rollback; rb != nil {
				rb(sc)
			}
		}
	}()
	for _, st := range stages {
		if err := st.do(sc); err != nil {
			failed = true
			return fmt.Errorf("spawn stage %s: %w", st.name, err)
		}
		executed = append(executed, st)
	}
	return nil
}

// recorder 记录 do/rollback 的调用顺序,供断言使用。
type recorder struct {
	mu  sync.Mutex
	log []string
}

func (r *recorder) add(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, name)
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.log))
	copy(out, r.log)
	return out
}

// makeStage 构造一个记录调用的 stage。failWith 非 nil 时其 do 返回该 error。
func makeStage(rec *recorder, name string, failWith error, hasRollback bool) spawnStage {
	st := spawnStage{
		name: name,
		do: func(*spawnCtx) error {
			rec.add("do:" + name)
			return failWith
		},
	}
	if hasRollback {
		st.rollback = func(*spawnCtx) {
			rec.add("rb:" + name)
		}
	}
	return st
}

func TestSpawnPipeline_EmptyStages(t *testing.T) {
	t.Parallel()
	if err := runSpawnPipelineForTest(nil, &spawnCtx{}); err != nil {
		t.Fatalf("empty stages: unexpected error: %v", err)
	}
}

func TestSpawnPipeline_AllSucceed_NoRollback(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	stages := []spawnStage{
		makeStage(rec, "a", nil, true),
		makeStage(rec, "b", nil, true),
		makeStage(rec, "c", nil, true),
	}
	if err := runSpawnPipelineForTest(stages, &spawnCtx{}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{"do:a", "do:b", "do:c"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("call order mismatch: got %v want %v", got, want)
	}
}

func TestSpawnPipeline_FirstStageFails_NoRollback(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	boom := errors.New("boom")
	stages := []spawnStage{
		makeStage(rec, "a", boom, true),
		makeStage(rec, "b", nil, true),
	}
	err := runSpawnPipelineForTest(stages, &spawnCtx{})
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped boom, got: %v", err)
	}
	// 第一个 stage 的 do 失败 → 自身不回滚, 后续 stage 不执行
	want := []string{"do:a"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("call order mismatch: got %v want %v", got, want)
	}
}

func TestSpawnPipeline_MiddleStageFails_RollbackInReverseOrder(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	boom := errors.New("kaboom")
	stages := []spawnStage{
		makeStage(rec, "a", nil, true),
		makeStage(rec, "b", nil, true),
		makeStage(rec, "c", boom, true), // 失败
		makeStage(rec, "d", nil, true),  // 不应执行
	}
	err := runSpawnPipelineForTest(stages, &spawnCtx{})
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped boom, got: %v", err)
	}
	if msg := err.Error(); msg != "spawn stage c: kaboom" {
		t.Fatalf("error wrap format: %q", msg)
	}
	// a, b 成功 → 失败时按倒序 rb:b, rb:a; c 自身不回滚; d 未执行
	want := []string{"do:a", "do:b", "do:c", "rb:b", "rb:a"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("call order mismatch:\n got %v\nwant %v", got, want)
	}
}

func TestSpawnPipeline_NilRollbackDoesNotPanic(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	boom := errors.New("x")
	stages := []spawnStage{
		makeStage(rec, "a", nil, false), // a 没有 rollback
		makeStage(rec, "b", nil, true),
		makeStage(rec, "c", boom, false), // c 也没有, 但失败时也不会回滚自己
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil rollback panicked: %v", r)
		}
	}()
	err := runSpawnPipelineForTest(stages, &spawnCtx{})
	if err == nil {
		t.Fatalf("expected error")
	}
	// b 是唯一带 rollback 的成功 stage
	want := []string{"do:a", "do:b", "do:c", "rb:b"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("call order mismatch:\n got %v\nwant %v", got, want)
	}
}

func TestSpawnPipeline_LastStageFails_RollbacksAllPriorButNotSelf(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	boom := errors.New("last")
	stages := []spawnStage{
		makeStage(rec, "a", nil, true),
		makeStage(rec, "b", nil, true),
		makeStage(rec, "c", boom, true),
	}
	if err := runSpawnPipelineForTest(stages, &spawnCtx{}); err == nil {
		t.Fatalf("expected error")
	}
	want := []string{"do:a", "do:b", "do:c", "rb:b", "rb:a"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("call order mismatch:\n got %v\nwant %v", got, want)
	}
}

// TestSpawnPipeline_StagesContract 是一个 snapshot 测试: 锁定
// PluginManager.spawnStages() 输出的 stage 名称顺序以及哪些 stage
// 配备了 rollback。这保证: 当来日有人新增/重排 stage 时,如果偏离
// 设计契约会立即在本测试失败,提示同步更新文档。
func TestSpawnPipeline_StagesContract(t *testing.T) {
	t.Parallel()
	m := &PluginManager{logger: slog.Default()}
	stages := m.spawnStages()

	type expected struct {
		name        string
		hasRollback bool
	}
	want := []expected{
		{"pipe", true},
		{"handshake", false},
		{"dial", true},
		{"manifest", false},
		{"capabilities", true},
		{"tables", false},
		{"events", true},
		{"settings", true},
		{"init", false},
		{"migrations", false},
		{"route_validate", false},
		{"route_install", true},
		{"health", false},
		{"pricing", false},
		{"platforms", true},
		{"maintenance", false},
		{"content_intercept", false},
	}
	if len(stages) != len(want) {
		t.Fatalf("stage count: got %d want %d", len(stages), len(want))
	}
	for i, w := range want {
		if stages[i].name != w.name {
			t.Errorf("stage[%d] name: got %q want %q", i, stages[i].name, w.name)
		}
		if got := stages[i].rollback != nil; got != w.hasRollback {
			t.Errorf("stage[%d %q] hasRollback: got %v want %v",
				i, w.name, got, w.hasRollback)
		}
		if stages[i].do == nil {
			t.Errorf("stage[%d %q] do is nil", i, w.name)
		}
	}
}

// -------------------------------------------------------------
// Layer B — 关键 rollback 函数副作用
// -------------------------------------------------------------

// newSDKServerForRollbackTest 构造一个零依赖的 SDKServer, 仅初始化
// rollback 路径会触碰的 capabilities + knownPlugins 子结构。
// 不调用 NewSDKServer (它启动 cleanupLoop goroutine 并要求真实 db/redis)。
func newSDKServerForRollbackTest() *SDKServer {
	return &SDKServer{
		capabilities: newPluginCapabilityRegistry(),
		knownPlugins: newKnownPluginsRegistry(),
	}
}

func TestSpawnRollback_Capabilities_ClearsRegistry(t *testing.T) {
	t.Parallel()
	m := &PluginManager{
		sdkServer: newSDKServerForRollbackTest(),
		logger:    slog.Default(),
	}
	const plug = "demo"

	// 模拟 do 阶段已写入 capability + knownPlugins
	m.sdkServer.RegisterPlugin(plug, []string{"db.own.read", "db.own.write"})
	if !m.sdkServer.knownPlugins.Contains(plug) {
		t.Fatalf("precondition: knownPlugins should contain %q", plug)
	}
	if !m.sdkServer.HasCapability(plug, "db.own.read") {
		t.Fatalf("precondition: should have db.own.read")
	}

	sc := &spawnCtx{inst: &PluginInstance{Name: plug}}
	m.spawnRollbackCapabilities(sc)

	if m.sdkServer.knownPlugins.Contains(plug) {
		t.Errorf("knownPlugins still contains %q after rollback", plug)
	}
	if m.sdkServer.HasCapability(plug, "db.own.read") {
		t.Errorf("capability not cleared after rollback")
	}
}

func TestSpawnRollback_Events_ForgetsAllowList(t *testing.T) {
	t.Parallel()
	m := &PluginManager{
		eventsAllowList: NewEventsAllowListRegistry(),
		logger:          slog.Default(),
	}
	const plug = "demo"

	m.eventsAllowList.Set(plug, []string{"order.created", "order.refunded"})
	if got := m.eventsAllowList.AllowedEvents(plug); len(got) != 2 {
		t.Fatalf("precondition: expected 2 events, got %v", got)
	}

	sc := &spawnCtx{inst: &PluginInstance{Name: plug}}
	m.spawnRollbackEvents(sc)

	if got := m.eventsAllowList.AllowedEvents(plug); len(got) != 0 {
		t.Errorf("events not forgotten after rollback: %v", got)
	}
}

func TestSpawnRollback_Events_NilAllowListIsNoop(t *testing.T) {
	t.Parallel()
	// eventsAllowList 为 nil 时 spawnRollbackEvents 必须 silently skip,
	// 不能 panic。manager 在不启用 events 子系统时这是合法配置。
	m := &PluginManager{logger: slog.Default()}
	sc := &spawnCtx{inst: &PluginInstance{Name: "demo"}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("rollback panicked on nil allow-list: %v", r)
		}
	}()
	m.spawnRollbackEvents(sc)
}

// fakeSettingsRegistrar 记录 UnregisterSchema 调用,用于验证 rollback 路径。
type fakeSettingsRegistrar struct {
	mu              sync.Mutex
	unregisterCalls []string
}

func (f *fakeSettingsRegistrar) RegisterSchemaWithInput(ctx context.Context, in service.RegisterSchemaInput) error {
	return nil
}

func (f *fakeSettingsRegistrar) UnregisterSchema(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unregisterCalls = append(f.unregisterCalls, name)
}

func (f *fakeSettingsRegistrar) Subscribe(name, key string) (<-chan service.PluginSettingsChange, func()) {
	ch := make(chan service.PluginSettingsChange)
	return ch, func() {}
}

func (f *fakeSettingsRegistrar) unregistered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.unregisterCalls))
	copy(out, f.unregisterCalls)
	return out
}

func TestSpawnRollback_Settings_NotRegistered_Noop(t *testing.T) {
	t.Parallel()
	// settingsRegistered=false 时 rollback 应直接返回, 不调用 service。
	// 这是 spawnSettingsCapabilities 在 manifest 没有 schema 时的快速跳过路径。
	fake := &fakeSettingsRegistrar{}
	m := &PluginManager{
		settingsService: fake,
		logger:          slog.Default(),
	}
	sc := &spawnCtx{
		inst:               &PluginInstance{Name: "demo"},
		settingsRegistered: false,
	}
	m.spawnRollbackSettings(sc)
	if got := fake.unregistered(); len(got) != 0 {
		t.Errorf("UnregisterSchema should NOT be called when settingsRegistered=false, got %v", got)
	}
}

func TestSpawnRollback_Settings_Registered_UnsubAndUnregister(t *testing.T) {
	t.Parallel()
	fake := &fakeSettingsRegistrar{}
	m := &PluginManager{
		settingsService: fake,
		logger:          slog.Default(),
	}
	const plug = "demo"

	unsubCalled := false
	inst := &PluginInstance{Name: plug}
	inst.settingsUnsubscribe = func() { unsubCalled = true }

	sc := &spawnCtx{inst: inst, settingsRegistered: true}
	m.spawnRollbackSettings(sc)

	if !unsubCalled {
		t.Errorf("settingsUnsubscribe was not invoked")
	}
	inst.mu.Lock()
	stillSet := inst.settingsUnsubscribe != nil
	inst.mu.Unlock()
	if stillSet {
		t.Errorf("settingsUnsubscribe field not cleared after rollback")
	}
	if got := fake.unregistered(); len(got) != 1 || got[0] != plug {
		t.Errorf("UnregisterSchema calls = %v, want [%q]", got, plug)
	}
}

func TestSpawnRollback_Routes_RemovesPluginEntries(t *testing.T) {
	t.Parallel()
	router := NewPluginRouter(nil, nil, nil, nil)
	m := &PluginManager{router: router, logger: slog.Default()}
	const plug = "demo"

	// 模拟 do 阶段已注入两条路由
	entries := []RouteEntry{
		{Method: "GET", PathPrefix: "/api/v1/plugins/demo/", AuthType: "user", ProxyURL: "http://127.0.0.1:1"},
		{Method: "POST", PathPrefix: "/api/v1/plugins/demo/", AuthType: "user", ProxyURL: "http://127.0.0.1:1"},
	}
	newTable := router.CurrentTable().AddPlugin(plug, entries)
	router.SwapRouteTable(newTable)
	if _, ok := router.CurrentTable().Match("GET", "/api/v1/plugins/demo/x"); !ok {
		t.Fatalf("precondition: route for demo not installed")
	}

	sc := &spawnCtx{inst: &PluginInstance{Name: plug}}
	m.spawnRollbackRoutes(sc)

	if _, ok := router.CurrentTable().Match("GET", "/api/v1/plugins/demo/x"); ok {
		t.Errorf("route for %q still present after rollback", plug)
	}
}

func TestSpawnRollback_Dial_NilConnIsNoop(t *testing.T) {
	t.Parallel()
	// spawnDial 失败时 sc.conn 仍为 nil。rollback 必须容忍这种情况。
	m := &PluginManager{logger: slog.Default()}
	sc := &spawnCtx{inst: &PluginInstance{Name: "demo"}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("rollback panicked on nil conn: %v", r)
		}
	}()
	m.spawnRollbackDial(sc)
}

func TestSpawnRollback_Pipe_NilFieldsAreNoop(t *testing.T) {
	t.Parallel()
	// spawnPipe 在 cmd.Start() 之前失败时 cancelProc / cmd 可能是 nil。
	// rollback 必须不 panic。
	m := &PluginManager{logger: slog.Default()}
	sc := &spawnCtx{inst: &PluginInstance{Name: "demo"}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("rollback panicked on nil fields: %v", r)
		}
	}()
	m.spawnRollbackPipe(sc)
}
