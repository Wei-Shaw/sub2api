// manifest_decls.go owns the static manifest data the plugin publishes. Moving
// the giant literal slices out of ChannelPlugin.Manifest() (T49) lets the
// Manifest() body stay a thin assembler: add/remove an endpoint, menu item or
// migration by editing the slice here, not by scrolling through 240-line
// inline literal.
//
// Ordering matters for two of these:
//   - migrationDecls is applied in lexicographical filename order (the host's
//     MigrationRunner sorts the slice before fetching bytes), so adding a new
//     migration appends at the end naturally.
//   - menuItemDecls Placement.Order controls sidebar ordering; see the host's
//     Placement DSL doc for the reserved bucket -> order rules.
package main

import (
	"net/http"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// endpointDecls enumerates the HTTP endpoints the plugin publishes through
// the host gateway. Paths include the pluginRoutePrefix so the host can
// forward by exact-prefix match without a second lookup table.
var endpointDecls = []pluginsdk.EndpointDecl{
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
	// Admin: channel-monitor request templates (reusable headers / body
	// override snapshots). Feeds the template picker ported in W14+.
	{Path: pluginRoutePrefix + "/admin/channel-monitor-templates", Methods: []string{http.MethodGet, http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/channel-monitor-templates/:id", Methods: []string{http.MethodGet, http.MethodPut, http.MethodDelete}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/channel-monitor-templates/:id/apply", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/channel-monitor-templates/:id/monitors", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
	// User-facing channel monitor read-only endpoints
	{Path: pluginRoutePrefix + "/monitors", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/monitors/:id", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
	// User: available channels (read-only, scoped by V4 X-Plugin-User-* headers, V5 W8)
	{Path: pluginRoutePrefix + "/available-channels", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
}

// capabilityDecls lists the host capabilities this plugin needs. Changes here
// must land in sync with the host's plugin_capabilities table — the operator
// approval flow rejects unrecognised names.
//
// channel-management writes the gateway cache contract (plugin:channel:meta:*,
// plugin:channel:mapping:*) which the core's ChannelCacheReader reads
// directly. Those keys live outside the per-plugin namespace, so we ask the
// core for raw key access via CapabilityRedisRaw (P12·B-1 dotted naming).
//
// P4 拆分: pricing (K2 / K3) 已经从 Redis 通道下线, 改由 PricingExtension
// gRPC stream 推送到 host in-memory cache. 但 meta (K1) 与 mapping (K4 / K5)
// 因为字段不在 PricingOverride proto 内, 仍走 Redis, 因此 RedisRaw capability
// 必须保留。
//
// CapabilitySecretsEncrypt / CapabilityJobsRegister / CapabilitySettingsOwnRead
// are required by the channel-monitor sub-feature (W6): api_key encryption,
// periodic check scheduling, and the admin-tunable feature flag / interval
// defaults.
//
// CapabilityDBOwnRead / CapabilityDBOwnWrite are mandatory under the SQL
// gate (P12·B-1) for queries against this plugin's owned tables.
// CapabilityDBCoreRead unlocks the host's shared whitelist (groups,
// user_allowed_groups, user_subscriptions) which available_channels_repo
// reads to compute per-user allowed channels.
var capabilityDecls = []string{
	pluginsdk.CapabilityRedisRaw,
	pluginsdk.CapabilitySecretsEncrypt,
	pluginsdk.CapabilityJobsRegister,
	pluginsdk.CapabilitySettingsOwnRead,
	pluginsdk.CapabilityDBOwnRead,
	pluginsdk.CapabilityDBOwnWrite,
	pluginsdk.CapabilityDBCoreRead,
}

// ownedTableDecls feeds the host's SQL gate (P12·B-1): the gate consults
// this list to decide whether a plugin query is targeting its own tables vs
// the host shared whitelist. Every CREATE TABLE in
// plugins/channel-management/migrations/*.sql plus the channel CRUD tables
// already present in the host schema (channels, channel_groups,
// channel_model_pricing, channel_pricing_intervals — pre-existing tables
// this plugin took ownership of when V5 W6/W7 ported channel CRUD into the
// plugin).
var ownedTableDecls = []string{
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
	"channel_account_stats_pricing_rules",
	"channel_account_stats_model_pricing",
}

// migrationDecls enumerates the SQL files shipped under
// plugins/channel-management/migrations/. The host calls
// PluginLifecycle.GetMigration for each entry, re-verifies the SHA-256
// against the bytes returned by OpenMigration in plugin.go, and applies
// them in lexicographical order under the plugin_migrations advisory lock.
//
// Append-only: never reorder or rewrite an existing file in place — the
// checksum pin is part of plugin_migrations history. To fix a shipped
// migration, add a new follow-up file that corrects state. See V5-DESIGN
// §1 (W1) for the full lifecycle.
var migrationDecls = []pluginsdk.MigrationDecl{
	{Filename: "001_add_channel_monitors.sql", ChecksumSha256: "0f91517a747bf9b604a2e1c98e7f602d70cd14d43233cb16bebc379384336302"},
	{Filename: "002_add_channel_monitor_aggregation.sql", ChecksumSha256: "4e6b3e94aaed169dd7a6a4e69aa61794779e943d617b78540210bba93063abfc"},
	{Filename: "003_drop_channel_monitor_deleted_at.sql", ChecksumSha256: "2f5c2f951c2b59ed706841135de6458093a52eff970d852aec5dd60d99f868d4"},
	{Filename: "004_add_channel_monitor_request_templates.sql", ChecksumSha256: "a35f2e016afe5fe4019a2aad70184eb20104c8c248a60e5ba763ebd888280ca1"},
	// 005-012: channel CRUD/pricing tables migrated from host
	// backend/migrations/{081,082,083,084,085,086,088,095}. All DDL is
	// idempotent (CREATE TABLE IF NOT EXISTS / ADD COLUMN IF NOT EXISTS),
	// so environments where the host already applied the originals will
	// see these as no-ops while still recording a plugin_migrations row
	// with checksum. 008 is the split variant of host 084: the
	// ALTER usage_logs ADD channel_id|model_mapping_chain|billing_tier
	// lines were dropped because usage_logs is a host-owned table; the
	// host migration keeps those ALTERs.
	{Filename: "005_create_channels.sql", ChecksumSha256: "4eb880b780b2a1df07ec8dff5a9cd1ecf5cd0d35119051018f3ed3b4ae594285"},
	{Filename: "006_refactor_channel_pricing.sql", ChecksumSha256: "fffe03fdbe1c5341976d2eedcaaea988dde974539d374045c7dcadf6b554b6ca"},
	{Filename: "007_channel_model_mapping.sql", ChecksumSha256: "7d9caceeef2829e141279bf7be168a21ae80166316488902449b41fbf18d1173"},
	{Filename: "008_channel_billing_model_source.sql", ChecksumSha256: "45bd859c8181e1fdb6f198b1f71c889335cfabde85ab6db42c9591e69478b640"},
	{Filename: "009_channel_restrict_and_per_request_price.sql", ChecksumSha256: "d2fa05f3d6f0d1f0108bd0a3c55e162841e0d115f433a16c0df29b8352dd6646"},
	{Filename: "010_channel_platform_pricing.sql", ChecksumSha256: "86ccfa03178144a658276c11ab0cd1bf8c6f435cf4ec6058bec727658b92e027"},
	{Filename: "011_channel_billing_model_source_channel_mapped.sql", ChecksumSha256: "f384ea6228cd862c80312c05a584c84617a3d68e223f71ee0d07d8bc6362e480"},
	{Filename: "012_channel_features.sql", ChecksumSha256: "3f13e9f362f5c1f88a07f7db6374cd3f66a47798b865a50ba39160f4445edf81"},
	// 013: account-stats custom pricing rules. Adds the
	// apply_pricing_to_account_stats toggle on channels (already
	// plugin-owned) plus channel_account_stats_pricing_rules /
	// channel_account_stats_model_pricing. usage_logs.account_stats_cost
	// is added by the host (host-owned table) via
	// backend/migrations/106_add_usage_logs_account_stats_cost.sql.
	{Filename: "013_add_account_stats_pricing.sql", ChecksumSha256: "92c29e6a31bcfd8c57f939dc442979c3622cca6c8eefffd54ecc9f10b8ad305d"},
	// 014: add image_output_price to channel_pricing_intervals so each tiered
	// band carries its own image-output rate (mirrors channel_model_pricing).
	// Unblocks the gRPC pricing publish path.
	{Filename: "014_pricing_interval_image_output_price.sql", ChecksumSha256: "8cd2246f00959379fa6d7be29ad0841e3f72ed2e35597add4ef9969602fae47e"},
	// 015: seed Claude Code request template (upstream a7415d4d2 port).
	// Uses ON CONFLICT DO NOTHING so re-applying is safe; operators can
	// customize the row after seed.
	{Filename: "015_seed_claude_code_template.sql", ChecksumSha256: "1c8ebe1e32033a5bd179038b3423b911fefd6e652a24fbc1b356292b8811b367"},
	// 016: add features_config JSONB column (upstream v0.1.115 port).
	// Uses IF NOT EXISTS so environments where the host migration 101
	// already ran see this as a no-op.
	{Filename: "016_add_channel_features_config.sql", ChecksumSha256: "1118c6694ba27909809e9bd1dbc6c1226ba478ad8bdce749404ac2075436111b"},
}

// menuItemDecls is the admin / user sidebar contribution. Placement groups
// and orders interoperate with the host's Placement DSL (see the V5/W7
// section of V5-DESIGN for the reserved buckets).
var menuItemDecls = []pluginsdk.MenuItemDecl{
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
	// User-facing "available channels" entry. The actual Vue component
	// is added in W9; declaring the menu item now lets the host SPA pick
	// it up the moment the frontend bundle rebuilds, with no further
	// backend change required.
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
}

// routeDecls maps SPA URLs onto Vue component paths. The matching file ships
// in the frontend/dist/entry.js install() export; the host SPA's PluginView
// resolves ComponentPath against that table at navigation time.
//
// 关于 Header 标题:
//
//	不在 RouteDecl.Meta 里填 titleKey/descriptionKey. 原因是 plugin
//	i18n 通过 install() -> sdk.i18n.registerNamespace 运行时合并,
//	而 install 只在 PluginView 首次加载 entry.js 之后才执行;
//	AppHeader 计算 pageTitle 时 i18n 资源可能还没就位, t() 会原样
//	返回 i18n key. 因此标题统一走 host loader 的兜底链路:
//	  menu_items[].labels (manifest 同步注入)
//	    -> route.meta.pluginLabels[locale]
//	    -> AppHeader pageTitle.
var routeDecls = []pluginsdk.RouteDecl{
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
	// declaration is harmless until then because vue-router will simply
	// 404 when the component map is empty.
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
}
