package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
)

const (
	pluginConfigMaxBytes  = 4 * 1024 * 1024
	pluginUIAssetMaxBytes = 32 * 1024 * 1024
	pluginReconcilePeriod = time.Second
	pluginHealthTimeout   = 5 * time.Second
	pluginUITokenPrefix   = "sub2api:plugin-ui:v1:"
)

type pluginRoute struct {
	pluginID       int64
	runtime        *pluginRuntime
	rolloutPercent int
	unavailable    string
}

// PluginManager 管理插件安装、配置、进程生命周期和 OpenAI OAuth 能力绑定。
type PluginManager struct {
	repo      PluginRepository
	encryptor SecretEncryptor
	cfg       *config.Config
	hostInfo  PluginHostInfo
	installer *PluginPackageInstaller

	operationMu        sync.Mutex
	mu                 sync.Mutex
	runtimes           map[int64]*pluginRuntime
	localInstallations map[int64]*PluginInstallation
	started            bool
	reconcileCancel    context.CancelFunc
	reconcileDone      chan struct{}
	route              atomic.Pointer[pluginRoute]
}

func NewPluginManager(repo PluginRepository, encryptor SecretEncryptor, cfg *config.Config, hostInfo PluginHostInfo) *PluginManager {
	return &PluginManager{
		repo:               repo,
		encryptor:          encryptor,
		cfg:                cfg,
		hostInfo:           hostInfo,
		installer:          NewPluginPackageInstaller(cfg, hostInfo),
		runtimes:           make(map[int64]*pluginRuntime),
		localInstallations: make(map[int64]*PluginInstallation),
	}
}

func (m *PluginManager) MaxUploadBytes() int64 {
	if m == nil || m.cfg == nil {
		return 0
	}
	return m.cfg.Plugins.MaxUploadBytes
}

func (m *PluginManager) Start(ctx context.Context) error {
	m.operationMu.Lock()
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		m.operationMu.Unlock()
		return nil
	}
	if err := os.MkdirAll(filepath.Join(m.installer.RootDir(), "runtime"), 0o700); err != nil {
		m.mu.Unlock()
		m.operationMu.Unlock()
		return fmt.Errorf("创建插件运行目录: %w", err)
	}
	reconcileCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.started = true
	m.reconcileCancel = cancel
	m.reconcileDone = make(chan struct{})
	done := m.reconcileDone
	m.mu.Unlock()
	m.operationMu.Unlock()

	go m.reconcileLoop(reconcileCtx, done)
	if err := m.reconcileOnce(reconcileCtx); err != nil {
		slog.Warn("plugin_initial_reconcile_failed", "error", err)
	}
	return nil
}

func (m *PluginManager) Stop() {
	m.mu.Lock()
	cancel := m.reconcileCancel
	done := m.reconcileDone
	m.reconcileCancel = nil
	m.reconcileDone = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	m.operationMu.Lock()
	m.mu.Lock()
	runtimes := make([]*pluginRuntime, 0, len(m.runtimes))
	for _, runtime := range m.runtimes {
		runtimes = append(runtimes, runtime)
	}
	m.runtimes = make(map[int64]*pluginRuntime)
	m.route.Store(nil)
	m.started = false
	m.mu.Unlock()
	m.operationMu.Unlock()
	for _, runtime := range runtimes {
		runtime.drain(10 * time.Second)
	}
}

func (m *PluginManager) List(ctx context.Context) ([]*PluginInstallation, error) {
	plugins, err := m.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	route := m.route.Load()
	for _, installation := range plugins {
		installation.Compatibility = EvaluatePluginCompatibility(installation.Manifest, m.hostInfo)
		if runtime := m.runtimes[installation.ID]; runtime != nil && !runtime.client.Exited() {
			installation.RuntimeHealthy = true
			installation.RuntimeMessage = "插件进程运行中"
		} else if installation.State == PluginStateEnabled {
			installation.RuntimeMessage = installation.LastError
		}
		if route != nil && route.pluginID == installation.ID && route.runtime == nil {
			installation.RuntimeMessage = route.unavailable
		}
	}
	return plugins, nil
}

func (m *PluginManager) Get(ctx context.Context, id int64) (*PluginInstallation, error) {
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	installation.Compatibility = EvaluatePluginCompatibility(installation.Manifest, m.hostInfo)
	m.mu.Lock()
	runtime := m.runtimes[id]
	m.mu.Unlock()
	installation.RuntimeHealthy = runtime != nil && !runtime.client.Exited()
	if installation.RuntimeHealthy {
		installation.RuntimeMessage = "插件进程运行中"
	} else if route := m.route.Load(); route != nil && route.pluginID == id {
		installation.RuntimeMessage = route.unavailable
	}
	return installation, nil
}

