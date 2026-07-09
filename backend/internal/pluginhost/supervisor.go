package pluginhost

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pluginkit"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
)

// 插件进程注入的环境变量（phase-4 决策 2 的控制面契约，SDK 侧同名解析）。
// env 仅承载非机密；机密（能力 token、私有配置）经 stdin 握手注入——
// 同 OS 用户下任何进程都能读 /proc/<pid>/environ，env 携带机密会击穿
// 插件间按 ID 硬隔离，stdin 只有父子进程可见。
const (
	// EnvPluginSocket 是插件应监听的 unix socket 路径。
	EnvPluginSocket = "SUB2API_PLUGIN_SOCKET"
	// EnvCapabilityURL 是宿主能力 API 的基址（http://127.0.0.1:<port>）。
	EnvCapabilityURL = "SUB2API_HOST_CAPABILITY_URL"
)

const (
	// socketDirName 是 unix socket 的统一目录名，置于 os.TempDir 下：
	// DATA_DIR 可能位于不支持 unix socket 的文件系统（如 Windows 挂载盘），
	// 系统临时目录则总是本地文件系统（phase-4 风险对策 1）。
	socketDirName = "sub2api-plugins"
	// crashLoopDisabledBy 是连挂自禁写入 plugin_states 的操作者标识。
	crashLoopDisabledBy = "supervisor:crash-loop"
)

// SupervisorDeps 是构造 Supervisor 的依赖集合。
type SupervisorDeps struct {
	Installs InstallationStore
	States   pluginkit.StateStore
	KV       KVStore
	Logger   *slog.Logger
	// HeaderFilter 复用核心网关的响应头白名单编译产物；nil 时走默认白名单
	// （responseheaders.FilterHeaders 对 nil filter 的回退语义）。
	HeaderFilter *responseheaders.CompiledHeaderFilter
	// HostVersion 是宿主构建版本（main.Version），用于 min_core_version 门槛；
	// 非法 semver 或开发构建（0.0.0-dev）时跳过检查。
	HostVersion string
}

// ExternalLayer 聚合外部插件层的装配产物，作为整体穿过 server 装配链
// 抵达 RegisterPluginRoutes（避免逐个依赖膨胀路由装配签名）。
type ExternalLayer struct {
	Installer  *Installer
	Installs   InstallationStore
	Supervisor *Supervisor
}

// ExternalPluginStatus 是单个外部插件的状态快照（admin 快照合并用）。
// 字段语义与 pluginkit.PluginStatus 对齐，另带安装版本与清单元数据
// （Name/Description 来自 manifest，插件列表展示用）。
type ExternalPluginStatus struct {
	ID          pluginkit.ID
	Enabled     bool
	State       pluginkit.State
	Err         string
	StartedAt   *time.Time
	Version     string
	Name        string
	Description string
}

// EnabledExternalPlugin 是用户态 enabled 清单里的一项外部插件：
// Assets 非空时指向其前端运行时入口（无前端资产的插件为空串）。
type EnabledExternalPlugin struct {
	ID     pluginkit.ID
	Assets string
}

// Supervisor 是外部插件子进程宿主：订阅 plugin_states 状态机（只认已安装的
// 外部 ID），enable→Launch（注入 env、等 healthz 就绪）、disable→Terminate
// （SIGTERM→超时 SIGKILL）、crash→指数退避重启、连挂 N 次→自动 disable。
// 数据面经 Dispatch（proxy.go）反代到插件 unix socket，控制面经能力 API
// （capability.go），前端资产经 ServeAsset（assets.go）。
type Supervisor struct {
	installs    InstallationStore
	states      pluginkit.StateStore
	logger      *slog.Logger
	capability  *capabilityServer
	proxyCfg    proxyConfig
	socketDir   string
	socketSeq   atomic.Uint64
	hostVersion string

	// extraChildEnv 在 allowlist 之外额外注入子进程的 env（"K=V" 形式）。
	// 生产恒为 nil（子进程只拿中性系统变量 + 宿主注入的 SUB2API_PLUGIN_*）；
	// 仅单测用它把 helper-process 标记透进重执行的测试二进制。
	extraChildEnv []string

	// 生命周期参数：var 字段而非 const，供单测收窄窗口（不导出）。
	launchTimeout      time.Duration // 等 healthz 就绪的上限
	healthPollInterval time.Duration
	terminateGrace     time.Duration // SIGTERM 后等待退出的宽限
	backoffBase        time.Duration // crash 退避基数（指数翻倍）
	backoffMax         time.Duration
	crashLoopLimit     int           // 连挂多少次触发自禁
	stableRunReset     time.Duration // 存活超过该时长的 crash 重置连挂计数

	// mu 保护 installed 索引与 entries 表（Dispatch/Snapshot 读，装卸与启停写）。
	mu        sync.RWMutex
	installed map[pluginkit.ID]*Installation
	entries   map[pluginkit.ID]*procEntry

	baseCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	startMu sync.Mutex
	started bool
}

