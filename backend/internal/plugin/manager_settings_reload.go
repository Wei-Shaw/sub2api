package plugin

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// =============================================================
// Settings change → reload plumbing
//
// 把 PluginSettingsService 的 value-change 事件流接到 plugin reload。
// 与 manager_lifecycle.go (stopInstance / restart scheduler) 解耦,
// 避免 lifecycle 文件再涨。spawn 期由 spawnSettingsCapabilities 触发
// `go m.handlePluginSettingsEvents(...)`,stop 期由 detachExtensions
// 通过 settingsUnsubscribe 关闭事件 channel,这里收到 channel close 后
// 自然退出 — 详见 DESIGN §4.4 / §4.5。
// =============================================================

// reloadCoalesceWindow caps how often a plugin can be reloaded in response
// to RequiresReload settings changes (DESIGN §4.4). Multiple events arriving
// within this window collapse into a single reload, protecting plugins from
// rapid-fire admin saves that would otherwise restart the process every few
// seconds. Pin to 2 seconds — DESIGN constant, do not vary per environment.
const reloadCoalesceWindow = 2 * time.Second

// handlePluginSettingsEvents drains the per-plugin settings change channel
// and triggers a plugin reload when any change carries RequiresReload=true.
//
// Lifecycle:
//   - Started from spawnAndConnect after a successful Subscribe.
//   - Exits when the channel is closed. Two paths close the channel:
//     1. stopInstance calls the unsubscribe func captured on the instance
//     (normal shutdown / restart). The handler simply returns.
//     2. PluginSettingsService.dropAllSubscribersForPlugin closes every
//     subscriber as part of a schema_version change (DESIGN §4.5). The
//     plugin is expected to restart shortly after — when spawnAndConnect
//     runs again, a fresh subscription + handler are established. The
//     manager does NOT auto-resubscribe because the dropped subscription
//     coincides with a plugin-level lifecycle transition.
//   - Pending reload timers are stopped on exit so we never fire a reload
//     for a stopped plugin.
func (m *PluginManager) handlePluginSettingsEvents(pluginName string, ch <-chan service.PluginSettingsChange) {
	// pendingReason holds the most recent key that triggered a coalesced
	// reload. atomic.Value lets the AfterFunc closure (which runs on its
	// own goroutine) read the freshest reason without racing the for-loop
	// writing new events. The DESIGN §4.4 pseudocode glosses over the
	// concurrent access — this typed wrapper closes the gap.
	var (
		pendingReason atomic.Value // string
		timer         *time.Timer
	)
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	fire := func() {
		reason, _ := pendingReason.Load().(string)
		m.reloadPlugin(context.Background(), pluginName, reason)
	}
	for change := range ch {
		if !change.RequiresReload {
			continue
		}
		pendingReason.Store("settings_change:" + change.Key)
		if timer == nil {
			timer = time.AfterFunc(reloadCoalesceWindow, fire)
			continue
		}
		// Reset returns false when the timer has already fired (its
		// AfterFunc goroutine has at least started). In that case the
		// previous reload is already in flight; we allocate a new Timer
		// so the next pending event still gets a fresh coalesce window.
		// The fire closure reads pendingReason atomically, so the reload
		// always reports the freshest key.
		if !timer.Reset(reloadCoalesceWindow) {
			timer = time.AfterFunc(reloadCoalesceWindow, fire)
		}
	}
	// Channel closed. Two paths reach here (DESIGN §4.4 / §4.5):
	//
	//   - stopInstance unsubscribed us: normal shutdown / restart, no
	//     follow-up needed because the plugin is on its way down.
	//   - PluginSettingsService.dropAllSubscribersForPlugin closed every
	//     subscriber after a schema_version bump. The plugin is expected
	//     to be re-spawned (manifest reload), and spawnAndConnect will
	//     install a fresh subscription on the next Subscribe call. We
	//     intentionally do not auto-resubscribe here — that would race
	//     against the in-flight UnregisterSchema / RegisterSchema
	//     sequence and could leak stray goroutines holding stale plugin
	//     state.
	m.logger.Debug("plugin settings event stream closed",
		"plugin", pluginName,
	)
}

// reloadPlugin restarts a plugin in response to a settings change. Mirrors
// RestartPlugin but logs the originating reason for traceability. Errors
// are logged and swallowed — the manager will eventually retry through the
// normal restart-on-unhealthy path if the reload leaves the plugin broken.
func (m *PluginManager) reloadPlugin(ctx context.Context, name, reason string) {
	m.logger.Info("plugin reload triggered by settings change",
		"plugin", name, "reason", reason)
	if err := m.RestartPlugin(ctx, name); err != nil {
		m.logger.Error("plugin reload failed",
			"plugin", name, "reason", reason, "error", err)
	}
}
