package pluginkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// defaultStopTimeout 是单插件 Stop 的默认超时；超时后记 failed 并继续，不阻塞宿主。
const defaultStopTimeout = 10 * time.Second

// 插件私有子路由器内部的鉴权面前缀。对外路径经宿主分发器映射：
// /api/v1/admin/plugins/:id/api/*path → <engine>/admin/*path
// /api/v1/plugins/:id/api/*path       → <engine>/user/*path
const (
	adminPathPrefix = "/admin"
	userPathPrefix  = "/user"
)

// SanitizeDispatchPath 把分发器捕获的 *path 参数拼接到鉴权面前缀上并归一化。
// 归一化后逃出前缀（如 rel 含 ".."）的路径返回 ok=false：/user 与 /admin 的
// 权限断言由宿主前缀承载，不得外包给插件侧 router 的路径归一化行为。
// 内建分发器与外部反代（pluginhost）共用本函数，保证两层语义严格一致。
func SanitizeDispatchPath(prefix, rel string) (string, bool) {
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	cleaned := path.Clean(prefix + rel)
	if cleaned != prefix && !strings.HasPrefix(cleaned, prefix+"/") {
		return "", false
	}
	return cleaned, true
}

// pluginRouter 是暴露给插件 MountRoutes 的受限路由面（Router 实现）。
type pluginRouter struct {
	admin gin.IRouter
	user  gin.IRouter
}

func (r *pluginRouter) Admin() gin.IRouter { return r.admin }
func (r *pluginRouter) User() gin.IRouter  { return r.user }

// pluginEntry 是单个插件在 Manager 内的运行时载体。
// 实例在 NewManager 时创建一次，enable→disable→enable 全程复用同一实例。
type pluginEntry struct {
	id     ID
	plugin Plugin
	host   *Host
	engine *gin.Engine // 仅 APIProvider 有；Bootstrap 时挂载一次

	// lifeMu 串行化该插件的生命周期操作（toggle 与 StopAll 互斥）。
	lifeMu sync.Mutex

	// stopFinished 非 nil 表示一次超时逃逸的 Stop 仍在后台执行（其 goroutine
	// 退出时关闭该通道）。在其退出前拒绝再次 Init/Start：契约要求 Stop 返回后
	// 实例方可复用，否则新生命周期会与仍在拆解的旧 Stop 并发。
	// 读写均在持有 lifeMu 时进行。
	stopFinished chan struct{}

	// mountErr 非空表示 Bootstrap 期间 MountRoutes panic（进程内不可恢复：
	// 路由只在启动时装配一次）。startLocked 据此永久拒绝启动，避免插件
	// 以"running 但无 API 面"的半活状态对外。Bootstrap 单线程写、此后只读。
	mountErr string

	// 以下状态字段由 Manager.statusMu 保护（写者同时持有 lifeMu）。
	state     State
	err       string
	startedAt *time.Time
}

// Manager 是插件生命周期驱动器：Bootstrap 一次性实例化与挂载路由，
// 运行期经 StateStore.Subscribe 驱动免重启启停，并为宿主分发器提供门控与转发。
type Manager struct {
	logger      *slog.Logger
	states      StateStore
	cfg         PluginsConfig
	entries     map[ID]*pluginEntry
	order       []ID // 按 ID 稳定排序，Snapshot 与 StopAll（逆序）共用
	stopTimeout time.Duration

	// statusMu 保护所有 entry 的 state/err/startedAt（GateAllows/Snapshot 读，生命周期写）。
	statusMu sync.RWMutex

	bootMu       sync.Mutex
	bootstrapped bool
}

// NewManager 校验并实例化全部注册插件（Factory 构造无副作用，此处即唯一一次实例化）。
// 重复 ID 或非法 ID 返回 error。
func NewManager(host HostDeps, states StateStore, cfg PluginsConfig, factories []Factory) (*Manager, error) {
	logger := host.Logger
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{
		logger:      logger,
		states:      states,
		cfg:         cfg,
		entries:     make(map[ID]*pluginEntry, len(factories)),
		order:       make([]ID, 0, len(factories)),
		stopTimeout: defaultStopTimeout,
	}
	for _, factory := range factories {
		p := factory()
		id := p.ID()
		if err := id.Validate(); err != nil {
			return nil, err
		}
		if _, dup := m.entries[id]; dup {
			return nil, fmt.Errorf("pluginkit: duplicate plugin id %q", id)
		}
		m.entries[id] = &pluginEntry{
			id:     id,
			plugin: p,
			host: &Host{
				Logger: logger.With("plugin", string(id)),
				DB:     host.DB,
				Redis:  host.Redis,
				Config: cfg.DecoderFor(id),
			},
			state: StateDisabled,
		}
		m.order = append(m.order, id)
	}
	sort.Slice(m.order, func(i, j int) bool { return m.order[i] < m.order[j] })
	return m, nil
}