// procEntry 是单个外部插件的运行时载体（与 pluginkit.Manager 的 pluginEntry 同构）。
type procEntry struct {
	id pluginkit.ID

	// lifeMu 串行化该插件的生命周期操作（launch/terminate/restart 互斥）。
	lifeMu sync.Mutex

	// mu 保护以下状态字段（写者同时持有 lifeMu；crash 收尾路径除外）。
	mu        sync.Mutex
	state     pluginkit.State
	err       string
	startedAt *time.Time
	proc      *pluginProc
	crashes   int // 连挂计数（稳定运行或人工停启后清零）
}

// pluginProc 是一次子进程拉起的全部现场。
type pluginProc struct {
	cmd        *exec.Cmd
	socketPath string
	token      string
	proxy      http.Handler
	launchedAt time.Time
	healthy    atomic.Bool
	stopping   atomic.Bool
	done       chan struct{} // 进程退出且现场回收（socket/token）后关闭
	waitErr    error         // close(done) 前写入
}

// NewSupervisor 创建外部插件子进程宿主（不产生副作用，Start 才开始工作）。
func NewSupervisor(deps SupervisorDeps) *Supervisor {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Supervisor{
		installs:    deps.Installs,
		states:      deps.States,
		logger:      logger,
		capability:  newCapabilityServer(deps.KV, deps.Installs, logger),
		proxyCfg:    proxyConfig{headerFilter: deps.HeaderFilter, logger: logger},
		socketDir:   filepath.Join(os.TempDir(), socketDirName),
		hostVersion: deps.HostVersion,

		launchTimeout:      15 * time.Second,
		healthPollInterval: 50 * time.Millisecond,
		terminateGrace:     10 * time.Second,
		backoffBase:        500 * time.Millisecond,
		backoffMax:         30 * time.Second,
		crashLoopLimit:     5,
		stableRunReset:     time.Minute,

		installed: make(map[pluginkit.ID]*Installation),
		entries:   make(map[pluginkit.ID]*procEntry),
		baseCtx:   ctx,
		cancel:    cancel,
	}
}

// Start 启动宿主：起能力服务 → 订阅状态机 → 加载安装登记 → 拉起 enabled 插件。
// 单插件 Launch 失败进 failed 态并隔离，不拖垮宿主启动；重复调用返回 error。
func (s *Supervisor) Start(ctx context.Context) error {
	s.startMu.Lock()
	if s.started {
		s.startMu.Unlock()
		return errors.New("pluginhost: supervisor already started")
	}
	s.started = true
	s.startMu.Unlock()

	// 能力服务不在此启动：首次 spawn 时惰性绑定端口，
	// 未安装任何外部插件时宿主不新增监听面（零行为变更纪律）。

	// 先订阅再加载：加载期间到达的 toggle 由幂等 launch/terminate 与
	// 内存快照复核兜底（对齐 Manager.Bootstrap 的次序纪律）。
	s.states.Subscribe(s.onToggle)

	installs, err := s.installs.List(ctx)
	if err != nil {
		return fmt.Errorf("pluginhost: load installations: %w", err)
	}
	s.mu.Lock()
	for _, inst := range installs {
		s.installed[inst.ID] = inst
	}
	s.mu.Unlock()

	for _, inst := range installs {
		if !s.states.Enabled(inst.ID) {
			continue
		}
		e := s.entryFor(inst.ID)
		e.lifeMu.Lock()
		s.launchLocked(e)
		e.lifeMu.Unlock()
	}
	return nil
}

