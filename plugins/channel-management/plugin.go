package main

import (
	"context"
	"embed"
	"io/fs"
	"path"
	"sync/atomic"

	entsql "entgo.io/ent/dialect/sql"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	pluginsdkpb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/channel-management/ent"
	chMaintenance "github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/maintenance"
	chPricing "github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/pricing"

	chHandler "github.com/Wei-Shaw/sub2api/plugins/channel-management/handler"
	chRepo "github.com/Wei-Shaw/sub2api/plugins/channel-management/repository"
	chService "github.com/Wei-Shaw/sub2api/plugins/channel-management/service"

	monitorHandler "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/handler"
	monitorRepo "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/repository"
	monitorService "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

// frontendAssets 是 channel-management 的 frontend bundle 嵌入式 FS.
// CI / 本地 build 前必须先跑 `pnpm --filter @sub2api/plugin-channel-management build`,
// 把 dist/entry.js + dist/entry.css 产出到 frontend/dist/, 否则该 embed 仅会
// 拿到占位文件 (.keep), 运行时 OpenFrontendFile 会返回 fs.ErrNotExist.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

// monitorMigrations embeds the channel-monitor SQL migration files so the host
// can fetch them via PluginLifecycle.GetMigration and apply them under the
// MigrationRunner advisory-lock + plugin_migrations bookkeeping path. See
// V5-DESIGN §1 (W1 MigrationRunnerCapability) and V5-DESIGN §6.1 (W6 Channel
// Monitor migration playbook).
//
//go:embed migrations/*.sql
var monitorMigrations embed.FS

// monitorSettingsSchemaJSON / monitorSettingsDefaultsJSON embed the
// channel-monitor settings schema + defaults declared via the V5 W3
// SettingsExtensionCapability. The host renders the schema as a tab on
// the admin Settings page; runtime.LoadMonitorRuntime reads the values
// back via SettingsClient.GetTyped at request time.
//
//go:embed monitor/settings/settings_schema.json
var monitorSettingsSchemaJSON []byte

//go:embed monitor/settings/settings_defaults.json
var monitorSettingsDefaultsJSON []byte

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

	// channelMonitorAdminFrontendPath is the admin-facing route for the
	// Channel Monitor management view (V5 W7). The Vue component
	// ChannelMonitorView.vue ships with W7.1.
	channelMonitorAdminFrontendPath = "/admin/monitor"

	// channelStatusFrontendPath is the user-facing read-only "channel
	// status" view (V5 W7.3).
	channelStatusFrontendPath = "/channel-status"
)

// ChannelPlugin is the channel-management plugin entry point. It owns the
// plugin's service/repository wiring and exposes a Gin engine to the SDK.
type ChannelPlugin struct {
	ctx atomic.Pointer[pluginsdk.PluginContext]

	// entClient is the ent ORM client backed by the SDK's SQL proxy.
	// Created in Init via entsql.OpenDB(pluginsdk.Dialect, ctx.DB()).
	entClient *pluginent.Client

	// engine is the Gin HTTP engine the SDK mounts under pluginRoutePrefix.
	// It is built once at construction time so that RegisterHTTP (called
	// before Init) can hand it to the SDK and Init can plug services into it.
	engine *gin.Engine

	channelHandler          *chHandler.ChannelHandler
	availableChannelHandler *chHandler.AvailableChannelHandler

	// monitor* fields back the channel-monitor sub-feature ported in V5 W6.
	// They are nil until Init wires them so HealthCheck can disambiguate
	// freshly-spawned vs initialised processes.
	monitorService         *monitorService.ChannelMonitorService
	monitorAdminHandler    *monitorHandler.AdminHandler
	monitorUserHandler     *monitorHandler.UserHandler
	monitorTemplateService *monitorService.ChannelMonitorRequestTemplateService
	monitorTemplateHandler *monitorHandler.TemplateHandler

	// pricingServer implements PricingExtension on the plugin's gRPC server.
	// It is constructed in RegisterGRPCServices (before Init) and wired with
	// the repository in Init via pricingServer.SetLister. RPCs that arrive
	// during the gap return codes.Unavailable so the host retries.
	pricingServer *chPricing.Server

	// maintenanceServer implements MaintenanceExtension on the plugin's gRPC
	// server. It is constructed in RegisterGRPCServices (before Init) and
	// wired with the monitor service in wireMonitor. RPCs that arrive before
	// wiring return an empty response (no tasks to run).
	maintenanceServer *chMaintenance.Server
}

