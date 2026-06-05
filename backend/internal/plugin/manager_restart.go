package plugin

import (
	"context"
	"fmt"
	"time"
)

// =============================================================
// Process exit watcher + auto restart scheduler
//
// 把"插件进程退出 → 决策是否重启 → 延迟后重新 spawn"的链路从
// manager_lifecycle.go (stopInstance 系列) 单独拆出来,使两条链路
// 在文件层面互不耦合:
//
//   - lifecycle: 主动 stop / detach / clear instance state
//   - restart  : 被动监听 cmd.Wait + RestartPolicy 推动 respawn
//
// 两边共享 inst.mu 的字段,但调用方向单一(restart 只调
// spawnAndConnect / repo.Get),不会回头改 stopInstance 的 helper。
// =============================================================

// waitProcessExit 等待插件进程退出,触发自动重启决策。
//
// 这是 cmd.Wait() 的*唯一*调用点 — Go 的 exec.Cmd.Wait() 不可重入, 第二次
// 调用 (在 OS 已经 reap child 之后) 会返回 "waitid: no child processes",
// 那个错误一旦冒泡到 LastError 就会在 admin UI 上显示"插件禁用报错". 所以
// stopInstance 改为 select <-inst.Exited 等待进程退出, 而不是再调 Wait().
func (m *PluginManager) waitProcessExit(inst *PluginInstance) {
	inst.mu.Lock()
	cmd := inst.Cmd
	exited := inst.Exited
	inst.mu.Unlock()
	if cmd == nil {
		if exited != nil {
			close(exited)
		}
		return
	}
	err := cmd.Wait()

	inst.mu.Lock()
	currentState := inst.State
	// 只在非主动 stop 路径下记录错误. 主动 stop (StateRegistered/StateStarting)
	// 时 cmd.Wait 即使返回非 nil 也是预期路径 (e.g. cancelProc kill), 不应
	// 污染 LastError.
	if err != nil && currentState != StateRegistered && currentState != StateStarting {
		inst.LastError = fmt.Errorf("process exited: %w", err)
	}
	inst.mu.Unlock()

	if exited != nil {
		close(exited)
	}

	if currentState == StateRegistered || currentState == StateStarting {
		// 主动 stop 触发的退出,无需重启。
		return
	}

	m.logger.Warn("plugin process exited unexpectedly",
		"plugin", inst.Name,
		"error", err,
	)
	m.scheduleRestart(inst)
}

// handleUnhealthy 由 HealthMonitor 在连续失败时回调。
func (m *PluginManager) handleUnhealthy(inst *PluginInstance) {
	m.logger.Warn("plugin unhealthy, scheduling restart",
		"plugin", inst.Name,
	)
	// 健康检查失败时,先杀进程让 waitProcessExit 触发重启路径。
	inst.mu.Lock()
	cancel := inst.CancelFunc
	inst.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// scheduleRestart 根据 RestartPolicy 决定是否重启,以及重启前的等待时长。
//
// T7 改造:
//   - 把内嵌 goroutine 抽成 runScheduledRestart, 主流程只负责 policy 计算 +
//     状态转移, 立即返回。
//   - runScheduledRestart 监听 m.shutdownCtx, ShutdownAll 触发后立即放弃
//     重启, 避免 host 关闭过程中又新生子进程。
func (m *PluginManager) scheduleRestart(inst *PluginInstance) {
	delay, ok := m.computeRestartDelay(inst)
	if !ok {
		return
	}
	go m.runScheduledRestart(inst, delay)
}

// computeRestartDelay 在持有 inst.mu 的前提下计算下一次重启的延迟。
// 返回 (delay, true) 表示应继续重启; (_, false) 表示已达到上限或无 policy,
// 调用方直接放弃。
func (m *PluginManager) computeRestartDelay(inst *PluginInstance) (time.Duration, bool) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.restartPolicy == nil {
		return 0, false
	}
	policy := inst.restartPolicy
	if !inst.StartedAt.IsZero() && policy.ShouldReset(time.Since(inst.StartedAt)) {
		inst.RestartCount = 0
	}
	delay, ok := policy.NextDelay(inst.RestartCount)
	if !ok {
		_ = inst.transitionTo(StateErrored)
		m.logger.Error("plugin reached max restart attempts, giving up",
			"plugin", inst.Name,
			"max_retries", policy.MaxRetries(),
		)
		return 0, false
	}
	inst.RestartCount++
	_ = inst.transitionTo(StateRestarting)
	m.logger.Info("scheduling plugin restart",
		"plugin", inst.Name,
		"delay", delay.String(),
	)
	return delay, true
}

// runScheduledRestart 在 delay 后实际执行 spawnAndConnect。失败时按 policy
// 再次 scheduleRestart;若期间 manager 进入 ShutdownAll 路径, shutdownCtx
// 被 cancel, 函数立即返回不再启动新进程。
func (m *PluginManager) runScheduledRestart(inst *PluginInstance, delay time.Duration) {
	shutdown := m.shutdownContext()
	select {
	case <-time.After(delay):
	case <-shutdown.Done():
		// host 正在关闭, 放弃后续重启 — inst 状态由 ShutdownAll 的 stopInstance
		// 调用统一收尾, 这里不需要进一步动作。
		return
	}

	// 用后台 ctx 启动,不与外部 enable 操作冲突。
	ctx, cancel := context.WithTimeout(context.Background(),
		m.cfg.HandshakeTimeout+m.cfg.GRPCDialTimeout+m.cfg.ManifestTimeout+10*time.Second)
	defer cancel()

	name := inst.Name
	if !m.transitionToStarting(inst) {
		return
	}

	rec, err := m.repo.Get(ctx, name)
	if err != nil {
		m.logger.Error("restart: load plugin record failed",
			"plugin", name, "error", err)
		inst.mu.Lock()
		_ = inst.transitionTo(StateErrored)
		inst.LastError = err
		inst.mu.Unlock()
		return
	}
	if !rec.Enabled {
		// DB 里已经被禁用,放弃重启。
		inst.mu.Lock()
		_ = inst.transitionTo(StateRegistered)
		inst.mu.Unlock()
		return
	}
	if err := m.spawnAndConnect(ctx, inst, copyConfig(rec.Config)); err != nil {
		inst.mu.Lock()
		_ = inst.transitionTo(StateErrored)
		inst.LastError = err
		inst.mu.Unlock()
		m.logger.Error("plugin restart failed",
			"plugin", name, "error", err)
		// 继续按策略再次重启。
		m.scheduleRestart(inst)
	}
}

// transitionToStarting 在 inst.mu 内把状态从 StateRestarting 推到 StateStarting。
// 返回 false 表示状态已偏移 (e.g. admin 主动 disable), 调用方直接放弃。
func (m *PluginManager) transitionToStarting(inst *PluginInstance) bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.State != StateRestarting {
		return false
	}
	if err := inst.transitionTo(StateStarting); err != nil {
		return false
	}
	return true
}
