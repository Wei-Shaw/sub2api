package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/gateway"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/plugin"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
)

// ProvideAdminHandlers creates the AdminHandlers struct
func ProvideAdminHandlers(
	dashboardHandler *admin.DashboardHandler,
	userHandler *admin.UserHandler,
	groupHandler *admin.GroupHandler,
	accountHandler *admin.AccountHandler,
	announcementHandler *admin.AnnouncementHandler,
	dataManagementHandler *admin.DataManagementHandler,
	backupHandler *admin.BackupHandler,
	oauthHandler *admin.OAuthHandler,
	proxyHandler *admin.ProxyHandler,
	redeemHandler *admin.RedeemHandler,
	promoHandler *admin.PromoHandler,
	settingHandler *admin.SettingHandler,
	opsHandler *admin.OpsHandler,
	systemHandler *admin.SystemHandler,
	subscriptionHandler *admin.SubscriptionHandler,
	usageHandler *admin.UsageHandler,
	userAttributeHandler *admin.UserAttributeHandler,
	errorPassthroughHandler *admin.ErrorPassthroughHandler,
	tlsFingerprintProfileHandler *admin.TLSFingerprintProfileHandler,
	apiKeyHandler *admin.AdminAPIKeyHandler,
	scheduledTestHandler *admin.ScheduledTestHandler,
	serviceQuotaHandler *admin.ServiceQuotaHandler,
	serviceQuotaMonitorHandler *admin.ServiceQuotaMonitorHandler,
	affiliateHandler *admin.AffiliateHandler,
	complianceHandler *admin.ComplianceHandler,
	pluginHandler *admin.PluginHandler,
	pluginSettingsHandler *admin.PluginSettingsHandler,
	uploadHandler *admin.UploadHandler,
	platformHandler *admin.PlatformHandler,
) *AdminHandlers {
	return &AdminHandlers{
		Dashboard:             dashboardHandler,
		User:                  userHandler,
		Group:                 groupHandler,
		Account:               accountHandler,
		Announcement:          announcementHandler,
		DataManagement:        dataManagementHandler,
		Backup:                backupHandler,
		OAuth:                 oauthHandler,
		Proxy:                 proxyHandler,
		Redeem:                redeemHandler,
		Promo:                 promoHandler,
		Setting:               settingHandler,
		Ops:                   opsHandler,
		System:                systemHandler,
		Subscription:          subscriptionHandler,
		Usage:                 usageHandler,
		UserAttribute:         userAttributeHandler,
		ErrorPassthrough:      errorPassthroughHandler,
		TLSFingerprintProfile: tlsFingerprintProfileHandler,
		APIKey:                apiKeyHandler,
		ScheduledTest:         scheduledTestHandler,
		ServiceQuota:          serviceQuotaHandler,
		ServiceQuotaMonitor:   serviceQuotaMonitorHandler,
		Affiliate:             affiliateHandler,
		Compliance:            complianceHandler,
		Plugin:                pluginHandler,
		PluginSettings:        pluginSettingsHandler,
		Upload:                uploadHandler,
		Platform:              platformHandler,
	}
}

// ProvideSystemHandler creates admin.SystemHandler with UpdateService
func ProvideSystemHandler(updateService *service.UpdateService, lockService *service.SystemOperationLockService) *admin.SystemHandler {
	return admin.NewSystemHandler(updateService, lockService)
}

// ProvideSettingHandler creates SettingHandler with version from BuildInfo
func ProvideSettingHandler(settingService *service.SettingService, buildInfo BuildInfo) *SettingHandler {
	return NewSettingHandler(settingService, buildInfo.Version)
}

// ProvidePluginHandler creates admin.PluginHandler.
//
// 当插件功能未启用(plugins.enabled=false)时,manager 为 nil,
// handler 内部的 requireManager 守卫会让所有插件 API 返回 503,
// 不影响其他 admin 路由。
func ProvidePluginHandler(manager *plugin.PluginManager) *admin.PluginHandler {
	if manager == nil {
		return admin.NewPluginHandler(nil)
	}
	return admin.NewPluginHandler(manager)
}