// StopAll 停止全部插件子进程与能力服务，接入 wire cleanup；错误聚合返回。
func (s *Supervisor) StopAll(ctx context.Context) error {
	s.cancel() // 退避重启等待者据此中止

	var errs []error
	for _, e := range s.sortedEntries() {
		e.lifeMu.Lock()
		if err := s.terminateLocked(e); err != nil {
			errs = append(errs, err)
		}
		e.lifeMu.Unlock()
	}
	if err := s.capability.close(); err != nil {
		errs = append(errs, fmt.Errorf("pluginhost: close capability server: %w", err))
	}

	// 等待 reaper/日志泵/重启等待者退出，受 ctx deadline 约束。
	waitDone := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-ctx.Done():
		errs = append(errs, fmt.Errorf("pluginhost: supervisor drain: %w", ctx.Err()))
	}
	return errors.Join(errs...)
}

// ============================================================
// ExternalRuntime 接缝（Installer → Supervisor）
// ============================================================

var _ ExternalRuntime = (*Supervisor)(nil)

// AwaitStopped 阻塞等待插件子进程完全退出（无进程视为已停止）。
func (s *Supervisor) AwaitStopped(ctx context.Context, id pluginkit.ID) error {
	e := s.entryOf(id)
	if e == nil {
		return nil
	}
	e.mu.Lock()
	proc := e.proc
	e.mu.Unlock()
	if proc == nil {
		return nil
	}
	select {
	case <-proc.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("pluginhost: await %s stopped: %w", id, ctx.Err())
	}
}

// NotifyInstalled 同步安装/升级后的登记到运行时索引（本实例内联通知；
// 跨实例经 onToggle 的 DB 回源兜底）。
func (s *Supervisor) NotifyInstalled(inst *Installation) {
	s.mu.Lock()
	s.installed[inst.ID] = inst
	s.mu.Unlock()
}

// NotifyUninstalled 从运行时索引摘除插件；进程若仍在（等停超时的兜底路径）
// 异步补杀，不阻塞卸载流程。
func (s *Supervisor) NotifyUninstalled(id pluginkit.ID) {
	s.mu.Lock()
	delete(s.installed, id)
	e := s.entries[id]
	delete(s.entries, id)
	s.mu.Unlock()

	if e == nil {
		return
	}
	e.mu.Lock()
	orphan := e.proc != nil
	e.mu.Unlock()
	if orphan {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			e.lifeMu.Lock()
			defer e.lifeMu.Unlock()
			if err := s.terminateLocked(e); err != nil {
				s.logger.Error("plugin_uninstall_orphan_terminate_failed", "plugin", string(id), "error", err)
			}
		}()
	}
}

// ReloadConfig 在配置变更后刷新索引，并对已启用且在跑的插件执行进程重启
// （配置经 env 注入，重启后生效；phase-4 决策 7）。
func (s *Supervisor) ReloadConfig(ctx context.Context, id pluginkit.ID) error {
	inst, err := s.installs.Get(ctx, id)
	if err != nil {
		return err
	}
	s.NotifyInstalled(inst)

	if !s.states.Enabled(id) {
		return nil
	}
	e := s.entryFor(id)
	e.lifeMu.Lock()
	defer e.lifeMu.Unlock()
	if err := s.terminateLocked(e); err != nil {
		return err
	}
	s.launchLocked(e)
	return nil
}

// ============================================================
// 查询面（路由分发器 / admin 快照 / enabled 清单）
// ============================================================

// Handles 报告 ID 是否为已安装的外部插件（分发器路由裁决用，O(1) 无 IO）。
func (s *Supervisor) Handles(id pluginkit.ID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.installed[id]
	return ok
}

// InstallationOf 返回已安装外部插件的登记（快照引用，调用方只读）。
func (s *Supervisor) InstallationOf(id pluginkit.ID) (*Installation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inst, ok := s.installed[id]
	return inst, ok
}

// Snapshot 返回全部已安装外部插件的状态快照，按 ID 稳定排序。
func (s *Supervisor) Snapshot() []ExternalPluginStatus {
	s.mu.RLock()
	ids := make([]pluginkit.ID, 0, len(s.installed))
	for id := range s.installed {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]ExternalPluginStatus, 0, len(ids))
	for _, id := range ids {
		if st, ok := s.StatusOf(id); ok {
			out = append(out, st)
		}
	}
	return out
}