// Bootstrap 完成一次性装配并拉起 enabled 插件：
// APIProvider 挂载到各自私有子路由器（独立 gin.Engine）→ 订阅 toggle → enabled 者 Init→Start。
// 单插件失败进 failed 态并记日志，不拖垮宿主；重复调用返回 error。
func (m *Manager) Bootstrap(ctx context.Context) error {
	m.bootMu.Lock()
	if m.bootstrapped {
		m.bootMu.Unlock()
		return errors.New("pluginkit: manager already bootstrapped")
	}
	m.bootstrapped = true
	m.bootMu.Unlock()

	// 配置里出现但未注册的 ID 打 warn：兜底 ParsePluginsConfig 的点分 ID 还原歧义。
	for id := range m.cfg {
		if _, ok := m.entries[id]; !ok {
			m.logger.Warn("plugin_config_unknown_id", "plugin", string(id))
		}
	}

	for _, id := range m.order {
		e := m.entries[id]
		ap, ok := e.plugin.(APIProvider)
		if !ok {
			continue
		}
		engine := gin.New()
		// 插件 handler 的 panic 在子路由器边界内兜住（宿主 Recovery 之前），
		// 日志带插件归属，响应与核心 500 语义一致。
		engine.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, rec any) {
			m.logger.Error("plugin_handler_panic", "plugin", string(id),
				"path", c.Request.URL.Path, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
			response.InternalError(c, "plugin handler error")
		}))
		engine.NoRoute(func(c *gin.Context) {
			response.NotFound(c, "plugin route not found")
		})
		// MountRoutes 是首方回调，必须与 Init/Start/Stop 同网兜 panic：
		// 路由注册 bug（重复路由 panic / nil deref）只隔离本插件，
		// 不得穿透 Bootstrap 令宿主 fail-fast 起不来。
		if err := m.guardPlugin(e, "mount_routes", func() error {
			ap.MountRoutes(&pluginRouter{
				admin: engine.Group(adminPathPrefix),
				user:  engine.Group(userPathPrefix),
			})
			return nil
		}); err != nil {
			e.mountErr = err.Error()
			m.setStatus(e, StateFailed, err.Error(), nil)
			continue
		}
		e.engine = engine
	}

	// DefaultEnabler 自播种：从未有显式记录的"默认启用"插件（典型：迁移自
	// 既有常驻服务的 job.*）落一条 enabled=true 记录。放在 Subscribe 之前，
	// 使播种不触发 onToggle（下方启动循环统一拉起），且此后管理员的显式停用
	// 因记录已存在而不会被再次翻开。
	for _, id := range m.order {
		de, ok := m.entries[id].plugin.(DefaultEnabler)
		if !ok || !de.DefaultEnabled() {
			continue
		}
		if _, exists := m.states.Lookup(id); exists {
			continue
		}
		if err := m.states.SetEnabled(ctx, id, true, "pluginkit:default-enabled"); err != nil {
			// 播种失败必须 fail-fast：默认启用的插件（如内容审计）若因瞬时 DB
			// 故障静默落空，会在服务照常对外的同时 fail-open 放行其挂钩点。
			return fmt.Errorf("pluginkit: seed default-enabled plugin %s: %w", id, err)
		}
	}

	// 先订阅再启动：Bootstrap 期间到达的 toggle 由幂等 start/stop 与内存快照复核兜底，
	// 既不丢事件也不会双启（per-plugin lifeMu 串行）。
	m.states.Subscribe(m.onToggle)

	for _, id := range m.order {
		e := m.entries[id]
		e.lifeMu.Lock()
		if m.states.Enabled(id) {
			m.startLocked(ctx, e)
		}
		e.lifeMu.Unlock()
	}
	return nil
}