func (m *PluginManager) Install(ctx context.Context, reader io.Reader, installedBy *int64) (*PluginInstallation, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	packageInfo, err := m.installer.Install(ctx, reader, installedBy)
	if err != nil {
		return nil, err
	}
	var previous *PluginInstallation
	if existing, getErr := m.repo.GetByKey(ctx, packageInfo.PluginKey); getErr == nil {
		if existing.State == PluginStateEnabled || hasEnabledOpenAIBinding(existing.Bindings) {
			cleanupErr := m.cleanupInstallationFiles(packageInfo)
			return nil, errors.Join(errors.New("请先停用当前插件，再上传同 ID 的新版本"), cleanupErr)
		}
		previous = existing
	} else if !errors.Is(getErr, sql.ErrNoRows) {
		cleanupErr := m.cleanupInstallationFiles(packageInfo)
		return nil, errors.Join(getErr, cleanupErr)
	}
	bindings := make([]PluginBinding, 0, len(packageInfo.Manifest.Capabilities))
	for _, capability := range packageInfo.Manifest.SortedCapabilities() {
		bindings = append(bindings, PluginBinding{
			Capability:     capability.ID,
			Platform:       capability.Platform,
			AccountType:    capability.AccountType,
			Enabled:        false,
			RolloutPercent: 100,
		})
	}
	installed, err := m.repo.Install(ctx, packageInfo, bindings)
	if err != nil {
		cleanupErr := m.cleanupInstallationFiles(packageInfo)
		return nil, errors.Join(err, cleanupErr)
	}
	local := *packageInfo
	local.ID = installed.ID
	local.ConfigEncrypted = installed.ConfigEncrypted
	local.Bindings = append([]PluginBinding(nil), installed.Bindings...)
	m.mu.Lock()
	localPrevious := m.localInstallations[installed.ID]
	m.localInstallations[installed.ID] = &local
	m.mu.Unlock()
	if previous != nil {
		if cleanupErr := m.cleanupInstallationFiles(previous); cleanupErr != nil {
			slog.Warn("plugin_previous_install_cleanup_failed", "plugin_id", previous.ID, "error", cleanupErr)
		}
		if localPrevious != nil && (localPrevious.InstallPath != previous.InstallPath || localPrevious.ArtifactPath != previous.ArtifactPath) {
			if cleanupErr := m.cleanupInstallationFiles(localPrevious); cleanupErr != nil {
				slog.Warn("plugin_previous_local_install_cleanup_failed", "plugin_id", previous.ID, "error", cleanupErr)
			}
		}
	}
	return m.Get(ctx, installed.ID)
}

func (m *PluginManager) cleanupInstallationFiles(installation *PluginInstallation) error {
	if installation == nil {
		return nil
	}
	var cleanupErr error
	for _, path := range []string{installation.ArtifactPath, installation.InstallPath} {
		if err := m.removeManagedPath(path); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func (m *PluginManager) reconcileLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(pluginReconcilePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.reconcileOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("plugin_reconcile_failed", "error", err)
			}
		}
	}
}