// StatusOf 返回单个外部插件的状态快照；未安装返回 false。
func (s *Supervisor) StatusOf(id pluginkit.ID) (ExternalPluginStatus, bool) {
	s.mu.RLock()
	inst, ok := s.installed[id]
	e := s.entries[id]
	s.mu.RUnlock()
	if !ok {
		return ExternalPluginStatus{}, false
	}

	st := ExternalPluginStatus{
		ID:          id,
		Enabled:     s.states.Enabled(id),
		State:       pluginkit.StateDisabled,
		Version:     inst.Version,
		Name:        inst.Manifest.Name,
		Description: inst.Manifest.Description,
	}
	if e != nil {
		e.mu.Lock()
		st.State, st.Err = e.state, e.err
		if e.startedAt != nil {
			t := *e.startedAt
			st.StartedAt = &t
		}
		e.mu.Unlock()
	}
	return st, true
}

// EnabledExternalPlugins 返回已启用的外部插件清单（用户态 enabled 端点用）：
// 带前端资产的插件附 assets 入口路径。
func (s *Supervisor) EnabledExternalPlugins() []EnabledExternalPlugin {
	s.mu.RLock()
	installs := make([]*Installation, 0, len(s.installed))
	for _, inst := range s.installed {
		installs = append(installs, inst)
	}
	s.mu.RUnlock()
	sort.Slice(installs, func(i, j int) bool { return installs[i].ID < installs[j].ID })

	out := make([]EnabledExternalPlugin, 0, len(installs))
	for _, inst := range installs {
		if !s.states.Enabled(inst.ID) {
			continue
		}
		item := EnabledExternalPlugin{ID: inst.ID}
		if inst.Manifest != nil && inst.Manifest.Frontend != nil {
			item.Assets = AssetURLPath(inst.ID, inst.Manifest.Frontend.Entry)
		}
		out = append(out, item)
	}
	return out
}

// ============================================================
// 状态机驱动（toggle → launch/terminate）
// ============================================================

// onToggle 经 states.Subscribe 驱动，只处理已安装的外部 ID（内建 ID 由 Manager
// 处理）。本实例索引未命中且为 enable 时回源 DB 一次，兜底跨实例安装后
// 广播先于索引同步到达的场景。
func (s *Supervisor) onToggle(ch pluginkit.StateChange) {
	if _, ok := s.InstallationOf(ch.ID); !ok {
		if !ch.Enabled {
			return // 本地无索引也无进程，disable 无事可做
		}
		ctx, cancelLookup := context.WithTimeout(s.baseCtx, 10*time.Second)
		inst, err := s.installs.Get(ctx, ch.ID)
		cancelLookup()
		if err != nil {
			if !errors.Is(err, ErrNotInstalled) {
				s.logger.Warn("plugin_toggle_lookup_failed", "plugin", string(ch.ID), "error", err)
			}
			return // 内建 ID / 未安装：不属于外部层
		}
		s.NotifyInstalled(inst)
	}

	e := s.entryFor(ch.ID)
	e.lifeMu.Lock()
	defer e.lifeMu.Unlock()
	// 复核内存快照：快速连翻时跳过已过期的事件（对齐 Manager.onToggle）。
	if ch.Enabled != s.states.Enabled(ch.ID) {
		return
	}
	if ch.Enabled {
		e.mu.Lock()
		e.crashes = 0 // 人工 enable 重置连挂计数
		e.mu.Unlock()
		s.launchLocked(e)
	} else {
		// 错误已在 terminateLocked 内记日志并落入状态字段，事件回调无处上抛。
		_ = s.terminateLocked(e)
	}
}

