package plugin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	"github.com/Wei-Shaw/sub2api/internal/gateway"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// =============================================================
// PluginManager 主文件
//
// 拆分约定 (T7 + T27 + T36):
//   - manager.go (本文件)        : PluginManager 类型 + 字段 + 构造 +
//                                  shutdownCtx 维护 + m.plugins map 封装
//                                  (deleteInstance/lookupInstance/
//                                  snapshotPluginNames)
//   - manager_lifecycle_api.go   : 高层生命周期 API (Start/ShutdownAll/
//                                  EnablePlugin/DisablePlugin/RestartPlugin)
//   - manager_query.go           : 查询 API (ListPlugins/ListPluginsExt/
//                                  GetPlugin)
//   - manager_setters.go         : Set/Bind 钩子 (BindRouter/SetSettingsService
//                                  /SetEvents/SetPricingExtension/...)
//   - manager_spawn.go           : spawn 状态机 (stages + spawnCtx) +
//                                  handshake / dialPlugin + pricing extension
//                                  probe + start
//   - manager_lifecycle.go       : stopInstance 多 helper + waitProcessExit +
//                                  scheduleRestart + handlePluginSettingsEvents
//   - manager_capabilities.go    : capability allow-list / legacy rename / approve +
//                                  outbound defaults
//   - manager_frontend.go        : GetPluginManifestsJSON + buildManifestEntry +
//                                  convertMenuItems / convertRoutes
//   - manager_routes.go          : 路由命名空间常量 + validatePluginRoutePaths +
//                                  buildRouteEntries
//   - manager_install.go         : Install / Uninstall / Purge +
//                                  executePurgeTx + revertCachedDownMigrationsTx
//   - manager_discovery.go       : syncFromDisk (磁盘扫描) + binaryPathFor +
//                                  isBuiltinPlugin (二进制路径解析)
//   - manager_instance_map.go    : getOrCreateInstance (惰性创建 PluginInstance)
//   - manager_sdk.go             : startSDKServer + buildLeaderLockProvider +
//                                  registerExtensionServices
//   - manager_admin.go           : admin handler 适配 (List/Get/Enable/...) +
//                                  PluginInfo<->PluginRecord 互转 / 配置序列化 helpers
//
// 所有文件共享 receiver `*PluginManager`, 公共 API 签名保持不变。
// =============================================================

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

	// shutdownCtx 在 Start 创建, ShutdownAll 触发 cancel。
	// scheduleRestart 派生的 goroutine 监听该 ctx, host 关闭过程中不再启动新进程。
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	// shutdownOnce 保证 Start / ShutdownAll 重复调用时不重复创建/取消 ctx。
	shutdownOnce sync.Once

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
	// settingsReader is the read-only seam GetPublicFlags uses to resolve
	// PublicFlagSourceSettings declarations against a plugin's settings
	// store. Wired via SetPluginSettingsReader before Start; nil disables
	// settings-backed flag values (they fall back to the declared default).
	// The wire layer typically points this at the same
	// *service.PluginSettingsService instance used by SetSettings, so the
	// public-flags read path sees the same data as plugin SDK Get calls.
	settingsReader PluginSettingsReader
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

	// eventPublisher fans typed host events out to subscribed plugins via
	// the EventsExtension gRPC service. Optional; nil means "events
	// disabled" — Publish* helpers degrade to no-ops.
	eventPublisher *EventPublisher
	// eventsAllowList tracks each plugin manifest.SubscribedEvents so the
	// EventsExtensionServer can validate Subscribe requests against the
	// declared allow-list. Populated alongside RegisterPlugin.
	eventsAllowList *EventsAllowListRegistry
	// eventsServer is the gRPC service registered when eventPublisher is
	// non-nil. Built in SetEvents.
	eventsServer *EventsExtensionServer

	// pricingCache is the host-wide PricingOverrideCache the
	// PricingExtensionClient writes into. Set via SetPricingExtension before
	// Start. nil disables the data layer entirely (the registrar still gets
	// AdjustCost calls via the per-plugin client if SetPricingAdjuster is
	// available, but no overrides are buffered).
	pricingCache *service.PricingOverrideCache
	// pricingAdjusterRegistrar is the host-side seam that lets PluginManager
	// install / detach a per-plugin PricingAdjuster on BillingService as
	// plugins start and stop. The registrar is wired via SetPricingExtension;
	// nil disables the per-request AdjustCost hook entirely (BillingService
	// stays at its baseline behaviour).
	pricingAdjusterRegistrar PricingAdjusterRegistrar

	// accountStatsResolverRegistrar is the analogous seam for the
	// per-request account-stats cost resolver wired into GatewayService /
	// OpenAIGatewayService. nil disables the hook (host keeps
	// usage_logs.account_stats_cost NULL).
	accountStatsResolverRegistrar AccountStatsResolverRegistrar

	// modelSupportCheckerRegistrar wires the plugin-backed model support
	// checker into the gateway services. nil disables the hook.
	modelSupportCheckerRegistrar ModelSupportCheckerRegistrar

	// schedulingHintsRegistrar wires the plugin-backed scheduling hints
	// provider into the gateway services. nil disables the hook.
	schedulingHintsRegistrar SchedulingHintsRegistrar

	// schedulabilityCheckerRegistrar wires the plugin-backed schedulability
	// checker into the gateway services. nil disables the hook.
	schedulabilityCheckerRegistrar SchedulabilityCheckerRegistrar

	// hostService holds the HostService gRPC server (plugin→host reverse
	// RPCs, see ADR-UPSTREAM-SYNC-115-121 §4). Wired via
	// SetHostPricingResolver before Start; nil disables the service
	// (plugin SDK calls surface codes.Unimplemented → degrade to nil
	// pricing, which is the same behaviour as older hosts that never
	// shipped HostService).
	hostService *HostServiceServer

	// hostPricingResolver / hostBalance / hostSubscription /
	// hostAffiliate / hostUserLookup cache the dependency injected via
	// the SetHost* setters. rebuildHostServiceLocked re-derives
	// m.hostService from these fields whenever any setter is called,
	// preserving previously-set deps. All optional — nil leaves the
	// corresponding RPC returning codes.Unimplemented.
	hostPricingResolver HostPricingResolver
	hostBalance         HostBalanceService
	hostSubscription    HostSubscriptionAssigner
	hostAffiliate       HostAffiliateAccruer
	hostUserLookup      HostUserLookup

	// platformRegistry collects platform declarations from all loaded gateway
	// plugins and provides lookup APIs for the account handler.
	platformRegistry *PlatformRegistry

	// gatewayProviderRegistry is the host's ProviderRegistry used by the
	// gateway pipeline. When a plugin declares a platform, the manager creates
	// a PluginGatewayProvider and registers it here so the pipeline can
	// forward requests to the plugin. Set via SetGatewayProviderRegistry
	// before Start; nil disables gateway provider auto-registration.
	gatewayProviderRegistry GatewayProviderRegistry

	// pipeline is the gateway pipeline for content intercept hook wiring.
	pipeline *gateway.GatewayPipeline
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
		plugins:          make(map[string]*PluginInstance),
		repo:             repo,
		router:           router,
		sdkServer:        sdk,
		cfg:              cfg,
		db:               db,
		logger:           slog.Default().With("component", "plugin_manager"),
		instanceID:       uuid.NewString(),
		platformRegistry: NewPlatformRegistry(),
	}, nil
}