// reconcileOnce 以数据库中的绑定为权威状态，让每个实例独立恢复并启动同一插件。
func (m *PluginManager) reconcileOnce(ctx context.Context) error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	installations, err := m.repo.List(ctx)
	if err != nil {
		// 无法读取权威绑定状态时不能假设插件未启用，否则会把 OAuth 请求静默回落到旧直连路径。
		m.publishUnavailableRoute(0, 100, "插件启用状态暂时无法读取")
		return fmt.Errorf("读取插件启用状态: %w", err)
	}
	m.cleanupStaleLocalInstallations(installations)
	var enabled *PluginInstallation
	for _, installation := range installations {
		if !hasEnabledOpenAIBinding(installation.Bindings) {
			continue
		}
		if enabled != nil {
			err := errors.New("检测到多个 OpenAI OAuth 出站插件同时启用")
			m.publishUnavailableRoute(enabled.ID, 100, err.Error())
			return err
		}
		enabled = installation
	}
	if enabled == nil {
		for _, installation := range installations {
			if installation.State != PluginStateStarting || !m.startingStateExpired(installation) {
				continue
			}
			if err := m.repo.UpdateState(
				ctx, installation.ID, PluginStateDisabled, "插件启动超时，已自动恢复为停用状态", nil,
				installation.BinarySHA256, PluginStateStarting,
			); err != nil && !errors.Is(err, ErrPluginStateChanged) {
				return fmt.Errorf("恢复超时插件状态: %w", err)
			}
		}
		runtimes := m.detachAllRuntimes()
		for _, runtime := range runtimes {
			runtime.drain(10 * time.Second)
		}
		return nil
	}

	rollout := bindingRollout(enabled.Bindings)
	current := m.route.Load()
	if current != nil && current.pluginID == enabled.ID && current.runtime != nil &&
		!current.runtime.client.Exited() && current.rolloutPercent == rollout &&
		current.runtime.installation.BinarySHA256 == enabled.BinarySHA256 &&
		current.runtime.installation.ConfigEncrypted == enabled.ConfigEncrypted {
		healthCtx, cancel := context.WithTimeout(ctx, pluginHealthTimeout)
		healthErr := current.runtime.checkHealth(healthCtx)
		cancel()
		if healthErr != nil {
			if stateErr := m.markRuntimeUnavailable(current, healthErr.Error()); stateErr != nil {
				return errors.Join(healthErr, stateErr)
			}
			return healthErr
		}
		if enabled.State == PluginStateError || (enabled.State == PluginStateStarting && m.startingStateExpired(enabled)) {
			return m.repo.MarkRuntimeHealthy(ctx, enabled.ID, enabled.BinarySHA256, enabled.ConfigEncrypted)
		}
		return nil
	}
	if enabled.State == PluginStateStarting && !m.startingStateExpired(enabled) {
		if current == nil {
			m.route.Store(&pluginRoute{pluginID: enabled.ID, rolloutPercent: rollout, unavailable: "插件正在其他实例中启动"})
		}
		return nil
	}

	local, err := m.ensureLocalInstallation(ctx, enabled)
	if err != nil {
		m.publishUnavailableRoute(enabled.ID, rollout, err.Error())
		return err
	}
	runtime, err := m.prepareRuntime(ctx, local, true)
	if err != nil {
		m.publishUnavailableRoute(enabled.ID, rollout, err.Error())
		return err
	}

	// 启动进程期间绑定可能已在其他实例上变化，发布前必须重新确认。
	latest, err := m.repo.GetByID(ctx, enabled.ID)
	if err != nil {
		runtime.kill()
		return err
	}
	if !hasEnabledOpenAIBinding(latest.Bindings) || latest.BinarySHA256 != enabled.BinarySHA256 ||
		latest.ConfigEncrypted != enabled.ConfigEncrypted || bindingRollout(latest.Bindings) != rollout {
		runtime.kill()
		return nil
	}
	if latest.State == PluginStateStarting && !m.startingStateExpired(latest) {
		runtime.kill()
		return nil
	}
	if err := m.repo.MarkRuntimeHealthy(ctx, enabled.ID, enabled.BinarySHA256, enabled.ConfigEncrypted); err != nil {
		runtime.kill()
		if errors.Is(err, ErrPluginStateChanged) {
			return nil
		}
		return err
	}

	m.mu.Lock()
	stale := make([]*pluginRuntime, 0, len(m.runtimes))
	for id, candidate := range m.runtimes {
		if candidate != runtime {
			candidate.draining.Store(true)
			stale = append(stale, candidate)
		}
		if id != enabled.ID {
			delete(m.runtimes, id)
		}
	}
	m.runtimes[enabled.ID] = runtime
	m.route.Store(&pluginRoute{pluginID: enabled.ID, runtime: runtime, rolloutPercent: rollout})
	m.mu.Unlock()
	for _, candidate := range stale {
		candidate.drain(10 * time.Second)
	}
	return nil
}

func (m *PluginManager) startingStateExpired(installation *PluginInstallation) bool {
	if installation == nil || installation.UpdatedAt.IsZero() {
		return false
	}
	startTimeout := 15 * time.Second
	if m.cfg != nil && m.cfg.Plugins.StartTimeoutSeconds > 0 {
		startTimeout = time.Duration(m.cfg.Plugins.StartTimeoutSeconds) * time.Second
	}
	recoveryDelay := startTimeout + 45*time.Second
	if recoveryDelay < time.Minute {
		recoveryDelay = time.Minute
	}
	return time.Since(installation.UpdatedAt) > recoveryDelay
}

