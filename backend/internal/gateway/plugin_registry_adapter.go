package gateway

import (
	"log/slog"

	"google.golang.org/grpc"
)

// PluginRegistryAdapter adapts the ProviderRegistry for use by the plugin
// manager. It satisfies the plugin.GatewayProviderRegistry interface,
// creating PluginGatewayProviders and registering/unregistering them.
//
// The wire layer constructs this adapter and passes it to
// PluginManager.SetGatewayProviderRegistry, avoiding a circular import
// between the plugin and gateway packages.
type PluginRegistryAdapter struct {
	registry *ProviderRegistry
	logger   *slog.Logger
}

// NewPluginRegistryAdapter creates an adapter backed by the given registry.
func NewPluginRegistryAdapter(registry *ProviderRegistry, logger *slog.Logger) *PluginRegistryAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &PluginRegistryAdapter{
		registry: registry,
		logger:   logger,
	}
}

// RegisterPlugin creates a PluginGatewayProvider and registers it into
// the ProviderRegistry.
func (a *PluginRegistryAdapter) RegisterPlugin(
	platform string,
	protocols []string,
	pluginName string,
	conn *grpc.ClientConn,
) {
	provider := NewPluginGatewayProvider(platform, protocols, pluginName, conn, a.logger)
	a.registry.Register(provider)
}

// UnregisterPlugin removes the provider for the given platform from the
// ProviderRegistry.
func (a *PluginRegistryAdapter) UnregisterPlugin(platform string) {
	a.registry.Unregister(platform)
}
