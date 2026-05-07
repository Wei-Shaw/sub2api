package main

import (
	"log/slog"
	"sync/atomic"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const pluginVersion = "0.1.0"

// AnthropicGatewayPlugin is the top-level plugin struct implementing
// pluginsdk.Plugin and optional extension interfaces.
type AnthropicGatewayPlugin struct {
	ctx atomic.Pointer[pluginsdk.PluginContext]

	// accountPlatformServer handles account-level operations (validate,
	// test, refresh). Constructed in RegisterGRPCServices; data sources
	// wired in Init.
	accountPlatformServer *accountPlatformServer

	// gatewayProviderServer handles upstream API forwarding for the
	// Anthropic Messages API. Constructed in RegisterGRPCServices.
	gatewayProviderServer *gatewayProviderServer
}

// Manifest returns the static plugin descriptor.
func (p *AnthropicGatewayPlugin) Manifest() *pluginsdk.Manifest {
	return buildManifest()
}

// Init stores the SDK-supplied context for handlers to use.
func (p *AnthropicGatewayPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)
	ctx.Logger().Info("gateway-anthropic plugin initialised", "version", pluginVersion)
	return nil
}

// Shutdown releases resources.
func (p *AnthropicGatewayPlugin) Shutdown() error {
	if c := p.context(); c != nil {
		c.Logger().Info("gateway-anthropic plugin shutting down")
	}
	return nil
}

// RegisterGRPCServices implements pluginsdk.GRPCServiceRegistrar. It registers
// the AccountPlatformExtension and GatewayProviderExtension gRPC services so
// the host can delegate account operations and API forwarding to this plugin.
func (p *AnthropicGatewayPlugin) RegisterGRPCServices(server *grpc.Server) {
	p.accountPlatformServer = newAccountPlatformServer()
	pb.RegisterAccountPlatformExtensionServer(server, p.accountPlatformServer)

	p.gatewayProviderServer = newGatewayProviderServer()
	pb.RegisterGatewayProviderExtensionServer(server, p.gatewayProviderServer)
}

// context returns the live PluginContext or nil if Init has not run yet.
func (p *AnthropicGatewayPlugin) context() pluginsdk.PluginContext {
	c := p.ctx.Load()
	if c == nil {
		return nil
	}
	return *c
}

// logger returns the plugin logger or a no-op fallback.
func (p *AnthropicGatewayPlugin) logger() *slog.Logger {
	if ctx := p.context(); ctx != nil {
		return ctx.Logger()
	}
	return slog.Default()
}

// compile-time interface assertions
var (
	_ pluginsdk.Plugin               = (*AnthropicGatewayPlugin)(nil)
	_ pluginsdk.GRPCServiceRegistrar = (*AnthropicGatewayPlugin)(nil)
)

