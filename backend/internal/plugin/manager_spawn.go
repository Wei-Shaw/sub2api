package plugin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// =============================================================
// Plugin spawn pipeline (state machine)
//
// spawnAndConnect 串联 11+ 个独立步骤(pipe / handshake / dial / GetManifest /
// approveCapabilities / RegisterPlugin / SetTables / settings / events / Init /
// migrations / route / health / pricing). 历史实现写成 260 行平铺函数,
// 嵌套 4 层, 每个失败分支都重复 5 行清理 — 共 5 处复制粘贴。
//
// T7 改造: 用 spawnStage + spawnCtx 表达成"只往前走 + 出错自动逆序回滚"
// 的数据驱动管线。每个 stage:
//
//   - do:       执行该 stage, 把跨 stage 共享的状态写回 spawnCtx
//   - rollback: do 失败时, 之前已执行的 stage 按倒序回滚 (do 自身失败的
//     stage 不调用其 rollback — 假设 do 内部 self-clean)
//
// 新增 stage 只需要在 spawnStages slice 里加一行,主流程不动;失败清理
// 不再散落在各 stage 内部 if 分支里。
//
// 文件拆分(T34):
//   - manager_spawn.go             : spawnCtx + spawnAndConnect + spawnStages
//                                    + 早期 stage(pipe / handshake / dial /
//                                    manifest)
//   - manager_spawn_capabilities.go: capability 层 stage(approve / tables /
//                                    events / settings)
//   - manager_spawn_runtime.go     : 运行期 stage(init / migrations / route /
//                                    health / pricing)
// =============================================================

// spawnStage 描述管线中的一步及其回滚动作。
type spawnStage struct {
	name     string
	do       func(*spawnCtx) error
	rollback func(*spawnCtx)
}

// spawnCtx 在 stages 之间传递的共享状态。
//
// 字段在每个 stage 内被赋值;rollback 函数靠这些字段撤销之前的副作用。
// 保持私有,不对包外暴露。
type spawnCtx struct {
	mgr          *PluginManager
	parentCtx    context.Context
	inst         *PluginInstance
	pluginConfig map[string]string

	// pipe / handshake
	cmd        *exec.Cmd
	cancelProc context.CancelFunc
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	hs         handshakeMessage

	// gRPC connection + initial RPCs
	conn      *grpc.ClientConn
	lifecycle pluginsdk.PluginLifecycleClient
	manifest  *pluginsdk.ManifestResponse
	approved  []string

	// tracks whether settings subscription was created so rollback can
	// detach it. spawnSettingsCapabilities sets to true only when both the
	// service is wired AND the plugin shipped a schema.
	settingsRegistered bool

	// healthCancel is the cancel func for the HealthMonitor goroutine the
	// final stage starts. spawnRollbackRoutes does NOT need to touch it
	// because rollback only runs on stage failure, and health is the last
	// stage — if it succeeds the spawn returns nil.
	healthCancel context.CancelFunc
}

// spawnAndConnect 启动子进程,读取握手,连接 gRPC,跑迁移,注册路由,启动健康监控。
// 任一步失败会按倒序回滚已执行的 stage 并返回错误。
//
// 每个 stage 函数 ≤ 30 行;rollback 函数 ≤ 20 行。新增能力只需:
//  1. 写一对 spawn<Stage> / spawnRollback<Stage>(在合适的 manager_spawn_*.go
//     文件中)
//  2. 把 stage 加进 spawnStages 返回值
//
// 主流程不动。
func (m *PluginManager) spawnAndConnect(parentCtx context.Context, inst *PluginInstance, pluginConfig map[string]string) error {
	sc := &spawnCtx{
		mgr:          m,
		parentCtx:    parentCtx,
		inst:         inst,
		pluginConfig: pluginConfig,
	}
	stages := m.spawnStages()

	executed := make([]spawnStage, 0, len(stages))
	failed := false
	defer func() {
		if !failed {
			return
		}
		// 倒序回滚已成功 stage。do 失败的 stage 不在 executed 中,
		// 假设 do 内部自身已清理它产生的局部资源。
		for i := len(executed) - 1; i >= 0; i-- {
			if rb := executed[i].rollback; rb != nil {
				rb(sc)
			}
		}
	}()

	for _, st := range stages {
		m.logger.Debug("spawn stage start",
			"plugin", inst.Name, "stage", st.name)
		if err := st.do(sc); err != nil {
			failed = true
			return fmt.Errorf("spawn stage %s: %w", st.name, err)
		}
		executed = append(executed, st)
	}
	return nil
}

