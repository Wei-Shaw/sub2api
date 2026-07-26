package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
)

func ProvideAuthHandler(
	cfg *config.Config,
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	promoService *service.PromoService,
	redeemService *service.RedeemService,
	totpService *service.TotpService,
	userAttributeService *service.UserAttributeService,
	ssoSessionService *service.SsoSessionService,
	organizationService *service.OrganizationService,
) *AuthHandler {
	h := NewAuthHandler(cfg, authService, userService, settingService, promoService, redeemService, totpService, userAttributeService, ssoSessionService)
	h.SetOrganizationService(organizationService)
	return h
}

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
	promptAuditHandler *securityaudit.PromptAdminHandler,
	paymentHandler *admin.PaymentHandler,
	rechargePromoHandler *admin.RechargePromoHandler,
	affiliateHandler *admin.AffiliateHandler,
	supportTicketHandler *admin.SupportTicketHandler,
	supportTicketNotificationHandler *admin.SupportTicketNotificationHandler,
	supportFaqHandler *admin.SupportFaqHandler,
	supportDocIndexHandler *admin.SupportDocIndexHandler,
	supportChatLogHandler *admin.SupportChatLogHandler,
	oidcClientHandler *admin.OidcClientHandler,
	oidcSigningKeyHandler *admin.OidcSigningKeyHandler,
	oidcProviderSettingsHandler *admin.OidcProviderSettingsHandler,
	billingAppHandler *admin.BillingAppHandler,
	complianceHandler *admin.ComplianceHandler,
	cosImageHandler *admin.COSImageHandler,
	asyncMediaConfigHandler *admin.AsyncMediaConfigHandler,
	auditLogHandler *admin.AuditLogHandler,
	upstreamBillingProbe *service.UpstreamBillingProbeService,
	ollamaCloudUsage *service.OllamaCloudUsageService,
) *AdminHandlers {
	accountHandler.SetUpstreamBillingProbeService(upstreamBillingProbe)
	accountHandler.SetOllamaCloudUsageService(ollamaCloudUsage)
	return &AdminHandlers{
		Dashboard:                 dashboardHandler,
		User:                      userHandler,
		Group:                     groupHandler,
		Account:                   accountHandler,
		Announcement:              announcementHandler,
		DataManagement:            dataManagementHandler,
		Backup:                    backupHandler,
		OAuth:                     oauthHandler,
		OpenAIOAuth:               openaiOAuthHandler,
		GeminiOAuth:               geminiOAuthHandler,
		AntigravityOAuth:          antigravityOAuthHandler,
		KiroOAuth:                 kiroOAuthHandler,
		GrokOAuth:                 grokOAuthHandler,
		Proxy:                     proxyHandler,
		Redeem:                    redeemHandler,
		Promo:                     promoHandler,
		Setting:                   settingHandler,
		Ops:                       opsHandler,
		System:                    systemHandler,
		Subscription:              subscriptionHandler,
		Usage:                     usageHandler,
		UserAttribute:             userAttributeHandler,
		ErrorPassthrough:          errorPassthroughHandler,
		TLSFingerprintProfile:     tlsFingerprintProfileHandler,
		APIKey:                    apiKeyHandler,
		ScheduledTest:             scheduledTestHandler,
		Channel:                   channelHandler,
		ChannelMonitor:            channelMonitorHandler,
		ChannelMonitorTemplate:    channelMonitorTemplateHandler,
		ContentModeration:         contentModerationHandler,
		PromptAudit:               promptAuditHandler,
		Payment:                   paymentHandler,
		RechargePromo:             rechargePromoHandler,
		Affiliate:                 affiliateHandler,
		SupportTicket:             supportTicketHandler,
		SupportTicketNotification: supportTicketNotificationHandler,
		SupportFaq:                supportFaqHandler,
		SupportDocIndex:           supportDocIndexHandler,
		SupportChatLog:            supportChatLogHandler,
		OidcClient:                oidcClientHandler,
		OidcSigningKey:            oidcSigningKeyHandler,
		OidcProviderSettings:      oidcProviderSettingsHandler,
		BillingApp:                billingAppHandler,
		Compliance:                complianceHandler,
		COSImage:                  cosImageHandler,
		AsyncMediaConfig:          asyncMediaConfigHandler,
		AuditLog:                  auditLogHandler,
	}
}

func ProvideGatewayHandler(
	gatewayService *service.GatewayService,
	openAIGatewayService *service.OpenAIGatewayService,
	geminiCompatService *service.GeminiMessagesCompatService,
	antigravityGatewayService *service.AntigravityGatewayService,
	userService *service.UserService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	usageService *service.UsageService,
	apiKeyService *service.APIKeyService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	errorPassthroughService *service.ErrorPassthroughService,
	contentModerationService *service.ContentModerationService,
	userMsgQueueService *service.UserMessageQueueService,
	cfg *config.Config,
	settingService *service.SettingService,
	coordinator *securityaudit.Coordinator,
	billingContextResolver *service.BillingContextResolver,
) *GatewayHandler {
	gatewayService.SetBillingContextResolver(billingContextResolver)
	openAIGatewayService.SetBillingContextResolver(billingContextResolver)
	usageService.SetBillingContextResolver(billingContextResolver)
	h := NewGatewayHandler(gatewayService, openAIGatewayService, geminiCompatService, antigravityGatewayService,
		userService, concurrencyService, billingCacheService, usageService, apiKeyService, usageRecordWorkerPool,
		errorPassthroughService, contentModerationService, userMsgQueueService, cfg, settingService)
	h.securityAuditCoordinator = coordinator
	return h
}

