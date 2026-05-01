package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// =============================================================
// Plugin disk discovery + binary path resolution
//
// 这一组方法负责把"磁盘上的插件目录"翻译成 PluginManager 能用的状态:
//   - syncFromDisk    : 扫描 BuiltinDir / PluginsDir, 把新插件登记到 DB
//   - binaryPathFor   : 决定 plugin name 对应哪个二进制 (BuiltinDir 优先)
//   - isBuiltinPlugin : 判断 name 是否对应 BuiltinDir 下的二进制
//
// 与 manager_install.go 里的 Install/Uninstall/Purge 解耦: 这里只读磁盘
// + 写 DB 的 plugins 表, 不操作内存 instance map, 也不触碰 plugin_settings
// 等附属表。 spawn pipeline (manager_spawn.go) 通过 binaryPathFor 拿到二进制
// 路径后, 只消费 inst.BinaryPath, 不再回头读磁盘。
// =============================================================

// syncFromDisk 把磁盘上发现的插件登记到 DB（若不存在）。
// 来自 BuiltinDir 的插件在 AutoEnableBuiltin=true 时默认 enabled=true,
// 来自 PluginsDir(用户目录)的插件默认 disabled,需通过 admin API 手动启用。
// 已经在 DB 中存在的记录保持不变,不会反复 reset 用户后续手动操作的 enabled 状态。
func (m *PluginManager) syncFromDisk(ctx context.Context) error {
	discovered, err := DiscoverFromDirs(m.cfg.BuiltinDir, m.cfg.PluginsDir)
	if err != nil {
		return err
	}
	for _, d := range discovered {
		// 用 GetIncludingUninstalled 避免重新登记已被软卸载的插件 — soft-uninstall
		// 期望持续存在直到 admin 显式恢复, 不能被磁盘扫描静默撤销。
		_, err := m.repo.GetIncludingUninstalled(ctx, d.Name)
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrPluginNotFound) {
			m.logger.Warn("get plugin from db failed",
				"plugin", d.Name, "error", err)
			continue
		}
		enabled := d.Builtin && m.cfg.AutoEnableBuiltin
		newRec := &PluginRecord{
			Name:    d.Name,
			Enabled: enabled,
			Config:  map[string]string{},
		}
		if err := m.repo.Create(ctx, newRec); err != nil {
			m.logger.Warn("register discovered plugin failed",
				"plugin", d.Name, "error", err)
			continue
		}
		m.logger.Info("plugin registered",
			"plugin", d.Name, "builtin", d.Builtin, "enabled", enabled)
	}
	return nil
}

// binaryPathFor 解析 plugin 二进制的绝对路径: BuiltinDir 优先 (同名时官方版本
// 覆盖用户版本), 然后 PluginsDir。两段路径都会过 IsValidPluginName + Rel
// 校验, 避免名字注入跳出 plugin 根目录。
func (m *PluginManager) binaryPathFor(name string) string {
	// 名字校验作为 defense-in-depth: 即便 admin 入口已经挡过, 这里再过一遍能挡住
	// disk discovery / repo.Create 这种非 HTTP 的注入路径。
	if !IsValidPluginName(name) {
		return ""
	}
	// BuiltinDir 优先（同名时官方版本覆盖用户版本）。
	for _, dir := range []string{m.cfg.BuiltinDir, m.cfg.PluginsDir} {
		if dir == "" {
			continue
		}
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		candidate := filepath.Join(absDir, name, pluginBinaryName(name))
		// rel 二次校验: 候选路径必须仍位于 absDir 之下。filepath.Join 已 Clean
		// 路径, 但若 name 是绝对路径或注入了平台相关分隔符仍可能逃逸 — Rel
		// 是最后一道关。
		rel, err := filepath.Rel(absDir, candidate)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

// isBuiltinPlugin 判断 name 是否对应 BuiltinDir 下的二进制。
//
// 实现细节: BuiltinDir 为空 (e.g. 没有内置插件目录) 时一律返回 false; 否则用
// filepath.Stat 检查 BuiltinDir/<name>/<binary> 是否存在, 命中即视为 builtin。
// 不依赖 DB 状态, 因此对软卸载行也能正确识别。
func (m *PluginManager) isBuiltinPlugin(name string) bool {
	if m.cfg.BuiltinDir == "" {
		return false
	}
	if !IsValidPluginName(name) {
		return false
	}
	absDir, err := filepath.Abs(m.cfg.BuiltinDir)
	if err != nil {
		return false
	}
	candidate := filepath.Join(absDir, name, pluginBinaryName(name))
	rel, err := filepath.Rel(absDir, candidate)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return true
	}
	return false
}