// launchLocked 拉起插件子进程（caller 持有 e.lifeMu）：
// 注入 env → 起进程 → 等 healthz 就绪（超时/早退 → failed）→ running。
// 已 running 时幂等 no-op；无后端声明的纯前端插件直接标记 running。
func (s *Supervisor) launchLocked(e *procEntry) {
	if s.baseCtx.Err() != nil {
		return
	}
	e.mu.Lock()
	alreadyRunning := e.state == pluginkit.StateRunning
	e.mu.Unlock()
	if alreadyRunning {
		return
	}

	inst, ok := s.InstallationOf(e.id)
	if !ok {
		return
	}
	// 纵深防御：DB 重载的清单在拉起前重新校验——安装时校验过，但登记内容
	// 可能被直接改库篡改或跨版本迁移损坏，非法清单拒绝拉起进 failed。
	if inst.Manifest == nil {
		e.setStatus(pluginkit.StateFailed, "pluginhost: installation has no manifest", nil)
		s.logger.Error("plugin_launch_rejected", "plugin", string(e.id), "error", "installation has no manifest")
		return
	}
	if err := inst.Manifest.Validate(); err != nil {
		e.setStatus(pluginkit.StateFailed, err.Error(), nil)
		s.logger.Error("plugin_launch_rejected", "plugin", string(e.id), "error", err)
		return
	}
	// min_core_version 门槛在 spawn 侧兜底（安装后宿主可能降级/换实例）。
	if ok, skipped := inst.Manifest.MinCoreVersionSatisfied(s.hostVersion); !ok {
		msg := fmt.Sprintf("pluginhost: requires core version >= %s, host is %s",
			inst.Manifest.MinCoreVersion, s.hostVersion)
		e.setStatus(pluginkit.StateFailed, msg, nil)
		s.logger.Error("plugin_launch_rejected", "plugin", string(e.id), "error", msg)
		return
	} else if skipped {
		s.logger.Warn("plugin_min_core_version_check_skipped",
			"plugin", string(e.id), "host_version", s.hostVersion)
	}
	if inst.Manifest.Backend == nil {
		// 纯前端插件：无子进程，enabled 即视为 running（资产门控只看 enabled）。
		now := time.Now()
		e.setStatus(pluginkit.StateRunning, "", &now)
		return
	}

	proc, err := s.spawn(e, inst)
	if err != nil {
		e.setStatus(pluginkit.StateFailed, err.Error(), nil)
		s.logger.Error("plugin_launch_failed", "plugin", string(e.id), "error", err)
		return
	}

	if err := s.awaitHealthy(proc); err != nil {
		// 就绪失败：终止进程并进 failed 态（Supervisor 不为未就绪进程重启）。
		s.killProc(proc)
		e.mu.Lock()
		if e.proc == proc {
			e.proc = nil
		}
		e.mu.Unlock()
		e.setStatus(pluginkit.StateFailed, err.Error(), nil)
		s.logger.Error("plugin_launch_failed", "plugin", string(e.id), "error", err)
		return
	}

	proc.healthy.Store(true)
	now := time.Now()
	e.setStatus(pluginkit.StateRunning, "", &now)
	s.logger.Info("plugin_process_started", "plugin", string(e.id),
		"version", inst.Version, "pid", proc.cmd.Process.Pid, "socket", proc.socketPath)
}

