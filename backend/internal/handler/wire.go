package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
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
	openaiOAuthHandler *admin.OpenAIOAuthHandler,
	geminiOAuthHandler *admin.GeminiOAuthHandler,
	antigravityOAuthHandler *admin.AntigravityOAuthHandler,
	kiroOAuthHandler *admin.KiroOAuthHandler,
	grokOAuthHandler *admin.GrokOAuthHandler,
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
	channelHandler *admin.ChannelHandler,
	channelMonitorHandler *admin.ChannelMonitorHandler,
	channelMonitorTemplateHandler *admin.ChannelMonitorRequestTemplateHandler,
	contentModerationHandler *admin.ContentModerationHandler,
	paymentHandler *admin.PaymentHandler,
	rechargePromoHandler *admin.RechargePromoHandler,
	affiliateHandler *admin.AffiliateHandler,
	supportTicketHandler *admin.SupportTicketHandler,
	supportFaqHandler *admin.SupportFaqHandler,
	supportDocIndexHandler *admin.SupportDocIndexHandler,
	oidcClientHandler *admin.OidcClientHandler,
	oidcSigningKeyHandler *admin.OidcSigningKeyHandler,
	oidcProviderSettingsHandler *admin.OidcProviderSettingsHandler,
	billingAppHandler *admin.BillingAppHandler,
	complianceHandler *admin.ComplianceHandler,
	cosImageHandler *admin.COSImageHandler,
	asyncMediaConfigHandler *admin.AsyncMediaConfigHandler,
) *AdminHandlers {
	return &AdminHandlers{
		Dashboard:              dashboardHandler,
		User:                   userHandler,
		Group:                  groupHandler,
		Account:                accountHandler,
		Announcement:           announcementHandler,
		DataManagement:         dataManagementHandler,
		Backup:                 backupHandler,
		OAuth:                  oauthHandler,
		OpenAIOAuth:            openaiOAuthHandler,
		GeminiOAuth:            geminiOAuthHandler,
		AntigravityOAuth:       antigravityOAuthHandler,
		KiroOAuth:              kiroOAuthHandler,
		GrokOAuth:              grokOAuthHandler,
		Proxy:                  proxyHandler,
		Redeem:                 redeemHandler,
		Promo:                  promoHandler,
		Setting:                settingHandler,
		Ops:                    opsHandler,
		System:                 systemHandler,
		Subscription:           subscriptionHandler,
		Usage:                  usageHandler,
		UserAttribute:          userAttributeHandler,
		ErrorPassthrough:       errorPassthroughHandler,
		TLSFingerprintProfile:  tlsFingerprintProfileHandler,
		APIKey:                 apiKeyHandler,
		ScheduledTest:          scheduledTestHandler,
		Channel:                channelHandler,
		ChannelMonitor:         channelMonitorHandler,
		ChannelMonitorTemplate: channelMonitorTemplateHandler,
		ContentModeration:      contentModerationHandler,
		Payment:                paymentHandler,
		RechargePromo:          rechargePromoHandler,
		Affiliate:              affiliateHandler,
		SupportTicket:          supportTicketHandler,
		SupportFaq:             supportFaqHandler,
		SupportDocIndex:        supportDocIndexHandler,
		OidcClient:             oidcClientHandler,
		OidcSigningKey:         oidcSigningKeyHandler,
		OidcProviderSettings:   oidcProviderSettingsHandler,
		BillingApp:             billingAppHandler,
		Compliance:             complianceHandler,
		COSImage:               cosImageHandler,
		AsyncMediaConfig:       asyncMediaConfigHandler,
	}
}

// ProvideSystemHandler creates admin.SystemHandler with UpdateService
func ProvideSystemHandler(updateService *service.UpdateService, lockService *service.SystemOperationLockService) *admin.SystemHandler {
	return admin.NewSystemHandler(updateService, lockService)
}

// ProvideSettingHandler creates SettingHandler with version from BuildInfo
func ProvideSettingHandler(settingService *service.SettingService, buildInfo BuildInfo, notificationEmailService *service.NotificationEmailService) *SettingHandler {
	h := NewSettingHandler(settingService, buildInfo.Version)
	h.SetNotificationEmailService(notificationEmailService)
	return h
}

