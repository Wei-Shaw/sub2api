package plugin

import (
	"context"

	"google.golang.org/grpc"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// =============================================================
// PluginManager setter / binder API
//
// 这一组方法只做"宿主把可选依赖注入 manager"的赋值动作。每个方法都是
// Lock + 写入字段, 内部不发起 RPC、不读取 m.plugins, 与 manager.go
// 的高层生命周期方法解耦。
// 所有 Set* 必须在 Start 之前调用, 否则 startSDKServer 已经把 service
// 列表冻结到 grpc.Server 上 — 后续覆盖只会改 manager 字段, 不会重新
// 注册 gRPC service。
// =============================================================

// SettingsExtensionRegistrar is the minimal surface PluginManager needs to
// attach the SettingsExtension gRPC service to the SDK's grpc.Server.
type SettingsExtensionRegistrar interface {
	Register(grpcServer *grpc.Server)
}

// SettingsExtensionBuilder is the callback the wire layer supplies; it
// receives the manager's resolveCaller function and returns a
// SettingsExtensionRegistrar ready to attach to the SDK gRPC server.
// We use a builder pattern instead of a direct dependency so the manager
// owns when the SDKServer's identity-resolution function is captured.
type SettingsExtensionBuilder func(resolver func(context.Context) string) SettingsExtensionRegistrar

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

// PricingAdjusterRegistrar is the minimal surface PluginManager needs to
// install a per-plugin PricingAdjuster on the host's BillingService.
//
// Implementations must be safe for concurrent calls; PluginManager invokes
// Set during plugin lifecycle transitions which are serialised by the
// manager but the BillingService itself reads the adjuster on every cost
// calculation.
type PricingAdjusterRegistrar interface {
	SetPricingAdjuster(adjuster service.PricingAdjuster)
}

// AccountStatsResolverRegistrar is the minimal surface PluginManager
// needs to install a per-plugin AccountStatsCostResolver on the host's
// gateway services. The host implementation typically wraps both
// GatewayService.SetAccountStatsResolver and
// OpenAIGatewayService.SetAccountStatsResolver behind a single Set call.
type AccountStatsResolverRegistrar interface {
	SetAccountStatsResolver(resolver service.AccountStatsCostResolver)
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
		// resolver 仅为兼容已生成的 wire_gen.go 调用点; 接收方不会真正调用
		// 它(caller 解析已交给 RequirePluginIdentity 拦截器)。这里直接传 nil
		// 也安全, 但为了不破坏旧 builder 实现保留参数传递。
		m.settingsServer = builder(m.sdkServer.resolveCaller)
	} else {
		m.settingsServer = nil
	}
}

// SetEvents wires the host-side EventPublisher and registers the
// EventsExtension gRPC service so plugins can subscribe to typed host
// events. Pass nil to disable; the SDK side returns PERMISSION_DENIED
// for any Subscribe call when no publisher is configured.
//
// Plugin Hook Phase B — see plugin-sdk/proto/sdk.proto EventsExtension
// service block for the delivery contract. Must be called before Start;
// calling it after has no effect on the already-running gRPC server.
func (m *PluginManager) SetEvents(publisher *EventPublisher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventPublisher = publisher
	if publisher == nil {
		m.eventsAllowList = nil
		m.eventsServer = nil
		return
	}
	m.eventsAllowList = NewEventsAllowListRegistry()
	m.eventsServer = NewEventsExtensionServer(
		publisher,
		m.eventsAllowList,
		m.sdkServer,
		nil, // caller 解析已交给 RequirePluginIdentityStream 拦截器
	)
}

// EventPublisher returns the configured publisher (or nil if SetEvents
// was not called). Services that need to publish events take a
// non-nil pointer; the publisher itself tolerates a nil receiver.
func (m *PluginManager) EventPublisher() *EventPublisher {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.eventPublisher
}

