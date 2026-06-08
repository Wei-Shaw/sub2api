package main

import (
	"embed"
	"io/fs"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

const pluginVersion = "0.1.0"

// GeminiGatewayPlugin is the top-level plugin struct implementing
// pluginsdk.Plugin and optional extension interfaces.
type GeminiGatewayPlugin struct {
	pluginsdk.BasePlugin

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

// Init stores the SDK-supplied context and wires the embedded frontend
// bundle so BasePlugin.OpenFrontendFile can serve assets.
func (p *GeminiGatewayPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.SetContext(ctx)
	sub, err := fs.Sub(frontendAssets, "frontend")
	if err != nil {
		return err
	}
	p.FrontendFS = sub
	ctx.Logger().Info("gateway-gemini plugin initialised", "version", pluginVersion)
	return nil
}

// Shutdown releases resources.
func (p *GeminiGatewayPlugin) Shutdown() error {
	if c := p.Context(); c != nil {
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

	p.gatewayProviderServer = newGatewayProviderServer(p.Logger)
	pb.RegisterGatewayProviderExtensionServer(server, p.gatewayProviderServer)
}

// compile-time interface assertions
var (
	_ pluginsdk.Plugin                 = (*GeminiGatewayPlugin)(nil)
	_ pluginsdk.GRPCServiceRegistrar   = (*GeminiGatewayPlugin)(nil)
	_ pluginsdk.FrontendBundleProvider = (*GeminiGatewayPlugin)(nil)
)
