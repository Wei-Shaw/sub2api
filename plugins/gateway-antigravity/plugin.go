package main

import (
	"log/slog"
	"sync/atomic"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const pluginVersion = "0.1.0"

// AntigravityGatewayPlugin is the top-level plugin struct implementing
// pluginsdk.Plugin and optional extension interfaces.
type AntigravityGatewayPlugin struct {
	ctx atomic.Pointer[pluginsdk.PluginContext]

	// accountPlatformServer handles account-level operations (validate,
	// test, refresh). Constructed in RegisterGRPCServices; data sources
	// wired in Init.
	accountPlatformServer *accountPlatformServer

	// gatewayProviderServer handles upstream API forwarding. Constructed
	// in RegisterGRPCServices.
	gatewayProviderServer *gatewayProviderServer
}

// Manifest returns the static plugin descriptor.
func (p *AntigravityGatewayPlugin) Manifest() *pluginsdk.Manifest {
	return buildManifest()
}

// Init stores the SDK-supplied context for handlers to use.
func (p *AntigravityGatewayPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)
	ctx.Logger().Info("gateway-antigravity plugin initialised", "version", pluginVersion)
	return nil
}

// Shutdown releases resources.
func (p *AntigravityGatewayPlugin) Shutdown() error {
	if c := p.context(); c != nil {
		c.Logger().Info("gateway-antigravity plugin shutting down")
	}
	return nil
}

// RegisterGRPCServices implements pluginsdk.GRPCServiceRegistrar. It registers
// both the AccountPlatformExtension and GatewayProviderExtension gRPC services.
// The host delegates account operations (validate, test, refresh) and gateway
// forwarding to this plugin.
func (p *AntigravityGatewayPlugin) RegisterGRPCServices(server *grpc.Server) {
	p.accountPlatformServer = newAccountPlatformServer()
	pb.RegisterAccountPlatformExtensionServer(server, p.accountPlatformServer)

	p.gatewayProviderServer = newGatewayProviderServer(p.logger())
	pb.RegisterGatewayProviderExtensionServer(server, p.gatewayProviderServer)
}

// context returns the live PluginContext or nil if Init has not run yet.
func (p *AntigravityGatewayPlugin) context() pluginsdk.PluginContext {
	c := p.ctx.Load()
	if c == nil {
		return nil
	}
	return *c
}

// logger returns the plugin logger or a no-op fallback.
func (p *AntigravityGatewayPlugin) logger() *slog.Logger {
	if ctx := p.context(); ctx != nil {
		return ctx.Logger()
	}
	return slog.Default()
}

// compile-time interface assertions
var (
	_ pluginsdk.Plugin               = (*AntigravityGatewayPlugin)(nil)
	_ pluginsdk.GRPCServiceRegistrar = (*AntigravityGatewayPlugin)(nil)
)

