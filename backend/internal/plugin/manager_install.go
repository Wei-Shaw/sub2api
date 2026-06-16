package plugin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// =============================================================
// Plugin install / uninstall / purge
//
// 管理"在册 / 软卸载 / 硬卸载"三种持久状态, 只读写 plugins /
// plugin_settings / plugin_settings_schemas / plugin_migrations 四张表
// 与对应的事务编排; 与 manager.go 的 Enable/Disable (运行时进程启停)
// 正交。磁盘扫描 / 二进制路径解析见 manager_discovery.go, 内存 instance
// map 的惰性创建见 manager_instance_map.go。
// =============================================================

// ErrPluginIsBuiltin 表示尝试对内置插件执行 Uninstall。内置插件随发行包装载,
// 不允许通过 admin 软卸载 — 否则下次启动时 syncFromDisk 与 admin 之间会陷入
// 反复登记/卸载的循环。返回 ErrPluginIsBuiltin 让 handler 映射成 400。
var ErrPluginIsBuiltin = errors.New("builtin plugins cannot be uninstalled")

// ErrPluginNotSoftUninstalled 表示在仍处于 active (uninstalled_at IS NULL) 的
// 插件上调用 Purge。硬卸载是不可逆的, 必须先经过软卸载阶段; 这强制 admin
// 走完 "先 Uninstall, 再 Purge" 的流程, 防止误操作直接清掉数据。
// handler 把这个错误映射成 HTTP 409 Conflict。
var ErrPluginNotSoftUninstalled = errors.New("plugin must be soft-uninstalled before purge")

// ErrPluginNotDisabled 表示在仍处于 enabled 的插件上调用 RemoveFiles。
// 必须先禁用(停止)插件才能删除其文件。handler 映射成 HTTP 409 Conflict。
var ErrPluginNotDisabled = errors.New("plugin must be disabled before removing files")

// Uninstall 软卸载插件: 停进程 + 标记 uninstalled_at = NOW(), 不删数据。
//
// 流程: 校验 name → 拒绝内置 → 取 DB 行 (含软卸载) → 已软卸载则 no-op →
// 走 disable 路径关进程 (保留数据) → MarkUninstalled (uninstalled_at=NOW,
// enabled=false) → 删 instance map entry + 失效前端缓存。
//
// 数据 (plugin_settings / plugin_settings_schemas / plugin_migrations / plugins
// 行本身) 全部保留, 可通过 Install 撤回。
func (m *PluginManager) Uninstall(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	if m.isBuiltinPlugin(name) {
		return ErrPluginIsBuiltin
	}

	rec, err := m.repo.GetIncludingUninstalled(ctx, name)
	if err != nil {
		return err
	}
	if rec.UninstalledAt != nil {
		// 已经软卸载, 幂等 no-op。
		return nil
	}

	// 关闭进程 (若运行中)。stopInstance 复用 disable 路径 — 数据保留,
	// 健康监控被取消, gRPC 连接关闭。markRegistered=true 让状态回到 Registered,
	// 与之后从 manager.plugins 删除的语义一致。
	if err := m.stopInstance(ctx, name, true); err != nil {
		return fmt.Errorf("stop plugin before uninstall: %w", err)
	}

	if err := m.repo.MarkUninstalled(ctx, name); err != nil {
		return err
	}

	// 从内存 instance map 中移除, 避免 GetPluginManifestsJSON / List 在窗口期内
	// 还看到一个停了的 instance 记录。下次 Install + Enable 时会重新创建。
	m.deleteInstance(name)

	m.logger.Info("plugin uninstalled (soft)",
		"plugin", name,
		"mode", "soft",
		"data_preserved", true,
	)
	m.invalidateFrontendCache()
	return nil
}

