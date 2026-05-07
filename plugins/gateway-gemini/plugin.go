package main

import (
	"log/slog"
	"sync/atomic"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

const pluginVersion = "0.1.0"

// GeminiGatewayPlugin is the top-level plugin struct implementing
// pluginsdk.Plugin and optional extension interfaces.
type GeminiGatewayPlugin struct {
	ctx atomic.Pointer[pluginsdk.PluginContext]

	// accountPlatformServer handles account-level operations (validate,
	// test, refresh). Constructed in RegisterGRPCServices.
	accountPlatformServer *accountPlatformServer

	// gatewayProviderServer handles upstream API forwarding. Constructed
	// in RegisterGRPCServices.
	gatewayProviderServer *gatewayProviderServer
}

// Manifest returns the static plugin descriptor.
func (p *GeminiGatewayPlugin) Manifest() *pluginsdk.Manifest {
	return buildManifest()
}

// Init stores the SDK-supplied context for handlers to use.
func (p *GeminiGatewayPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)
	ctx.Logger().Info("gateway-gemini plugin initialised", "version", pluginVersion)
	return nil
}

// Shutdown releases resources.
func (p *GeminiGatewayPlugin) Shutdown() error {
	if c := p.context(); c != nil {
		c.Logger().Info("gateway-gemini plugin shutting down")
	}
	return nil
}

// RegisterGRPCServices implements pluginsdk.GRPCServiceRegistrar. It registers
// both the AccountPlatformExtension and GatewayProviderExtension gRPC services.
// The host delegates account operations (validate, test, refresh) and gateway
// forwarding to this plugin.
func (p *GeminiGatewayPlugin) RegisterGRPCServices(server *grpc.Server) {
	p.accountPlatformServer = newAccountPlatformServer()
	pb.RegisterAccountPlatformExtensionServer(server, p.accountPlatformServer)

	p.gatewayProviderServer = newGatewayProviderServer(p.logger())
	pb.RegisterGatewayProviderExtensionServer(server, p.gatewayProviderServer)
}

// context returns the live PluginContext or nil if Init has not run yet.
func (p *GeminiGatewayPlugin) context() pluginsdk.PluginContext {
	c := p.ctx.Load()
	if c == nil {
		return nil
	}
	return *c
}

// logger returns the plugin logger or a no-op fallback.
func (p *GeminiGatewayPlugin) logger() *slog.Logger {
	if ctx := p.context(); ctx != nil {
		return ctx.Logger()
	}
	return slog.Default()
}

// compile-time interface assertions
var (
	_ pluginsdk.Plugin               = (*GeminiGatewayPlugin)(nil)
	_ pluginsdk.GRPCServiceRegistrar = (*GeminiGatewayPlugin)(nil)
)