// onToggle 经 states.Subscribe 驱动（本实例与跨实例变更均触发），per-plugin 串行。
func (m *Manager) onToggle(ch StateChange) {
	e, ok := m.entries[ch.ID]
	if !ok {
		m.logger.Warn("plugin_toggle_unknown_id", "plugin", string(ch.ID), "enabled", ch.Enabled)
		return
	}
	e.lifeMu.Lock()
	defer e.lifeMu.Unlock()
	// 复核内存快照：快速连翻时跳过已过期的事件，只让终态事件生效。
	if ch.Enabled != m.states.Enabled(ch.ID) {
		return
	}
	if ch.Enabled {
		m.startLocked(context.Background(), e)
	} else {
		// 错误已在 stopLocked 内记日志并落入状态字段，事件回调无处上抛。
		_ = m.stopLocked(context.Background(), e)
	}
}

// guardPlugin 在插件生命周期调用边界统一 recover：panic 转为 error 并携带
// 归属与堆栈记日志。生命周期调用可能跑在无兜底的 goroutine 上（Redis 订阅
// 回调、对账循环、wire cleanup），不设防的插件 panic 会直接崩溃宿主进程。
func (m *Manager) guardPlugin(e *pluginEntry, op string, fn func() error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			m.logger.Error("plugin_panic", "plugin", string(e.id), "op", op,
				"panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
			err = fmt.Errorf("%s panicked: %v", op, rec)
		}
	}()
	return fn()
}

// startLocked 对同一实例执行 Init→Start（caller 持有 e.lifeMu）。
// 已 running 时幂等 no-op；失败进 failed 态并记日志。
func (m *Manager) startLocked(ctx context.Context, e *pluginEntry) {
	if m.currentState(e) == StateRunning {
		return
	}
	// MountRoutes 已失败的插件永久拒绝启动：路由装配只发生在 Bootstrap，
	// 启动它只会得到"running 但 API 面 404"的半活实例。
	if e.mountErr != "" {
		m.setStatus(e, StateFailed, "mount routes failed: "+e.mountErr, nil)
		m.logger.Error("plugin_start_rejected_mount_failed", "plugin", string(e.id), "error", e.mountErr)
		return
	}
	// 上一轮超时逃逸的 Stop 仍在执行时拒绝启动：等它退出前，同一实例上跑
	// Init/Start 会与拆解中的旧 Stop 并发，违反实例复用契约。管理员稍后
	// 重试 enable 即可（届时通道已关闭则照常启动）。
	if e.stopFinished != nil {
		select {
		case <-e.stopFinished:
			e.stopFinished = nil
		default:
			m.setStatus(e, StateFailed, "previous stop still in progress; retry enable later", nil)
			m.logger.Error("plugin_start_rejected_stop_in_flight", "plugin", string(e.id))
			return
		}
	}
	if init, ok := e.plugin.(Initializer); ok {
		if err := m.guardPlugin(e, "init", func() error { return init.Init(ctx, e.host) }); err != nil {
			m.setStatus(e, StateFailed, err.Error(), nil)
			m.logger.Error("plugin_init_failed", "plugin", string(e.id), "error", err)
			return
		}
	}
	if r, ok := e.plugin.(Runner); ok {
		if err := m.guardPlugin(e, "start", func() error { return r.Start(ctx) }); err != nil {
			m.setStatus(e, StateFailed, err.Error(), nil)
			m.logger.Error("plugin_start_failed", "plugin", string(e.id), "error", err)
			// 补偿 Stop（best-effort）：Start 失败可能遗留 Init/半启动已获取的
			// 资源，不回收则 re-enable 时双重获取。复用 stopRunner 的超时与
			// 在途登记；其结果仅记日志，failed 现场保留原始 Start 错误。
			if serr := m.stopRunner(ctx, e, r); serr != nil {
				m.logger.Error("plugin_start_compensate_stop_failed", "plugin", string(e.id), "error", serr)
			}
			return
		}
	}
	now := time.Now()
	m.setStatus(e, StateRunning, "", &now)
	m.logger.Info("plugin_started", "plugin", string(e.id))
}