// Manifest declares the plugin's capabilities to the core. Static data lives
// in manifest_decls.go (T49) so this body stays a thin assembler — adding an
// endpoint, menu item or migration means editing the matching slice there,
// not the Manifest() function.
func (p *ChannelPlugin) Manifest() *pluginsdk.Manifest {
	return &pluginsdk.Manifest{
		Name:        pluginName,
		DisplayName: pluginDisplayName,
		Version:     pluginVersion,
		Description: pluginDescription,
		Author:      "Sub2API",
		// IconSVG opts the plugin card on the admin plugins page into using
		// the channel/branch icon so the visual matches the sidebar entry.
		IconSVG:         pluginsdk.IconBranchFork,
		PluginEndpoints: endpointDecls,
		Capabilities:    capabilityDecls,
		OwnedTables:     ownedTableDecls,
		Migrations:      migrationDecls,
		SettingsSchema: &pluginsdk.SettingsSchemaDoc{
			Schema:   monitorSettingsSchemaJSON,
			Defaults: monitorSettingsDefaultsJSON,
			Version:  "1.0.0", // V5/W6 SETTINGS-V2 demo
			// PropertyMeta 留空 — 让 host 从 schema vendor extensions 反向推导,
			// 这样 plugin author 不需要重复声明 (INDUSTRY §3 行 4 决策).
		},
		Frontend: &pluginsdk.FrontendManifest{
			// EntryJS 路径相对于 plugin frontend 内的 dist/ 根, 核心拼成
			// /api/v1/plugin-assets/channel-management/dist/entry.js 暴露给浏览器.
			EntryJS: "dist/entry.js",
			// CSS 与 JS 一同产出 (cssCodeSplit:false + assetFileNames=entry.[ext]),
			// 声明给 host loader-runtime 注入 <link>.
			EntryCSS:       "dist/entry.css",
			MenuItems:      menuItemDecls,
			Routes:         routeDecls,
			I18nNamespaces: []string{"channel-management"},
		},
	}
}

// Init wires the repository, service and handler layers using the SDK's DB.
// The Gin engine is mounted on the SDK HTTP server in RegisterHTTP, so this
// only needs to attach routes to the engine that was created earlier.
//
// T50: the four wiring concerns (pricing, cache, monitor, available) each
// live in their own helper so adding a new sub-feature means appending one
// call to this body rather than extending an already-long function.
func (p *ChannelPlugin) Init(ctx pluginsdk.PluginContext) error {
	p.ctx.Store(&ctx)

	// Register custom gin/binding validators backed by the service-layer
	// enum lists. Must run before any ShouldBindJSON call sees a request,
	// so wiring it at the top of Init keeps the contract obvious. Failing
	// here means gin's validator engine was swapped out — surface that as
	// a startup error rather than continuing with permissive binds.
	if err := chHandler.RegisterCustomValidators(); err != nil {
		ctx.Logger().Error("channel-management: failed to register custom validators",
			"error", err)
		return err
	}

	// Initialise the ent ORM client using the SDK's SQL proxy driver.
	// The SDK driver transparently proxies queries through gRPC to the
	// core's connection pool, and entsql.OpenDB wraps it with ent's
	// dialect-specific query builder.
	drv := entsql.OpenDB(pluginsdk.Dialect, ctx.DB())
	p.entClient = pluginent.NewClient(pluginent.Driver(drv))
	ctx.Logger().Info("channel-management: ent client initialised",
		"dialect", pluginsdk.Dialect)

	repo := chRepo.NewChannelRepository(ctx.DB(), p.entClient)
	// authCacheInvalidator is currently nil — the in-process invalidator
	// lives in the core monolith. Until the auth cache itself is exposed
	// through the SDK, channel updates will not actively bust API-key auth
	// caches. The cache eventually expires on its own TTL.
	svc := chService.NewChannelService(repo, nil)

	p.wirePricing(svc, repo)
	p.wireCacheWriter(ctx, svc)
	p.channelHandler = chHandler.NewChannelHandler(svc, chHandler.NewHostPricingLookup(ctx.Host()))
	p.wireMonitor(ctx)
	p.wireAvailableChannels(ctx, repo)

	p.registerRoutes()

	ctx.Logger().Info("channel-management plugin initialised", "version", pluginVersion)
	return nil
}

// wirePricing hooks the PricingExtension data source (repo) and the CRUD
// event publisher + account-stats resolver (svc) onto the gRPC server that
// RegisterGRPCServices already registered. Any host RPC that arrived in the
// gap between RegisterGRPCServices and Init saw codes.Unavailable; subsequent
// calls land on the live repository / broker.
func (p *ChannelPlugin) wirePricing(svc *chService.ChannelService, repo chService.ChannelRepository) {
	if p.pricingServer == nil {
		return
	}
	p.pricingServer.SetLister(&pricingListerAdapter{repo: repo})
	// After every CRUD commit ChannelService publishes UPSERT/DELETE events
	// that the pricingServer's WatchPricingOverrides handler forwards to
	// the host's PricingExtension cache (P4). Sub-second freshness without
	// touching Redis. The host's 5-minute List re-sync still acts as a
	// safety net against silently dropped events.
	svc.SetEventPublisher(p.pricingServer.Publisher())
	// Hand the channel service to the gRPC server so per-request
	// ResolveAccountStatsCost RPCs from the host can delegate into the
	// plugin's matching engine without an import cycle.
	p.pricingServer.SetAccountStatsResolver(svc)
}