func (m *PluginManager) detachAllRuntimes() []*pluginRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtimes := make([]*pluginRuntime, 0, len(m.runtimes))
	for id, runtime := range m.runtimes {
		runtime.draining.Store(true)
		runtimes = append(runtimes, runtime)
		delete(m.runtimes, id)
	}
	m.route.Store(nil)
	return runtimes
}

func (m *PluginManager) publishUnavailableRoute(pluginID int64, rollout int, message string) {
	m.mu.Lock()
	stale := make([]*pluginRuntime, 0, len(m.runtimes))
	for id, runtime := range m.runtimes {
		runtime.draining.Store(true)
		stale = append(stale, runtime)
		delete(m.runtimes, id)
	}
	m.route.Store(&pluginRoute{pluginID: pluginID, rolloutPercent: rollout, unavailable: message})
	m.mu.Unlock()
	for _, runtime := range stale {
		runtime.drain(10 * time.Second)
	}
}

func (m *PluginManager) ensureLocalInstallation(ctx context.Context, installation *PluginInstallation) (*PluginInstallation, error) {
	if installation == nil {
		return nil, errors.New("插件安装记录为空")
	}
	m.mu.Lock()
	local := m.localInstallations[installation.ID]
	m.mu.Unlock()
	if local != nil && local.BinarySHA256 == installation.BinarySHA256 && local.Version == installation.Version {
		if err := verifyLocalPluginBinary(local, m.installer.RootDir()); err == nil {
			return mergeLocalInstallation(local, installation), nil
		}
	}
	if err := verifyLocalPluginBinary(installation, m.installer.RootDir()); err == nil {
		local = mergeLocalInstallation(installation, installation)
		m.mu.Lock()
		m.localInstallations[installation.ID] = local
		m.mu.Unlock()
		return local, nil
	}

	artifact, err := m.repo.GetArtifact(ctx, installation.ID)
	if err != nil {
		return nil, fmt.Errorf("读取插件包原件: %w", err)
	}
	if len(artifact) == 0 {
		return nil, errors.New("插件包原件缺失，请重新上传插件")
	}
	restored, err := m.installer.Install(ctx, bytes.NewReader(artifact), installation.InstalledBy)
	if err != nil {
		return nil, fmt.Errorf("恢复并复验插件包: %w", err)
	}
	if !samePluginPackage(restored, installation) {
		cleanupErr := m.cleanupInstallationFiles(restored)
		return nil, errors.Join(errors.New("数据库插件包与安装记录不一致"), cleanupErr)
	}
	local = mergeLocalInstallation(restored, installation)
	m.mu.Lock()
	m.localInstallations[installation.ID] = local
	m.mu.Unlock()
	return local, nil
}

// cleanupStaleLocalInstallations 回收本实例缓存中已从数据库删除或已被新包替换的文件。
// 数据库是跨实例的权威状态，本地目录不能因其他实例的卸载/升级永久残留。
func (m *PluginManager) cleanupStaleLocalInstallations(installations []*PluginInstallation) {
	persisted := make(map[int64]*PluginInstallation, len(installations))
	for _, installation := range installations {
		if installation != nil {
			persisted[installation.ID] = installation
		}
	}
	m.mu.Lock()
	stale := make([]*PluginInstallation, 0)
	for id, local := range m.localInstallations {
		current := persisted[id]
		if current == nil || current.BinarySHA256 != local.BinarySHA256 || current.Version != local.Version {
			stale = append(stale, local)
			delete(m.localInstallations, id)
		}
	}
	m.mu.Unlock()
	for _, local := range stale {
		if err := m.cleanupInstallationFiles(local); err != nil {
			slog.Warn("plugin_stale_local_install_cleanup_failed", "plugin_id", local.ID, "error", err)
		}
	}
}

func mergeLocalInstallation(local, persisted *PluginInstallation) *PluginInstallation {
	merged := *persisted
	merged.ArtifactData = nil
	merged.ArtifactPath = local.ArtifactPath
	merged.InstallPath = local.InstallPath
	merged.BinaryPath = local.BinaryPath
	merged.Bindings = append([]PluginBinding(nil), persisted.Bindings...)
	return &merged
}

func samePluginPackage(local, persisted *PluginInstallation) bool {
	if local == nil || persisted == nil || local.PluginKey != persisted.PluginKey ||
		local.Version != persisted.Version || local.BinarySHA256 != persisted.BinarySHA256 {
		return false
	}
	localManifest, localErr := json.Marshal(local.Manifest)
	persistedManifest, persistedErr := json.Marshal(persisted.Manifest)
	return localErr == nil && persistedErr == nil && bytes.Equal(localManifest, persistedManifest)
}

