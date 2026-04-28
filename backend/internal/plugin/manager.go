package plugin

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/Wei-Shaw/sub2api/internal/pkg/leaderlock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// PluginManager 是插件子系统的协调器,负责:
//   - 发现 / 启动 / 停止插件子进程
//   - 维护 PluginInstance 状态,执行 gRPC handshake 与 manifest 拉取
//   - 把插件路由注入 PluginRouter
//   - 触发健康监控与自动重启
type PluginManager struct {
	mu        sync.RWMutex
	plugins   map[string]*PluginInstance
	repo      *PluginRepository
	router    *PluginRouter
	sdkServer *SDKServer
	sdkAddr   string // SDK gRPC 实际监听地址(含端口)
	sdkLn     net.Listener
	sdkGRPC   *grpc.Server
	cfg       Config
	db        *sql.DB
	logger    *slog.Logger

	// instanceID identifies this host process for leader-lock contention.
	// Generated once per manager so a restart releases the lease only via
	// TTL expiry, never accidentally because two managers share an ID.
	instanceID string

	// jobScheduler holds the V5 W2 JobScheduler service so admin handlers
	// (W2.4) can call ManualFire without going through the SDKServer.
	jobScheduler *JobSchedulerServer

	// jobHistory is the persistence sink for plugin job runs. Optional; nil
	// means "log-only", which is the default until the W2.4 admin handler
	// installs a DB-backed implementation via SetJobHistoryRecorder.
	jobHistory JobHistoryRecorder
	// settingsService is the host-side store for plugin SettingsExtension
	// schemas + values. Set via SetSettingsService before Start; nil is
	// allowed and disables the capability for every plugin (the SDK side
	// returns a clear error rather than silently no-oping).
	settingsService PluginSettingsRegistrar
	// settingsServer is the gRPC service that bridges SDK calls to the
	// settingsService. Lazily registered when settingsService is non-nil
	// so that running without the plugin settings subsystem incurs no
	// additional gRPC services.
	settingsServer SettingsExtensionRegistrar
	// frontendCacheInvalidator clears the FrontendServer HTML cache so the
	// next first-byte response carries an up-to-date __PLUGIN_MANIFESTS__
	// payload. Wired in via SetFrontendCacheInvalidator from server/router.go
	// once both PluginManager and FrontendServer are constructed. Optional —
	// nil means "skip" (e.g. embedded frontend disabled at build time).
	//
	// Must be called whenever the active set of plugins or any plugin's
	// manifest changes (enable/disable/restart), otherwise the SSR-cached
	// HTML returned to subsequent reloads still references the old plugin
	// list and the frontend Sidebar / vue-router pick up stale state.
	frontendCacheInvalidator func()
}

// PluginSettingsRegistrar is the minimal surface PluginManager needs from
// service.PluginSettingsService.
//
// V5/W6 SETTINGS-V2:
//   - RegisterSchemaWithInput is the aggregate-input form (DESIGN §4.1)
//     that carries schema_version + properties_meta from the manifest.
//     PluginManager calls this exclusively; the legacy RegisterSchema
//     wrapper is preserved on the concrete service but not surfaced
//     here because every code path the manager owns now ships the v2
//     fields.
//   - Subscribe lets PluginManager observe value-change events for a
//     single plugin so it can coalesce reload triggers when a key with
//     x-requires-reload=true is updated (DESIGN §4.4). Subscribe only
//     watches one (plugin, key) pair per call; passing key="" subscribes
//     to the whole namespace, which is the mode the manager uses.
type PluginSettingsRegistrar interface {
	RegisterSchemaWithInput(ctx context.Context, in service.RegisterSchemaInput) error
	UnregisterSchema(pluginName string)
	Subscribe(pluginName, key string) (<-chan service.PluginSettingsChange, func())
}

// SettingsExtensionRegistrar is the minimal surface PluginManager needs to
// attach the SettingsExtension gRPC service to the SDK's grpc.Server.
type SettingsExtensionRegistrar interface {
	Register(grpcServer *grpc.Server)
}

// NewPluginManager 构造 manager。
// 注意:此函数不启动 SDK gRPC 服务,需调用 Start 才会监听端口与启用插件。
func NewPluginManager(db *sql.DB, rdb *redis.Client, cfg Config, router *PluginRouter) (*PluginManager, error) {
	if db == nil {
		return nil, errors.New("nil sql db")
	}
	// router 允许为 nil:wire 树中 PluginManager 可能先于 PluginRouter 创建,
	// 调用方负责在 Start 之前调用 BindRouter 注入。
	cfg = cfg.withDefaults()

	repo := NewPluginRepository(db)
	sdk, err := NewSDKServer(db, rdb, cfg.SecretEncryptionMasterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("init plugin SDK server: %w", err)
	}

	return &PluginManager{
		plugins:    make(map[string]*PluginInstance),
		repo:       repo,
		router:     router,
		sdkServer:  sdk,
		cfg:        cfg,
		db:         db,
		logger:     slog.Default().With("component", "plugin_manager"),
		instanceID: uuid.NewString(),
	}, nil
}

// SetFrontendCacheInvalidator wires a callback the manager invokes whenever
// the active plugin set or any plugin's manifest changes. server/router.go
// passes FrontendServer.InvalidateCache so the next HTML render reflects the
// current plugin list.
func (m *PluginManager) SetFrontendCacheInvalidator(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frontendCacheInvalidator = fn
}

// invalidateFrontendCache fires the registered invalidator if any. Safe to
// call from any goroutine; the callback is expected to be cheap and idempotent.
func (m *PluginManager) invalidateFrontendCache() {
	m.mu.RLock()
	fn := m.frontendCacheInvalidator
	m.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// SetJobHistoryRecorder installs the persistence sink the JobScheduler uses
// to record runs. Wire-tree usage: admin handler builds a DB-backed
// JobHistoryRecorder and calls this BEFORE PluginManager.Start so the
// scheduler picks it up at construction time. Calling after Start is a
// programmer error (the scheduler captures the recorder by value at
// startSDKServer) but is non-fatal — only future restarts will pick up the
// new value.
func (m *PluginManager) SetJobHistoryRecorder(rec JobHistoryRecorder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobHistory = rec
}

// JobScheduler returns the active JobScheduler so admin handlers can call
// ManualFire / inspect schedulers. nil before Start has wired the SDK gRPC
// server.
func (m *PluginManager) JobScheduler() *JobSchedulerServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobScheduler
}