// wireCacheWriter installs the Redis cache writer that mirrors channel state
// to the gateway-side cache keys (channel:meta:*, plugin:channel:meta:*,
// etc.) documented in GATEWAY_CACHE_SPEC.md. Those keys are read directly by
// the core and therefore MUST NOT receive the SDK's automatic per-plugin
// namespace — we obtain a raw twin of the Redis client to bypass it.
//
// If the core has not granted the redis_raw_keys capability (e.g. an
// operator rolled out the plugin before bumping the core version), Raw()
// returns nil and we leave the cache writer disabled instead of silently
// writing to the wrong keys.
func (p *ChannelPlugin) wireCacheWriter(ctx pluginsdk.PluginContext, svc *chService.ChannelService) {
	redis := ctx.Redis()
	if redis == nil {
		ctx.Logger().Warn("channel cache writer disabled: redis client unavailable")
		return
	}
	raw := redis.Raw()
	if raw == nil {
		ctx.Logger().Warn("channel cache writer disabled: core did not grant redis_raw_keys capability — check plugin manifest is in sync with core")
		return
	}
	cacheWriter := chService.NewCacheWriter(raw, svc.GroupPlatforms)
	svc.SetCacheWriter(cacheWriter)
	// Warm the cache asynchronously so the gateway has data immediately
	// on cold start without blocking plugin startup.
	go svc.RebuildAllCacheNow(context.Background())
}

// wireMonitor stitches together the channel-monitor sub-feature (V5 W6):
// repository, optional secret encryptor, service, handlers, and the
// JobScheduler runner that replaces the legacy in-process ticker pool.
//
// runner 注册后不再被 service 回引：CRUD 修改无需即时通知，60 秒 tick 会在
// 下一轮基于 ListEnabled + LastCheckedAt 自然吸收。
func (p *ChannelPlugin) wireMonitor(ctx pluginsdk.PluginContext) {
	monRepo := monitorRepo.NewChannelMonitorRepository(ctx.DB())
	var monEncryptor monitorService.SecretEncryptor
	if secrets := ctx.Secrets(); secrets != nil {
		monEncryptor = monitorService.NewSDKSecretEncryptor(context.Background(), secrets)
	} else {
		ctx.Logger().Warn("channel-monitor encryption disabled: SDK Secrets() unavailable; api keys will fail to encrypt — check CapabilitySecretEncryption is in plugin manifest")
	}
	p.monitorService = monitorService.NewChannelMonitorService(monRepo, monEncryptor)
	if p.maintenanceServer != nil {
		p.maintenanceServer.SetMonitorService(p.monitorService)
	}
	p.monitorAdminHandler = monitorHandler.NewAdminHandler(p.monitorService)
	p.monitorUserHandler = monitorHandler.NewUserHandler(p.monitorService, ctx.Settings())

	// Request template sub-feature (W6 follow-up): CRUD + apply picker.
	// Shares DB / logger with the monitor service; repo is stateless so no
	// extra lifecycle hook required.
	tmplRepo := monitorRepo.NewChannelMonitorTemplateRepository(ctx.DB())
	p.monitorTemplateService = monitorService.NewChannelMonitorRequestTemplateService(tmplRepo)
	p.monitorTemplateHandler = monitorHandler.NewTemplateHandler(p.monitorTemplateService)

	// Host fires monitor.run every 60s on each replica — concurrency,
	// leader election and history are all owned by the host. Daily
	// maintenance is handled via the MaintenanceExtension gRPC service.
	jobRunner := monitorService.NewMonitorJobRunner(p.monitorService, ctx.Jobs(), ctx.Logger())
	if err := jobRunner.Register(); err != nil {
		ctx.Logger().Warn("channel-monitor: job registration failed; periodic checks disabled", "error", err)
	}
}

// wireAvailableChannels wires the user-facing "available channels" read view
// (V5 W8). It reuses the same channel repository for ListAll and adds a
// small read-only repository for groups + user permissions. The user
// identity is derived from V4 X-Plugin-User-* request headers inside the
// handler, so this file only has to hand the plumbing together.
//
// ctx.Settings() feeds the `availableChannelsEnabled` feature switch
// (declared in monitor/settings/settings_schema.json — the plugin owns a
// single settings namespace shared by monitor + available-channels).
func (p *ChannelPlugin) wireAvailableChannels(ctx pluginsdk.PluginContext, repo chService.ChannelRepository) {
	availableRepo := chRepo.NewAvailableChannelsRepository(ctx.DB())
	// ctx.Host() is always non-nil (the SDK wires the HostClient
	// unconditionally); when the host has not registered HostService
	// the client surfaces ErrHostPricingUnavailable and the view
	// degrades to nil pricing — same behaviour as before the wire.
	availableSvc := chService.NewAvailableChannelsService(repo, availableRepo, ctx.Host(), ctx.Logger())
	p.availableChannelHandler = chHandler.NewAvailableChannelHandler(availableSvc, ctx.Settings())
}