func verifyLocalPluginBinary(installation *PluginInstallation, root string) error {
	if installation == nil {
		return errors.New("插件安装记录为空")
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	installPath, err := filepath.Abs(installation.InstallPath)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(rootPath, installPath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("插件安装目录不在受管目录内")
	}
	runtimeEntry, ok := installation.Manifest.Runtimes[installation.Manifest.RuntimeKey()]
	if !ok {
		return errors.New("插件未声明当前平台运行时")
	}
	binaryPath, err := safePluginJoin(installPath, runtimeEntry.Path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != installation.BinarySHA256 {
		return errors.New("本地插件二进制哈希不匹配")
	}
	installation.BinaryPath = binaryPath
	return nil
}

func (m *PluginManager) Enable(ctx context.Context, id int64, acceptUntested bool, rolloutPercent int) (*PluginInstallation, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	if rolloutPercent < 1 || rolloutPercent > 100 {
		return nil, errors.New("灰度比例必须在 1 到 100 之间")
	}
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if active := m.route.Load(); active != nil && active.pluginID != id {
		return nil, errors.New("OpenAI OAuth 出站能力已有启用插件，请先停用当前插件")
	}
	if installation.State == PluginStateEnabled && hasEnabledOpenAIBinding(installation.Bindings) {
		installation.Compatibility = EvaluatePluginCompatibility(installation.Manifest, m.hostInfo)
		m.mu.Lock()
		runtime := m.runtimes[id]
		m.mu.Unlock()
		installation.RuntimeHealthy = runtime != nil && !runtime.client.Exited()
		if installation.RuntimeHealthy {
			return installation, nil
		}
	}
	compatibility := EvaluatePluginCompatibility(installation.Manifest, m.hostInfo)
	if !compatibility.Compatible {
		stateErr := m.repo.UpdateState(ctx, id, PluginStateIncompatible, compatibility.Message, nil, installation.BinarySHA256, installation.State)
		return nil, errors.Join(errors.New(compatibility.Message), stateErr)
	}
	if !compatibility.Tested && !acceptUntested {
		return nil, errors.New("插件未声明已测试当前 Sub2API 版本，需要管理员确认后启用")
	}
	installation, err = m.ensureLocalInstallation(ctx, installation)
	if err != nil {
		return nil, err
	}
	originalBindings := append([]PluginBinding(nil), installation.Bindings...)
	for index := range installation.Bindings {
		installation.Bindings[index].Enabled = true
		installation.Bindings[index].RolloutPercent = rolloutPercent
	}
	if err := m.repo.BeginEnable(ctx, id, installation.BinarySHA256, installation.State); err != nil {
		return nil, err
	}
	runtime, err := m.prepareRuntime(ctx, installation, true)
	if err != nil {
		stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		stateErr := m.repo.UpdateState(stateCtx, id, PluginStateError, err.Error(), nil, installation.BinarySHA256, PluginStateStarting)
		cancel()
		if hasEnabledOpenAIBinding(originalBindings) {
			m.route.Store(&pluginRoute{pluginID: id, rolloutPercent: bindingRollout(originalBindings), unavailable: err.Error()})
		}
		return nil, errors.Join(err, stateErr)
	}
	now := time.Now()
	if err := m.repo.UpdateBindingsAndState(ctx, id, installation.Bindings, PluginStateEnabled, "", &now, PluginStateStarting, installation.BinarySHA256); err != nil {
		runtime.kill()
		if errors.Is(err, ErrPluginStateChanged) {
			return nil, err
		}
		stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		stateErr := m.repo.UpdateState(stateCtx, id, PluginStateError, err.Error(), nil, installation.BinarySHA256, PluginStateStarting)
		cancel()
		return nil, errors.Join(err, stateErr)
	}
	m.mu.Lock()
	m.publishRuntimeLocked(installation, runtime)
	m.mu.Unlock()
	result, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	result.Compatibility = compatibility
	result.RuntimeHealthy = true
	result.RuntimeMessage = "插件进程运行中"
	return result, nil
}

func (m *PluginManager) Disable(ctx context.Context, id int64) (*PluginInstallation, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	m.mu.Lock()
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	for index := range installation.Bindings {
		installation.Bindings[index].Enabled = false
	}
	if err := m.repo.UpdateBindingsAndState(ctx, id, installation.Bindings, PluginStateDisabled, "", nil, "", installation.BinarySHA256); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	runtime := m.removeRuntimeLocked(id)
	m.mu.Unlock()
	if runtime != nil {
		runtime.drain(10 * time.Second)
	}
	return m.Get(ctx, id)
}

func (m *PluginManager) Delete(ctx context.Context, id int64) error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	m.mu.Lock()
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	if installation.State == PluginStateEnabled || hasEnabledOpenAIBinding(installation.Bindings) {
		m.mu.Unlock()
		return errors.New("请先停用插件，再执行卸载")
	}
	if err := m.repo.Delete(ctx, id, installation.BinarySHA256); err != nil {
		m.mu.Unlock()
		return err
	}
	runtime := m.removeRuntimeLocked(id)
	local := m.localInstallations[id]
	delete(m.localInstallations, id)
	m.mu.Unlock()
	if runtime != nil {
		runtime.drain(10 * time.Second)
	}
	cleanupErr := m.cleanupInstallationFiles(installation)
	if local != nil && (local.InstallPath != installation.InstallPath || local.ArtifactPath != installation.ArtifactPath) {
		cleanupErr = errors.Join(cleanupErr, m.cleanupInstallationFiles(local))
	}
	return cleanupErr
}

func (m *PluginManager) GetConfig(ctx context.Context, id int64) (json.RawMessage, error) {
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return m.decryptConfig(installation)
}

func (m *PluginManager) SaveConfig(ctx context.Context, id int64, raw json.RawMessage) (json.RawMessage, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	if len(raw) == 0 || len(raw) > pluginConfigMaxBytes || !json.Valid(raw) {
		return nil, errors.New("插件配置必须是有效且大小受限的 JSON")
	}
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	previousConfig, err := m.decryptConfig(installation)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	if normalized == nil {
		return nil, errors.New("插件配置 JSON 根节点必须是对象")
	}
	if _, ok := normalized.(map[string]any); !ok {
		return nil, errors.New("插件配置 JSON 根节点必须是对象")
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	runtime := m.runtimes[id]
	m.mu.Unlock()
	temporary := false
	if runtime == nil {
		installation, err = m.ensureLocalInstallation(ctx, installation)
		if err != nil {
			return nil, err
		}
		runtime, err = m.newRuntime(ctx, installation)
		if err != nil {
			return nil, err
		}
		temporary = true
		defer runtime.kill()
	}
	if runtime != nil {
		applyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		canonical, err = runtime.validateAndApplyNormalizedConfig(applyCtx, canonical)
		cancel()
		if err != nil {
			return nil, err
		}
	}
	encrypted, err := m.encryptor.Encrypt(string(canonical))
	if err != nil {
		if !temporary {
			err = errors.Join(err, m.restoreRuntimeConfig(id, runtime, previousConfig))
		}
		return nil, fmt.Errorf("加密插件配置: %w", err)
	}
	if err := m.repo.UpdateConfig(ctx, id, encrypted, installation.BinarySHA256); err != nil {
		if !temporary {
			err = errors.Join(err, m.restoreRuntimeConfig(id, runtime, previousConfig))
		}
		return nil, err
	}
	if !temporary {
		runtime.installation.ConfigEncrypted = encrypted
	}
	return canonical, nil
}

func (m *PluginManager) restoreRuntimeConfig(id int64, runtime *pluginRuntime, previous json.RawMessage) error {
	if runtime == nil {
		return nil
	}
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := runtime.validateAndApplyConfig(rollbackCtx, previous); err != nil {
		route := m.route.Load()
		if route != nil && route.pluginID == id && route.runtime == runtime {
			stateErr := m.markRuntimeUnavailable(route, "插件配置回滚失败: "+err.Error())
			return errors.Join(err, stateErr)
		}
		return err
	}
	return nil
}

func (m *PluginManager) Test(ctx context.Context, id int64) (*pluginv1.TestConfigResponse, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	configJSON, err := m.decryptConfig(installation)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	runtime := m.runtimes[id]
	m.mu.Unlock()
	temporary := false
	if runtime == nil {
		installation, err = m.ensureLocalInstallation(ctx, installation)
		if err != nil {
			return nil, err
		}
		runtime, err = m.newRuntime(ctx, installation)
		if err != nil {
			return nil, err
		}
		temporary = true
	}
	if temporary {
		defer runtime.kill()
	}
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := runtime.validateAndApplyConfig(testCtx, configJSON); err != nil {
		return nil, err
	}
	return runtime.api.TestConfig(testCtx, &pluginv1.TestConfigRequest{ConfigJson: configJSON})
}

type pluginUIAssetClaims struct {
	Version  int   `json:"version"`
	PluginID int64 `json:"plugin_id"`
	Expires  int64 `json:"expires"`
}

// CreateUIAssetToken 创建可跨实例校验的短时能力令牌，令牌不包含管理员凭据。
func (m *PluginManager) CreateUIAssetToken(ctx context.Context, id int64, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 || ttl > time.Hour {
		return "", time.Time{}, errors.New("插件 UI 会话有效期无效")
	}
	if _, err := m.repo.GetByID(ctx, id); err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(ttl)
	raw, err := json.Marshal(pluginUIAssetClaims{Version: 1, PluginID: id, Expires: expires.Unix()})
	if err != nil {
		return "", time.Time{}, err
	}
	// 加用途前缀，避免复用同一 AES-GCM 密钥的其他密文被当作 UI 能力令牌。
	encrypted, err := m.encryptor.Encrypt(pluginUITokenPrefix + string(raw))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("加密插件 UI 会话: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(encrypted)), expires, nil
}

func (m *PluginManager) ResolveUIAssetToken(token string) (int64, error) {
	if len(token) == 0 || len(token) > 4096 {
		return 0, errors.New("插件 UI 会话无效")
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, errors.New("插件 UI 会话无效")
	}
	plaintext, err := m.encryptor.Decrypt(string(encrypted))
	if err != nil {
		return 0, errors.New("插件 UI 会话无效")
	}
	plaintext, ok := strings.CutPrefix(plaintext, pluginUITokenPrefix)
	if !ok {
		return 0, errors.New("插件 UI 会话无效")
	}
	var claims pluginUIAssetClaims
	decoder := json.NewDecoder(strings.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil || claims.Version != 1 || claims.PluginID <= 0 {
		return 0, errors.New("插件 UI 会话无效")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return 0, errors.New("插件 UI 会话无效")
	}
	now := time.Now().Unix()
	if now >= claims.Expires {
		return 0, errors.New("插件 UI 会话已过期")
	}
	if claims.Expires > now+int64(time.Hour/time.Second) {
		return 0, errors.New("插件 UI 会话无效")
	}
	return claims.PluginID, nil
}

func (m *PluginManager) ReadUIAsset(ctx context.Context, id int64, relative string) ([]byte, string, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	installation, err = m.ensureLocalInstallation(ctx, installation)
	if err != nil {
		return nil, "", err
	}
	path := strings.TrimPrefix(strings.ReplaceAll(relative, "\\", "/"), "/")
	if path == "" || path == "index.html" {
		path = installation.Manifest.UI.Entrypoint
	} else {
		path = "ui/" + path
	}
	if _, declared := installation.Manifest.Files[path]; !declared || !strings.HasPrefix(path, "ui/") {
		return nil, "", os.ErrNotExist
	}
	fullPath, err := safePluginJoin(installation.InstallPath, path)
	if err != nil {
		return nil, "", err
	}
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, pluginUIAssetMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > pluginUIAssetMaxBytes {
		return nil, "", errors.New("插件 UI 资源超过大小限制")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != installation.Manifest.Files[path] {
		return nil, "", errors.New("插件 UI 资源哈希不匹配")
	}
	return data, path, nil
}

func (m *PluginManager) RoundTripOpenAIOAuth(ctx context.Context, request *http.Request, proxyURL string, account *Account) (*http.Response, bool, error) {
	if !m.ShouldRouteOpenAIOAuth(account) {
		return nil, false, nil
	}
	route := m.route.Load()
	if route == nil {
		return nil, false, nil
	}
	if route.runtime == nil {
		return nil, true, fmt.Errorf("OpenAI OAuth 插件不可用: %s", route.unavailable)
	}
	if route.runtime.client.Exited() {
		runtimeErr := errors.New("OpenAI OAuth 插件进程已退出")
		if stateErr := m.markRuntimeUnavailable(route, runtimeErr.Error()); stateErr != nil {
			return nil, true, errors.Join(runtimeErr, stateErr)
		}
		return nil, true, runtimeErr
	}
	if !route.runtime.beginRequest() {
		return nil, true, errors.New("OpenAI OAuth 插件正在停止")
	}
	response, err := route.runtime.roundTrip(ctx, request, proxyURL, account)
	if err != nil {
		route.runtime.finishRequest()
		if route.runtime.client.Exited() {
			if stateErr := m.markRuntimeUnavailable(route, err.Error()); stateErr != nil {
				err = errors.Join(err, stateErr)
			}
		}
		return nil, true, err
	}
	return response, true, nil
}

// ShouldRouteOpenAIOAuth 判断该账号是否命中当前 OpenAI OAuth 插件绑定。
// WebSocket 入口用它把命中的账号切换到 HTTP Bridge，避免绕过 v1 HTTP 插件协议。
func (m *PluginManager) ShouldRouteOpenAIOAuth(account *Account) bool {
	if m == nil || account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return false
	}
	route := m.route.Load()
	return route != nil && route.rolloutPercent > 0 && int(stablePluginBucket(account.ID)) < route.rolloutPercent
}

func (m *PluginManager) markRuntimeUnavailable(failedRoute *pluginRoute, message string) error {
	m.mu.Lock()
	current := m.route.Load()
	if current != failedRoute {
		m.mu.Unlock()
		return nil
	}
	if failedRoute.runtime != nil {
		failedRoute.runtime.kill()
	}
	delete(m.runtimes, failedRoute.pluginID)
	m.route.Store(&pluginRoute{
		pluginID:       failedRoute.pluginID,
		rolloutPercent: failedRoute.rolloutPercent,
		unavailable:    message,
	})
	m.mu.Unlock()
	return nil
}

func (m *PluginManager) prepareRuntime(ctx context.Context, installation *PluginInstallation, validateConfig bool) (*pluginRuntime, error) {
	runtime, err := m.newRuntime(ctx, installation)
	if err != nil {
		return nil, err
	}
	configJSON, err := m.decryptConfig(installation)
	if err == nil && validateConfig {
		applyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = runtime.validateAndApplyConfig(applyCtx, configJSON)
		cancel()
	}
	if err != nil {
		runtime.kill()
		return nil, err
	}
	return runtime, nil
}

func (m *PluginManager) publishRuntimeLocked(installation *PluginInstallation, runtime *pluginRuntime) {
	if old := m.runtimes[installation.ID]; old != nil {
		old.kill()
	}
	m.runtimes[installation.ID] = runtime
	m.route.Store(&pluginRoute{
		pluginID:       installation.ID,
		runtime:        runtime,
		rolloutPercent: bindingRollout(installation.Bindings),
	})
}

func (m *PluginManager) newRuntime(ctx context.Context, installation *PluginInstallation) (*pluginRuntime, error) {
	socketDir := filepath.Join(m.installer.RootDir(), "runtime")
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		return nil, err
	}
	timeout := time.Duration(m.cfg.Plugins.StartTimeoutSeconds) * time.Second
	return startPluginRuntime(ctx, installation, timeout, socketDir)
}

func (m *PluginManager) removeRuntimeLocked(id int64) *pluginRuntime {
	runtime := m.runtimes[id]
	delete(m.runtimes, id)
	if route := m.route.Load(); route != nil && route.pluginID == id {
		m.route.Store(nil)
	}
	return runtime
}

func (m *PluginManager) decryptConfig(installation *PluginInstallation) (json.RawMessage, error) {
	if installation == nil || strings.TrimSpace(installation.ConfigEncrypted) == "" {
		return json.RawMessage(`{}`), nil
	}
	plaintext, err := m.encryptor.Decrypt(installation.ConfigEncrypted)
	if err != nil {
		return nil, fmt.Errorf("解密插件配置: %w", err)
	}
	if !json.Valid([]byte(plaintext)) {
		return nil, errors.New("已保存的插件配置不是有效 JSON")
	}
	return json.RawMessage(plaintext), nil
}

func (m *PluginManager) removeManagedPath(target string) error {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	root, err := filepath.Abs(m.installer.RootDir())
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, absTarget)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("拒绝删除插件根目录之外的路径")
	}
	return os.RemoveAll(absTarget)
}

func hasEnabledOpenAIBinding(bindings []PluginBinding) bool {
	for _, binding := range bindings {
		if binding.Enabled && binding.Capability == PluginCapabilityOpenAIOAuthOutbound &&
			binding.Platform == PlatformOpenAI && binding.AccountType == AccountTypeOAuth {
			return true
		}
	}
	return false
}

func bindingRollout(bindings []PluginBinding) int {
	for _, binding := range bindings {
		if binding.Capability == PluginCapabilityOpenAIOAuthOutbound {
			return binding.RolloutPercent
		}
	}
	return 100
}

func stablePluginBucket(accountID int64) uint64 {
	value := uint64(accountID)
	value ^= value >> 33
	value *= 0xff51afd7ed558ccd
	value ^= value >> 33
	return value % 100
}
