package main

import (
	"context"
	"net/http"
	"sync/atomic"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"

	chHandler "github.com/Wei-Shaw/sub2api/plugins/channel-management/handler"
	chRepo "github.com/Wei-Shaw/sub2api/plugins/channel-management/repository"
	chService "github.com/Wei-Shaw/sub2api/plugins/channel-management/service"

	"github.com/gin-gonic/gin"
)

const (
	pluginName        = "channel-management"
	pluginDisplayName = "Channel Management"
	pluginVersion     = "0.1.0"
	pluginDescription = "Channel CRUD, channel pricing and channel monitoring"

	// pluginRoutePrefix is the path prefix the core gateway uses when
	// proxying plugin endpoints to this plugin. It MUST match the prefix
	// declared in PluginEndpoints below.
	pluginRoutePrefix = "/api/v1/plugin/" + pluginName
)

// ChannelPlugin is the channel-management plugin entry point. It owns the
// plugin's service/repository wiring and exposes a Gin engine to the SDK.
type ChannelPlugin struct {
	ctx atomic.Pointer[pluginsdk.PluginContext]

	// engine is the Gin HTTP engine the SDK mounts under pluginRoutePrefix.
	// It is built once at construction time so that RegisterHTTP (called
	// before Init) can hand it to the SDK and Init can plug services into it.
	engine *gin.Engine

	channelHandler *chHandler.ChannelHandler
}

// Manifest declares the plugin's capabilities to the core. Endpoint paths are
// expressed without the gateway prefix; the core composes the full path.
func (p *ChannelPlugin) Manifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:        pluginName,
		DisplayName: pluginDisplayName,
		Version:     pluginVersion,
		Description: pluginDescription,
		Author:      "Sub2API",
		PluginEndpoints: []pluginsdk.EndpointDecl{
			// Admin: channel CRUD
			{Path: pluginRoutePrefix + "/admin/channels", Methods: []string{http.MethodGet, http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/admin/channels/:id", Methods: []string{http.MethodGet, http.MethodPut, http.MethodDelete}, AuthType: pluginsdk.AuthTypeAdmin},
			// Admin: model pricing helper
			{Path: pluginRoutePrefix + "/admin/channels/model-pricing", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
		},
		Frontend: &pluginsdk.FrontendManifest{
			MenuItems: []pluginsdk.MenuItemDecl{
				{
					Path:          "/admin/channels",
					IconSVG:       pluginsdk.IconBranchFork,
					Labels:        pluginsdk.Labels("渠道管理", "Channel Management"),
					Section:       pluginsdk.SectionAdmin,
					SortOrder:     200,
					RequiresAdmin: true,
					Children: []pluginsdk.MenuItemDecl{
						{
							Path: "/admin/channels",
							// TODO(plugin-sdk): switch to pluginsdk.IconTag once
							// the SDK ships verified outline-tag SVG path data.
							// Until then, reuse IconBranchFork so the child
							// menu still has a recognisable icon.
							IconSVG:       pluginsdk.IconBranchFork,
							Labels:        pluginsdk.Labels("渠道定价", "Channel Pricing"),
							Section:       pluginsdk.SectionAdmin,
							SortOrder:     210,
							RequiresAdmin: true,
						},
					},
				},
			},
			I18nNamespaces: []string{"channel-management"},
		},
	}
}

// Init wires the repository, service and handler layers using the SDK's DB.
// The Gin engine is mounted on the SDK HTTP server in RegisterHTTP, so this
// only needs to attach routes to the engine that was created earlier.
func (p *ChannelPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)

	repo := chRepo.NewChannelRepository(ctx.DB())
	// authCacheInvalidator is currently nil — the in-process invalidator
	// lives in the core monolith. Until the auth cache itself is exposed
	// through the SDK, channel updates will not actively bust API-key auth
	// caches. The cache eventually expires on its own TTL.
	svc := chService.NewChannelService(repo, nil)

	// Wire the Redis cache writer that mirrors channel state to the
	// gateway-side Redis cache (see GATEWAY_CACHE_SPEC.md). When Redis is
	// unavailable the writer is left nil and CRUD becomes a no-op for the
	// cache layer; the gateway then falls through its safe-degradation path.
	if redis := ctx.Redis(); redis != nil {
		cacheWriter := chService.NewCacheWriter(redis, svc.GroupPlatforms)
		svc.SetCacheWriter(cacheWriter)
		// Warm the cache asynchronously so the gateway has data immediately
		// on cold start without blocking plugin startup.
		go svc.RebuildAllCacheNow(context.Background())
	} else {
		ctx.Logger().Warn("channel cache writer disabled: redis client unavailable")
	}

	p.channelHandler = chHandler.NewChannelHandler(svc, nil)

	p.registerRoutes()

	ctx.Logger().Info("channel-management plugin initialised", "version", pluginVersion)
	return nil
}

// Shutdown is a no-op for now; service objects hold no goroutines or external
// resources beyond the SDK-managed DB handle.
func (p *ChannelPlugin) Shutdown() error {
	if c := p.ctx.Load(); c != nil {
		(*c).Logger().Info("channel-management plugin shutting down")
	}
	return nil
}

// RegisterHTTP hands the Gin engine to the SDK. The engine is mounted at the
// plugin's gateway prefix so paths in routes match the manifest declarations.
func (p *ChannelPlugin) RegisterHTTP(mux pluginsdk.HTTPMux) {
	if p.engine == nil {
		gin.SetMode(gin.ReleaseMode)
		p.engine = gin.New()
		p.engine.Use(gin.Recovery())
	}
	mux.Handle(pluginRoutePrefix+"/", p.engine)
}

// registerRoutes attaches the channel admin routes to the Gin engine. It is
// called from Init once handlers are constructed.
func (p *ChannelPlugin) registerRoutes() {
	if p.engine == nil {
		// RegisterHTTP was not called (e.g. --no-http); skip routing.
		return
	}
	admin := p.engine.Group(pluginRoutePrefix + "/admin")
	{
		channels := admin.Group("/channels")
		channels.GET("", p.channelHandler.List)
		channels.POST("", p.channelHandler.Create)
		channels.GET("/model-pricing", p.channelHandler.GetModelDefaultPricing)
		channels.GET("/:id", p.channelHandler.GetByID)
		channels.PUT("/:id", p.channelHandler.Update)
		channels.DELETE("/:id", p.channelHandler.Delete)
	}
}

// HealthCheck reports unhealthy until the plugin context is wired so the core
// can distinguish a freshly-spawned process from a fully-initialised one.
func (p *ChannelPlugin) HealthCheck() (bool, string) {
	if p.ctx.Load() == nil {
		return false, "plugin not yet initialised"
	}
	return true, "ok"
}
