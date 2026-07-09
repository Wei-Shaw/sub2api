//go:build unit

package pluginhost

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
	"github.com/Wei-Shaw/sub2api/internal/pluginkit/kittest"

	"github.com/stretchr/testify/require"
)

// memInstallStore 是 InstallationStore 的内存实现（仅本包单测用）。
type memInstallStore struct {
	mu   sync.Mutex
	rows map[pluginkit.ID]Installation
}

var _ InstallationStore = (*memInstallStore)(nil)

func newMemInstallStore() *memInstallStore {
	return &memInstallStore{rows: make(map[pluginkit.ID]Installation)}
}

func (s *memInstallStore) Upsert(_ context.Context, inst *Installation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[inst.ID] = *inst
	return nil
}

func (s *memInstallStore) Get(_ context.Context, id pluginkit.ID) (*Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok {
		return nil, ErrNotInstalled
	}
	out := row
	return &out, nil
}

func (s *memInstallStore) List(_ context.Context) ([]*Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Installation, 0, len(s.rows))
	for _, row := range s.rows {
		r := row
		out = append(out, &r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *memInstallStore) Delete(_ context.Context, id pluginkit.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, id)
	return nil
}

func (s *memInstallStore) SetConfig(_ context.Context, id pluginkit.ID, config json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[id]
	if !ok {
		return ErrNotInstalled
	}
	row.Config = config
	s.rows[id] = row
	return nil
}

// fakeRuntime 记录 AwaitStopped 与 Notify* 调用（Supervisor 的接缝替身）。
type fakeRuntime struct {
	mu          sync.Mutex
	calls       []pluginkit.ID
	installed   []pluginkit.ID
	uninstalled []pluginkit.ID
}

func (r *fakeRuntime) AwaitStopped(_ context.Context, id pluginkit.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, id)
	return nil
}

func (r *fakeRuntime) NotifyInstalled(inst *Installation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.installed = append(r.installed, inst.ID)
}

func (r *fakeRuntime) NotifyUninstalled(id pluginkit.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.uninstalled = append(r.uninstalled, id)
}

func (r *fakeRuntime) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// installerFixture 组装安装器与全部依赖替身。
type installerFixture struct {
	installer *Installer
	store     *PackageStore
	installs  *memInstallStore
	states    pluginkit.StateStore
	runtime   *fakeRuntime
}