// Start 启动 SDK gRPC 服务并加载所有 enabled 插件。
// ctx 仅用于初始的 SDK 启动与 enabled 插件加载,不影响后续插件的健康监控。
func (m *PluginManager) Start(ctx context.Context) error {
	m.mu.RLock()
	router := m.router
	m.mu.RUnlock()
	if router == nil {
		return errors.New("plugin manager: router not bound; call BindRouter before Start")
	}
	if err := m.repo.EnsureSchema(ctx); err != nil {
		// 表不存在时回退到 EnsureSchema;若仍失败则视为致命。
		return err
	}

	if err := m.startSDKServer(); err != nil {
		return err
	}

	// 从磁盘扫描插件,合并到 DB。Builtin 目录中的插件首次启动时按
	// AutoEnableBuiltin 设置自动启用；Disk 目录的插件创建为 disabled。
	if m.cfg.BuiltinDir != "" || m.cfg.PluginsDir != "" {
		if err := m.syncFromDisk(ctx); err != nil {
			m.logger.Warn("sync plugins from disk failed", "error", err)
		}
	}

	records, err := m.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list plugins: %w", err)
	}
	for _, rec := range records {
		if !rec.Enabled {
			continue
		}
		if err := m.EnablePlugin(ctx, rec.Name); err != nil {
			m.logger.Error("auto enable plugin failed",
				"plugin", rec.Name,
				"error", err,
			)
		}
	}
	return nil
}

// BindRouter 在 wire 树构建完成后,把 PluginRouter 绑定到 manager。
// 必须在 Start 之前调用;重复调用会覆盖已有 router。
//
// 该方法存在的原因:wire 创建顺序中 PluginManager 先于 PluginRouter,
// 否则会形成 handler->manager->router->engine->handlers 的循环依赖。
func (m *PluginManager) BindRouter(router *PluginRouter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.router = router
}

// startSDKServer 监听 SDK gRPC 端口并启动 grpc.Server。
//
// 这里同时初始化 V5 W2 引入的 JobSchedulerServer:scheduler 与 SDKServer
// 共享同一个 Redis client(通过 leaderlock helper 复用 OpsCleanupService 的
// 锁实现),并把 caller 解析委托给 SDKServer.resolveCaller,确保 plugin
// metadata 头与 RedisProxy/EventBus 等服务用同一套身份识别。
func (m *PluginManager) startSDKServer() error {
	ln, err := net.Listen("tcp", m.cfg.SDKListenAddr)
	if err != nil {
		return fmt.Errorf("listen sdk addr %s: %w", m.cfg.SDKListenAddr, err)
	}
	m.sdkLn = ln
	m.sdkAddr = ln.Addr().String()

	// Construct the JobScheduler before RegisterServices so the gRPC
	// service descriptor is registered. m.jobHistory may be nil (no admin
	// handler configured yet) — the scheduler tolerates that and just
	// skips persistence.
	leaderProvider := m.buildLeaderLockProvider()
	jobScheduler := NewJobSchedulerServer(
		m.sdkServer.resolveCaller,
		leaderProvider,
		m.jobHistory,
		m.logger,
	)
	m.sdkServer.AttachJobScheduler(jobScheduler)
	m.jobScheduler = jobScheduler

	srv := grpc.NewServer()
	m.sdkServer.RegisterServices(srv)
	if m.settingsServer != nil {
		m.settingsServer.Register(srv)
	}
	m.sdkGRPC = srv

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			m.logger.Error("sdk grpc server stopped", "error", err)
		}
	}()
	m.logger.Info("sdk grpc server listening", "addr", m.sdkAddr)
	return nil
}

// buildLeaderLockProvider mints a leaderlock.Provider sharing the manager's
// Redis client (when present) with a stable per-process instance ID. We pull
// the Redis client off SDKServer because the manager no longer keeps a
// reference of its own — see NewPluginManager.
func (m *PluginManager) buildLeaderLockProvider() leaderlock.Provider {
	return leaderlock.New(m.sdkServer.redis, m.db, leaderlock.Config{
		InstanceID: m.instanceID,
		TTL:        jobLeaderLockTTL,
		// SingleInstance is intentionally false: even in simple mode we
		// take the Redis lease so subsequent multi-replica deploys do not
		// silently double-fire jobs. The SetNX cost is negligible.
		Logger: m.logger.With("component", "plugin_job_leader_lock"),
	})
}

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
		_, err := m.repo.Get(ctx, d.Name)
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

// ShutdownAll 优雅关闭所有插件,然后停止 SDK gRPC。
func (m *PluginManager) ShutdownAll(ctx context.Context) {
	m.mu.RLock()
	names := make([]string, 0, len(m.plugins))
	for name := range m.plugins {
		names = append(names, name)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if err := m.stopInstance(ctx, n, false); err != nil {
				m.logger.Warn("shutdown plugin failed", "plugin", n, "error", err)
			}
		}(name)
	}
	wg.Wait()

	if m.sdkGRPC != nil {
		m.sdkGRPC.GracefulStop()
	}
	// Stop the JobScheduler before SDKServer so per-plugin schedulers can
	// flush pending acks while their RecordRun sink is still alive.
	if m.jobScheduler != nil {
		m.jobScheduler.Stop()
	}
	if m.sdkServer != nil {
		m.sdkServer.Stop()
	}
}

// EnablePlugin 启动指定插件。已经在运行则视为无操作并返回成功。
func (m *PluginManager) EnablePlugin(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	rec, err := m.repo.Get(ctx, name)
	if err != nil {
		return err
	}

	binPath := m.binaryPathFor(name)
	if binPath == "" {
		return fmt.Errorf("plugin binary not found for %s", name)
	}

	inst := m.getOrCreateInstance(name, binPath)

	inst.mu.Lock()
	if inst.State == StateRunning || inst.State == StateStarting {
		inst.mu.Unlock()
		// 持久化 enabled=true,即便已经在运行。
		_ = m.repo.SetEnabled(ctx, name, true)
		return nil
	}
	if err := inst.transitionTo(StateStarting); err != nil {
		inst.mu.Unlock()
		return err
	}
	cfgCopy := copyConfig(rec.Config)
	inst.mu.Unlock()

	if err := m.spawnAndConnect(ctx, inst, cfgCopy); err != nil {
		inst.mu.Lock()
		_ = inst.transitionTo(StateErrored)
		inst.LastError = err
		inst.mu.Unlock()
		return err
	}

	if err := m.repo.SetEnabled(ctx, name, true); err != nil {
		m.logger.Warn("persist enabled flag failed", "plugin", name, "error", err)
	}
	// Plugin manifest is now part of GetPluginManifestsJSON output; drop the
	// frontend HTML cache so the next reload sees this plugin in sidebar/router.
	m.invalidateFrontendCache()
	return nil
}