// allowlistedHostEnv 从宿主环境挑出插件运行必需的中性变量，
// 过滤掉一切机密（DB/Redis/JWT 凭据等）。名单只含进程正常运行、
// 时区与本地化所需的通用变量。
func allowlistedHostEnv() []string {
	const prefixKeep = "LC_" // 本地化（LC_ALL/LC_CTYPE 等）
	keep := map[string]bool{
		"PATH": true, "HOME": true, "TMPDIR": true, "TZ": true,
		"LANG": true, "USER": true, "SHELL": true, "TERM": true,
	}
	out := make([]string, 0, len(keep))
	for _, kv := range os.Environ() {
		eq := indexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		name := kv[:eq]
		if keep[name] || hasPrefix(name, prefixKeep) {
			out = append(out, kv)
		}
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}

// spawn 组装并启动子进程，登记 reaper 与日志泵。
func (s *Supervisor) spawn(e *procEntry, inst *Installation) (*pluginProc, error) {
	rel, ok := inst.Manifest.ExecutableFor(CurrentPlatform())
	if !ok {
		return nil, fmt.Errorf("pluginhost: no executable for platform %s", CurrentPlatform())
	}
	exePath := filepath.Join(inst.InstallPath, filepath.FromSlash(rel))
	if _, err := os.Stat(exePath); err != nil {
		return nil, fmt.Errorf("pluginhost: executable missing: %w", err)
	}

	if err := s.capability.ensureStarted(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.socketDir, 0o700); err != nil {
		return nil, fmt.Errorf("pluginhost: create socket dir: %w", err)
	}
	socketPath := s.newSocketPath(e.id)
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("pluginhost: remove stale socket: %w", err)
	}

	token, err := newPluginToken()
	if err != nil {
		return nil, err
	}

	// 子进程 stdout/stderr 接宿主结构化日志（带 plugin 字段）。
	// 用 io.Writer 而非 StdoutPipe：exec 内部 goroutine 保证 Wait 前排干，
	// 避免 pipe 并发读与 Wait 的竞争。
	logger := s.logger.With("plugin", string(e.id))
	stdoutSink := &lineWriter{sink: func(line string) { logger.Info("plugin_stdout", "line", line) }}
	stderrSink := &lineWriter{sink: func(line string) { logger.Warn("plugin_stderr", "line", line) }}

	cmd := exec.Command(exePath)
	cmd.Dir = inst.InstallPath
	// 独立进程组：终止时对整组发信号，插件 fork 的孙进程一并回收。
	setProcGroup(cmd)
	// 最小 env 白名单：只透传插件运行所必需的中性系统变量，绝不继承
	// 宿主的 DATABASE_URL / JWT_SECRET / Redis 凭据等机密（纵深防御——
	// 插件的一切能力都应经宿主注入的 token 走能力面，而非读环境变量）。
	childEnv := append(allowlistedHostEnv(),
		EnvPluginSocket+"="+socketPath,
		EnvCapabilityURL+"="+s.capability.baseURL,
	)
	cmd.Env = append(childEnv, s.extraChildEnv...)
	cmd.Stdout = stdoutSink
	cmd.Stderr = stderrSink
	// 机密经 stdin 握手注入（一行 JSON 后即 EOF；SDK readHandshake 对端解析）：
	// 配置变更走"重启进程重新注入"，每次 spawn 都取安装登记的最新 config。
	hs, err := json.Marshal(struct {
		Token  string          `json:"token"`
		Config json.RawMessage `json:"config"`
	}{Token: token, Config: configOrEmptyObject(inst.Config)})
	if err != nil {
		return nil, fmt.Errorf("pluginhost: marshal handshake: %w", err)
	}
	cmd.Stdin = bytes.NewReader(append(hs, '\n'))

	s.capability.register(token, e.id, inst.Manifest.PermissionSet())
	if err := cmd.Start(); err != nil {
		s.capability.unregister(token)
		return nil, fmt.Errorf("pluginhost: start process: %w", err)
	}

	proc := &pluginProc{
		cmd:        cmd,
		socketPath: socketPath,
		token:      token,
		proxy:      newSocketReverseProxy(e.id, socketPath, s.proxyCfg),
		launchedAt: time.Now(),
		done:       make(chan struct{}),
	}
	e.mu.Lock()
	e.proc = proc
	e.mu.Unlock()

	// reaper：回收进程与现场；非预期退出走 crash 路径。
	s.wg.Add(1)
	go s.reap(e, proc, stdoutSink, stderrSink)
	return proc, nil
}

// lineWriter 把子进程输出按行送入 sink（io.Writer 形态，exec 负责排干）。
type lineWriter struct {
	mu   sync.Mutex
	buf  []byte
	sink func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		w.sink(string(bytes.TrimRight(w.buf[:idx], "\r")))
		w.buf = w.buf[idx+1:]
	}
	// 防不换行输出无限膨胀：超过 256KB 强制冲刷为一行。
	if len(w.buf) > 256<<10 {
		w.sink(string(w.buf))
		w.buf = nil
	}
	return len(p), nil
}

// flush 冲刷残留的最后一行（进程退出后调用）。
func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.sink(string(w.buf))
		w.buf = nil
	}
}

// reap 等待进程退出并回收现场（socket 文件 / 能力 token / 日志尾行），
// 之后非停机路径进入 crash 处理。
func (s *Supervisor) reap(e *procEntry, proc *pluginProc, sinks ...*lineWriter) {
	defer s.wg.Done()
	proc.waitErr = proc.cmd.Wait()
	for _, sink := range sinks {
		sink.flush()
	}
	proc.healthy.Store(false)
	s.capability.unregister(proc.token)
	if err := os.Remove(proc.socketPath); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("plugin_socket_cleanup_failed", "plugin", string(e.id), "error", err)
	}
	close(proc.done)

	if proc.stopping.Load() || s.baseCtx.Err() != nil {
		return // 预期退出：terminate/StopAll 侧负责状态归置
	}
	s.handleCrash(e, proc)
}