// Install 撤销软卸载, 让插件回到 disabled / enabled 状态。
//
// 幂等: 已经活动的插件是 no-op。enabled 字段保持原值 — 如果插件被 Uninstall 时
// 是 enabled=true, MarkUninstalled 已经把它清成 false, 这里不会再自动启进程;
// admin 自行决定是否后续 Enable。
//
// 流程: 校验 name → 取 DB 行 → uninstalled_at==nil 则 no-op → MarkInstalled
// (UPDATE uninstalled_at=NULL) → 失效前端缓存。
func (m *PluginManager) Install(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	rec, err := m.repo.GetIncludingUninstalled(ctx, name)
	if err != nil {
		return err
	}
	if rec.UninstalledAt == nil {
		// 已经活动, 幂等 no-op。
		return nil
	}
	if err := m.repo.MarkInstalled(ctx, name); err != nil {
		return err
	}
	m.logger.Info("plugin installed (restored from soft-uninstall)",
		"plugin", name,
	)
	m.invalidateFrontendCache()
	return nil
}

// Purge 硬卸载: 物理清除一个已经软卸载的插件的全部数据 — plugin_migrations
// (含逐条回滚 down_sql_cached) 、 plugin_settings 、 plugin_settings_schemas 、
// plugins 表本身, 全部 DELETE。不可逆。
//
// 前置条件:
//   - 插件必须先经过 Uninstall (uninstalled_at IS NOT NULL); 否则返回
//     ErrPluginNotSoftUninstalled (handler 映射 409)。 这强制 admin 流程
//     "先软再硬" , 不提供 ?force=true 之类的逃生口避免误清。
//   - 内置插件 (BuiltinDir 下) 一律拒绝 ErrPluginIsBuiltin: 它们的二进制
//     在每次启动时被重新登记, Purge 反而会让 syncFromDisk 立刻把它们重建。
//
// 实现细节: 整个删除流程在单个事务里运行 — 任何 down SQL 失败 / DELETE
// 失败都会整体回滚, 让插件保持软卸载状态可重试。 down_sql_cached IS NULL
// (插件没声明 down_filename) 的迁移视为不可逆, 跳过 SQL 执行只删 bookkeeping。
func (m *PluginManager) Purge(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	if m.isBuiltinPlugin(name) {
		return ErrPluginIsBuiltin
	}

	inst, err := m.repo.GetIncludingUninstalled(ctx, name)
	if err != nil {
		return err
	}
	if inst.UninstalledAt == nil {
		return ErrPluginNotSoftUninstalled
	}

	reverted, skipped, err := m.executePurgeTx(ctx, name)
	if err != nil {
		return err
	}

	m.deleteInstance(name)

	m.logger.Info("plugin purged",
		"plugin", name,
		"migrations_reverted", reverted,
		"migrations_skipped", skipped,
	)
	m.invalidateFrontendCache()
	return nil
}

// RemoveFiles 删除插件的二进制文件目录, 但保留所有数据库数据。
//
// 前置条件:
//   - 插件必须已禁用 (enabled=false); 否则返回 ErrPluginNotDisabled (409)。
//   - 内置插件 (BuiltinDir 下) 一律拒绝 ErrPluginIsBuiltin (400)。
//
// 流程: 校验 name → 拒绝内置 → 确认已禁用 → 停进程 (安全起见) →
// 删除 PluginsDir/<name>/ 目录 → 标记 uninstalled_at → 移除 instance map。
//
// 数据保留: plugins / plugin_settings / plugin_settings_schemas /
// plugin_migrations 全部保留, 只清除磁盘文件。
func (m *PluginManager) RemoveFiles(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	if m.isBuiltinPlugin(name) {
		return ErrPluginIsBuiltin
	}

	rec, err := m.repo.GetIncludingUninstalled(ctx, name)
	if err != nil {
		return err
	}
	if rec.Enabled {
		return ErrPluginNotDisabled
	}

	if err := m.stopInstance(ctx, name, true); err != nil {
		m.logger.Warn("stop instance before remove files (non-fatal)",
			"plugin", name, "error", err)
	}

	pluginDir := m.userPluginDir(name)
	if pluginDir == "" {
		return fmt.Errorf("cannot resolve plugin directory for %s", name)
	}
	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("remove plugin files %s: %w", name, err)
	}

	if rec.UninstalledAt == nil {
		if err := m.repo.MarkUninstalled(ctx, name); err != nil {
			return err
		}
	}

	m.deleteInstance(name)

	m.logger.Info("plugin files removed",
		"plugin", name,
		"dir", pluginDir,
		"data_preserved", true,
	)
	m.invalidateFrontendCache()
	return nil
}