// DisablePlugin 停止指定插件,并将 DB 中的 enabled 字段置为 false。
func (m *PluginManager) DisablePlugin(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	if err := m.stopInstance(ctx, name, true); err != nil {
		return err
	}
	if err := m.repo.SetEnabled(ctx, name, false); err != nil {
		return err
	}
	// Plugin removed from GetPluginManifestsJSON output; invalidate so the
	// next reload removes its sidebar/router entries.
	m.invalidateFrontendCache()
	return nil
}

// RestartPlugin 先停后启。
func (m *PluginManager) RestartPlugin(ctx context.Context, name string) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	if err := m.stopInstance(ctx, name, false); err != nil {
		m.logger.Warn("restart: stop step failed", "plugin", name, "error", err)
	}
	return m.EnablePlugin(ctx, name)
}

// ListPlugins 返回所有已注册插件的运行时信息。
func (m *PluginManager) ListPlugins(ctx context.Context) ([]PluginInfo, error) {
	records, err := m.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]PluginInfo, 0, len(records))
	for _, rec := range records {
		m.mu.RLock()
		inst, ok := m.plugins[rec.Name]
		m.mu.RUnlock()

		var info PluginInfo
		if ok {
			info = inst.SnapshotInfo()
		} else {
			info = PluginInfo{
				Name:  rec.Name,
				State: StateRegistered.String(),
			}
		}
		mergeRecordIntoInfo(&info, rec)
		out = append(out, info)
	}
	return out, nil
}

// GetPlugin 返回单个插件的信息。
func (m *PluginManager) GetPlugin(ctx context.Context, name string) (*PluginInfo, error) {
	rec, err := m.repo.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	inst, ok := m.plugins[name]
	m.mu.RUnlock()

	var info PluginInfo
	if ok {
		info = inst.SnapshotInfo()
	} else {
		info = PluginInfo{
			Name:  rec.Name,
			State: StateRegistered.String(),
		}
	}
	mergeRecordIntoInfo(&info, *rec)
	return &info, nil
}

// mergeRecordIntoInfo 把数据库字段(Enabled/SortOrder/Description/Config 等)填入 info,
// 同时在运行时 manifest 缺失 DisplayName/Version 时回退到数据库记录。
func mergeRecordIntoInfo(info *PluginInfo, rec PluginRecord) {
	info.Enabled = rec.Enabled
	info.SortOrder = rec.SortOrder
	info.Description = rec.Description
	if info.DisplayName == "" {
		info.DisplayName = rec.DisplayName
	}
	if info.Version == "" {
		info.Version = rec.Version
	}
	info.Config = configToAny(rec.Config)
}

// configToAny 把存储层的 map[string]string 转成 API 暴露的 map[string]any。
// 配置为空时返回 nil,避免 API 输出空对象。
func configToAny(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// SDKAddr 返回 SDK gRPC 实际监听地址,主要用于测试与诊断。
func (m *PluginManager) SDKAddr() string {
	return m.sdkAddr
}

// =============================================================
// Admin handler 适配方法
//
// PluginHandler (internal/handler/admin) 通过最小化接口与 manager 交互,
// 命名风格为短动词(List/Get/Enable/Disable/Restart/UpdateConfig)。
// 这里通过适配方法转发到主实现, 避免上层因命名差异而依赖具体类型。
// =============================================================

// List 是 ListPlugins 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) List(ctx context.Context) ([]PluginInfo, error) {
	return m.ListPlugins(ctx)
}

// Get 是 GetPlugin 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) Get(ctx context.Context, name string) (*PluginInfo, error) {
	return m.GetPlugin(ctx, name)
}

// Enable 是 EnablePlugin 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) Enable(ctx context.Context, name string) error {
	return m.EnablePlugin(ctx, name)
}

// Disable 是 DisablePlugin 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) Disable(ctx context.Context, name string) error {
	return m.DisablePlugin(ctx, name)
}

// Restart 是 RestartPlugin 的别名,实现 admin.PluginManager 接口。
func (m *PluginManager) Restart(ctx context.Context, name string) error {
	return m.RestartPlugin(ctx, name)
}

// UpdateConfig 持久化插件的配置 JSON。
//
// 当前实现只更新数据库,**不会**自动重启插件子进程,
// 是否需要重启由调用方根据具体配置项决定;插件可以在 Init 时拉取最新配置,
// 或通过自定义 RPC 在运行时热加载。
//
// 因为存储层使用 map[string]string,这里把 any 值统一序列化为字符串形式:
// 简单类型走 fmt.Sprintf("%v"),复合类型(map/slice)走 json.Marshal,
// 保证 round-trip 时配置语义不丢失。
func (m *PluginManager) UpdateConfig(ctx context.Context, name string, cfg map[string]any) error {
	if !IsValidPluginName(name) {
		return ErrInvalidPluginName
	}
	strCfg, err := configToString(cfg)
	if err != nil {
		return err
	}
	return m.repo.UpdateConfig(ctx, name, strCfg)
}

// configToString 把 admin API 收到的 map[string]any 转成存储层的 map[string]string。
// 复合值序列化为 JSON 以便后续读取时还原。
func configToString(in map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for k, v := range in {
		switch val := v.(type) {
		case nil:
			out[k] = ""
		case string:
			out[k] = val
		case bool, int, int32, int64, float32, float64:
			out[k] = fmt.Sprintf("%v", val)
		default:
			b, err := json.Marshal(val)
			if err != nil {
				return nil, fmt.Errorf("marshal config key %q: %w", k, err)
			}
			out[k] = string(b)
		}
	}
	return out, nil
}

