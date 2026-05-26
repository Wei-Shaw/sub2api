package main

import (
	"embed"
	"io/fs"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/grpc"
)

// frontendAssets embeds the compiled frontend bundle (entry.js + entry.css).
// Build with `pnpm --filter @sub2api/plugin-gateway-anthropic build` before
// compiling; otherwise only the .keep placeholder is embedded and
// OpenFrontendFile will return fs.ErrNotExist at runtime.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

const pluginVersion = "0.1.0"

// AnthropicGatewayPlugin is the top-level plugin struct implementing
// pluginsdk.Plugin and optional extension interfaces.
type AnthropicGatewayPlugin struct {
	pluginsdk.BasePlugin

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

// Init stores the SDK-supplied context and wires the embedded frontend
// bundle so BasePlugin.OpenFrontendFile can serve assets.
func (p *AnthropicGatewayPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.SetContext(ctx)
	sub, err := fs.Sub(frontendAssets, "frontend")
	if err != nil {
		return err
	}
	p.FrontendFS = sub
	ctx.Logger().Info("gateway-anthropic plugin initialised", "version", pluginVersion)
	return nil
}

// Shutdown releases resources.
func (p *AnthropicGatewayPlugin) Shutdown() error {
	if c := p.Context(); c != nil {
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

// compile-time interface assertions
var (
	_ pluginsdk.Plugin                 = (*AnthropicGatewayPlugin)(nil)
	_ pluginsdk.GRPCServiceRegistrar   = (*AnthropicGatewayPlugin)(nil)
	_ pluginsdk.FrontendBundleProvider = (*AnthropicGatewayPlugin)(nil)
)
