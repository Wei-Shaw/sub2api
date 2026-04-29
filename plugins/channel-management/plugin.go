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

	monitorHandler "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/handler"
	monitorRepo "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/repository"
	monitorService "github.com/Wei-Shaw/sub2api/plugins/channel-management/monitor/service"

	"github.com/gin-gonic/gin"
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

	// engine is the Gin HTTP engine the SDK mounts under pluginRoutePrefix.
	// It is built once at construction time so that RegisterHTTP (called
	// before Init) can hand it to the SDK and Init can plug services into it.
	engine *gin.Engine

	channelHandler          *chHandler.ChannelHandler
	availableChannelHandler *chHandler.AvailableChannelHandler

	// monitor* fields back the channel-monitor sub-feature ported in V5 W6.
	// They are nil until Init wires them so HealthCheck can disambiguate
	// freshly-spawned vs initialised processes.
	monitorService      *monitorService.ChannelMonitorService
	monitorAdminHandler *monitorHandler.AdminHandler
	monitorUserHandler  *monitorHandler.UserHandler
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
		// IconSVG opts the plugin card on the admin plugins page into using
		// the channel/branch icon so the visual matches the sidebar entry.
		IconSVG: pluginsdk.IconBranchFork,
		PluginEndpoints: []pluginsdk.EndpointDecl{
			// Admin: channel CRUD
			{Path: pluginRoutePrefix + "/admin/channels", Methods: []string{http.MethodGet, http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/admin/channels/:id", Methods: []string{http.MethodGet, http.MethodPut, http.MethodDelete}, AuthType: pluginsdk.AuthTypeAdmin},
			// Admin: model pricing helper
			{Path: pluginRoutePrefix + "/admin/channels/model-pricing", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},

			// Admin: channel-monitor CRUD + run-now + history (V5 W6)
			{Path: pluginRoutePrefix + "/admin/monitors", Methods: []string{http.MethodGet, http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/admin/monitors/:id", Methods: []string{http.MethodGet, http.MethodPut, http.MethodDelete}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/admin/monitors/:id/run", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
			{Path: pluginRoutePrefix + "/admin/monitors/:id/history", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
			// User-facing channel monitor read-only endpoints
			{Path: pluginRoutePrefix + "/monitors", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
			{Path: pluginRoutePrefix + "/monitors/:id", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
			// User: available channels (read-only, scoped by V4 X-Plugin-User-* headers, V5 W8)
			{Path: pluginRoutePrefix + "/available-channels", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
		},
		// Capabilities — channel-management writes the gateway cache contract
		// (channel:active, channel:by_id:*, …) which the core's
		// ChannelCacheReader reads directly. Those keys live outside the
		// per-plugin namespace, so we ask the core for raw key access via
		// CapabilityRedisRaw (P12·B-1 dotted naming).
		//
		// CapabilitySecretsEncrypt / CapabilityJobsRegister /
		// CapabilitySettingsOwnRead are required by the channel-monitor
		// sub-feature (W6): api_key encryption, periodic check scheduling,
		// and the admin-tunable feature flag / interval defaults.
		//
		// CapabilityDBOwnRead / CapabilityDBOwnWrite are mandatory under the
		// SQL gate (P12·B-1) for queries against this plugin's owned tables.
		// CapabilityDBCoreRead unlocks the host's shared whitelist (groups,
		// user_allowed_groups, user_subscriptions) which available_channels_repo
		// reads to compute per-user allowed channels.
		Capabilities: []string{
			pluginsdk.CapabilityRedisRaw,
			pluginsdk.CapabilitySecretsEncrypt,
			pluginsdk.CapabilityJobsRegister,
			pluginsdk.CapabilitySettingsOwnRead,
			pluginsdk.CapabilityDBOwnRead,
			pluginsdk.CapabilityDBOwnWrite,
			pluginsdk.CapabilityDBCoreRead,
		},
		// OwnedTables: the SQL gate (P12·B-1) consults this list to decide
		// whether a plugin query is targeting its own tables vs the host
		// shared whitelist. Every CREATE TABLE in plugins/channel-management/
		// migrations/*.sql plus the channel CRUD tables already present in
		// the host schema (channels, channel_groups, channel_model_pricing,
		// channel_pricing_intervals — pre-existing tables this plugin took
		// ownership of when V5 W6/W7 ported channel CRUD into the plugin).
		OwnedTables: []string{
			// pre-existing host tables this plugin now owns
			"channels",
			"channel_groups",
			"channel_model_pricing",
			"channel_pricing_intervals",
			// tables created by plugins/channel-management/migrations/
			"channel_monitors",
			"channel_monitor_histories",
			"channel_monitor_daily_rollups",
			"channel_monitor_aggregation_watermark",
			"channel_monitor_request_templates",
		},
		// Migrations enumerates the SQL files shipped under
		// plugins/channel-management/migrations/. The host calls
		// PluginLifecycle.GetMigration for each entry, re-verifies the
		// SHA-256 against the bytes returned by OpenMigration below, and
		// applies them in lexicographical order under the
		// plugin_migrations advisory lock.
		//
		// Append-only: never reorder or rewrite an existing file in place
		// — the checksum pin is part of plugin_migrations history. To fix a
		// shipped migration, add a new follow-up file that corrects state.
		// See V5-DESIGN §1 (W1) for the full lifecycle.
		Migrations: []pluginsdk.MigrationDecl{
			{Filename: "001_add_channel_monitors.sql", ChecksumSha256: "0f91517a747bf9b604a2e1c98e7f602d70cd14d43233cb16bebc379384336302"},
			{Filename: "002_add_channel_monitor_aggregation.sql", ChecksumSha256: "4e6b3e94aaed169dd7a6a4e69aa61794779e943d617b78540210bba93063abfc"},
			{Filename: "003_drop_channel_monitor_deleted_at.sql", ChecksumSha256: "2f5c2f951c2b59ed706841135de6458093a52eff970d852aec5dd60d99f868d4"},
			{Filename: "004_add_channel_monitor_request_templates.sql", ChecksumSha256: "a35f2e016afe5fe4019a2aad70184eb20104c8c248a60e5ba763ebd888280ca1"},
		},
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
			EntryCSS: "dist/entry.css",
			MenuItems: []pluginsdk.MenuItemDecl{
				// Admin "渠道管理" parent — owns 渠道定价 (channel CRUD/pricing in
				// ChannelsView) 和 渠道监控 (V5 W7 ChannelMonitorView). V5 整体迁移
				// 到插件后, host sidebar 已删除硬编码 /admin/channels 顶级项,
				// 由 plugin 提供唯一入口.
				{
					Path:    "/admin/channels",
					IconSVG: pluginsdk.IconBranchFork,
					Labels:  pluginsdk.Labels("渠道管理", "Channel Management"),
					Section: pluginsdk.SectionAdmin,
					// V5/W7 Placement DSL — 把渠道管理放在 admin/main 桶内
					// /admin/accounts (host placementOrder=60) 之后, 在
					// /admin/announcements (placementOrder=80) 之前。host 给
					// base item 显式 placementOrder 后, 70 让"渠道管理"直接
					// 相邻于"账号管理", 形成核心业务相邻的菜单顺序。
					Placement:     &pluginsdk.Placement{Group: pluginsdk.PlacementAdminMain, Order: 70},
					SortOrder:     200,
					RequiresAdmin: true,
					Children: []pluginsdk.MenuItemDecl{
						{
							Path:    "/admin/channels",
							IconSVG: pluginsdk.IconTag,
							Labels:  pluginsdk.Labels("渠道定价", "Channel Pricing"),
							// Descriptions 走 manifest 提供 AppHeader 副标题,
							// view 内 PluginPageLayout 不再自己渲染 title/description。
							Descriptions:  pluginsdk.Descriptions("管理渠道和自定义模型定价", "Manage channels and custom model pricing"),
							Section:       pluginsdk.SectionAdmin,
							SortOrder:     210,
							RequiresAdmin: true,
						},
						{
							Path:          channelMonitorAdminFrontendPath,
							IconSVG:       pluginsdk.IconCog,
							Labels:        pluginsdk.Labels("渠道监控", "Channel Monitor"),
							Descriptions:  pluginsdk.Descriptions("监控渠道可用性、延迟和状态", "Monitor channel availability, latency, and status"),
							Section:       pluginsdk.SectionAdmin,
							SortOrder:     220,
							RequiresAdmin: true,
						},
					},
				},
				// User-facing "available channels" entry. The actual Vue
				// component is added in W9; declaring the menu item now lets
				// the host SPA pick it up the moment the frontend bundle
				// rebuilds, with no further backend change required.
				{
					Path:         availableChannelsFrontendPath,
					IconSVG:      pluginsdk.IconBranchFork,
					Labels:       pluginsdk.Labels("可用渠道", "Available Channels"),
					Descriptions: pluginsdk.Descriptions("您可以访问的渠道及其支持的模型与定价", "Channels you can access with their supported models and pricing"),
					Section:      pluginsdk.SectionUser,
					// Placement: user/main 桶内排在 50, 让"可用渠道"作为业务
					// 主菜单的一部分跟在 host 的 dashboard/keys 等之后。
					Placement:     &pluginsdk.Placement{Group: pluginsdk.PlacementUserMain, Order: 50},
					SortOrder:     200,
					RequiresAdmin: false,
				},
				// User-facing read-only "Channel Status" view (V5 W7.3).
				{
					Path:         channelStatusFrontendPath,
					IconSVG:      pluginsdk.IconCog,
					Labels:       pluginsdk.Labels("渠道状态", "Channel Status"),
					Descriptions: pluginsdk.Descriptions("查看渠道可用性、延迟和近期状态", "View channel availability, latency, and recent status"),
					Section:      pluginsdk.SectionUser,
					// Placement: 渠道状态属于"次级信息", 落入 user/end 桶,
					// 跟在 redeem/profile 后面但仍排在自定义菜单之前。
					Placement:     &pluginsdk.Placement{Group: pluginsdk.PlacementUserEnd, Order: 10},
					SortOrder:     210,
					RequiresAdmin: false,
				},
			},
			Routes: []pluginsdk.RouteDecl{
				// 关于 Header 标题:
				//   不在 RouteDecl.Meta 里填 titleKey/descriptionKey. 原因是 plugin
				//   i18n 通过 install() -> sdk.i18n.registerNamespace 运行时合并,
				//   而 install 只在 PluginView 首次加载 entry.js 之后才执行;
				//   AppHeader 计算 pageTitle 时 i18n 资源可能还没就位, t() 会原样
				//   返回 i18n key. 因此标题统一走 host loader 的兜底链路:
				//     menu_items[].labels (manifest 同步注入)
				//       -> route.meta.pluginLabels[locale]
				//       -> AppHeader pageTitle.
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
				// Admin Channel Monitor route (V5 W7).
				{
					Path:          channelMonitorAdminFrontendPath,
					Name:          "AdminChannelMonitor",
					ComponentPath: "ChannelMonitorView.vue",
				},
				// User-facing Channel Status route (V5 W7.3).
				{
					Path:          channelStatusFrontendPath,
					Name:          "UserChannelStatus",
					ComponentPath: "ChannelStatusView.vue",
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

	// Wire the channel-monitor sub-feature (V5 W6). Repository / handler are
	// stub-backed for now (every method returns ErrNotPorted) so HTTP routes
	// respond 501 — the wiring smoke-tests the manifest, capabilities, and
	// migration declarations end-to-end while the heavier implementations
	// land in subsequent commits.
	monRepo := monitorRepo.NewChannelMonitorRepository(ctx.DB())
	var monEncryptor monitorService.SecretEncryptor
	if secrets := ctx.Secrets(); secrets != nil {
		monEncryptor = monitorService.NewSDKSecretEncryptor(context.Background(), secrets)
	} else {
		ctx.Logger().Warn("channel-monitor encryption disabled: SDK Secrets() unavailable; api keys will fail to encrypt — check CapabilitySecretEncryption is in plugin manifest")
	}
	p.monitorService = monitorService.NewChannelMonitorService(monRepo, monEncryptor)
	p.monitorAdminHandler = monitorHandler.NewAdminHandler(p.monitorService)
	p.monitorUserHandler = monitorHandler.NewUserHandler(p.monitorService, ctx.Settings())

	// Register the channel-monitor JobScheduler specs (V5 W6 step 3). The
	// host fires monitor.run every 60s on each replica and the leader-only
	// monitor.daily-rollup once per day. The runner replaces the legacy
	// in-process ticker pool — concurrency, leader election and history
	// are all owned by the host.
	jobRunner := monitorService.NewMonitorJobRunner(p.monitorService, ctx.Jobs(), ctx.Logger())
	if err := jobRunner.Register(); err != nil {
		ctx.Logger().Warn("channel-monitor: job registration failed; periodic checks disabled", "error", err)
	}
	p.monitorService.SetScheduler(jobRunner)

	// Wire the user-facing "available channels" view (V5 W8). It reuses the
	// same channel repository for ListAll and adds a small read-only
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