// allowedPluginCapabilities is the universe of capabilities the core honours.
// Any value outside this set is rejected even if a plugin requests it. Adding
// a new capability requires both an entry here and the corresponding
// enforcement in the SDK server (e.g. grpc_server.go).
//
// Keep these strings literal (not pluginsdk constants) to avoid pulling the
// SDK package into manager.go's already-large import set. Each entry MUST
// match the corresponding pluginsdk.Capability* constant.
var allowedPluginCapabilities = map[string]struct{}{
	"redis_raw_keys":     {}, // pluginsdk.CapabilityRedisRawKeys
	"safe_outbound_http": {}, // pluginsdk.CapabilitySafeOutboundHTTP — V5 W4
	"secret_encryption":  {}, // pluginsdk.CapabilitySecretEncryption — V5 W5
	// job_scheduler is the V5 W2 capability granting access to the
	// JobScheduler stream RPC. Default-allow because scheduling work is
	// not a privileged cross-plugin operation — see V5-DESIGN §2.7.
	"job_scheduler": {},
	// settings_extension lets a plugin register a JSON-Schema-described
	// settings tab in the admin SettingsView and read its persisted values
	// via SDK Settings(). See V5-DESIGN §W3 (SettingsExtensionCapability).
	"settings_extension": {}, // pluginsdk.CapabilitySettingsExtension — V5 W3
}

// defaultOutboundBlockedCIDRs is the host-side default block list pushed to
// every plugin at Init time. Plugins can layer ExtraBlockedCIDRs on top via
// pluginsdk.OutboundConfig but cannot remove entries. See V5-DESIGN §W4.
var defaultOutboundBlockedCIDRs = []string{
	"127.0.0.0/8",    // IPv4 loopback
	"10.0.0.0/8",     // RFC1918
	"172.16.0.0/12",  // RFC1918
	"192.168.0.0/16", // RFC1918
	"169.254.0.0/16", // link-local (cloud metadata 169.254.169.254)
	"100.64.0.0/10",  // CGNAT
	"0.0.0.0/8",      // "this network"
	"224.0.0.0/4",    // multicast
	"::1/128",        // IPv6 loopback
	"fc00::/7",       // IPv6 ULA
	"fe80::/10",      // IPv6 link-local
}

// buildOutboundDefaults produces the OutboundDefaults the core pushes to a
// plugin at Init. V5 keeps this as a pure-default helper — once W3
// SettingsExtension lands, the admin-configured block/allow-lists will be
// merged in here.
func (m *PluginManager) buildOutboundDefaults() *pluginsdk.OutboundDefaults {
	return &pluginsdk.OutboundDefaults{
		BlockedCidrs: defaultOutboundBlockedCIDRs,
		AllowedHosts: nil, // empty = no global host allow-list
		MaxRedirects: 3,
		TimeoutNanos: int64(30 * time.Second),
		MaxBodyBytes: 1 << 20, // 1 MiB
	}
}

// approveCapabilities filters the capabilities a plugin requested in its
// manifest down to the subset the core is willing to grant. Unknown values
// are dropped with a warning so operators can spot typos in plugin manifests.
//
// The returned slice is what we forward to the plugin via PluginInitRequest;
// it doubles as the authoritative list the SDK server uses to police Do
// requests with raw_key=true.
func approveCapabilities(pluginName string, requested []string, logger *slog.Logger) []string {
	if len(requested) == 0 {
		return nil
	}
	out := make([]string, 0, len(requested))
	for _, c := range requested {
		if _, ok := allowedPluginCapabilities[c]; !ok {
			if logger != nil {
				logger.Warn("plugin requested unknown capability — ignored",
					"plugin", pluginName, "capability", c)
			}
			continue
		}
		out = append(out, c)
	}
	return out
}

// GetPluginManifestsJSON 返回所有正在运行插件的前端清单(menu_items + routes),
// 用于 FrontendServer 注入 window.__PLUGIN_MANIFESTS__。
//
// 仅 StateRunning 且声明了 frontend manifest 的插件会被包含;
// 失败/未启动/无前端的插件直接跳过,避免前端拿到半成品配置。
//
// 输出始终是合法 JSON: 无可用插件时返回 "[]" 而非 nil,
// 让 FrontendServer 的注入逻辑无需特判长度。
func (m *PluginManager) GetPluginManifestsJSON() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	manifests := make([]map[string]any, 0, len(m.plugins))
	for _, inst := range m.plugins {
		entry := buildManifestEntry(inst)
		if entry == nil {
			continue
		}
		manifests = append(manifests, entry)
	}
	if len(manifests) == 0 {
		return []byte("[]")
	}
	data, err := json.Marshal(manifests)
	if err != nil {
		// 这里 manifests 全部是基本类型,理论上不会序列化失败;
		// 退回空数组而非 panic,避免影响前端首屏。
		m.logger.Error("marshal plugin manifests failed", "error", err)
		return []byte("[]")
	}
	return data
}

// buildManifestEntry 在持有 m.mu 读锁的前提下构造单个插件的 manifest 项。
// 实例未运行或没有 manifest 时返回 nil。
func buildManifestEntry(inst *PluginInstance) map[string]any {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.State != StateRunning || inst.Manifest == nil {
		return nil
	}
	frontend := inst.Manifest.GetFrontend()
	entry := map[string]any{
		"name":         inst.Manifest.GetName(),
		"display_name": inst.Manifest.GetDisplayName(),
		"version":      inst.Manifest.GetVersion(),
		"description":  inst.Manifest.GetDescription(),
		"menu_items":   convertMenuItems(frontend.GetMenuItems()),
		"routes":       convertRoutes(frontend.GetRoutes()),
	}
	if entryJS := frontend.GetEntryJs(); entryJS != "" {
		entry["entry_js"] = entryJS
		// entry_js_url 是前端实际访问的 HTTP 路径; 走核心代理 -> gRPC GetFrontendBundle.
		// 路径里的 plugin name 使用 manifest.GetName(), 与 PluginInstance.Name 等价.
		entry["entry_js_url"] = "/api/v1/plugin-assets/" + inst.Manifest.GetName() + "/" + entryJS
	}
	if entryCSS := frontend.GetEntryCss(); entryCSS != "" {
		entry["entry_css"] = entryCSS
		entry["entry_css_url"] = "/api/v1/plugin-assets/" + inst.Manifest.GetName() + "/" + entryCSS
	}
	return entry
}

