package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"sync/atomic"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"

	chHandler "github.com/Wei-Shaw/sub2api/plugins/channel-management/handler"
	chRepo "github.com/Wei-Shaw/sub2api/plugins/channel-management/repository"
	chService "github.com/Wei-Shaw/sub2api/plugins/channel-management/service"

	"github.com/gin-gonic/gin"
)

// frontendAssets 是 channel-management 的 frontend bundle 嵌入式 FS.
// CI / 本地 build 前必须先跑 `pnpm --filter @sub2api/plugin-channel-management build`,
// 把 dist/entry.js + dist/entry.css 产出到 frontend/dist/, 否则该 embed 仅会
// 拿到占位文件 (.keep), 运行时 OpenFrontendFile 会返回 fs.ErrNotExist.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

const (
	pluginName        = "channel-management"
	pluginDisplayName = "Channel Management"
	pluginVersion     = "0.1.0"
	pluginDescription = "Channel CRUD, channel pricing and channel monitoring"

	// pluginRoutePrefix is the path prefix the core gateway uses when
	// proxying plugin endpoints to this plugin. It MUST match the prefix
	// declared in PluginEndpoints below.
	pluginRoutePrefix = "/api/v1/plugin/" + pluginName

	// availableChannelsFrontendPath is the user-facing route the SPA
	// registers for the "available channels" view. The matching
	// frontend component file (AvailableChannelsView.vue) ships with
	// W9 of the plugin migration; the manifest declares the route up
	// front so the host SPA router has the entry as soon as the
	// frontend bundle is rebuilt.
	availableChannelsFrontendPath = "/available-channels"
)