// handleCrash 处理非预期退出：连挂计数（稳定运行重置）→ 达上限自禁，
// 否则指数退避后重启。
func (s *Supervisor) handleCrash(e *procEntry, proc *pluginProc) {
	e.mu.Lock()
	if e.proc != proc {
		e.mu.Unlock()
		return // 现场已被更新（terminate/relaunch 竞争），交给持有者
	}
	e.proc = nil
	uptime := time.Since(proc.launchedAt)
	if uptime >= s.stableRunReset {
		e.crashes = 1
	} else {
		e.crashes++
	}
	crashes := e.crashes
	e.mu.Unlock()

	exitDesc := "exited unexpectedly"
	if proc.waitErr != nil {
		exitDesc = proc.waitErr.Error()
	}

	if crashes >= s.crashLoopLimit {
		e.setStatus(pluginkit.StateFailed,
			fmt.Sprintf("crash loop: %d consecutive crashes, disabled by supervisor (last: %s)", crashes, exitDesc), nil)
		s.logger.Error("plugin_crash_loop_disabled", "plugin", string(e.id),
			"crashes", crashes, "error", exitDesc)
		ctx, cancelDisable := context.WithTimeout(s.baseCtx, 10*time.Second)
		defer cancelDisable()
		if err := s.states.SetEnabled(ctx, e.id, false, crashLoopDisabledBy); err != nil {
			s.logger.Error("plugin_crash_loop_disable_failed", "plugin", string(e.id), "error", err)
		}
		return
	}

	backoff := s.crashBackoff(crashes)
	e.setStatus(pluginkit.StateFailed,
		fmt.Sprintf("crashed (%s), restart %d/%d in %s", exitDesc, crashes, s.crashLoopLimit, backoff), nil)
	s.logger.Warn("plugin_crashed_restarting", "plugin", string(e.id),
		"crashes", crashes, "backoff", backoff.String(), "error", exitDesc)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		timer := time.NewTimer(backoff)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-s.baseCtx.Done():
			return
		}
		if !s.states.Enabled(e.id) || !s.Handles(e.id) {
			return // 退避期间被停用/卸载
		}
		e.lifeMu.Lock()
		defer e.lifeMu.Unlock()
		if s.states.Enabled(e.id) {
			s.launchLocked(e)
		}
	}()
}

// crashBackoff 计算第 n 次连挂的退避时长：base * 2^(n-1)，封顶 backoffMax。
func (s *Supervisor) crashBackoff(n int) time.Duration {
	backoff := s.backoffBase
	for i := 1; i < n && backoff < s.backoffMax; i++ {
		backoff *= 2
	}
	if backoff > s.backoffMax {
		backoff = s.backoffMax
	}
	return backoff
}

// terminateLocked 停止插件子进程（caller 持有 e.lifeMu）：
// SIGTERM → 宽限超时 SIGKILL → 等待现场回收。无进程时幂等归置状态
// （failed 现场保留不覆盖，便于排障；对齐 Manager.stopLocked 语义）。
func (s *Supervisor) terminateLocked(e *procEntry) error {
	e.mu.Lock()
	proc := e.proc
	e.mu.Unlock()

	if proc == nil {
		e.mu.Lock()
		if e.state == pluginkit.StateRunning { // 纯前端插件的 running 标记
			e.state, e.err, e.startedAt = pluginkit.StateStopped, "", nil
		}
		e.crashes = 0
		e.mu.Unlock()
		return nil
	}

	proc.stopping.Store(true)
	proc.healthy.Store(false)
	if err := signalProc(proc, syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		s.logger.Warn("plugin_sigterm_failed", "plugin", string(e.id), "error", err)
	}
	select {
	case <-proc.done:
	case <-time.After(s.terminateGrace):
		s.logger.Warn("plugin_terminate_grace_exceeded", "plugin", string(e.id),
			"grace", s.terminateGrace.String())
		if err := signalProc(proc, syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("pluginhost: kill plugin %s: %w", e.id, err)
		}
		<-proc.done
	}

	e.mu.Lock()
	if e.proc == proc {
		e.proc = nil
	}
	e.state, e.err, e.startedAt = pluginkit.StateStopped, "", nil
	e.crashes = 0
	e.mu.Unlock()
	s.logger.Info("plugin_process_stopped", "plugin", string(e.id))
	return nil
}