// convertMenuItems 把 proto 菜单项递归转成可被前端直接消费的 map 列表。
// 保持空列表语义: 输入为空时返回空切片,前端可统一按数组处理。
//
// `icon_svg` 和 `labels` 是 SDK 新引入的字段(参见 plugin.proto),让插件
// 直接交付完整 SVG 与按 locale 分类的翻译文本,核心不再需要维护"图标名 ->
// SVG"或"label_key -> 翻译"的映射表。旧的 `icon` / `label_key` 仍透传作为
// 前端的 fallback,保留对早期插件的兼容性。
func convertMenuItems(items []*pluginsdk.MenuItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, mi := range items {
		if mi == nil {
			continue
		}
		entry := map[string]any{
			"path":                mi.GetPath(),
			"label_key":           mi.GetLabelKey(),
			"icon":                mi.GetIcon(),
			"icon_svg":            mi.GetIconSvg(),
			"section":             mi.GetSection(),
			"sort_order":          mi.GetSortOrder(),
			"requires_admin":      mi.GetRequiresAdmin(),
			"hide_in_simple_mode": mi.GetHideInSimpleMode(),
			"feature_flag":        mi.GetFeatureFlag(),
			// labels: copy into a fresh map so JSON serialization always emits
			// {} rather than null when the plugin supplied an empty map. nil
			// is fine here because encoding/json renders it as null and the
			// frontend treats null + empty as equivalent.
			"labels": mi.GetLabels(),
		}
		if children := convertMenuItems(mi.GetChildren()); len(children) > 0 {
			entry["children"] = children
		}
		out = append(out, entry)
	}
	return out
}

// convertRoutes 把 proto 路由定义转成前端可消费的 map 列表。
func convertRoutes(routes []*pluginsdk.RouteDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(routes))
	for _, r := range routes {
		if r == nil {
			continue
		}
		out = append(out, map[string]any{
			"path":           r.GetPath(),
			"name":           r.GetName(),
			"component_path": r.GetComponentPath(),
			"meta":           r.GetMeta(),
		})
	}
	return out
}

// =============================================================
// 内部实现
// =============================================================

func (m *PluginManager) getOrCreateInstance(name, binPath string) *PluginInstance {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inst, ok := m.plugins[name]; ok {
		return inst
	}
	inst := NewPluginInstance(name, binPath, NewRestartPolicy(m.cfg.Restart))
	m.plugins[name] = inst
	return inst
}

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

// handshakeMessage 是插件子进程通过 stdout 输出的握手 JSON。
type handshakeMessage struct {
	GRPCAddr string `json:"grpc_addr"`
	HTTPAddr string `json:"http_addr"`
}

