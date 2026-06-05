package plugin

import (
	"context"
	"os/exec"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// =============================================================
// Lifecycle: stopInstance 分解 + attachInstanceState
//
// 历史 stopInstance 是 100+ 行的串联操作 (state transition / shutdown
// RPC / 等待退出 / 路由摘除 / 撤销授权 / 释放扩展). 把它拆成多个 ≤ 30 行
// 的单一职责 helper, 配合一个 stopCtx 在 helper 间传递快照, 主入口
// 只负责调度。
//
// 文件拆分(T34):
//   - 进程退出监听 / RestartPolicy 调度    → manager_restart.go
//   - settings 变更触发的 reload 链路     → manager_settings_reload.go
//   - spawn pipeline 的 stage / rollback  → manager_spawn*.go
//
// 本文件只保留 stop 路径的临界区控制 + spawn 完成后的 instance state
// attach (与 stop 路径写同一组字段, 共享 invariant)。
// =============================================================

// stopCtx 是 stopInstance 在多个 helper 之间共享的瞬时快照。
// 字段对应 instance 在加锁阶段读取出的副本, 后续 helper 不再回头读 inst.mu。
type stopCtx struct {
	name           string
	inst           *PluginInstance
	state          PluginState
	cmd            *exec.Cmd
	stub           pluginsdk.PluginLifecycleClient
	cancelProc     context.CancelFunc
	exited         chan struct{}
	markRegistered bool
}

// stopInstance 优雅停止某个插件:Shutdown RPC -> 等待退出 -> SIGKILL -> 清理。
// markRegistered 决定是否把状态置为 StateRegistered(true)还是保持当前状态(false,通常是重启场景)。
//
// 拆为多个 helper 后, 每步都 ≤ 30 行; 锁的边界:
//   - prepareStop / clearInstanceState 显式持有 inst.mu (短临界区)
//   - removeInstanceFromRouting / unregisterCapabilities / detachExtensions
//     不持锁 (读 manager 字段时显式 RLock 一次)
func (m *PluginManager) stopInstance(ctx context.Context, name string, markRegistered bool) error {
	sx := m.prepareStop(name, markRegistered)
	if sx == nil {
		return nil
	}

	m.gracefulShutdownRPC(ctx, sx)
	m.waitProcessOrTimeout(sx)
	m.removeInstanceFromRouting(sx)
	m.unregisterCapabilities(sx)
	m.detachExtensions(sx)
	m.clearInstanceState(sx)
	return nil
}

// prepareStop 在持有 inst.mu 的临界区内拍快照并把状态推入 StateRegistered
// (markRegistered 时)。返回 nil 表示插件未注册 / 已经处于 Registered 状态,
// 调用方直接返回。
func (m *PluginManager) prepareStop(name string, markRegistered bool) *stopCtx {
	inst, ok := m.lookupInstance(name)
	if !ok {
		return nil
	}

	inst.mu.Lock()
	state := inst.State
	cmd := inst.Cmd
	stub := inst.LifecycleStub
	cancelProc := inst.CancelFunc
	exited := inst.Exited
	// 把 state 提前置为 Registered, 防止 waitProcessExit 把这次主动 stop
	// 的 exit 当成 unexpected 后写 LastError + 触发 scheduleRestart.
	if markRegistered {
		inst.State = StateRegistered
	}
	inst.CleanupHealth()
	inst.mu.Unlock()

	if state == StateRegistered {
		return nil
	}
	return &stopCtx{
		name:           name,
		inst:           inst,
		state:          state,
		cmd:            cmd,
		stub:           stub,
		cancelProc:     cancelProc,
		exited:         exited,
		markRegistered: markRegistered,
	}
}

// gracefulShutdownRPC 调用插件 Shutdown RPC,失败仅记录日志 (后续 SIGKILL 兜底).
func (m *PluginManager) gracefulShutdownRPC(ctx context.Context, sx *stopCtx) {
	if sx.stub == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, m.cfg.ShutdownTimeout)
	_, err := sx.stub.Shutdown(shutdownCtx, &emptypb.Empty{})
	cancel()
	if err != nil {
		m.logger.Warn("plugin shutdown rpc failed",
			"plugin", sx.name, "error", err)
	}
}

// waitProcessOrTimeout 等待进程退出 — 通过 inst.Exited (waitProcessExit 完成时
// close), 不再自己调 cmd.Wait. 超时则 cancel context 触发 kill, 然后再等 Exited.
func (m *PluginManager) waitProcessOrTimeout(sx *stopCtx) {
	if sx.cmd == nil || sx.exited == nil {
		return
	}
	select {
	case <-sx.exited:
	case <-time.After(m.cfg.ProcessExitTimeout):
		if sx.cancelProc != nil {
			sx.cancelProc()
		}
		<-sx.exited
	}
}

