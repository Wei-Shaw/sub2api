package main

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync/atomic"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	cmHandler "github.com/Wei-Shaw/sub2api/plugins/content-moderation/handler"
	cmRepo "github.com/Wei-Shaw/sub2api/plugins/content-moderation/repository"
	cmService "github.com/Wei-Shaw/sub2api/plugins/content-moderation/service"
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

	// service / admin are wired in Init. admin is read by RegisterHTTP's
	// handlers, which the host only invokes after Init completes.
	service *cmService.ModerationService
	admin   *cmHandler.AdminHandler
}

// Init wires the repository, hash cache, moderation service and admin handler
// on top of the SDK-supplied DB / Redis. The async worker pool and retention
// loop start inside NewModerationService.
func (p *ContentModerationPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)

	repo := cmRepo.NewContentModerationRepository(ctx.DB())
	hashCache := cmRepo.NewContentModerationHashCache(ctx.Redis())
	p.service = cmService.NewModerationService(ctx, repo, hashCache)
	p.admin = cmHandler.NewAdminHandler(p.service)

	ctx.Logger().Info("content-moderation plugin initialised", "version", pluginVersion)
	return nil
}

func (p *ContentModerationPlugin) Shutdown() error {
	if c := p.ctx.Load(); c != nil {
		(*c).Logger().Info("content-moderation plugin shutting down")
	}
	if p.service != nil {
		p.service.Shutdown()
	}
	return nil
}

// RegisterHTTP wires the admin management routes onto the SDK-managed mux. The
// core gateway forwards the original path unchanged, so handlers register at
// the full paths declared in Manifest. Method dispatch is done per-handler
// because /config (GET+PUT) and /flagged-hashes (DELETE) share a path with
// different verbs, and /flagged-hashes/{hash} carries a path parameter that the
// stdlib mux exposes only via prefix matching.
func (p *ContentModerationPlugin) RegisterHTTP(mux pluginsdk.HTTPMux) {
	mux.Handle(pluginRoutePrefix+"/config", http.HandlerFunc(p.handleConfig))
	mux.Handle(pluginRoutePrefix+"/test-api-keys", methodOnly(http.MethodPost, p.adminFunc(func(h *cmHandler.AdminHandler) http.HandlerFunc { return h.TestAPIKeys })))
	mux.Handle(pluginRoutePrefix+"/status", methodOnly(http.MethodGet, p.adminFunc(func(h *cmHandler.AdminHandler) http.HandlerFunc { return h.GetStatus })))
	mux.Handle(pluginRoutePrefix+"/logs", methodOnly(http.MethodGet, p.adminFunc(func(h *cmHandler.AdminHandler) http.HandlerFunc { return h.ListLogs })))
	mux.Handle(pluginRoutePrefix+"/unban-user", methodOnly(http.MethodPost, p.adminFunc(func(h *cmHandler.AdminHandler) http.HandlerFunc { return h.UnbanUser })))
	// The trailing-slash prefix captures /flagged-hashes/{hash}; the exact
	// path /flagged-hashes (clear-all) is registered separately so the stdlib
	// mux's longest-prefix rule routes each to the right handler.
	mux.Handle(pluginRoutePrefix+"/flagged-hashes/", http.HandlerFunc(p.handleFlaggedHashByID))
	mux.Handle(pluginRoutePrefix+"/flagged-hashes", methodOnly(http.MethodDelete, p.adminFunc(func(h *cmHandler.AdminHandler) http.HandlerFunc { return h.ClearFlaggedHashes })))
}

// handleConfig dispatches GET/PUT on /config to the admin handler.
func (p *ContentModerationPlugin) handleConfig(w http.ResponseWriter, r *http.Request) {
	if p.admin == nil {
		http.Error(w, "content-moderation: not initialised", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		p.admin.GetConfig(w, r)
	case http.MethodPut:
		p.admin.UpdateConfig(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleFlaggedHashByID handles DELETE /flagged-hashes/{hash}, extracting the
// hash from the path segment after the prefix.
func (p *ContentModerationPlugin) handleFlaggedHashByID(w http.ResponseWriter, r *http.Request) {
	if p.admin == nil {
		http.Error(w, "content-moderation: not initialised", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hash := strings.TrimPrefix(r.URL.Path, pluginRoutePrefix+"/flagged-hashes/")
	p.admin.DeleteFlaggedHash(w, r, hash)
}

// adminFunc lazily resolves the admin handler at request time (Init always runs
// before the host forwards a request) and returns 503 if it is somehow absent.
func (p *ContentModerationPlugin) adminFunc(pick func(*cmHandler.AdminHandler) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p.admin == nil {
			http.Error(w, "content-moderation: not initialised", http.StatusServiceUnavailable)
			return
		}
		pick(p.admin)(w, r)
	}
}

// methodOnly rejects any request whose method differs from the declared verb.
func methodOnly(method string, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	})
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