// spawnAndConnect 启动子进程,读取握手,连接 gRPC,跑迁移,注册路由,启动健康监控。
// 任一步失败会清理已创建资源并返回错误。
func (m *PluginManager) spawnAndConnect(parentCtx context.Context, inst *PluginInstance, pluginConfig map[string]string) error {
	procCtx, cancelProc := context.WithCancel(context.Background())

	cmd := exec.CommandContext(procCtx, inst.BinaryPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancelProc()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancelProc()
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancelProc()
		return fmt.Errorf("start plugin process: %w", err)
	}

	// 后台转发 stderr 到日志,便于排查启动问题。
	go forwardStderr(stderr, inst.Name)

	hs, err := readHandshake(stdout, m.cfg.HandshakeTimeout)
	if err != nil {
		cancelProc()
		_ = cmd.Wait()
		return fmt.Errorf("read handshake: %w", err)
	}

	// Reject plugin gRPC/HTTP addresses that are not on the loopback interface.
	// SDK default is "127.0.0.1:0" so this is a no-op for any plugin built with
	// pluginsdk.Run; the check fires only when a third-party/replaced binary
	// hand-rolled handshake tries to expose the plugin channel cross-tenant.
	if !m.cfg.AllowNonLoopbackPluginAddr {
		if err := validateLoopbackAddr(hs.GRPCAddr, false); err != nil {
			cancelProc()
			_ = cmd.Wait()
			slog.Error("plugin grpc_addr rejected: not loopback",
				"plugin", inst.Name, "grpc_addr", hs.GRPCAddr, "error", err)
			return fmt.Errorf("plugin grpc_addr not loopback: %w", err)
		}
		if err := validateLoopbackAddr(hs.HTTPAddr, true); err != nil {
			cancelProc()
			_ = cmd.Wait()
			slog.Error("plugin http_addr rejected: not loopback",
				"plugin", inst.Name, "http_addr", hs.HTTPAddr, "error", err)
			return fmt.Errorf("plugin http_addr not loopback: %w", err)
		}
	}

	conn, lifecycle, err := dialPlugin(parentCtx, hs.GRPCAddr, m.cfg.GRPCDialTimeout)
	if err != nil {
		cancelProc()
		_ = cmd.Wait()
		return err
	}

	// 先拉取 manifest（GetManifest 不依赖 Init 注入的 PluginContext，可以
	// 独立调用），这样我们能在 Init 时把核心批准的 capabilities 一并下发，
	// 让 SDK 在构建 RedisClient 时就知道是否允许 Raw key 访问。
	manifestCtx, cancelManifest := context.WithTimeout(parentCtx, m.cfg.ManifestTimeout)
	manifest, err := lifecycle.GetManifest(manifestCtx, &emptypb.Empty{})
	cancelManifest()
	if err != nil {
		_ = conn.Close()
		cancelProc()
		_ = cmd.Wait()
		return fmt.Errorf("get manifest: %w", err)
	}

	// 把 manifest 声明的 capabilities 与核心 allow-list 取交集,作为最终授权。
	approvedCaps := approveCapabilities(inst.Name, manifest.GetCapabilities(), m.logger)

	// 在 SDKServer 上注册插件 → 授权 capabilities 的映射,后续 Redis Do 会查询。
	m.sdkServer.RegisterPlugin(inst.Name, approvedCaps)

	// V5/W3 SettingsExtension: persist the plugin's schema + defaults so
	// the admin UI can render the form and so reads have something to
	// fall back to. The registrar handles the missing-service case
	// internally; we only skip when the plugin did not ship a schema.
	if m.settingsService != nil && len(manifest.GetSettingsSchemaJson()) > 0 {
		regCtx, cancelReg := context.WithTimeout(parentCtx, m.cfg.ManifestTimeout)
		// V5/W6 SETTINGS-V2 (DESIGN §4.1): pass the full manifest envelope
		// — schema_version + properties_meta_json — so the service can
		// (1) detect schema_version bumps and drop existing watchers,
		// (2) prefer the SDK-authoritative properties_meta over re-deriving
		// it from x-* vendor extensions inside the schema bytes.
		err := m.settingsService.RegisterSchemaWithInput(regCtx, service.RegisterSchemaInput{
			PluginName:         inst.Name,
			SchemaJSON:         manifest.GetSettingsSchemaJson(),
			DefaultsJSON:       manifest.GetSettingsDefaultsJson(),
			SchemaVersion:      manifest.GetSettingsSchemaVersion(),
			PropertiesMetaJSON: manifest.GetSettingsPropertiesMetaJson(),
		})
		cancelReg()
		if err != nil {
			m.sdkServer.UnregisterPlugin(inst.Name)
			_ = conn.Close()
			cancelProc()
			_ = cmd.Wait()
			return fmt.Errorf("plugin settings register schema: %w", err)
		}

		// V5/W6 SETTINGS-V2 (DESIGN §4.4): subscribe to value-change events
		// so RequiresReload-flagged updates can trigger a coalesced reload.
		// Empty key = whole namespace. The unsubscribe is captured on the
		// instance and invoked from stopInstance — see DESIGN §4.4 cleanup.
		ch, unsubscribe := m.settingsService.Subscribe(inst.Name, "")
		inst.mu.Lock()
		inst.settingsUnsubscribe = unsubscribe
		inst.mu.Unlock()
		go m.handlePluginSettingsEvents(inst.Name, ch)
	}

	// 调用 Init 把 SDK 地址、plugin_name 与已批准的 capabilities 传给子进程。
	// V5/W6 SETTINGS-V2: PluginInitRequest.config (proto field 2) is
	// reserved — pluginConfig stays host-side only (e.g. skip_migration);
	// it is no longer wired into the plugin process. See
	// SETTINGS-V2-DESIGN §3 (decision 1).
	initCtx, cancelInit := context.WithTimeout(parentCtx, m.cfg.ManifestTimeout)
	initResp, initErr := lifecycle.Init(initCtx, &pluginsdk.PluginInitRequest{
		SdkAddress:       m.sdkAddr,
		PluginName:       inst.Name,
		Capabilities:     approvedCaps,
		OutboundDefaults: m.buildOutboundDefaults(),
	})
	cancelInit()
	if initErr != nil {
		m.sdkServer.UnregisterPlugin(inst.Name)
		_ = conn.Close()
		cancelProc()
		_ = cmd.Wait()
		return fmt.Errorf("plugin init rpc: %w", initErr)
	}
	if initResp != nil && !initResp.Success {
		m.sdkServer.UnregisterPlugin(inst.Name)
		_ = conn.Close()
		cancelProc()
		_ = cmd.Wait()
		return fmt.Errorf("plugin init reported failure: %s", initResp.Error)
	}

	// 跑插件迁移 (V5/W1)。manifest.migrations 是新版插件声明的 SQL 文件列表,
	// 每条带有 sha256 pin。host 通过 PluginLifecycle.GetMigration 拉取 SQL body,
	// 重新校验 checksum 后调用 RunPluginMigrations(advisory lock + 记录 plugin_migrations)。
	//
	// 失败语义 (V5-CURATE Q3): 默认阻塞插件启动 — 返回 error 让 EnablePlugin 失败,
	// 上层会清理 instance 状态。escape hatch: PluginRecord.Config["skip_migration"]="true"
	// 时跳过执行,用于线上紧急绕过有问题的迁移声明。
	//
	// 旧插件只填了 deprecated 的 migration_files 字段时,manifest.GetMigrations() 为空,
	// fetchAndRunPluginMigrations 会直接返回 nil;这里保留旧的日志行为方便观察。
	if shouldSkipPluginMigrations(pluginConfig) {
		m.logger.Warn("plugin migrations skipped via skip_migration config",
			"plugin", inst.Name,
			"declared_count", len(manifest.GetMigrations()),
		)
	} else if len(manifest.GetMigrations()) > 0 {
		if err := fetchAndRunPluginMigrations(parentCtx, m.db, lifecycle, manifest, inst.Name, m.logger); err != nil {
			m.sdkServer.UnregisterPlugin(inst.Name)
			_ = conn.Close()
			cancelProc()
			_ = cmd.Wait()
			return fmt.Errorf("plugin migrations: %w", err)
		}
	} else if len(manifest.GetMigrationFiles()) > 0 {
		// 兼容旧插件二进制:没有 sha256 pin,host 无法拉取 SQL,只记录日志。
		m.logger.Info("plugin declared legacy migration_files (no sha256 pin, skipped)",
			"plugin", inst.Name,
			"count", len(manifest.GetMigrationFiles()),
		)
	}

	entries := buildRouteEntries(inst.Name, manifest, hs.HTTPAddr)
	newTable := m.router.CurrentTable().AddPlugin(inst.Name, entries)
	m.router.SwapRouteTable(newTable)

	// 健康监控独立 ctx,不受 parentCtx 影响。
	healthCtx, healthCancel := context.WithCancel(context.Background())
	monitor := NewHealthMonitor(m.cfg.HealthInterval, m.cfg.HealthFailThreshold)

	inst.mu.Lock()
	inst.Cmd = cmd
	inst.CancelFunc = cancelProc
	inst.GRPCAddr = hs.GRPCAddr
	inst.HTTPAddr = hs.HTTPAddr
	inst.GRPCConn = conn
	inst.LifecycleStub = lifecycle
	inst.Manifest = manifest
	inst.StartedAt = time.Now()
	inst.HealthCancel = healthCancel
	// Re-arm the exited channel for this spawn — Exited is one-shot per process,
	// stopInstance and waitProcessExit synchronize on it. A previous spawn's
	// already-closed chan would let stopInstance return immediately before the
	// new process actually exits.
	inst.Exited = make(chan struct{})
	if err := inst.transitionTo(StateRunning); err != nil {
		inst.mu.Unlock()
		// 不太可能发生,但保留 defensive 处理。
		healthCancel()
		_ = conn.Close()
		cancelProc()
		_ = cmd.Wait()
		return err
	}
	inst.mu.Unlock()

	go monitor.Monitor(healthCtx, inst, func() {
		m.handleUnhealthy(inst)
	})

	go m.waitProcessExit(inst)

	return nil
}