// stopLocked 停止 running 插件（caller 持有 e.lifeMu）；非 running 态幂等 no-op
// （failed 现场保留不覆盖，便于排障；门控只看 enabled+running，不受影响）。
func (m *Manager) stopLocked(ctx context.Context, e *pluginEntry) error {
	if m.currentState(e) != StateRunning {
		return nil
	}
	if r, ok := e.plugin.(Runner); ok {
		if err := m.stopRunner(ctx, e, r); err != nil {
			m.setStatus(e, StateFailed, err.Error(), nil)
			m.logger.Error("plugin_stop_failed", "plugin", string(e.id), "error", err)
			return fmt.Errorf("pluginkit: stop plugin %s: %w", e.id, err)
		}
	}
	m.setStatus(e, StateStopped, "", nil)
	m.logger.Info("plugin_stopped", "plugin", string(e.id))
	return nil
}

// stopRunner 在独立 goroutine 中调用 Stop 并施加超时（caller 持有 e.lifeMu）：
// 即使插件不遵守 ctx 卡死，也只泄漏该 goroutine 而不阻塞宿主。超时逃逸时把
// 在途通道记到 e.stopFinished，startLocked 据此拒绝与旧 Stop 并发的重启。
func (m *Manager) stopRunner(ctx context.Context, e *pluginEntry, r Runner) error {
	sctx, cancel := context.WithTimeout(ctx, m.stopTimeout)
	defer cancel()
	finished := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		defer close(finished)
		done <- m.guardPlugin(e, "stop", func() error { return r.Stop(sctx) })
	}()
	select {
	case err := <-done:
		return err
	case <-sctx.Done():
		e.stopFinished = finished
		return fmt.Errorf("stop timed out after %s: %w", m.stopTimeout, sctx.Err())
	}
}

// GateAllows 是分发器门控：enabled（内存快照）且 state==running 才放行。
func (m *Manager) GateAllows(id ID) bool {
	e, ok := m.entries[id]
	if !ok || !m.states.Enabled(id) {
		return false
	}
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	return e.state == StateRunning
}

// Dispatch 是宿主分发器入口：把 /api/v1[/admin]/plugins/:id/api/*path 的请求
// 重写为插件相对路径后转发给其私有子路由器。未知 ID、未启用、非 running、
// 无 API 面一律同一个 404，不区分原因（避免探测插件装配情况）。
func (m *Manager) Dispatch(side Side, id string, c *gin.Context) {
	var prefix string
	switch side {
	case SideAdmin:
		prefix = adminPathPrefix
	case SideUser:
		prefix = userPathPrefix
	default:
		response.NotFound(c, "plugin not found")
		return
	}
	e, ok := m.entries[ID(id)]
	if !ok || e.engine == nil || !m.GateAllows(e.id) {
		response.NotFound(c, "plugin not found")
		return
	}
	// gin 的 *path 参数自带前导 /；路由无尾段时为空。归一化后逃出鉴权面
	// 前缀的路径（".." 穿越）与未知插件同一个 404，不泄露拒绝原因。
	target, ok := SanitizeDispatchPath(prefix, c.Param("path"))
	if !ok {
		response.NotFound(c, "plugin not found")
		return
	}
	req := c.Request
	origPath, origRawPath := req.URL.Path, req.URL.RawPath
	req.URL.Path = target
	req.URL.RawPath = ""
	// 转发后还原，宿主侧后置中间件（日志/指标）看到的仍是原始路径。
	defer func() { req.URL.Path, req.URL.RawPath = origPath, origRawPath }()
	e.engine.ServeHTTP(c.Writer, req)
}

// StopAll 逆序停止所有 running 插件，接入 wire cleanup；错误聚合返回。
func (m *Manager) StopAll(ctx context.Context) error {
	var errs []error
	for i := len(m.order) - 1; i >= 0; i-- {
		e := m.entries[m.order[i]]
		e.lifeMu.Lock()
		err := m.stopLocked(ctx, e)
		e.lifeMu.Unlock()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Snapshot 返回全部插件的状态快照，按 ID 稳定排序。
func (m *Manager) Snapshot() []PluginStatus {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	out := make([]PluginStatus, 0, len(m.order))
	for _, id := range m.order {
		e := m.entries[id]
		st := PluginStatus{
			ID:      id,
			Enabled: m.states.Enabled(id),
			State:   e.state,
			Err:     e.err,
		}
		if e.startedAt != nil {
			t := *e.startedAt
			st.StartedAt = &t
		}
		out = append(out, st)
	}
	return out
}

func (m *Manager) setStatus(e *pluginEntry, s State, errMsg string, startedAt *time.Time) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	e.state, e.err, e.startedAt = s, errMsg, startedAt
}

func (m *Manager) currentState(e *pluginEntry) State {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	return e.state
}
