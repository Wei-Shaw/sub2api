package plugin

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// PluginInstance 持有一个插件运行时所需的全部状态。
// 所有字段(除常量外)的读写均需在 mu 保护下进行。
type PluginInstance struct {
	mu sync.Mutex

	Name       string
	BinaryPath string

	State      PluginState
	Cmd        *exec.Cmd
	CancelFunc context.CancelFunc

	GRPCAddr string
	HTTPAddr string

	GRPCConn      *grpc.ClientConn
	LifecycleStub pluginsdk.PluginLifecycleClient
	Manifest      *pluginsdk.ManifestResponse
	LastError     error
	RestartCount  int
	StartedAt     time.Time
	HealthCancel  context.CancelFunc
	restartPolicy *RestartPolicy

	// settingsUnsubscribe releases the PluginSettingsService subscription
	// that PluginManager opens after RegisterSchema succeeds. Set in
	// spawnAndConnect, called from stopInstance. nil when the plugin did
	// not register a schema (or the host had no settings service wired).
	// V5/W6 SETTINGS-V2 — see DESIGN §4.4.
	settingsUnsubscribe func()

	// pricingClient is the host-side proxy started after the plugin's gRPC
	// connection is up if the plugin implements PricingExtension. nil when
	// the plugin does not register the service (the host detects this by
	// observing codes.Unimplemented on the bootstrap List call). Stopped on
	// stopInstance and re-created on the next spawnAndConnect.
	pricingClient *PricingExtensionClient

	// maintenanceClient is the host-side proxy created after the plugin's gRPC
	// connection is up. Unlike pricingClient no probe RPC is issued at spawn;
	// codes.Unimplemented is handled at call-time in RunPluginMaintenance.
	// Detached in detachExtensions and re-created on the next spawnAndConnect.
	maintenanceClient *MaintenanceExtensionClient

	// EntryJSHash / EntryCSSHash hold the short content hash (8 hex chars)
	// of the plugin's entry.js / entry.css bundle, prefetched once after
	// the plugin transitions to StateRunning. Empty when the plugin ships
	// no frontend bundle, the prefetch failed, or it has not run yet.
	// Read by buildManifestEntry to suffix entry_*_url with `?v=<hash>` so
	// browsers automatically refetch after a redeploy instead of holding
	// a stale disk cache. Cleared together with the lifecycle/connection
	// fields in clearInstanceState so a restart recomputes them.
	EntryJSHash  string
	EntryCSSHash string

	// Exited closes after waitProcessExit returns from cmd.Wait(). Lets
	// stopInstance wait for the process to actually exit *without* calling
	// cmd.Wait() a second time — the first Wait() reaps the child, any
	// subsequent Wait() returns "waitid: no child processes" which then
	// gets stamped onto LastError and surfaces as a confusing error in the
	// admin UI after a clean disable.
	//
	// Set in spawnAndConnect right before launching the wait goroutine,
	// closed by waitProcessExit. nil for instances that have not been
	// spawned yet (StateRegistered).
	Exited chan struct{}
}

// NewPluginInstance 构造新的插件实例,初始状态为 StateRegistered。
func NewPluginInstance(name, binaryPath string, policy *RestartPolicy) *PluginInstance {
	return &PluginInstance{
		Name:          name,
		BinaryPath:    binaryPath,
		State:         StateRegistered,
		restartPolicy: policy,
	}
}

// transitionTo 在持有 mu 的前提下变更状态;
// 调用方负责加锁。非法转换返回 error 但不修改状态。
func (inst *PluginInstance) transitionTo(newState PluginState) error {
	if inst.State == newState {
		return nil
	}
	if !inst.State.CanTransitionTo(newState) {
		return fmt.Errorf("invalid state transition: %s -> %s (plugin=%s)",
			inst.State, newState, inst.Name)
	}
	inst.State = newState
	return nil
}

// SetError 在 mu 保护下记录最近一次错误。
func (inst *PluginInstance) SetError(err error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.LastError = err
}

// CleanupHealth 取消健康检查 goroutine,通常在停止/重启时调用。
// 调用方需要持有 mu。
func (inst *PluginInstance) CleanupHealth() {
	if inst.HealthCancel != nil {
		inst.HealthCancel()
		inst.HealthCancel = nil
	}
}

// CloseGRPC 关闭 gRPC 连接,失败错误仅记录到 LastError。
// 调用方需要持有 mu。
func (inst *PluginInstance) CloseGRPC() {
	if inst.GRPCConn == nil {
		return
	}
	if err := inst.GRPCConn.Close(); err != nil {
		// 关闭已经断开的连接通常会拿到 codes.Canceled (旧 ErrClientConnClosing
		// sentinel 等价值),无需视为错误。grpc v1.79+ 已弃用 ErrClientConnClosing
		// 并建议改用 status code 检查。
		if status.Code(err) != codes.Canceled {
			inst.LastError = fmt.Errorf("close grpc conn: %w", err)
		}
	}
	inst.GRPCConn = nil
	inst.LifecycleStub = nil
}