// stopInstance 优雅停止某个插件:Shutdown RPC -> 等待退出 -> SIGKILL -> 清理。
// markRegistered 决定是否把状态置为 StateRegistered(true)还是保持当前状态(false,通常是重启场景)。
func (m *PluginManager) stopInstance(ctx context.Context, name string, markRegistered bool) error {
	m.mu.RLock()
	inst, ok := m.plugins[name]
	m.mu.RUnlock()
	if !ok {
		return nil
	}

	inst.mu.Lock()
	state := inst.State
	cmd := inst.Cmd
	stub := inst.LifecycleStub
	cancelProc := inst.CancelFunc
	exited := inst.Exited
	// 把 state 提前置为 Registered, 防止 waitProcessExit 把这次主动 stop
	// 的 exit 当成 unexpected 后写 LastError + 触发 scheduleRestart.
	if markRegistered {
		inst.State = StateRegistered
	}
	inst.CleanupHealth()
	inst.mu.Unlock()

	if state == StateRegistered {
		return nil
	}

	// 优雅 Shutdown RPC。
	if stub != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, m.cfg.ShutdownTimeout)
		_, err := stub.Shutdown(shutdownCtx, &emptypb.Empty{})
		cancel()
		if err != nil {
			m.logger.Warn("plugin shutdown rpc failed",
				"plugin", name, "error", err)
		}
	}

	// 等待进程退出 — 通过 inst.Exited (waitProcessExit 完成时 close), 不再
	// 自己调 cmd.Wait. 超时则 cancel context 触发 kill, 然后再等 Exited.
	if cmd != nil && exited != nil {
		select {
		case <-exited:
		case <-time.After(m.cfg.ProcessExitTimeout):
			if cancelProc != nil {
				cancelProc()
			}
			<-exited
		}
	}

	// 移除路由,关闭连接,撤销 SDK 端的 capability 授权。
	newTable := m.router.CurrentTable().RemovePlugin(name)
	m.router.SwapRouteTable(newTable)
	m.sdkServer.UnregisterPlugin(name)

	// V5/W6 SETTINGS-V2: release the per-plugin settings subscription
	// before unregistering the schema so the handler goroutine drains
	// cleanly. The subscription is recreated by the next spawnAndConnect.
	inst.mu.Lock()
	settingsUnsub := inst.settingsUnsubscribe
	inst.settingsUnsubscribe = nil
	inst.mu.Unlock()
	if settingsUnsub != nil {
		settingsUnsub()
	}

	if m.settingsService != nil {
		m.settingsService.UnregisterSchema(name)
	}

	inst.mu.Lock()
	inst.CloseGRPC()
	inst.Cmd = nil
	inst.CancelFunc = nil
	inst.GRPCAddr = ""
	inst.HTTPAddr = ""
	inst.Exited = nil
	// State 已经在函数开头根据 markRegistered 置为 Registered, 这里不再重复.
	inst.mu.Unlock()
	return nil
}

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

// waitProcessExit 等待插件进程退出,触发自动重启决策。
//
// 这是 cmd.Wait() 的*唯一*调用点 — Go 的 exec.Cmd.Wait() 不可重入, 第二次
// 调用 (在 OS 已经 reap child 之后) 会返回 "waitid: no child processes",
// 那个错误一旦冒泡到 LastError 就会在 admin UI 上显示"插件禁用报错". 所以
// stopInstance 改为 select <-inst.Exited 等待进程退出, 而不是再调 Wait().
func (m *PluginManager) waitProcessExit(inst *PluginInstance) {
	inst.mu.Lock()
	cmd := inst.Cmd
	exited := inst.Exited
	inst.mu.Unlock()
	if cmd == nil {
		if exited != nil {
			close(exited)
		}
		return
	}
	err := cmd.Wait()

	inst.mu.Lock()
	currentState := inst.State
	// 只在非主动 stop 路径下记录错误. 主动 stop (StateRegistered/StateStarting)
	// 时 cmd.Wait 即使返回非 nil 也是预期路径 (e.g. cancelProc kill), 不应
	// 污染 LastError.
	if err != nil && currentState != StateRegistered && currentState != StateStarting {
		inst.LastError = fmt.Errorf("process exited: %w", err)
	}
	inst.mu.Unlock()

	if exited != nil {
		close(exited)
	}

	if currentState == StateRegistered || currentState == StateStarting {
		// 主动 stop 触发的退出,无需重启。
		return
	}

	m.logger.Warn("plugin process exited unexpectedly",
		"plugin", inst.Name,
		"error", err,
	)
	m.scheduleRestart(inst)
}

// handleUnhealthy 由 HealthMonitor 在连续失败时回调。
func (m *PluginManager) handleUnhealthy(inst *PluginInstance) {
	m.logger.Warn("plugin unhealthy, scheduling restart",
		"plugin", inst.Name,
	)
	// 健康检查失败时,先杀进程让 waitProcessExit 触发重启路径。
	inst.mu.Lock()
	cancel := inst.CancelFunc
	inst.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// scheduleRestart 根据 RestartPolicy 决定是否重启,以及重启前的等待时长。
func (m *PluginManager) scheduleRestart(inst *PluginInstance) {
	inst.mu.Lock()
	if inst.restartPolicy == nil {
		inst.mu.Unlock()
		return
	}
	policy := inst.restartPolicy
	if !inst.StartedAt.IsZero() && policy.ShouldReset(time.Since(inst.StartedAt)) {
		inst.RestartCount = 0
	}
	delay, ok := policy.NextDelay(inst.RestartCount)
	if !ok {
		_ = inst.transitionTo(StateErrored)
		inst.mu.Unlock()
		m.logger.Error("plugin reached max restart attempts, giving up",
			"plugin", inst.Name,
			"max_retries", policy.MaxRetries(),
		)
		return
	}
	inst.RestartCount++
	_ = inst.transitionTo(StateRestarting)
	name := inst.Name
	inst.mu.Unlock()

	m.logger.Info("scheduling plugin restart",
		"plugin", name,
		"delay", delay.String(),
	)

	go func() {
		time.Sleep(delay)
		// 用后台 ctx 启动,不与外部 enable 操作冲突。
		ctx, cancel := context.WithTimeout(context.Background(), m.cfg.HandshakeTimeout+m.cfg.GRPCDialTimeout+m.cfg.ManifestTimeout+10*time.Second)
		defer cancel()

		// 直接 spawnAndConnect,跳过 EnablePlugin 中的 DB 写入(避免覆盖手动 disable)。
		inst.mu.Lock()
		if inst.State != StateRestarting {
			inst.mu.Unlock()
			return
		}
		if err := inst.transitionTo(StateStarting); err != nil {
			inst.mu.Unlock()
			return
		}
		inst.mu.Unlock()

		rec, err := m.repo.Get(ctx, name)
		if err != nil {
			m.logger.Error("restart: load plugin record failed",
				"plugin", name, "error", err)
			inst.mu.Lock()
			_ = inst.transitionTo(StateErrored)
			inst.LastError = err
			inst.mu.Unlock()
			return
		}
		if !rec.Enabled {
			// DB 里已经被禁用,放弃重启。
			inst.mu.Lock()
			_ = inst.transitionTo(StateRegistered)
			inst.mu.Unlock()
			return
		}
		if err := m.spawnAndConnect(ctx, inst, copyConfig(rec.Config)); err != nil {
			inst.mu.Lock()
			_ = inst.transitionTo(StateErrored)
			inst.LastError = err
			inst.mu.Unlock()
			m.logger.Error("plugin restart failed",
				"plugin", name, "error", err)
			// 继续按策略再次重启。
			m.scheduleRestart(inst)
		}
	}()
}

// =============================================================
// 工具函数
// =============================================================

func copyConfig(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func forwardStderr(r io.Reader, pluginName string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4*1024), 1024*1024)
	for scanner.Scan() {
		slog.Warn("plugin stderr",
			"plugin", pluginName,
			"line", scanner.Text(),
		)
	}
}