// Shutdown closes the ent client (which releases the ent driver wrapper
// but does NOT close the underlying SDK-managed *sql.DB).
func (p *ChannelPlugin) Shutdown() error {
	if c := p.ctx.Load(); c != nil {
		(*c).Logger().Info("channel-management plugin shutting down")
	}
	if p.entClient != nil {
		return p.entClient.Close()
	}
	return nil
}

// RegisterGRPCServices attaches the plugin-side gRPC services that the host
// invokes via the existing per-plugin connection. PricingExtension is
// registered here so the host's PricingExtensionClient can call
// ListPricingOverrides / WatchPricingOverrides / AdjustCost without an
// extra connection. The Server's data source is wired later in Init via
// pricingServer.SetLister.
func (p *ChannelPlugin) RegisterGRPCServices(server *grpc.Server) {
	if p.pricingServer == nil {
		p.pricingServer = chPricing.NewServer()
	}
	pluginsdkpb.RegisterPricingExtensionServer(server, p.pricingServer)

	if p.maintenanceServer == nil {
		p.maintenanceServer = chMaintenance.NewServer()
	}
	p.maintenanceServer.Register(server)
}

// pricingListerAdapter exposes the *ChannelRepository to the pricing
// server. The repository's interface returns chService types verbatim so
// the adapter is a thin pass-through; it exists only to satisfy the
// pricing.channelLister type without dragging the proto types into the
// service package.
type pricingListerAdapter struct {
	repo chService.ChannelRepository
}

func (a *pricingListerAdapter) ListAll(ctx context.Context) ([]chService.Channel, error) {
	return a.repo.ListAll(ctx)
}

func (a *pricingListerAdapter) GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error) {
	return a.repo.GetGroupPlatforms(ctx, groupIDs)
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

		// Channel monitor admin routes (V5 W6). Each method currently returns
		// 501 except List, which short-circuits to an empty page so the admin
		// UI's smoke-test passes once the frontend lands.
		if p.monitorAdminHandler != nil {
			monitors := admin.Group("/monitors")
			monitors.GET("", p.monitorAdminHandler.List)
			monitors.POST("", p.monitorAdminHandler.Create)
			monitors.GET("/:id", p.monitorAdminHandler.GetByID)
			monitors.PUT("/:id", p.monitorAdminHandler.Update)
			monitors.DELETE("/:id", p.monitorAdminHandler.Delete)
			monitors.POST("/:id/run", p.monitorAdminHandler.RunNow)
			monitors.GET("/:id/history", p.monitorAdminHandler.History)
		}

		// Channel monitor request templates (picker + CRUD). Frontend
		// client lives at frontend/src/api/admin/channelMonitorTemplate.ts
		// and hits these 7 endpoints under the same /admin prefix.
		if p.monitorTemplateHandler != nil {
			templates := admin.Group("/channel-monitor-templates")
			templates.GET("", p.monitorTemplateHandler.List)
			templates.POST("", p.monitorTemplateHandler.Create)
			templates.GET("/:id", p.monitorTemplateHandler.Get)
			templates.PUT("/:id", p.monitorTemplateHandler.Update)
			templates.DELETE("/:id", p.monitorTemplateHandler.Delete)
			templates.POST("/:id/apply", p.monitorTemplateHandler.Apply)
			templates.GET("/:id/monitors", p.monitorTemplateHandler.AssociatedMonitors)
		}
	}

	// User-facing channel monitor routes.
	if p.monitorUserHandler != nil {
		users := p.engine.Group(pluginRoutePrefix + "/monitors")
		users.GET("", p.monitorUserHandler.List)
		users.GET("/:id", p.monitorUserHandler.GetStatus)
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

// OpenMigration implements pluginsdk.MigrationProvider so the SDK runner
// can serve migration bodies to the host's MigrationRunner via
// PluginLifecycle.GetMigration. Each filename listed in
// Manifest.Migrations is resolved against the embedded monitorMigrations FS
// rooted at plugins/channel-management/migrations/. Unknown filenames
// surface as fs.ErrNotExist so the SDK can translate them into
// codes.NotFound.
func (p *ChannelPlugin) OpenMigration(filename string) ([]byte, error) {
	return monitorMigrations.ReadFile("migrations/" + filename)
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