func ProvideOpenAIGatewayHandler(
	gatewayService *service.OpenAIGatewayService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	apiKeyService *service.APIKeyService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	errorPassthroughService *service.ErrorPassthroughService,
	contentModerationService *service.ContentModerationService,
	opsService *service.OpsService,
	grokQuotaService *service.GrokQuotaService,
	cfg *config.Config,
	coordinator *securityaudit.Coordinator,
) *OpenAIGatewayHandler {
	h := NewOpenAIGatewayHandler(gatewayService, concurrencyService, billingCacheService, apiKeyService,
		usageRecordWorkerPool, errorPassthroughService, contentModerationService, opsService, cfg)
	h.securityAuditCoordinator = coordinator
	h.grokMediaEligibilityProber = grokQuotaService
	return h
}

func ProvideBatchImageHandler(
	batchService *service.BatchImagePublicService,
	download *service.BatchImageDownloadService,
	cleanup *service.BatchImageCleanupService,
	openAI *OpenAIGatewayHandler,
) *BatchImageHandler {
	h := NewBatchImageHandler(batchService, download, cleanup)
	h.openAI = openAI
	return h
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
func ProvideAdminSettingHandler(settingService *service.SettingService, emailService *service.EmailService, captchaService *service.CaptchaService, opsService *service.OpsService, paymentConfigService *service.PaymentConfigService, paymentService *service.PaymentService, userAttributeService *service.UserAttributeService, notificationEmailService *service.NotificationEmailService, totpService *service.TotpService, userService *service.UserService) *admin.SettingHandler {
	h := admin.NewSettingHandler(settingService, emailService, captchaService, opsService, paymentConfigService, paymentService, userAttributeService)
	h.SetNotificationEmailService(notificationEmailService)
	h.SetStepUpDeps(totpService, userService)
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
	supportTicketNotificationHandler *SupportTicketNotificationHandler,
	supportChatHandler *SupportChatHandler,
	oidcProviderHandler *OidcProviderHandler,
	asyncImageHandler *AsyncImageHandler,
	batchImageHandler *BatchImageHandler,
	organizationHandler *OrganizationHandler,
	_ *service.IdempotencyCoordinator,
	_ *service.IdempotencyCleanupService,
) *Handlers {
	return &Handlers{
		Auth:                      authHandler,
		User:                      userHandler,
		APIKey:                    apiKeyHandler,
		Usage:                     usageHandler,
		Redeem:                    redeemHandler,
		Subscription:              subscriptionHandler,
		Announcement:              announcementHandler,
		ChannelMonitor:            channelMonitorUserHandler,
		Admin:                     adminHandlers,
		Gateway:                   gatewayHandler,
		OpenAIGateway:             openaiGatewayHandler,
		FalGateway:                falGatewayHandler,
		Setting:                   settingHandler,
		Totp:                      totpHandler,
		Payment:                   paymentHandler,
		PaymentWebhook:            paymentWebhookHandler,
		AvailableChannel:          availableChannelHandler,
		Plaza:                     plazaHandler,
		SupportTicket:             supportTicketHandler,
		SupportTicketAttachment:   supportTicketAttachmentHandler,
		SupportTicketNotification: supportTicketNotificationHandler,
		SupportChat:               supportChatHandler,
		OidcProvider:              oidcProviderHandler,
		AsyncImage:                asyncImageHandler,
		BatchImage:                batchImageHandler,
		Organization:              organizationHandler,
	}
}

// ProviderSet is the Wire provider set for all handlers
var ProviderSet = wire.NewSet(
	// Top-level handlers
	ProvideAuthHandler,
	NewUserHandler,
	NewAPIKeyHandler,
	NewUsageHandler,
	NewRedeemHandler,
	NewSubscriptionHandler,
	NewAnnouncementHandler,
	NewChannelMonitorUserHandler,
	ProvideGatewayHandler,
	ProvideOpenAIGatewayHandler,
	NewFalGatewayHandler,
	NewTotpHandler,
	ProvideSettingHandler,
	NewPaymentHandler,
	NewPaymentWebhookHandler,
	NewAvailableChannelHandler,
	NewPlazaHandler,
	NewSupportTicketHandler,             // 工单系统：用户端
	NewSupportTicketAttachmentHandler,   // 工单系统：用户端附件上传
	NewSupportTicketNotificationHandler, // 工单通知/未读计数：用户端
	NewSupportChatHandler,               // 客服浮窗：用户端 SSE / FAQ
	NewOidcProviderHandler,
	NewAsyncImageHandler,
	ProvideBatchImageHandler,
	NewOrganizationHandler,

	// Admin handlers
	admin.NewDashboardHandler,
	admin.NewUserHandler,
	admin.NewGroupHandler,
	admin.ProvideAccountHandler,
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
	admin.NewSupportTicketHandler,             // 工单系统：admin 端
	admin.NewSupportTicketNotificationHandler, // 工单通知/未读计数：admin 端
	admin.NewSupportFaqHandler,                // 客服知识库 RAG：admin FAQ CRUD
	admin.NewSupportDocIndexHandler,           // 客服知识库 RAG：admin 文档索引控制
	admin.NewSupportChatLogHandler,            // 客服对话记录：admin 只读查看
	admin.NewOidcClientHandler,
	admin.NewOidcSigningKeyHandler,
	admin.NewOidcProviderSettingsHandler,
	admin.NewBillingAppHandler,
	admin.NewComplianceHandler,
	admin.NewAuditLogHandler,

	// AdminHandlers and Handlers constructors
	ProvideAdminHandlers,
	ProvideHandlers,
)