// readHandshake 从插件 stdout 第一行读取握手 JSON,带超时。
func readHandshake(r io.Reader, timeout time.Duration) (handshakeMessage, error) {
	type result struct {
		hs  handshakeMessage
		err error
	}
	ch := make(chan result, 1)
	go func() {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 4*1024), 64*1024)
		if !scanner.Scan() {
			err := scanner.Err()
			if err == nil {
				err = errors.New("plugin closed stdout before handshake")
			}
			ch <- result{err: err}
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			ch <- result{err: errors.New("empty handshake line")}
			return
		}
		var hs handshakeMessage
		if err := json.Unmarshal([]byte(line), &hs); err != nil {
			ch <- result{err: fmt.Errorf("decode handshake: %w (raw=%q)", err, line)}
			return
		}
		if hs.GRPCAddr == "" {
			ch <- result{err: errors.New("handshake missing grpc_addr")}
			return
		}
		ch <- result{hs: hs}
	}()

	select {
	case r := <-ch:
		return r.hs, r.err
	case <-time.After(timeout):
		return handshakeMessage{}, fmt.Errorf("handshake timeout after %s", timeout)
	}
}

// dialPlugin 拨号到插件 gRPC server,失败时关闭未完成的资源。
//
// grpc v1.79+ 已弃用 grpc.DialContext + grpc.WithBlock。这里改用
// grpc.NewClient(lazy) + 显式 Connect() + WaitForStateChange 轮询,
// 保持原有"在 timeout 内等待连接 Ready,失败则返回错误"的同步语义。
// 紧随其后的 GetManifest/Init RPC 才不会因连接未就绪而立即拿到 Unavailable。
func dialPlugin(ctx context.Context, addr string, timeout time.Duration) (*grpc.ClientConn, pluginsdk.PluginLifecycleClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("dial plugin grpc %s: %w", addr, err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// NewClient 是 lazy 的,显式触发连接。
	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return conn, pluginsdk.NewPluginLifecycleClient(conn), nil
		}
		if state == connectivity.Shutdown {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("dial plugin grpc %s: connection shutdown", addr)
		}
		if !conn.WaitForStateChange(dialCtx, state) {
			// dialCtx 超时或被取消。
			_ = conn.Close()
			return nil, nil, fmt.Errorf("dial plugin grpc %s: %w", addr, dialCtx.Err())
		}
	}
}

// buildRouteEntries 把 manifest 中声明的 endpoint 转换成 RouteEntry。
// gateway_endpoints 与 plugin_endpoints 的差异通过 IsGateway 字段标记。
func buildRouteEntries(pluginName string, manifest *pluginsdk.ManifestResponse, httpAddr string) []RouteEntry {
	if manifest == nil || httpAddr == "" {
		return nil
	}
	proxyURL := normalizeProxyURL(httpAddr)

	entries := make([]RouteEntry, 0, len(manifest.GatewayEndpoints)+len(manifest.PluginEndpoints))
	entries = append(entries, expandEndpoints(pluginName, manifest.GatewayEndpoints, proxyURL, true)...)
	entries = append(entries, expandEndpoints(pluginName, manifest.PluginEndpoints, proxyURL, false)...)
	return entries
}

func expandEndpoints(pluginName string, decls []*pluginsdk.EndpointDeclaration, proxyURL string, isGateway bool) []RouteEntry {
	out := make([]RouteEntry, 0, len(decls))
	for _, ep := range decls {
		if ep == nil || ep.GetPath() == "" {
			continue
		}
		methods := ep.GetMethods()
		if len(methods) == 0 {
			methods = []string{"*"}
		}
		auth := ep.GetAuthType()
		if auth == "" {
			auth = AuthTypeNone
		}
		for _, mth := range methods {
			out = append(out, RouteEntry{
				Method:     strings.ToUpper(mth),
				PathPrefix: ep.GetPath(),
				PluginName: pluginName,
				AuthType:   auth,
				ProxyURL:   proxyURL,
				IsGateway:  isGateway,
			})
		}
	}
	return out
}

// normalizeProxyURL 把 host:port 形式补成 http://host:port。
func normalizeProxyURL(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}

// SetSettings wires the host-side plugin settings subsystem into the
// manager. Pass nil to disable; the SDK side returns a clear error
// rather than silently no-oping.
//
// The manager constructs the SettingsExtension gRPC server itself so it
// can pass the SDKServer's resolveCaller function value, keeping caller
// identity consistent with the rest of the SDK surface.
//
// Must be called before Start; calling it after has no effect on the
// already-running gRPC server's service list.
func (m *PluginManager) SetSettings(svc PluginSettingsRegistrar, builder SettingsExtensionBuilder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settingsService = svc
	if svc != nil && builder != nil {
		m.settingsServer = builder(m.sdkServer.ResolveCaller)
	} else {
		m.settingsServer = nil
	}
}

// SettingsExtensionBuilder is the callback the wire layer supplies; it
// receives the manager's resolveCaller function and returns a
// SettingsExtensionRegistrar ready to attach to the SDK gRPC server.
// We use a builder pattern instead of a direct dependency so the manager
// owns when the SDKServer's identity-resolution function is captured.
type SettingsExtensionBuilder func(resolver func(context.Context) string) SettingsExtensionRegistrar