// SnapshotInfo 返回一个无锁可读的实例快照,用于 List 之类的查询。
func (inst *PluginInstance) SnapshotInfo() PluginInfo {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	info := PluginInfo{
		Name:         inst.Name,
		State:        inst.State.String(),
		GRPCAddr:     inst.GRPCAddr,
		HTTPAddr:     inst.HTTPAddr,
		RestartCount: inst.RestartCount,
		StartedAt:    inst.StartedAt,
	}
	if inst.LastError != nil {
		info.LastError = inst.LastError.Error()
	}
	if inst.Manifest != nil {
		info.DisplayName = inst.Manifest.DisplayName
		info.Version = inst.Manifest.Version
		// V5/W7: surface manifest IconSVG and the presence of a settings
		// schema so the admin plugin-management UI can render the plugin's
		// own icon and only show the Settings tab for plugins that actually
		// declare one. Both fields are zero-value when the plugin omitted
		// them, preserving the legacy display.
		info.IconSVG = inst.Manifest.GetIconSvg()
		info.HasSettings = len(inst.Manifest.GetSettingsSchemaJson()) > 0
		// P12·B-1: surface declared capabilities so the admin UI can audit
		// what the plugin requested. The list mirrors what the manifest
		// shipped — host normalisation (legacy → canonical) happens at the
		// approveCapabilities boundary, so plugins shipping legacy aliases
		// will appear with their legacy strings here. Frontend resolves the
		// canonical name via its capability metadata table.
		if caps := inst.Manifest.GetCapabilities(); len(caps) > 0 {
			info.Capabilities = append([]string(nil), caps...)
		}
	}
	return info
}

// PluginInfo 是面向外部的只读视图,同时作为 admin API 的响应载体。
//
// 字段语义与 plugins 数据表保持一致:
//   - State 反映运行时生命周期(registered/starting/running/errored/restarting),
//     与数据库 Enabled 标志位独立: Enabled 表示用户期望状态, State 表示实际状态。
//   - GRPCAddr/HTTPAddr/RestartCount/StartedAt 仅在插件实例已启动时有意义,
//     未启动的插件这些字段为零值。
type PluginInfo struct {
	Name         string         `json:"name"`
	DisplayName  string         `json:"display_name"`
	Description  string         `json:"description"`
	Version      string         `json:"version"`
	Enabled      bool           `json:"enabled"`
	SortOrder    int            `json:"sort_order"`
	State        string         `json:"state"`
	GRPCAddr     string         `json:"grpc_addr,omitempty"`
	HTTPAddr     string         `json:"http_addr,omitempty"`
	RestartCount int            `json:"restart_count,omitempty"`
	StartedAt    time.Time      `json:"started_at,omitempty"`
	LastError    string         `json:"last_error,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
	// IconSVG is the plugin manifest-supplied SVG markup the admin UI
	// renders next to DisplayName. Empty means the host falls back to its
	// generic plugin icon. Always omitted on the wire when empty so the
	// payload stays compact for plugins that did not opt in.
	IconSVG string `json:"icon_svg,omitempty"`
	// HasSettings flags whether the plugin shipped a non-empty
	// SettingsSchema in its manifest. The admin UI hides the Settings tab
	// when false to avoid a "no settings declared" empty state for every
	// plugin that simply does not contribute settings.
	HasSettings bool `json:"has_settings"`
	// UninstalledAt is non-nil when the plugin has been soft-uninstalled
	// (P13/C-1). The admin UI surfaces these only in the "uninstalled"
	// listing alongside a Restore button. When nil the plugin is active.
	UninstalledAt *time.Time `json:"uninstalled_at,omitempty"`
	// Capabilities is the canonical (post-normalisation) capability list
	// declared by the plugin manifest. Empty when the plugin requested no
	// capabilities. Surfaced to the admin UI so operators can audit what
	// each plugin is asking for. Legacy snake_case names declared by the
	// plugin are reported here in their canonical dotted-lowercase form;
	// see PLUGIN-AUTHOR-GUIDE §12 for the migration table.
	Capabilities []string `json:"capabilities,omitempty"`
	// Builtin flags whether the plugin ships in the host image (under the
	// configured BuiltinDir). Builtins cannot be soft-uninstalled or hard
	// purged; the admin UI hides those buttons for them.
	Builtin bool `json:"builtin,omitempty"`
}