func newInstallerFixture(t *testing.T, reserved ...pluginkit.ID) *installerFixture {
	t.Helper()
	f := &installerFixture{
		store:    NewPackageStore(t.TempDir()),
		installs: newMemInstallStore(),
		states:   kittest.NewMemoryStateStore(),
		runtime:  &fakeRuntime{},
	}
	reservedSet := make(map[pluginkit.ID]struct{}, len(reserved))
	for _, id := range reserved {
		reservedSet[id] = struct{}{}
	}
	f.installer = NewInstaller(InstallerDeps{
		Store:         f.store,
		Installations: f.installs,
		States:        f.states,
		Runtime:       f.runtime,
		Reserved:      reservedSet,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return f
}

func TestInstallerInstallFresh(t *testing.T) {
	f := newInstallerFixture(t)
	ctx := context.Background()
	zipPath := testPluginZip(t, "acme.hello", "1.0.0")

	inst, err := f.installer.InstallOrUpgrade(ctx, zipPath, "admin:1")
	require.NoError(t, err)
	require.Equal(t, pluginkit.ID("acme.hello"), inst.ID)
	require.Equal(t, "1.0.0", inst.Version)
	require.Equal(t, "admin:1", inst.InstalledBy)
	require.Nil(t, inst.Config)

	wantSum, err := FileSHA256(zipPath)
	require.NoError(t, err)
	require.Equal(t, wantSum, inst.Checksum)

	// 文件落位 + 二进制执行位
	require.Equal(t, f.store.Dir("acme.hello", "1.0.0"), inst.InstallPath)
	info, err := os.Stat(filepath.Join(inst.InstallPath, "bin", "plugin"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	// 登记存在，且默认 disabled（不写 plugin_states）
	stored, err := f.installs.Get(ctx, "acme.hello")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", stored.Version)
	require.False(t, f.states.Enabled("acme.hello"))
	require.Zero(t, f.runtime.callCount())
}

func TestInstallerRejectsBuiltinConflict(t *testing.T) {
	f := newInstallerFixture(t, "demo")
	_, err := f.installer.InstallOrUpgrade(context.Background(), testPluginZip(t, "demo", "1.0.0"), "admin:1")
	require.ErrorIs(t, err, ErrBuiltinConflict)
	_, err = f.installs.Get(context.Background(), "demo")
	require.ErrorIs(t, err, ErrNotInstalled)
}

func TestInstallerRejectsInvalidManifest(t *testing.T) {
	f := newInstallerFixture(t)
	ctx := context.Background()

	// 缺当前平台二进制：错误需说明缺哪个平台
	raw := []byte(`{"id":"acme.hello","name":"x","version":"1.0.0","protocol":"http/1",` +
		`"backend":{"executables":{"plan9-mips":"bin/plugin"}}}`)
	zipPath := writeTestZip(t, []zipEntry{{name: ManifestFileName, body: raw}})
	_, err := f.installer.InstallOrUpgrade(ctx, zipPath, "admin:1")
	require.ErrorIs(t, err, ErrInvalidPackage)
	require.Contains(t, err.Error(), CurrentPlatform())

	// 声明了当前平台但包内没有对应文件：解包目录不得残留
	zipPath = writeTestZip(t, []zipEntry{{name: ManifestFileName, body: testManifestBytes(t, "acme.hello", "1.0.0")}})
	_, err = f.installer.InstallOrUpgrade(ctx, zipPath, "admin:1")
	require.ErrorIs(t, err, ErrInvalidPackage)
	require.NoDirExists(t, f.store.Dir("acme.hello", "1.0.0"))
	_, err = f.installs.Get(ctx, "acme.hello")
	require.ErrorIs(t, err, ErrNotInstalled)
}

// TestInstallerMinCoreVersion 安装期版本门槛：宿主低于 min_core_version 拒装；
// 达标放行；宿主为开发构建（0.0.0-dev）跳过检查放行。
func TestInstallerMinCoreVersion(t *testing.T) {
	ctx := context.Background()
	manifestWithMin := func(id, minCore string) []byte {
		raw, err := json.Marshal(map[string]any{
			"id": id, "name": "Test Plugin", "version": "1.0.0",
			"protocol":         ProtocolHTTP1,
			"min_core_version": minCore,
			"backend": map[string]any{
				"executables": map[string]string{CurrentPlatform(): "bin/plugin"},
			},
		})
		require.NoError(t, err)
		return raw
	}
	zipWithMin := func(id, minCore string) string {
		return writeTestZip(t, []zipEntry{
			{name: ManifestFileName, body: manifestWithMin(id, minCore)},
			{name: "bin/plugin", body: []byte("#!/bin/sh\n")},
		})
	}
	newInstallerWithHost := func(hostVersion string) (*Installer, *memInstallStore) {
		installs := newMemInstallStore()
		return NewInstaller(InstallerDeps{
			Store:         NewPackageStore(t.TempDir()),
			Installations: installs,
			States:        kittest.NewMemoryStateStore(),
			HostVersion:   hostVersion,
			Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		}), installs
	}

	// 宿主低于门槛：拒装且无登记残留。
	installer, installs := newInstallerWithHost("0.5.0")
	_, err := installer.InstallOrUpgrade(ctx, zipWithMin("acme.needsnew", "1.0.0"), "admin:1")
	require.ErrorIs(t, err, ErrInvalidPackage)
	require.Contains(t, err.Error(), "requires core version >= 1.0.0")
	_, err = installs.Get(ctx, "acme.needsnew")
	require.ErrorIs(t, err, ErrNotInstalled)

	// 宿主达标：放行。
	installer, _ = newInstallerWithHost("1.2.0")
	_, err = installer.InstallOrUpgrade(ctx, zipWithMin("acme.needsnew", "1.0.0"), "admin:1")
	require.NoError(t, err)

	// 开发构建：跳过检查放行。
	installer, _ = newInstallerWithHost("0.0.0-dev")
	_, err = installer.InstallOrUpgrade(ctx, zipWithMin("acme.devhost", "9.9.9"), "admin:1")
	require.NoError(t, err)
}

// TestInstallerUpgradeEnabledPlugin 升级启用中的插件：
// 停旧（含等子进程退出）→ 换文件 → 保留 config → 恢复启用，旧版本目录清理。
func TestInstallerUpgradeEnabledPlugin(t *testing.T) {
	f := newInstallerFixture(t)
	ctx := context.Background()

	_, err := f.installer.InstallOrUpgrade(ctx, testPluginZip(t, "acme.hello", "1.0.0"), "admin:1")
	require.NoError(t, err)
	require.NoError(t, f.installs.SetConfig(ctx, "acme.hello", json.RawMessage(`{"port":8080}`)))
	require.NoError(t, f.states.SetEnabled(ctx, "acme.hello", true, "admin:1"))

	inst, err := f.installer.InstallOrUpgrade(ctx, testPluginZip(t, "acme.hello", "2.0.0"), "admin:2")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", inst.Version)
	require.JSONEq(t, `{"port":8080}`, string(inst.Config), "升级必须保留管理员配置")

	// 换文件前等待了子进程退出，结束后恢复原 enabled 态
	require.Equal(t, 1, f.runtime.callCount())
	require.True(t, f.states.Enabled("acme.hello"))

	require.DirExists(t, f.store.Dir("acme.hello", "2.0.0"))
	require.NoDirExists(t, f.store.Dir("acme.hello", "1.0.0"), "旧版本目录应被清理")

	stored, err := f.installs.Get(ctx, "acme.hello")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", stored.Version)
	require.Equal(t, "admin:2", stored.InstalledBy)
}

// TestInstallerUpgradeDisabledPlugin 升级停用中的插件：不触发等停，保持 disabled。
func TestInstallerUpgradeDisabledPlugin(t *testing.T) {
	f := newInstallerFixture(t)
	ctx := context.Background()

	_, err := f.installer.InstallOrUpgrade(ctx, testPluginZip(t, "acme.hello", "1.0.0"), "admin:1")
	require.NoError(t, err)

	inst, err := f.installer.InstallOrUpgrade(ctx, testPluginZip(t, "acme.hello", "2.0.0"), "admin:1")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", inst.Version)
	require.Zero(t, f.runtime.callCount())
	require.False(t, f.states.Enabled("acme.hello"))
}

// TestInstallerUpgradeFailureRestoresEnabled 升级中途失败（坏包）：
// 旧版本文件未动，enabled 态恢复，登记仍指向旧版本。
func TestInstallerUpgradeFailureRestoresEnabled(t *testing.T) {
	f := newInstallerFixture(t)
	ctx := context.Background()

	_, err := f.installer.InstallOrUpgrade(ctx, testPluginZip(t, "acme.hello", "1.0.0"), "admin:1")
	require.NoError(t, err)
	require.NoError(t, f.states.SetEnabled(ctx, "acme.hello", true, "admin:1"))

	// 新包声明了当前平台二进制但包内缺失 → MarkExecutable 失败
	badZip := writeTestZip(t, []zipEntry{{name: ManifestFileName, body: testManifestBytes(t, "acme.hello", "2.0.0")}})
	_, err = f.installer.InstallOrUpgrade(ctx, badZip, "admin:1")
	require.ErrorIs(t, err, ErrInvalidPackage)

	require.True(t, f.states.Enabled("acme.hello"), "失败后必须恢复原 enabled 态")
	require.DirExists(t, f.store.Dir("acme.hello", "1.0.0"))
	require.NoDirExists(t, f.store.Dir("acme.hello", "2.0.0"))
	stored, err := f.installs.Get(ctx, "acme.hello")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", stored.Version)
}

func TestInstallerUninstall(t *testing.T) {
	f := newInstallerFixture(t)
	ctx := context.Background()

	_, err := f.installer.InstallOrUpgrade(ctx, testPluginZip(t, "acme.hello", "1.0.0"), "admin:1")
	require.NoError(t, err)
	require.NoError(t, f.states.SetEnabled(ctx, "acme.hello", true, "admin:1"))

	require.NoError(t, f.installer.Uninstall(ctx, "acme.hello", "admin:1"))
	require.Equal(t, 1, f.runtime.callCount(), "启用中的插件卸载前必须等停")
	require.False(t, f.states.Enabled("acme.hello"))
	require.NoDirExists(t, filepath.Join(f.store.Root(), "acme.hello"))
	_, err = f.installs.Get(ctx, "acme.hello")
	require.ErrorIs(t, err, ErrNotInstalled)

	// 重复卸载 → ErrNotInstalled
	require.ErrorIs(t, f.installer.Uninstall(ctx, "acme.hello", "admin:1"), ErrNotInstalled)
}

// TestInstallerUninstallDisabledPluginSkipsAwait 停用态卸载不触发等停。
func TestInstallerUninstallDisabledPluginSkipsAwait(t *testing.T) {
	f := newInstallerFixture(t)
	ctx := context.Background()

	_, err := f.installer.InstallOrUpgrade(ctx, testPluginZip(t, "acme.hello", "1.0.0"), "admin:1")
	require.NoError(t, err)
	require.NoError(t, f.installer.Uninstall(ctx, "acme.hello", "admin:1"))
	require.Zero(t, f.runtime.callCount())
}

// TestInstallerNilRuntime Runtime 未接入（TASK-002 前的生产形态）时全流程不 panic。
func TestInstallerNilRuntime(t *testing.T) {
	f := newInstallerFixture(t)
	f.installer.runtime = nil
	ctx := context.Background()

	_, err := f.installer.InstallOrUpgrade(ctx, testPluginZip(t, "acme.hello", "1.0.0"), "admin:1")
	require.NoError(t, err)
	require.NoError(t, f.states.SetEnabled(ctx, "acme.hello", true, "admin:1"))
	_, err = f.installer.InstallOrUpgrade(ctx, testPluginZip(t, "acme.hello", "2.0.0"), "admin:1")
	require.NoError(t, err)
	require.NoError(t, f.installer.Uninstall(ctx, "acme.hello", "admin:1"))
}

func TestReservedIDs(t *testing.T) {
	factories := []pluginkit.Factory{
		func() pluginkit.Plugin { return fakeIDPlugin{id: "demo"} },
		func() pluginkit.Plugin { return fakeIDPlugin{id: "job.backup"} },
	}
	reserved := ReservedIDs(factories)
	require.Len(t, reserved, 2)
	_, ok := reserved["demo"]
	require.True(t, ok)
	_, ok = reserved["job.backup"]
	require.True(t, ok)
}

type fakeIDPlugin struct{ id pluginkit.ID }

func (p fakeIDPlugin) ID() pluginkit.ID { return p.id }