// ensureShutdownCtx 在第一次 Start 时构造 shutdownCtx。重复调用是 no-op,
// scheduleRestart 拿到的始终是同一个 ctx。
func (m *PluginManager) ensureShutdownCtx() {
	m.shutdownOnce.Do(func() {
		m.shutdownCtx, m.shutdownCancel = context.WithCancel(context.Background())
	})
}

// shutdownContext 返回一个永远可订阅的 ctx:Start 之前 / 测试中直接构造
// PluginManager 但未调用 Start 时, 退化成永不取消的 Background。
// 这避免了 scheduleRestart 调用方需要在每次操作前判 nil。
func (m *PluginManager) shutdownContext() context.Context {
	m.mu.RLock()
	ctx := m.shutdownCtx
	m.mu.RUnlock()
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

// =============================================================
// m.plugins map 封装
//
// 测试 (manager_purge_test.go / manager_uninstall_test.go) 直接读写
// m.plugins, 因此字段保持包级可见; 这些 helper 只是为生产代码提供
// 单一访问点, 让裸 m.mu.Lock+map operation 不再散落 12 处。
// 历史曾保留 putInstance / takeInstance 作 "未来 API 形态" 占位,
// 但 unused 警告 + YAGNI 原则下已删除; 真要 "停 + 释放为一步" 时
// 再补一个 6 行函数即可, 不用提前预留死代码。
// =============================================================

// deleteInstance 仅从 map 中删除 (不 stop)。Uninstall / Purge 已经先执行
// stopInstance 再调它, 因此不需要返回旧实例。
func (m *PluginManager) deleteInstance(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.plugins, name)
}

// lookupInstance 在 m.mu 的读锁下查找实例。
func (m *PluginManager) lookupInstance(name string) (*PluginInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.plugins[name]
	return inst, ok
}

// snapshotPluginNames 返回当前所有 plugin 的名字快照。返回新切片, 调用方
// 可以在不持锁的情况下迭代。
func (m *PluginManager) snapshotPluginNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.plugins))
	for name := range m.plugins {
		out = append(out, name)
	}
	return out
}