// ChannelPlugin is the channel-management plugin entry point. It owns the
// plugin's service/repository wiring and exposes a Gin engine to the SDK.
type ChannelPlugin struct {
	ctx atomic.Pointer[pluginsdk.PluginContext]

	// engine is the Gin HTTP engine the SDK mounts under pluginRoutePrefix.
	// It is built once at construction time so that RegisterHTTP (called
	// before Init) can hand it to the SDK and Init can plug services into it.
	engine *gin.Engine

	channelHandler          *chHandler.ChannelHandler
	availableChannelHandler *chHandler.AvailableChannelHandler
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
			// User: available channels (read-only, scoped by V4 X-Plugin-User-* headers)
			{Path: pluginRoutePrefix + "/available-channels", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
		},
		// Capabilities — channel-management writes the gateway cache contract
		// (channel:active, channel:by_id:*, …) which the core's
		// ChannelCacheReader reads directly. Those keys live outside the
		// per-plugin namespace, so we ask the core for raw key access.
		Capabilities: []string{pluginsdk.CapabilityRedisRawKeys},
		Frontend: &pluginsdk.FrontendManifest{
			// EntryJS 路径相对于 plugin frontend 内的 dist/ 根, 核心拼成
			// /api/v1/plugin-assets/channel-management/dist/entry.js 暴露给浏览器.
			EntryJS: "dist/entry.js",
			// CSS 与 JS 一同产出 (cssCodeSplit:false + assetFileNames=entry.[ext]),
			// 声明给 host loader-runtime 注入 <link>.
			EntryCSS: "dist/entry.css",
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
							Path:          "/admin/channels",
							IconSVG:       pluginsdk.IconTag,
							Labels:        pluginsdk.Labels("渠道定价", "Channel Pricing"),
							Section:       pluginsdk.SectionAdmin,
							SortOrder:     210,
							RequiresAdmin: true,
						},
					},
				},
				// User-facing "available channels" entry. The actual Vue
				// component is added in W9; declaring the menu item now lets
				// the host SPA pick it up the moment the frontend bundle
				// rebuilds, with no further backend change required.
				{
					Path:          availableChannelsFrontendPath,
					IconSVG:       pluginsdk.IconBranchFork,
					Labels:        pluginsdk.Labels("可用渠道", "Available Channels"),
					Section:       pluginsdk.SectionUser,
					SortOrder:     200,
					RequiresAdmin: false,
				},
			},
			Routes: []pluginsdk.RouteDecl{
				{
					// path 与 menu item path 一致, 让 vue-router 命中 PluginView,
					// PluginView 通过 meta.componentPath 找到 install 返回的
					// components 表里 "ChannelsView.vue" key 对应的组件.
					Path:          "/admin/channels",
					Name:          "AdminChannels",
					ComponentPath: "ChannelsView.vue",
				},
				// User-facing route — same dance as the menu item: the file
				// AvailableChannelsView.vue is created in W9, but the route
				// declaration is harmless until then because vue-router will
				// simply 404 when the component map is empty.
				{
					Path:          availableChannelsFrontendPath,
					Name:          "UserAvailableChannels",
					ComponentPath: "AvailableChannelsView.vue",
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
	// gateway-side Redis cache (see GATEWAY_CACHE_SPEC.md). The cache keys
	// (channel:meta:*, plugin:channel:meta:*, etc.) are documented in
	// GATEWAY_CACHE_SPEC.md and read by the core directly, so they must NOT
	// receive the SDK's automatic per-plugin namespace. We obtain a raw twin
	// of the Redis client to bypass it.
	//
	// If the core has not granted the redis_raw_keys capability (e.g. an
	// operator rolled out the plugin before bumping the core version),
	// Raw() returns nil and we leave the cache writer disabled instead of
	// silently writing to the wrong keys.
	if redis := ctx.Redis(); redis != nil {
		raw := redis.Raw()
		if raw == nil {
			ctx.Logger().Warn("channel cache writer disabled: core did not grant redis_raw_keys capability — check plugin manifest is in sync with core")
		} else {
			cacheWriter := chService.NewCacheWriter(raw, svc.GroupPlatforms)
			svc.SetCacheWriter(cacheWriter)
			// Warm the cache asynchronously so the gateway has data immediately
			// on cold start without blocking plugin startup.
			go svc.RebuildAllCacheNow(context.Background())
		}
	} else {
		ctx.Logger().Warn("channel cache writer disabled: redis client unavailable")
	}

	p.channelHandler = chHandler.NewChannelHandler(svc, nil)

	// Wire the user-facing "available channels" view (V5 / W8). It reuses
	// the same channel repository for ListAll and adds a small read-only
	// repository for groups + user permissions. The user identity is
	// derived from V4 X-Plugin-User-* request headers inside the handler.
	availableRepo := chRepo.NewAvailableChannelsRepository(ctx.DB())
	availableSvc := chService.NewAvailableChannelsService(repo, availableRepo)
	p.availableChannelHandler = chHandler.NewAvailableChannelHandler(availableSvc)

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
	// User-facing read-only endpoint. The host gateway routes
	// /api/v1/plugin/channel-management/available-channels to this engine
	// and the manifest declares it as AuthTypeUser, so the host
	// authenticates the caller before it gets here; we just project the
	// V4 X-Plugin-User-* headers into the response shape.
	if p.availableChannelHandler != nil {
		p.engine.GET(pluginRoutePrefix+"/available-channels", p.availableChannelHandler.List)
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

// OpenFrontendFile implements pluginsdk.FrontendBundleProvider so the core can
// fetch frontend assets (entry.js / entry.css / source maps / 等) over gRPC.
//
// path 来自 manifest.Frontend.EntryJS / EntryCSS 或 host /api/v1/plugin-assets
// HTTP 请求带过来的相对 path. 调用方在核心侧已经做过路径穿越校验, 这里再做一次
// 最小化的兜底以防误用.
func (p *ChannelPlugin) OpenFrontendFile(rel string) ([]byte, error) {
	clean := path.Clean("/" + rel)
	if clean == "/" || clean == "/." {
		return nil, fs.ErrInvalid
	}
	clean = clean[1:] // strip leading "/"
	full := "frontend/" + clean
	return frontendAssets.ReadFile(full)
}
