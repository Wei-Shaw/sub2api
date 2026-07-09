package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
)

const (
	// stopWaitTimeout 是升级/卸载时等待子进程完全退出的上限，
	// 超时后继续动文件（Supervisor 侧有 SIGKILL 兜底，见 TASK-002）。
	stopWaitTimeout = 30 * time.Second
)

var (
	// ErrBuiltinConflict 标记插件 ID 与内建清单冲突（拒绝安装/卸载内建层）。
	ErrBuiltinConflict = errors.New("pluginhost: plugin id conflicts with a builtin plugin")
	// ErrNotInstalled 标记插件未在 plugin_installations 登记。
	ErrNotInstalled = errors.New("pluginhost: plugin not installed")
)

// Installation 是一条外部插件安装登记（plugin_installations 表的领域对象）。
type Installation struct {
	ID          pluginkit.ID    `json:"id"`
	Version     string          `json:"version"`
	Manifest    *Manifest       `json:"manifest"`
	InstallPath string          `json:"install_path"`
	Checksum    string          `json:"checksum"`
	Config      json.RawMessage `json:"config,omitempty"`
	InstalledBy string          `json:"installed_by"`
	InstalledAt time.Time       `json:"installed_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// InstallationStore 是安装登记表端口（对齐 pluginkit.StateStore 惯例：
// 端口定义在插件内核侧，实现位于 repository 层）。
type InstallationStore interface {
	// Upsert 按 ID 插入或整行替换一条登记。
	Upsert(ctx context.Context, inst *Installation) error
	// Get 读取一条登记；未登记返回 ErrNotInstalled。
	Get(ctx context.Context, id pluginkit.ID) (*Installation, error)
	// List 返回全部登记（按 ID 稳定排序）。
	List(ctx context.Context) ([]*Installation, error)
	// Delete 删除一条登记（不存在时为 no-op）。
	Delete(ctx context.Context, id pluginkit.ID) error
	// SetConfig 更新插件私有配置；未登记返回 ErrNotInstalled。
	SetConfig(ctx context.Context, id pluginkit.ID, config json.RawMessage) error
}

// ExternalRuntime 是安装器与外部插件子进程运行时（Supervisor）的接缝：
// 升级/卸载在 SetEnabled(false) 之后等待子进程完全退出，再替换/删除文件；
// 登记变更后经 Notify* 同步运行时索引（本实例内联，跨实例由运行时自行兜底）。
type ExternalRuntime interface {
	// AwaitStopped 阻塞等待插件子进程完全退出（受 ctx deadline 约束）。
	AwaitStopped(ctx context.Context, id pluginkit.ID) error
	// NotifyInstalled 在安装/升级登记落库后同步运行时索引。
	NotifyInstalled(inst *Installation)
	// NotifyUninstalled 在卸载登记删除后从运行时索引摘除插件。
	NotifyUninstalled(id pluginkit.ID)
}

// InstallerDeps 是构造 Installer 的依赖集合。
type InstallerDeps struct {
	Store         *PackageStore
	Installations InstallationStore
	States        pluginkit.StateStore
	// Runtime 允许为 nil：未接 Supervisor 的测试装配没有子进程需要协调。
	Runtime ExternalRuntime
	// KV 允许为 nil：卸载时清理插件 KV 命名空间（能力面存储）。
	KV KVStore
	// Reserved 是禁止占用的插件 ID 集合（内建清单，见 ReservedIDs）。
	Reserved map[pluginkit.ID]struct{}
	// HostVersion 是宿主构建版本（main.Version），安装时施加 min_core_version
	// 门槛；非法 semver 或开发构建（0.0.0-dev）时跳过检查。
	HostVersion string
	Logger      *slog.Logger
}

// ReservedIDs 从内建插件 Factory 清单提取保留 ID 集合
// （Factory 构造无副作用，仅取 ID）。
func ReservedIDs(factories []pluginkit.Factory) map[pluginkit.ID]struct{} {
	reserved := make(map[pluginkit.ID]struct{}, len(factories))
	for _, factory := range factories {
		reserved[factory().ID()] = struct{}{}
	}
	return reserved
}

// Installer 驱动外部插件的安装/升级/卸载：磁盘落位经 PackageStore，
// 登记经 InstallationStore，启停经 plugin_states 状态机（与内建层共用）。
// 所有操作串行化（admin 低频操作，避免同 ID 并发安装互踩）。
type Installer struct {
	store       *PackageStore
	installs    InstallationStore
	states      pluginkit.StateStore
	runtime     ExternalRuntime
	kv          KVStore
	reserved    map[pluginkit.ID]struct{}
	hostVersion string
	logger      *slog.Logger

	mu sync.Mutex
}

// NewInstaller 创建安装器。
func NewInstaller(deps InstallerDeps) *Installer {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Installer{
		store:       deps.Store,
		installs:    deps.Installations,
		states:      deps.States,
		runtime:     deps.Runtime,
		kv:          deps.KV,
		reserved:    deps.Reserved,
		hostVersion: deps.HostVersion,
		logger:      logger,
	}
}

// MaxPackageBytes 返回插件包大小上限（供 handler 施加 multipart 上限）。
func (i *Installer) MaxPackageBytes() int64 { return i.store.MaxBytes() }

// InstallOrUpgrade 安装 zip 插件包；同 ID 已登记时走升级路径：
//   - 安装（新 ID）：校验清单 → 解包 → 登记 DB。默认 disabled——
//     不写 plugin_states（无行 = disabled），零行为变更；
//   - 升级（已登记）：读原 enabled 态 → 若启用先 SetEnabled(false) 并等子进程停
//     → 解包新版本 → 更新登记（保留 config）→ 移除旧版本目录 → 恢复原 enabled 态。
//
// 包不合法返回 ErrInvalidPackage（含具体原因），ID 撞内建清单返回 ErrBuiltinConflict。
func (i *Installer) InstallOrUpgrade(ctx context.Context, zipPath, installedBy string) (*Installation, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	manifest, err := i.store.ReadManifest(zipPath)
	if err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPackage, err)
	}
	if ok, skipped := manifest.MinCoreVersionSatisfied(i.hostVersion); !ok {
		return nil, fmt.Errorf("%w: requires core version >= %s, host is %s",
			ErrInvalidPackage, manifest.MinCoreVersion, i.hostVersion)
	} else if skipped {
		i.logger.Warn("plugin_min_core_version_check_skipped",
			"plugin", string(manifest.ID), "host_version", i.hostVersion)
	}
	if _, conflict := i.reserved[manifest.ID]; conflict {
		return nil, fmt.Errorf("%w: %q", ErrBuiltinConflict, manifest.ID)
	}
	checksum, err := FileSHA256(zipPath)
	if err != nil {
		return nil, err
	}

	existing, err := i.installs.Get(ctx, manifest.ID)
	if err != nil && !errors.Is(err, ErrNotInstalled) {
		return nil, err
	}

	// 升级路径：换文件前先停（读原 enabled 态，结束后恢复）。
	wasEnabled := false
	if existing != nil {
		wasEnabled = i.states.Enabled(manifest.ID)
		if wasEnabled {
			if err := i.disableAndAwait(ctx, manifest.ID, installedBy); err != nil {
				return nil, err
			}
		}
	}
	restoreEnabled := func() {
		if !wasEnabled {
			return
		}
		if err := i.states.SetEnabled(ctx, manifest.ID, true, installedBy); err != nil {
			i.logger.Error("plugin_upgrade_restore_enabled_failed", "plugin", string(manifest.ID), "error", err)
		}
	}

	installDir, err := i.store.Extract(zipPath, manifest.ID, manifest.Version)
	if err != nil {
		restoreEnabled() // 旧版本文件未动，可安全恢复
		return nil, err
	}
	if err := i.store.MarkExecutable(installDir, manifest); err != nil {
		_ = i.store.RemoveVersion(manifest.ID, manifest.Version)
		restoreEnabled()
		return nil, err
	}

	now := time.Now()
	inst := &Installation{
		ID:          manifest.ID,
		Version:     manifest.Version,
		Manifest:    manifest,
		InstallPath: installDir,
		Checksum:    checksum,
		InstalledBy: installedBy,
		InstalledAt: now,
		UpdatedAt:   now,
	}
	if existing != nil {
		inst.Config = existing.Config // 升级保留管理员配置
	}
	if err := i.installs.Upsert(ctx, inst); err != nil {
		if existing == nil || existing.Version != manifest.Version {
			_ = i.store.RemoveVersion(manifest.ID, manifest.Version)
		}
		restoreEnabled()
		return nil, err
	}
	// 先同步运行时索引再恢复 enabled：restoreEnabled 触发的 Launch 必须看到新登记。
	if i.runtime != nil {
		i.runtime.NotifyInstalled(inst)
	}

	if existing != nil && existing.Version != manifest.Version {
		// 旧版本目录清理失败不影响登记结果，仅记日志（磁盘残留可重试卸载清理）。
		if err := i.store.RemoveVersion(manifest.ID, existing.Version); err != nil {
			i.logger.Warn("plugin_upgrade_remove_old_version_failed",
				"plugin", string(manifest.ID), "version", existing.Version, "error", err)
		}
	}
	restoreEnabled()

	if existing != nil {
		i.logger.Info("plugin_upgraded", "plugin", string(manifest.ID),
			"from_version", existing.Version, "to_version", manifest.Version, "enabled_restored", wasEnabled)
	} else {
		i.logger.Info("plugin_installed", "plugin", string(manifest.ID), "version", manifest.Version)
	}
	return inst, nil
}

// Uninstall 卸载外部插件：若启用先停 → 删除全部版本文件 → 删除登记。
// 未登记返回 ErrNotInstalled。启停行在 plugin_states 中保留为 disabled
// （与无行等价，不需要状态机提供删除能力）。
func (i *Installer) Uninstall(ctx context.Context, id pluginkit.ID, updatedBy string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, err := i.installs.Get(ctx, id); err != nil {
		return err
	}
	if i.states.Enabled(id) {
		if err := i.disableAndAwait(ctx, id, updatedBy); err != nil {
			return err
		}
	}
	// 文件先于登记删除：文件删除失败时保留登记行，便于重试卸载。
	if err := i.store.Remove(id); err != nil {
		return err
	}
	if err := i.installs.Delete(ctx, id); err != nil {
		return err
	}
	if i.runtime != nil {
		i.runtime.NotifyUninstalled(id)
	}
	// KV 命名空间清理为尽力而为：登记与文件已删除，卸载语义已达成，
	// 清理失败仅记日志（残留条目由同 ID 重装后覆盖或人工清理）。
	if i.kv != nil {
		if err := i.kv.DeleteAll(ctx, id); err != nil {
			i.logger.Warn("plugin_uninstall_kv_cleanup_failed", "plugin", string(id), "error", err)
		}
	}
	i.logger.Info("plugin_uninstalled", "plugin", string(id))
	return nil
}

// disableAndAwait 写入 disabled 并等待子进程完全退出（Runtime 未接入时仅写状态）。
func (i *Installer) disableAndAwait(ctx context.Context, id pluginkit.ID, updatedBy string) error {
	if err := i.states.SetEnabled(ctx, id, false, updatedBy); err != nil {
		return fmt.Errorf("pluginhost: disable %s before file swap: %w", id, err)
	}
	if i.runtime == nil {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, stopWaitTimeout)
	defer cancel()
	if err := i.runtime.AwaitStopped(waitCtx, id); err != nil {
		// 等停超时不中止操作：Supervisor 侧有 SIGTERM→SIGKILL 兜底，文件替换继续。
		i.logger.Warn("plugin_await_stopped_failed", "plugin", string(id), "error", err)
	}
	return nil
}