// SetPricingExtension wires the host-side PricingExtension subsystem. The
// cache is populated by per-plugin PricingExtensionClients during
// List + Watch; the registrar is the BillingService seam that receives the
// adjuster on plugin start and a nil on plugin stop.
//
// Either argument may be nil:
//
//   - cache=nil disables the data layer (List + Watch results are
//     dropped). AdjustCost still works because it does not consult the
//     cache.
//   - registrar=nil disables the per-request AdjustCost hook. List + Watch
//     still populate the cache for read-side consumers (channel cache
//     reader fallback path).
//
// Must be called before Start; calling after has no effect on already-
// running plugins (their pricing client is bound at spawn time).
func (m *PluginManager) SetPricingExtension(cache *service.PricingOverrideCache, registrar PricingAdjusterRegistrar) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pricingCache = cache
	m.pricingAdjusterRegistrar = registrar
}

// SetAccountStatsResolverRegistrar wires the host-side seam used to
// install / detach the per-plugin AccountStatsCostResolver. Pass nil to
// disable the hook entirely (the host keeps usage_logs.account_stats_cost
// NULL and aggregation queries fall back to total_cost via COALESCE).
//
// Must be called before Start; calling after has no effect on already-
// running plugins.
func (m *PluginManager) SetAccountStatsResolverRegistrar(registrar AccountStatsResolverRegistrar) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accountStatsResolverRegistrar = registrar
}

// SetHostPricingResolver wires the HostService.ResolveModelPricing RPC
// to a concrete host-side pricing backend (typically *BillingService).
// Pass nil to leave HostService unregistered — plugins that call
// ctx.Host().ResolveModelPricing will then see codes.Unimplemented,
// which the SDK translates into ErrHostPricingUnavailable so the
// feature degrades to nil pricing.
//
// Must be called before Start; the gRPC service list is frozen onto
// the SDK grpc.Server in startSDKServer and later calls only mutate
// the manager field (which today has no runtime effect).
//
// Setters share a single mutating helper (rebuildHostServiceLocked) so
// every call refreshes one slot and preserves the others. Calling
// setters in arbitrary order produces a coherent server.
func (m *PluginManager) SetHostPricingResolver(resolver HostPricingResolver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostPricingResolver = resolver
	m.rebuildHostServiceLocked()
}

// SetHostBalanceService wires HostService.CreditBalance / DeductBalance.
// Pass nil to disable those RPCs (they return codes.Unimplemented).
func (m *PluginManager) SetHostBalanceService(svc HostBalanceService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostBalance = svc
	m.rebuildHostServiceLocked()
}

// SetHostSubscriptionAssigner wires HostService.AssignSubscription /
// RevokeSubscriptionDays.
func (m *PluginManager) SetHostSubscriptionAssigner(svc HostSubscriptionAssigner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostSubscription = svc
	m.rebuildHostServiceLocked()
}

// SetHostAffiliateAccruer wires HostService.AccrueRebate.
func (m *PluginManager) SetHostAffiliateAccruer(svc HostAffiliateAccruer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostAffiliate = svc
	m.rebuildHostServiceLocked()
}

// SetHostUserLookup wires HostService.GetUserByID.
func (m *PluginManager) SetHostUserLookup(svc HostUserLookup) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostUserLookup = svc
	m.rebuildHostServiceLocked()
}

// rebuildHostServiceLocked refreshes m.hostService from the cached
// dependency fields. The caller MUST hold m.mu (write lock). When every
// dep is nil the service is dropped — semantically identical to never
// calling any setter.
func (m *PluginManager) rebuildHostServiceLocked() {
	deps := HostServiceDeps{
		Pricing:      m.hostPricingResolver,
		Balance:      m.hostBalance,
		Subscription: m.hostSubscription,
		Affiliate:    m.hostAffiliate,
		UserLookup:   m.hostUserLookup,
	}
	if deps.Pricing == nil && deps.Balance == nil && deps.Subscription == nil &&
		deps.Affiliate == nil && deps.UserLookup == nil {
		m.hostService = nil
		return
	}
	m.hostService = NewHostServiceServerWithDeps(deps)
}

// SDKAddr 返回 SDK gRPC 实际监听地址,主要用于测试与诊断。
func (m *PluginManager) SDKAddr() string {
	return m.sdkAddr
}
