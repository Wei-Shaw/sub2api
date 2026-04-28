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
}