// userPluginDir 返回用户插件目录下 plugin name 对应的绝对路径。
// 仅解析 PluginsDir (不含 BuiltinDir), 内置插件不可删除文件。
func (m *PluginManager) userPluginDir(name string) string {
	if m.cfg.PluginsDir == "" || !IsValidPluginName(name) {
		return ""
	}
	absDir, err := filepath.Abs(m.cfg.PluginsDir)
	if err != nil {
		return ""
	}
	candidate := filepath.Join(absDir, name)
	rel, err := filepath.Rel(absDir, candidate)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return ""
	}
	return candidate
}

// executePurgeTx 跑 Purge 事务: 倒序回滚迁移 + DELETE 四张表。
// 任何步骤失败都会整体 rollback, 让插件保持软卸载状态可重试。
func (m *PluginManager) executePurgeTx(ctx context.Context, name string) (reverted, skipped int, err error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("purge plugin %s: begin tx: %w", name, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	reverted, skipped, err = revertCachedDownMigrationsTx(ctx, tx, name)
	if err != nil {
		return 0, 0, fmt.Errorf("purge plugin %s: revert migrations: %w", name, err)
	}

	deletes := []struct {
		table string
		query string
	}{
		{"plugin_migrations", "DELETE FROM plugin_migrations WHERE plugin_name = $1"},
		{"plugin_settings", "DELETE FROM plugin_settings WHERE plugin_name = $1"},
		{"plugin_settings_schemas", "DELETE FROM plugin_settings_schemas WHERE plugin_name = $1"},
		{"plugins", "DELETE FROM plugins WHERE name = $1"},
	}
	for _, d := range deletes {
		if _, err := tx.ExecContext(ctx, d.query, name); err != nil {
			return 0, 0, fmt.Errorf("purge plugin %s: delete from %s: %w", name, d.table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("purge plugin %s: commit: %w", name, err)
	}
	committed = true
	return reverted, skipped, nil
}

// revertCachedDownMigrationsTx 在事务内倒序执行 plugin_migrations 中缓存的
// down_sql_cached, 返回成功执行的条数和因为没有 down 缓存被跳过的条数。
//
// 顺序: filename DESC, 与 up 的 lexicographical 顺序逆向, 与传统数据库迁移
// 工具对称。down_sql_cached IS NULL 的行视为不可逆, 仅计入 skipped, 不报错。
//
// 在 Purge 的事务上下文中直接对外部表执行 DDL — PostgreSQL 允许在事务内
// 跑 DROP TABLE / ALTER TABLE, 任何失败都会随事务回滚, plugin_migrations
// 行也不会被后续的 DELETE 清掉。
func revertCachedDownMigrationsTx(ctx context.Context, tx *sql.Tx, pluginName string) (reverted, skipped int, err error) {
	const q = `SELECT filename, down_sql_cached
	FROM plugin_migrations
	WHERE plugin_name = $1
	ORDER BY filename DESC`
	rows, err := tx.QueryContext(ctx, q, pluginName)
	if err != nil {
		return 0, 0, fmt.Errorf("list plugin migrations: %w", err)
	}
	type entry struct {
		filename string
		downSQL  sql.NullString
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if scanErr := rows.Scan(&e.filename, &e.downSQL); scanErr != nil {
			_ = rows.Close()
			return 0, 0, fmt.Errorf("scan plugin migration: %w", scanErr)
		}
		entries = append(entries, e)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		_ = rows.Close()
		return 0, 0, fmt.Errorf("iterate plugin migrations: %w", rowsErr)
	}
	_ = rows.Close()

	for _, e := range entries {
		if !e.downSQL.Valid || strings.TrimSpace(e.downSQL.String) == "" {
			skipped++
			continue
		}
		if _, execErr := tx.ExecContext(ctx, e.downSQL.String); execErr != nil {
			return reverted, skipped, fmt.Errorf("apply down migration %s: %w", e.filename, execErr)
		}
		reverted++
	}
	return reverted, skipped, nil
}