// ProvideHandlers creates the Handlers struct
func ProvideHandlers(
	authHandler *AuthHandler,
	userHandler *UserHandler,
	apiKeyHandler *APIKeyHandler,
	usageHandler *UsageHandler,
	redeemHandler *RedeemHandler,
	subscriptionHandler *SubscriptionHandler,
	announcementHandler *AnnouncementHandler,
	adminHandlers *AdminHandlers,
	gatewayHandler *GatewayHandler,
	openaiGatewayHandler *OpenAIGatewayHandler,
	settingHandler *SettingHandler,
	totpHandler *TotpHandler,
	userServiceQuotaHandler *UserServiceQuotaHandler,
	uploadHandler *UploadHandler,
	gatewayPipeline *gateway.GatewayPipeline,
	_ *service.IdempotencyCoordinator,
	_ *service.IdempotencyCleanupService,
) *Handlers {
	gatewayHandler.SetPipeline(gatewayPipeline)
	openaiGatewayHandler.SetPipeline(gatewayPipeline)
	return &Handlers{
		Auth:             authHandler,
		User:             userHandler,
		APIKey:           apiKeyHandler,
		Usage:            usageHandler,
		Redeem:           redeemHandler,
		Subscription:     subscriptionHandler,
		Announcement:     announcementHandler,
		Admin:            adminHandlers,
		Gateway:          gatewayHandler,
		OpenAIGateway:    openaiGatewayHandler,
		Setting:          settingHandler,
		Totp:             totpHandler,
		UserServiceQuota: userServiceQuotaHandler,
		Upload:           uploadHandler,
		GatewayPipeline:  gatewayPipeline,
	}
}

// ProvidePlatformHandler creates admin.PlatformHandler.
// When plugins are disabled (manager is nil), the handler returns empty lists.
func ProvidePlatformHandler(manager *plugin.PluginManager) *admin.PlatformHandler {
	if manager == nil {
		return admin.NewPlatformHandler(plugin.NewPlatformRegistry())
	}
	return admin.NewPlatformHandler(manager.PlatformRegistry())
}

// ProviderSet is the Wire provider set for all handlers
var ProviderSet = wire.NewSet(
	// Top-level handlers
	NewAuthHandler,
	NewUserHandler,
	NewAPIKeyHandler,
	NewUsageHandler,
	NewRedeemHandler,
	NewSubscriptionHandler,
	NewAnnouncementHandler,
	NewGatewayHandler,
	NewOpenAIGatewayHandler,
	NewTotpHandler,
	ProvideSettingHandler,
	NewUserServiceQuotaHandler,
	NewUploadHandler,

	// Admin handlers
	admin.NewDashboardHandler,
	admin.NewUserHandler,
	admin.NewGroupHandler,
	admin.NewAccountHandler,
	admin.NewAnnouncementHandler,
	admin.NewDataManagementHandler,
	admin.NewBackupHandler,
	admin.NewOAuthHandler,
	admin.NewProxyHandler,
	admin.NewRedeemHandler,
	admin.NewPromoHandler,
	admin.NewSettingHandler,
	admin.NewOpsHandler,
	ProvideSystemHandler,
	admin.NewSubscriptionHandler,
	admin.NewUsageHandler,
	admin.NewUserAttributeHandler,
	admin.NewErrorPassthroughHandler,
	admin.NewTLSFingerprintProfileHandler,
	admin.NewAdminAPIKeyHandler,
	admin.NewScheduledTestHandler,
	// admin.NewChannelHandler / NewChannelMonitorHandler / NewChannelMonitorRequestTemplateHandler 已迁移到 plugins/channel-management/
	// admin.NewPaymentHandler 已迁移到 plugins/payment/
	// admin.NewContentModerationHandler 已迁移到 plugins/content-moderation/
	admin.NewServiceQuotaHandler,
	admin.NewServiceQuotaMonitorHandler,
	admin.NewAffiliateHandler,
	admin.NewComplianceHandler,
	admin.NewUploadHandler,
	ProvidePluginHandler,
	ProvidePluginSettingsHandler,
	ProvidePlatformHandler,

	// AdminHandlers and Handlers constructors
	ProvideAdminHandlers,
	ProvideHandlers,
)

// ProvidePluginSettingsHandler creates admin.PluginSettingsHandler.
//
// 当插件功能未启用时,settingsService 可能为 nil,
// handler 内部的 requireService 守卫会让所有接口返回 503,
// 不影响其他 admin 路由。
func ProvidePluginSettingsHandler(settingsService *service.PluginSettingsService) *admin.PluginSettingsHandler {
	if settingsService == nil {
		return admin.NewPluginSettingsHandler(nil)
	}
	return admin.NewPluginSettingsHandler(settingsService)
}
