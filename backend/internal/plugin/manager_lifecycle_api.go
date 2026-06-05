package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// =============================================================
// 高层生命周期 API (T27 拆分)
//
// 与 manager_lifecycle.go 的关系:
//   - 本文件 (manager_lifecycle_api.go): 公开的高层入口, 由 admin handler /
//     CLI / 自动化任务调用; 只描述 "做什么" (Start / ShutdownAll /
//     EnablePlugin / DisablePlugin / RestartPlugin)。
//   - manager_lifecycle.go               : 内部生命周期实现细节 (stopInstance
//     的多个 helper / waitProcessExit / scheduleRestart /
//     handlePluginSettingsEvents)。
//
// 拆分原则: 协议级入口 (高层 API) 与内部状态机 (低层 helper) 解耦, 文件单一职责。
// =============================================================

// Start 启动 SDK gRPC 服务并加载所有 enabled 插件。
// ctx 仅用于初始的 SDK 启动与 enabled 插件加载,不影响后续插件的健康监控。
func (m *PluginManager) Start(ctx context.Context) error {
	m.mu.RLock()
	router := m.router
	m.mu.RUnlock()
	if router == nil {
		return errors.New("plugin manager: router not bound; call BindRouter before Start")
	}
	m.ensureShutdownCtx()

	if err := m.repo.EnsureSchema(ctx); err != nil {
		// 表不存在时回退到 EnsureSchema;若仍失败则视为致命。
		return err
	}

	// Wire the plugin model support checker — it wraps PlatformRegistry
	// so it dynamically resolves to whichever plugin owns each platform.
	m.wireModelSupportChecker()
	// Wire the plugin scheduling hints provider — same pattern.
	m.wireSchedulingHintsProvider()
	// Wire the plugin schedulability checker — same pattern.
	m.wireSchedulabilityChecker()
	if err := m.startSDKServer(); err != nil {
		return err
	}

	// 从磁盘扫描插件,合并到 DB。Builtin 目录中的插件首次启动时按
	// AutoEnableBuiltin 设置自动启用；Disk 目录的插件创建为 disabled。
	if m.cfg.BuiltinDir != "" || m.cfg.PluginsDir != "" {
		if err := m.syncFromDisk(ctx); err != nil {
			m.logger.Warn("sync plugins from disk failed", "error", err)
		}
	}

	records, err := m.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list plugins: %w", err)
	}
	for _, rec := range records {
		if !rec.Enabled {
			continue
		}
		if err := m.EnablePlugin(ctx, rec.Name); err != nil {
			m.logger.Error("auto enable plugin failed",
				"plugin", rec.Name,
				"error", err,
			)
		}
	}
	return nil
}

// ShutdownAll 优雅关闭所有插件,然后停止 SDK gRPC。
func (m *PluginManager) ShutdownAll(ctx context.Context) {
	// 先 cancel shutdown ctx, 让 scheduleRestart 派生的 goroutine 立刻退出,
	// 不会与即将到来的 stopInstance 抢资源。
	if m.shutdownCancel != nil {
		m.shutdownCancel()
	}

	names := m.snapshotPluginNames()

	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if err := m.stopInstance(ctx, n, false); err != nil {
				m.logger.Warn("shutdown plugin failed", "plugin", n, "error", err)
			}
		}(name)
	}
	wg.Wait()

	if m.sdkGRPC != nil {
		m.sdkGRPC.GracefulStop()
	}
	// Stop the JobScheduler before SDKServer so per-plugin schedulers can
	// flush pending acks while their RecordRun sink is still alive.
	if m.jobScheduler != nil {
		m.jobScheduler.Stop()
	}
	if m.sdkServer != nil {
		m.sdkServer.Stop()
	}
}

// EnablePlugin 启动指定插件。已经在运行则视为无操作并返回成功。
func (m *PluginManager) EnablePlugin(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	rec, err := m.repo.Get(ctx, name)
	if err != nil {
		return err
	}

	binPath := m.binaryPathFor(name)
	if binPath == "" {
		return fmt.Errorf("plugin binary not found for %s", name)
	}

	inst := m.getOrCreateInstance(name, binPath)

	inst.mu.Lock()
	if inst.State == StateRunning || inst.State == StateStarting {
		inst.mu.Unlock()
		// 持久化 enabled=true,即便已经在运行。
		_ = m.repo.SetEnabled(ctx, name, true)
		return nil
	}
	if err := inst.transitionTo(StateStarting); err != nil {
		inst.mu.Unlock()
		return err
	}
	cfgCopy := copyConfig(rec.Config)
	inst.mu.Unlock()

	if err := m.spawnAndConnect(ctx, inst, cfgCopy); err != nil {
		inst.mu.Lock()
		_ = inst.transitionTo(StateErrored)
		inst.LastError = err
		inst.mu.Unlock()
		return err
	}

	if err := m.repo.SetEnabled(ctx, name, true); err != nil {
		m.logger.Warn("persist enabled flag failed", "plugin", name, "error", err)
	}
	// Plugin manifest is now part of GetPluginManifestsJSON output; drop the
	// frontend HTML cache so the next reload sees this plugin in sidebar/router.
	m.invalidateFrontendCache()
	return nil
}

// DisablePlugin 停止指定插件,并将 DB 中的 enabled 字段置为 false。
func (m *PluginManager) DisablePlugin(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	if err := m.stopInstance(ctx, name, true); err != nil {
		return err
	}
	if err := m.repo.SetEnabled(ctx, name, false); err != nil {
		return err
	}
	// Plugin removed from GetPluginManifestsJSON output; invalidate so the
	// next reload removes its sidebar/router entries.
	m.invalidateFrontendCache()
	return nil
}

// RestartPlugin 先停后启。
func (m *PluginManager) RestartPlugin(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	if err := m.stopInstance(ctx, name, false); err != nil {
		m.logger.Warn("restart: stop step failed", "plugin", name, "error", err)
	}
	return m.EnablePlugin(ctx, name)
}
