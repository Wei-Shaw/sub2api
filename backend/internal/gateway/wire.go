package gateway

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ProvideProviderRegistry creates an empty ProviderRegistry. Platform
// providers are registered dynamically by plugins at spawn time via
// PluginRegistryAdapter.RegisterPlugin. If no plugin is enabled for a
// given platform, the Pipeline returns a clear "no provider for platform"
// error instead of panicking.
func ProvideProviderRegistry() *ProviderRegistry {
	return NewProviderRegistry()
}

// ProvideGatewayPipeline creates a GatewayPipeline with all required
// service dependencies and the provider registry.
func ProvideGatewayPipeline(
	registry *ProviderRegistry,
	gw *service.GatewayService,
	billing *service.BillingCacheService,
	conc *service.ConcurrencyService,
	settings *service.SettingService,
	cfg *config.Config,
) *GatewayPipeline {
	return NewGatewayPipeline(registry, gw, billing, conc, settings, cfg)
}

// ProvideOpsRetryForwarder creates an OpsRetryForwarderAdapter that
// implements service.OpsRetryForwarder via the ProviderRegistry.
func ProvideOpsRetryForwarder(registry *ProviderRegistry) service.OpsRetryForwarder {
	return NewOpsRetryForwarderAdapter(registry)
}
