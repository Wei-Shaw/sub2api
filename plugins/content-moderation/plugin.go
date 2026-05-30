package main

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"sync/atomic"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// frontendAssets embeds the plugin's compiled frontend bundle. The directory
// is populated during the frontend-migration task; the embed directive uses
// `all:` so an empty dir still compiles.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

// Compile-time assertions that the plugin satisfies the SDK contracts it
// declares behaviour for. ContentInterceptExtension is intentionally absent:
// Check is wired via GRPCServiceRegistrar once the SDK exposes the proto
// bindings (CM 插件: Proto 类型和 gRPC 接口定义).
var (
	_ pluginsdk.Plugin                 = (*ContentModerationPlugin)(nil)
	_ pluginsdk.HTTPRegistrar          = (*ContentModerationPlugin)(nil)
	_ pluginsdk.MigrationProvider      = (*ContentModerationPlugin)(nil)
	_ pluginsdk.FrontendBundleProvider = (*ContentModerationPlugin)(nil)
)

// ContentModerationPlugin holds the SDK-injected resources. pluginCtx is stored
// atomically so HTTP handlers (and the future ContentInterceptExtension.Check)
// can read it without a mutex even when they race with Init — in practice Init
// always completes before the host issues any request.
type ContentModerationPlugin struct {
	ctx atomic.Pointer[pluginsdk.PluginContext]
}

// Init stores the SDK-supplied context for handlers to use. The migrated
// service / repository wiring is constructed here in a follow-up task.
func (p *ContentModerationPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)
	ctx.Logger().Info("content-moderation plugin initialised", "version", pluginVersion)
	return nil
}

func (p *ContentModerationPlugin) Shutdown() error {
	if c := p.ctx.Load(); c != nil {
		(*c).Logger().Info("content-moderation plugin shutting down")
	}
	return nil
}

// RegisterHTTP wires the admin management routes onto the SDK-managed mux. The
// core gateway forwards the original path unchanged, so handlers register at
// the full paths declared in Manifest. The handlers themselves are stubs until
// the handler layer is migrated.
func (p *ContentModerationPlugin) RegisterHTTP(mux pluginsdk.HTTPMux) {
	mux.Handle(pluginRoutePrefix+"/config", http.HandlerFunc(p.handleNotImplemented))
	mux.Handle(pluginRoutePrefix+"/test-api-keys", http.HandlerFunc(p.handleNotImplemented))
	mux.Handle(pluginRoutePrefix+"/status", http.HandlerFunc(p.handleNotImplemented))
	mux.Handle(pluginRoutePrefix+"/logs", http.HandlerFunc(p.handleNotImplemented))
	mux.Handle(pluginRoutePrefix+"/unban-user", http.HandlerFunc(p.handleNotImplemented))
	mux.Handle(pluginRoutePrefix+"/flagged-hashes/", http.HandlerFunc(p.handleNotImplemented))
	mux.Handle(pluginRoutePrefix+"/flagged-hashes", http.HandlerFunc(p.handleNotImplemented))
}

// OpenMigration implements pluginsdk.MigrationProvider, serving the embedded
// SQL bodies the host fetches via PluginLifecycle.GetMigration.
func (p *ContentModerationPlugin) OpenMigration(filename string) ([]byte, error) {
	clean := path.Base(filename)
	if clean == "." || clean == "/" {
		return nil, fs.ErrInvalid
	}
	return migrationAssets.ReadFile("migrations/" + clean)
}

// OpenFrontendFile implements pluginsdk.FrontendBundleProvider, mapping the
// host's GetFrontendBundle requests onto the embedded bundle.
func (p *ContentModerationPlugin) OpenFrontendFile(rel string) ([]byte, error) {
	clean := path.Clean("/" + rel)
	if clean == "/" || clean == "/." {
		return nil, fs.ErrInvalid
	}
	return frontendAssets.ReadFile("frontend/" + clean[1:])
}

// context returns the live PluginContext or nil if Init has not run yet.
func (p *ContentModerationPlugin) context() pluginsdk.PluginContext {
	c := p.ctx.Load()
	if c == nil {
		return nil
	}
	return *c
}

// handleNotImplemented is the placeholder for admin endpoints until the handler
// layer is migrated.
func (p *ContentModerationPlugin) handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "content-moderation: not implemented", http.StatusNotImplemented)
}
