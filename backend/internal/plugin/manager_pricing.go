package plugin

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// =============================================================
// Pricing extension probe + start (spawn-side wiring, lifecycle-managed)
//
// 这些 helper 与 manager_lifecycle.go 中 detachExtensions 是一对：都围绕
// PricingExtensionClient 的"创建一次 / Stop 一次"的不变量。由于一并放在
// lifecycle 文件后导致超过 500 行（T20 拆分），抽到本文件单独维护。
//
// spawn 阶段在 manager_spawn.go::spawnTryPricing 入口调用
// tryStartPricingExtension；shutdown 路径在 lifecycle 的 detachExtensions
// 里通过 detachPricingClient 解绑。
// =============================================================

// pluginImplementsPricing reports whether the plugin at conn implements
// PricingExtension by issuing a single ListPricingOverrides RPC. The probe
// uses a 2-second timeout; codes.Unimplemented is the explicit "no" answer
// (proto-default UnimplementedPricingExtensionServer returns it). Any other
// error is treated as "yes, but transiently broken" - the retry loops are
// the right place to surface that, not the spawn path. The probe also
// catches plugins that registered the service but ship a placeholder Server
// implementation that itself returns Unimplemented.
//
// The plugin's grpc.Server only ever has the host as a client, so a single
// extra round trip during spawn is cheap relative to the value of avoiding
// noisy Watch/Resync logs for plugins without a pricing layer.
func (m *PluginManager) pluginImplementsPricing(ctx context.Context, pluginName string, conn *grpc.ClientConn) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	stub := pluginsdk.NewPricingExtensionClient(conn)
	_, err := stub.ListPricingOverrides(probeCtx, &pluginsdk.ListPricingOverridesRequest{})
	if err == nil {
		return true
	}
	// codes.Unimplemented means the plugin did not register the service.
	// Anything else (Unavailable from a half-ready server, DeadlineExceeded
	// from a slow first ListAll, ...) is treated as "available, retry later".
	if st, ok := status.FromError(err); ok && st.Code() == codes.Unimplemented {
		m.logger.Debug("plugin does not implement PricingExtension - skipping wire",
			"plugin", pluginName)
		return false
	}
	m.logger.Debug("PricingExtension probe returned non-Unimplemented error - wiring anyway",
		"plugin", pluginName, "error", err)
	return true
}

// tryStartPricingExtension attempts to bind a PricingExtensionClient to the
// just-spawned plugin and register it with BillingService. The function is
// a no-op when SetPricingExtension was not called or when the plugin does
// not implement PricingExtension (codes.Unimplemented). It never returns
// an error; failures only affect the pricing data layer, not the plugin's
// availability.
func (m *PluginManager) tryStartPricingExtension(ctx context.Context, inst *PluginInstance) {
	m.mu.RLock()
	cache := m.pricingCache
	registrar := m.pricingAdjusterRegistrar
	m.mu.RUnlock()

	// No host wiring -> nothing to do. The plugin's PricingExtension server
	// (if any) is reachable from elsewhere only via this client today, so
	// the early exit is the right behaviour.
	if cache == nil && registrar == nil {
		return
	}

	inst.mu.Lock()
	conn := inst.GRPCConn
	inst.mu.Unlock()
	if conn == nil {
		return
	}

	// Probe first so we don't start watch/resync loops against a plugin
	// that does not implement PricingExtension (codes.Unimplemented). The
	// probe is a single ListPricingOverrides call with a tight timeout;
	// any other error (network, transient) lets us proceed and treat the
	// pricing extension as available - the client's normal retry loops
	// take over from there.
	if !m.pluginImplementsPricing(ctx, inst.Name, conn) {
		return
	}

	// parentCtx = Background: the watch/resync loops live for as long as the
	// plugin instance does, decoupled from the spawn / admin-request ctx
	// that triggered tryStartPricingExtension. Stop() handles teardown.
	client := NewPricingExtensionClient(context.Background(), inst.Name, cache, m.logger)
	if err := client.Start(ctx, conn); err != nil {
		// Probe failure is benign - Start returns nil when the bootstrap
		// List was skipped or returned codes.Unimplemented; the only path
		// that surfaces an error here is "nil receiver / nil conn", both
		// guarded above. Keep the log entry so misconfigurations are
		// visible.
		m.logger.Debug("pricing extension client start failed",
			"plugin", inst.Name, "error", err)
		return
	}

	inst.mu.Lock()
	inst.pricingClient = client
	inst.mu.Unlock()

	// Wire BillingService -> adjuster only when the registrar is configured.
	// The cache is always populated regardless so that read-side consumers
	// (channel_cache_reader fallback) see the same data.
	if registrar != nil {
		registrar.SetPricingAdjuster(client)
	}

	// AccountStats hook: same client (via its ResolveAccountStatsCost
	// method) feeds the GatewayService / OpenAIGatewayService
	// account-stats override path. nil registrar disables the hook.
	m.mu.RLock()
	statsRegistrar := m.accountStatsResolverRegistrar
	m.mu.RUnlock()
	if statsRegistrar != nil {
		statsRegistrar.SetAccountStatsResolver(client)
	}
}

// detachPricingClient 把 BillingService → PricingAdjuster 解绑并 Stop client。
// pricingClient 为 nil 时 no-op。
//
// 该 helper 在 lifecycle 的 detachExtensions 阶段被调用：必须先把 BillingService
// / GatewayService 上的 adjuster / resolver 字段置 nil，再 Stop() —— 避免
// in-flight CalculateCostUnified / ResolveAccountStatsCost 与即将退出的 watch
// loop 竞争。
func (m *PluginManager) detachPricingClient(pricingClient *PricingExtensionClient) {
	if pricingClient == nil {
		return
	}
	m.mu.RLock()
	registrar := m.pricingAdjusterRegistrar
	statsRegistrar := m.accountStatsResolverRegistrar
	m.mu.RUnlock()
	if registrar != nil {
		registrar.SetPricingAdjuster(nil)
	}
	if statsRegistrar != nil {
		statsRegistrar.SetAccountStatsResolver(nil)
	}
	pricingClient.Stop()
}
