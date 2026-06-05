package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/gateway"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
)

// AdminHandlers contains all admin-related HTTP handlers
type AdminHandlers struct {
	Dashboard             *admin.DashboardHandler
	User                  *admin.UserHandler
	Group                 *admin.GroupHandler
	Account               *admin.AccountHandler
	Announcement          *admin.AnnouncementHandler
	DataManagement        *admin.DataManagementHandler
	Backup                *admin.BackupHandler
	OAuth                 *admin.OAuthHandler
	Proxy                 *admin.ProxyHandler
	Redeem                *admin.RedeemHandler
	Promo                 *admin.PromoHandler
	Setting               *admin.SettingHandler
	Ops                   *admin.OpsHandler
	System                *admin.SystemHandler
	Subscription          *admin.SubscriptionHandler
	Usage                 *admin.UsageHandler
	UserAttribute         *admin.UserAttributeHandler
	ErrorPassthrough      *admin.ErrorPassthroughHandler
	TLSFingerprintProfile *admin.TLSFingerprintProfileHandler
	APIKey                *admin.AdminAPIKeyHandler
	ScheduledTest         *admin.ScheduledTestHandler
	// Channel/ChannelMonitor/ChannelMonitorTemplate handlers 已迁移到 plugins/channel-management/
	// Payment handler 已迁移到 plugins/payment/
	// ContentModeration handler 已迁移到 plugins/content-moderation/
	ServiceQuota        *admin.ServiceQuotaHandler
	ServiceQuotaMonitor *admin.ServiceQuotaMonitorHandler
	Affiliate           *admin.AffiliateHandler
	Plugin              *admin.PluginHandler
	Platform            *admin.PlatformHandler
	PluginSettings      *admin.PluginSettingsHandler
	Upload              *admin.UploadHandler
}

// Handlers contains all HTTP handlers
type Handlers struct {
	Auth          *AuthHandler
	User          *UserHandler
	APIKey        *APIKeyHandler
	Usage         *UsageHandler
	Redeem        *RedeemHandler
	Subscription  *SubscriptionHandler
	Announcement  *AnnouncementHandler
	Admin         *AdminHandlers
	Gateway       *GatewayHandler
	OpenAIGateway *OpenAIGatewayHandler
	Setting       *SettingHandler
	Totp          *TotpHandler
	// Payment / PaymentWebhook handlers 已迁移到 plugins/payment/
	UserServiceQuota *UserServiceQuotaHandler
	// AvailableChannel / ChannelMonitor (user) 已迁移到 plugins/channel-management/
	Upload *UploadHandler

	// GatewayPipeline is the Phase 1 M5 gateway pipeline. Currently held
	// for compile-time validation only; handler methods continue to use
	// their existing per-platform dispatch logic. Future milestones will
	// progressively route endpoints through the pipeline.
	GatewayPipeline *gateway.GatewayPipeline
}

// BuildInfo contains build-time information
type BuildInfo struct {
	Version   string
	BuildType string // "source" for manual builds, "release" for CI builds
}