// removeInstanceFromRouting 把 plugin 的路由表项摘掉。
func (m *PluginManager) removeInstanceFromRouting(sx *stopCtx) {
	newTable := m.router.CurrentTable().RemovePlugin(sx.name)
	m.router.SwapRouteTable(newTable)
}

// unregisterCapabilities 撤销 SDKServer 上的 capability 授权 + events allow-list。
func (m *PluginManager) unregisterCapabilities(sx *stopCtx) {
	m.sdkServer.UnregisterPlugin(sx.name)
	if m.eventsAllowList != nil {
		m.eventsAllowList.Forget(sx.name)
	}
}

// detachExtensions 释放 Settings 订阅 + Pricing 客户端。两者都在 spawn 时挂上,
// 这里逆向解开 — 注意 Pricing 必须先从 BillingService 摘掉再 Stop, 避免
// in-flight CalculateCostUnified 与即将退出的 watch loop 竞争。
//
// detachPricingClient 实现挪到 manager_pricing.go (T20 拆分),
// 与 spawn 期的 tryStartPricingExtension 配对维护。
func (m *PluginManager) detachExtensions(sx *stopCtx) {
	sx.inst.mu.Lock()
	settingsUnsub := sx.inst.settingsUnsubscribe
	sx.inst.settingsUnsubscribe = nil
	pricingClient := sx.inst.pricingClient
	sx.inst.pricingClient = nil
	maintenanceClient := sx.inst.maintenanceClient
	sx.inst.maintenanceClient = nil
	sx.inst.mu.Unlock()

	if settingsUnsub != nil {
		settingsUnsub()
	}
	m.detachPricingClient(pricingClient)
	m.detachMaintenanceClient(maintenanceClient)
	m.unregisterGatewayProviders(sx.name, sx.inst.Manifest)
	m.platformRegistry.Unregister(sx.name)
	if m.settingsService != nil {
		m.settingsService.UnregisterSchema(sx.name)
	}
}

// clearInstanceState 关闭 gRPC + 清空 instance 上的运行时字段。
// State 已经在 prepareStop 阶段根据 markRegistered 置为 Registered, 这里不再重复.
func (m *PluginManager) clearInstanceState(sx *stopCtx) {
	sx.inst.mu.Lock()
	defer sx.inst.mu.Unlock()
	sx.inst.CloseGRPC()
	sx.inst.Cmd = nil
	sx.inst.CancelFunc = nil
	sx.inst.GRPCAddr = ""
	sx.inst.HTTPAddr = ""
	sx.inst.Exited = nil
	sx.inst.EntryJSHash = ""
	sx.inst.EntryCSSHash = ""
}

// =============================================================
// Spawn -> instance state attachment
//
// attachInstanceState lives here (not in manager_spawn.go) because the field
// set it writes - Cmd / GRPCConn / LifecycleStub / HealthCancel / Exited - is
// owned by the lifecycle layer: stopInstance / clearInstanceState read and
// nil them out under inst.mu. Keeping write + clear in the same file keeps
// the invariant ("these fields move together under inst.mu") visible.
// =============================================================

// attachInstanceState 把 spawnCtx 中的就绪结果写入 PluginInstance 并
// 把状态推到 StateRunning。锁住 inst.mu 一次完成所有字段更新, 避免
// 半就绪状态被外部观察到。
func (m *PluginManager) attachInstanceState(sc *spawnCtx, healthCancel context.CancelFunc) error {
	sc.inst.mu.Lock()
	defer sc.inst.mu.Unlock()

	sc.inst.Cmd = sc.cmd
	sc.inst.CancelFunc = sc.cancelProc
	sc.inst.GRPCAddr = sc.hs.GRPCAddr
	sc.inst.HTTPAddr = sc.hs.HTTPAddr
	sc.inst.GRPCConn = sc.conn
	sc.inst.LifecycleStub = sc.lifecycle
	sc.inst.Manifest = sc.manifest
	sc.inst.StartedAt = time.Now()
	sc.inst.HealthCancel = healthCancel
	// Re-arm the exited channel for this spawn - Exited is one-shot per process,
	// stopInstance and waitProcessExit synchronize on it. A previous spawn's
	// already-closed chan would let stopInstance return immediately before the
	// new process actually exits.
	sc.inst.Exited = make(chan struct{})
	if err := sc.inst.transitionTo(StateRunning); err != nil {
		return err
	}
	return nil
}