// spawnStages 返回当前生命周期阶段定义。顺序敏感:每个 stage 假定其
// 上一个 stage 已经填充 spawnCtx 中所需字段。
//
// 实现按文件分布(T34):早期 stage 在本文件,capability 在
// manager_spawn_capabilities.go,运行期在 manager_spawn_runtime.go。
func (m *PluginManager) spawnStages() []spawnStage {
	return []spawnStage{
		{name: "pipe", do: m.spawnPipe, rollback: m.spawnRollbackPipe},
		{name: "handshake", do: m.spawnHandshake, rollback: nil},
		{name: "dial", do: m.spawnDial, rollback: m.spawnRollbackDial},
		{name: "manifest", do: m.spawnManifest, rollback: nil},
		{name: "capabilities", do: m.spawnApproveCapabilities, rollback: m.spawnRollbackCapabilities},
		{name: "tables", do: m.spawnSetTables, rollback: nil},
		{name: "events", do: m.spawnEventsCapabilities, rollback: m.spawnRollbackEvents},
		{name: "settings", do: m.spawnSettingsCapabilities, rollback: m.spawnRollbackSettings},
		{name: "init", do: m.spawnInit, rollback: nil},
		{name: "migrations", do: m.spawnMigrations, rollback: nil},
		{name: "route_validate", do: m.spawnValidateRoutes, rollback: nil},
		{name: "route_install", do: m.spawnInstallRoutes, rollback: m.spawnRollbackRoutes},
		{name: "health", do: m.spawnStartHealth, rollback: nil},
		{name: "pricing", do: m.spawnTryPricing, rollback: nil},
		{name: "platforms", do: m.spawnRegisterPlatforms, rollback: m.spawnRollbackPlatforms},
		{name: "maintenance", do: m.spawnTryMaintenance, rollback: nil},
		{name: "content_intercept", do: m.spawnTryContentIntercept, rollback: nil},
	}
}

// -------------------------------------------------------------
// Stage: pipe — 启动子进程并捕获 stdio
// -------------------------------------------------------------

func (m *PluginManager) spawnPipe(sc *spawnCtx) error {
	procCtx, cancelProc := context.WithCancel(context.Background())
	cmd := exec.CommandContext(procCtx, sc.inst.BinaryPath)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancelProc()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancelProc()
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancelProc()
		return fmt.Errorf("start plugin process: %w", err)
	}

	// 后台转发 stderr 到日志,便于排查启动问题。
	go forwardStderr(stderr, sc.inst.Name)

	sc.cmd = cmd
	sc.cancelProc = cancelProc
	sc.stdout = stdout
	sc.stderr = stderr
	return nil
}

func (m *PluginManager) spawnRollbackPipe(sc *spawnCtx) {
	if sc.cancelProc != nil {
		sc.cancelProc()
	}
	if sc.cmd != nil {
		_ = sc.cmd.Wait()
	}
}

// -------------------------------------------------------------
// Stage: handshake — 读取插件握手 + loopback 校验
// -------------------------------------------------------------

func (m *PluginManager) spawnHandshake(sc *spawnCtx) error {
	hs, err := readHandshake(sc.stdout, m.cfg.HandshakeTimeout)
	if err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}

	// Reject plugin gRPC/HTTP addresses that are not on the loopback interface.
	// SDK default is "127.0.0.1:0" so this is a no-op for any plugin built with
	// pluginsdk.Run; the check fires only when a third-party/replaced binary
	// hand-rolled handshake tries to expose the plugin channel cross-tenant.
	if !m.cfg.AllowNonLoopbackPluginAddr {
		if err := validateLoopbackAddr(hs.GRPCAddr, false); err != nil {
			slog.Error("plugin grpc_addr rejected: not loopback",
				"plugin", sc.inst.Name, "grpc_addr", hs.GRPCAddr, "error", err)
			return fmt.Errorf("plugin grpc_addr not loopback: %w", err)
		}
		if err := validateLoopbackAddr(hs.HTTPAddr, true); err != nil {
			slog.Error("plugin http_addr rejected: not loopback",
				"plugin", sc.inst.Name, "http_addr", hs.HTTPAddr, "error", err)
			return fmt.Errorf("plugin http_addr not loopback: %w", err)
		}
	}
	sc.hs = hs
	return nil
}

// -------------------------------------------------------------
// Stage: dial — 建立到插件的 gRPC 连接
// -------------------------------------------------------------

func (m *PluginManager) spawnDial(sc *spawnCtx) error {
	conn, lifecycle, err := dialPlugin(sc.parentCtx, sc.hs.GRPCAddr, m.cfg.GRPCDialTimeout)
	if err != nil {
		return err
	}
	sc.conn = conn
	sc.lifecycle = lifecycle
	return nil
}

func (m *PluginManager) spawnRollbackDial(sc *spawnCtx) {
	if sc.conn != nil {
		_ = sc.conn.Close()
	}
}

// -------------------------------------------------------------
// Stage: manifest — 拉取 plugin manifest
// -------------------------------------------------------------

func (m *PluginManager) spawnManifest(sc *spawnCtx) error {
	manifestCtx, cancelManifest := context.WithTimeout(sc.parentCtx, m.cfg.ManifestTimeout)
	manifest, err := sc.lifecycle.GetManifest(manifestCtx, &emptypb.Empty{})
	cancelManifest()
	if err != nil {
		return fmt.Errorf("get manifest: %w", err)
	}
	sc.manifest = manifest
	return nil
}