// ProvideAdminSettingHandler creates admin.SettingHandler with notification template APIs.
func ProvideAdminSettingHandler(settingService *service.SettingService, emailService *service.EmailService, captchaService *service.CaptchaService, opsService *service.OpsService, paymentConfigService *service.PaymentConfigService, paymentService *service.PaymentService, userAttributeService *service.UserAttributeService, notificationEmailService *service.NotificationEmailService) *admin.SettingHandler {
	h := admin.NewSettingHandler(settingService, emailService, captchaService, opsService, paymentConfigService, paymentService, userAttributeService)
	h.SetNotificationEmailService(notificationEmailService)
	return h
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
	channelMonitorUserHandler *ChannelMonitorUserHandler,
	adminHandlers *AdminHandlers,
	gatewayHandler *GatewayHandler,
	openaiGatewayHandler *OpenAIGatewayHandler,
	falGatewayHandler *FalGatewayHandler,
	settingHandler *SettingHandler,
	totpHandler *TotpHandler,
	paymentHandler *PaymentHandler,
	paymentWebhookHandler *PaymentWebhookHandler,
	availableChannelHandler *AvailableChannelHandler,
	plazaHandler *PlazaHandler,
	supportTicketHandler *SupportTicketHandler,
	supportTicketAttachmentHandler *SupportTicketAttachmentHandler,
	supportChatHandler *SupportChatHandler,
	oidcProviderHandler *OidcProviderHandler,
	batchImageHandler *BatchImageHandler,
	_ *service.IdempotencyCoordinator,
	_ *service.IdempotencyCleanupService,
) *Handlers {
	return &Handlers{
		Auth:                    authHandler,
		User:                    userHandler,
		APIKey:                  apiKeyHandler,
		Usage:                   usageHandler,
		Redeem:                  redeemHandler,
		Subscription:            subscriptionHandler,
		Announcement:            announcementHandler,
		ChannelMonitor:          channelMonitorUserHandler,
		Admin:                   adminHandlers,
		Gateway:                 gatewayHandler,
		OpenAIGateway:           openaiGatewayHandler,
		FalGateway:              falGatewayHandler,
		Setting:                 settingHandler,
		Totp:                    totpHandler,
		Payment:                 paymentHandler,
		PaymentWebhook:          paymentWebhookHandler,
		AvailableChannel:        availableChannelHandler,
		Plaza:                   plazaHandler,
		SupportTicket:           supportTicketHandler,
		SupportTicketAttachment: supportTicketAttachmentHandler,
		SupportChat:             supportChatHandler,
		OidcProvider:            oidcProviderHandler,
		BatchImage:              batchImageHandler,
	}
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
	NewChannelMonitorUserHandler,
	NewGatewayHandler,
	NewOpenAIGatewayHandler,
	NewFalGatewayHandler,
	NewTotpHandler,
	ProvideSettingHandler,
	NewPaymentHandler,
	NewPaymentWebhookHandler,
	NewAvailableChannelHandler,
	NewPlazaHandler,
	NewSupportTicketHandler,           // 工单系统：用户端
	NewSupportTicketAttachmentHandler, // 工单系统：用户端附件上传
	NewSupportChatHandler,             // 客服浮窗：用户端 SSE / FAQ
	NewOidcProviderHandler,
	NewBatchImageHandler,

	// Admin handlers
	admin.NewDashboardHandler,
	admin.NewUserHandler,
	admin.NewGroupHandler,
	admin.NewAccountHandler,
	admin.NewAnnouncementHandler,
	admin.NewDataManagementHandler,
	admin.NewBackupHandler,
	admin.NewCOSImageHandler,
	admin.NewAsyncMediaConfigHandler,
	admin.NewOAuthHandler,
	admin.NewOpenAIOAuthHandler,
	admin.NewGeminiOAuthHandler,
	admin.NewAntigravityOAuthHandler,
	admin.NewKiroOAuthHandler,
	admin.NewGrokOAuthHandler,
	admin.NewProxyHandler,
	admin.NewRedeemHandler,
	admin.NewPromoHandler,
	ProvideAdminSettingHandler,
	admin.NewOpsHandler,
	ProvideSystemHandler,
	admin.NewSubscriptionHandler,
	admin.NewUsageHandler,
	admin.NewUserAttributeHandler,
	admin.NewErrorPassthroughHandler,
	admin.NewTLSFingerprintProfileHandler,
	admin.NewAdminAPIKeyHandler,
	admin.NewScheduledTestHandler,
	admin.NewChannelHandler,
	admin.NewChannelMonitorHandler,
	admin.NewChannelMonitorRequestTemplateHandler,
	admin.NewContentModerationHandler,
	admin.NewPaymentHandler,
	admin.NewRechargePromoHandler,
	admin.NewAffiliateHandler,
	admin.NewSupportTicketHandler,   // 工单系统：admin 端
	admin.NewSupportFaqHandler,      // 客服知识库 RAG：admin FAQ CRUD
	admin.NewSupportDocIndexHandler, // 客服知识库 RAG：admin 文档索引控制
	admin.NewOidcClientHandler,
	admin.NewOidcSigningKeyHandler,
	admin.NewOidcProviderSettingsHandler,
	admin.NewBillingAppHandler,
	admin.NewComplianceHandler,

	// AdminHandlers and Handlers constructors
	ProvideAdminHandlers,
	ProvideHandlers,
)
