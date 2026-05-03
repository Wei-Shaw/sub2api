// manifest_decls.go owns the static manifest data the plugin publishes.
// Moving the slice literals out of PaymentPlugin.Manifest() keeps the
// Manifest() body a thin assembler: add/remove an endpoint, menu item or
// migration by editing the slice here, not by scrolling through a giant
// inline literal.
//
// Ordering matters for two of these:
//   - migrationDecls is applied in lexicographical filename order (the
//     host's MigrationRunner sorts the slice before fetching bytes), so
//     adding a new migration appends at the end naturally.
//   - menuItemDecls Placement.Order controls sidebar ordering; see the
//     host's Placement DSL doc for the reserved bucket -> order rules.
package main

import (
	"net/http"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

// endpointDecls enumerates the HTTP endpoints the plugin publishes through
// the host gateway. Paths include the pluginRoutePrefix so the host can
// forward by exact-prefix match without a second lookup table.
var endpointDecls = []pluginsdk.EndpointDecl{
	// -------- User-facing (auth=user) --------
	{Path: pluginRoutePrefix + "/config", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/checkout-info", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/plans", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/limits", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/orders", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/orders/verify", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/orders/my", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/orders/refund-eligible-providers", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/orders/:id", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/orders/:id/cancel", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeUser},
	{Path: pluginRoutePrefix + "/orders/:id/refund-request", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeUser},

	// -------- Public (auth=none) — webhooks + resume lookup --------
	{Path: pluginRoutePrefix + "/public/orders/verify", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeNone},
	{Path: pluginRoutePrefix + "/public/orders/resolve", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeNone},
	{Path: pluginRoutePrefix + "/webhook/easypay", Methods: []string{http.MethodGet, http.MethodPost}, AuthType: pluginsdk.AuthTypeNone},
	{Path: pluginRoutePrefix + "/webhook/alipay", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeNone},
	{Path: pluginRoutePrefix + "/webhook/wxpay", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeNone},
	{Path: pluginRoutePrefix + "/webhook/stripe", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeNone},

	// -------- Admin (auth=admin) --------
	{Path: pluginRoutePrefix + "/admin/dashboard", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/config", Methods: []string{http.MethodGet, http.MethodPut}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/orders", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/orders/:id", Methods: []string{http.MethodGet}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/orders/:id/cancel", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/orders/:id/retry", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/orders/:id/refund", Methods: []string{http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/plans", Methods: []string{http.MethodGet, http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/plans/:id", Methods: []string{http.MethodPut, http.MethodDelete}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/providers", Methods: []string{http.MethodGet, http.MethodPost}, AuthType: pluginsdk.AuthTypeAdmin},
	{Path: pluginRoutePrefix + "/admin/providers/:id", Methods: []string{http.MethodPut, http.MethodDelete}, AuthType: pluginsdk.AuthTypeAdmin},
}

// capabilityDecls lists the host capabilities this plugin needs. Changes
// here must land in sync with the host's plugin_capabilities table — the
// operator approval flow rejects unrecognised names.
//
// CapabilitySecretsEncrypt is required to encrypt provider configs at
// rest (replacing the legacy shared TOTP encryption key). CapabilityOutboundHTTP
// is required for Stripe / Alipay / WeChat API calls. CapabilityJobsRegister
// powers the order-expiry sweep that replaces the legacy in-process
// PaymentOrderExpiryService ticker. CapabilityDBCoreRead unlocks the host's
// shared whitelist (users, subscription_plans, groups) which the plugin
// reads when computing fulfillment / plan visibility. CapabilityEventsPublishPayment
// gates EventsExtension.Publish for payment.* events.
var capabilityDecls = []string{
	pluginsdk.CapabilitySecretsEncrypt,
	pluginsdk.CapabilityOutboundHTTP,
	pluginsdk.CapabilityJobsRegister,
	pluginsdk.CapabilitySettingsOwnRead,
	pluginsdk.CapabilityDBOwnRead,
	pluginsdk.CapabilityDBOwnWrite,
	pluginsdk.CapabilityDBCoreRead,
	pluginsdk.CapabilityEventsPublishPayment,
}

// ownedTableDecls feeds the host's SQL gate: the gate consults this list
// to decide whether a plugin query is targeting its own tables vs the
// host shared whitelist. Every CREATE TABLE in plugins/payment/migrations
// must appear here.
var ownedTableDecls = []string{
	"payment_orders",
	"payment_audit_logs",
	"payment_provider_instances",
}

// migrationDecls enumerates the SQL files shipped under
// plugins/payment/migrations/. The host calls
// PluginLifecycle.GetMigration for each entry, re-verifies the SHA-256
// against the bytes returned by OpenMigration in plugin.go, and applies
// them in lexicographical order under the plugin_migrations advisory
// lock.
//
// Append-only: never reorder or rewrite an existing file in place — the
// checksum pin is part of plugin_migrations history.
var migrationDecls = []pluginsdk.MigrationDecl{
	{Filename: "001_payment_orders.sql", ChecksumSha256: "f4fa67bd0b246eb04b70ec23d2bcf5d1fc16f546c030a0df66fc2a6f3276f317"},
	{Filename: "002_payment_audit_logs.sql", ChecksumSha256: "fefa3b74154e5a26784e30ec4a738474fc217edd0e48a97e03914146b40f7f8a"},
	{Filename: "003_payment_provider_instances.sql", ChecksumSha256: "680c46cd2b1917e985823bc433b7c053910b164f41ee764aceef433fe2830191"},
	{Filename: "004_add_payment_mode.sql", ChecksumSha256: "acc645d43ca517904364e627244128a4124e50dd17b1ec092ab29c0b9e57333f"},
	{Filename: "005_add_out_trade_no_to_payment_orders.sql", ChecksumSha256: "6624aa0b8ad4cb23c89007d969482e6e94fc327390b6889018e01b0d6d2880e3"},
	{Filename: "006_payment_routing_and_scheduler_flags.sql", ChecksumSha256: "7e99ba24a6b4b23e3600d865fc38a8f21e5a7ee9c8ee5bea8e34770a38ceb461"},
	{Filename: "007_provider_key_snapshot.sql", ChecksumSha256: "b75f8f56d39455682787696a3d92ad25b055444ca328fb7fca9a460a15d68d99"},
	{Filename: "008_provider_snapshot.sql", ChecksumSha256: "69320a2f03662ea02b4a0f09ef293b1ee600dd55d474f20a1b0d6fe03f4482cc"},
	{Filename: "009_enforce_out_trade_no_unique.sql", ChecksumSha256: "66ba0ae221454ca8742770428ede26695cc5b065d0119cc9fe33fd2a26a4d38f"},
	{Filename: "010_enforce_out_trade_no_unique_notx.sql", ChecksumSha256: "39f9323ace96b835c74e8de83ba15b977d4c65609c22eecc4e9afbf382f7667d"},
	{Filename: "011_align_out_trade_no_index_name.sql", ChecksumSha256: "846d179e41eb1218698e2aa312bb280e7ed41c949dd0669ff45fc7e838a643bc"},
}

// publicFlagDecls exposes plugin-owned flags through the host's
// GetPublicSettings response. payment_enabled is consumed by the
// frontend's "show recharge button" logic; the host reads it directly
// from the plugin's settings store so a stopped plugin still honours the
// last-known value.
var publicFlagDecls = []pluginsdk.PublicFlagDecl{
	{
		Key:         "payment_enabled",
		Source:      pluginsdk.PublicFlagSourceSettings,
		SettingsKey: "enabled",
		Type:        pluginsdk.PublicFlagTypeBool,
		Default:     false,
	},
}

// menuItemDecls is the admin / user sidebar contribution. Placement
// groups and orders interoperate with the host's Placement DSL.
var menuItemDecls = []pluginsdk.MenuItemDecl{
	// Admin "支付管理" parent — owns 支付配置 + 订单 + 充值套餐 + 收款渠道.
	{
		Path:          "/admin/payment",
		IconSVG:       pluginsdk.IconCog,
		Labels:        pluginsdk.Labels("支付管理", "Payment"),
		Section:       pluginsdk.SectionAdmin,
		Placement:     &pluginsdk.Placement{Group: pluginsdk.PlacementAdminMain, Order: 75},
		SortOrder:     250,
		RequiresAdmin: true,
		Children: []pluginsdk.MenuItemDecl{
			{
				Path:          "/admin/payment/dashboard",
				Labels:        pluginsdk.Labels("仪表盘", "Dashboard"),
				Descriptions:  pluginsdk.Descriptions("支付订单与收入概览", "Order and revenue overview"),
				Section:       pluginsdk.SectionAdmin,
				SortOrder:     251,
				RequiresAdmin: true,
			},
			{
				Path:          "/admin/payment/orders",
				Labels:        pluginsdk.Labels("支付订单", "Orders"),
				Descriptions:  pluginsdk.Descriptions("查看与处理支付订单", "View and manage payment orders"),
				Section:       pluginsdk.SectionAdmin,
				SortOrder:     252,
				RequiresAdmin: true,
			},
			{
				Path:          "/admin/payment/plans",
				Labels:        pluginsdk.Labels("订阅套餐", "Subscription Plans"),
				Descriptions:  pluginsdk.Descriptions("管理可购买的订阅套餐", "Manage subscription plans available for purchase"),
				Section:       pluginsdk.SectionAdmin,
				SortOrder:     253,
				RequiresAdmin: true,
			},
			{
				Path:          "/admin/payment/providers",
				Labels:        pluginsdk.Labels("收款渠道", "Providers"),
				Descriptions:  pluginsdk.Descriptions("配置 EasyPay / 支付宝 / 微信 / Stripe", "Configure EasyPay / Alipay / WeChat / Stripe"),
				Section:       pluginsdk.SectionAdmin,
				SortOrder:     254,
				RequiresAdmin: true,
			},
			{
				Path:          "/admin/payment/config",
				Labels:        pluginsdk.Labels("支付配置", "Settings"),
				Descriptions:  pluginsdk.Descriptions("支付限额、提示文案与可见方式", "Limits, copy and visible methods"),
				Section:       pluginsdk.SectionAdmin,
				SortOrder:     255,
				RequiresAdmin: true,
			},
		},
	},
	// User-facing "充值" entry surfaces the recharge view.
	{
		Path:          "/recharge",
		IconSVG:       pluginsdk.IconCog,
		Labels:        pluginsdk.Labels("充值", "Recharge"),
		Descriptions:  pluginsdk.Descriptions("为账户余额充值", "Top up your account balance"),
		Section:       pluginsdk.SectionUser,
		Placement:     &pluginsdk.Placement{Group: pluginsdk.PlacementUserMain, Order: 60},
		SortOrder:     300,
		RequiresAdmin: false,
	},
}

// routeDecls maps SPA URLs onto Vue component paths. The matching files
// ship in the frontend/dist/entry.js install() export. Phase 2 of the
// payment plugin migration replaces the stub bundle with real components.
var routeDecls = []pluginsdk.RouteDecl{
	// Admin
	{Path: "/admin/payment/dashboard", Name: "AdminPaymentDashboard", ComponentPath: "PaymentDashboardView.vue"},
	{Path: "/admin/payment/orders", Name: "AdminPaymentOrders", ComponentPath: "PaymentOrdersView.vue"},
	{Path: "/admin/payment/plans", Name: "AdminPaymentPlans", ComponentPath: "PaymentPlansView.vue"},
	{Path: "/admin/payment/providers", Name: "AdminPaymentProviders", ComponentPath: "PaymentProvidersView.vue"},
	{Path: "/admin/payment/config", Name: "AdminPaymentConfig", ComponentPath: "PaymentConfigView.vue"},

	// User-facing
	{Path: "/recharge", Name: "UserRecharge", ComponentPath: "RechargeView.vue"},
	{Path: "/purchase", Name: "UserPurchase", ComponentPath: "PaymentView.vue"},
	{Path: "/orders", Name: "UserOrders", ComponentPath: "UserOrdersView.vue"},
	{Path: "/payment/qrcode", Name: "PaymentQRCode", ComponentPath: "PaymentQRCodeView.vue"},
	{Path: "/payment/result", Name: "PaymentResult", ComponentPath: "PaymentResultView.vue"},
	{Path: "/payment/stripe", Name: "PaymentStripe", ComponentPath: "StripePaymentView.vue"},
	{Path: "/payment/stripe-popup", Name: "PaymentStripePopup", ComponentPath: "StripePopupView.vue"},

	// Auth callback (no auth, no chrome)
	{Path: "/auth/wechat/payment/callback", Name: "WechatPaymentCallback", ComponentPath: "WechatPaymentCallbackView.vue"},
}