// killProc 对未通过就绪检查的进程直接 SIGKILL 并等待回收。
func (s *Supervisor) killProc(proc *pluginProc) {
	proc.stopping.Store(true)
	if err := signalProc(proc, syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		s.logger.Warn("plugin_kill_failed", "error", err)
	}
	<-proc.done
}

// signalProc 优先对整个进程组发信号（spawn 时 Setpgid 成组，孙进程一并
// 收到），组信号失败或平台不支持时回退直接子进程——组已消亡时直接信号
// 以 ErrProcessDone 收敛，与既有 caller 的容错口径一致。
func signalProc(proc *pluginProc, sig syscall.Signal) error {
	if err := signalProcGroup(proc.cmd.Process.Pid, sig); err == nil {
		return nil
	}
	return proc.cmd.Process.Signal(sig)
}

// awaitHealthy 轮询插件 socket 的 GET /healthz 直到 200；
// 进程早退或超过 launchTimeout 判失败。
func (s *Supervisor) awaitHealthy(proc *pluginProc) error {
	client := newSocketHTTPClient(proc.socketPath)
	defer client.CloseIdleConnections()

	if healthzOK(client) { // 快路径：进程通常在首个轮询间隔内就绪
		return nil
	}
	deadline := time.NewTimer(s.launchTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.healthPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-proc.done:
			exitDesc := "exited during startup"
			if proc.waitErr != nil {
				exitDesc = fmt.Sprintf("exited during startup: %s", proc.waitErr)
			}
			return errors.New("pluginhost: " + exitDesc)
		case <-deadline.C:
			return fmt.Errorf("pluginhost: health check not ready within %s", s.launchTimeout)
		case <-ticker.C:
			if healthzOK(client) {
				return nil
			}
		}
	}
}

// healthzOK 发送单次健康检查请求。
func healthzOK(client *http.Client) bool {
	resp, err := client.Get("http://plugin/healthz")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// newSocketHTTPClient 构造经 unix socket 拨号的 HTTP 客户端（健康检查用）。
func newSocketHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

// newSocketPath 生成插件本次拉起的 socket 路径；unix socket 路径有 ~108 字节
// 上限，过长时以 ID 摘要代替明文。
func (s *Supervisor) newSocketPath(id pluginkit.ID) string {
	name := string(id)
	candidate := filepath.Join(s.socketDir, fmt.Sprintf("%s-%d.sock", name, s.socketSeq.Add(1)))
	if len(candidate) > 100 {
		sum := sha256.Sum256([]byte(id))
		name = hex.EncodeToString(sum[:6])
		candidate = filepath.Join(s.socketDir, fmt.Sprintf("%s-%d.sock", name, s.socketSeq.Add(1)))
	}
	return candidate
}

// newPluginToken 生成能力 API 的 Bearer token（32 字节随机数 hex）。
func newPluginToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("pluginhost: generate plugin token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// ============================================================
// 内部索引
// ============================================================

// entryFor 取或建插件的运行时载体。
func (s *Supervisor) entryFor(id pluginkit.ID) *procEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		e = &procEntry{id: id, state: pluginkit.StateDisabled}
		s.entries[id] = e
	}
	return e
}

// entryOf 取插件的运行时载体；不存在返回 nil。
func (s *Supervisor) entryOf(id pluginkit.ID) *procEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entries[id]
}

// sortedEntries 返回按 ID 稳定排序的全部载体（StopAll 用）。
func (s *Supervisor) sortedEntries() []*procEntry {
	s.mu.RLock()
	out := make([]*procEntry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// setStatus 更新状态三元组（state/err/startedAt）。
func (e *procEntry) setStatus(st pluginkit.State, errMsg string, startedAt *time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state, e.err, e.startedAt = st, errMsg, startedAt
}
