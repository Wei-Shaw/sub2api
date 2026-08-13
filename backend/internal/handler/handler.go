package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
)

// AdminHandlers contains all admin-related HTTP handlers
type AdminHandlers struct {
	Dashboard                 *admin.DashboardHandler
	User                      *admin.UserHandler
	Group                     *admin.GroupHandler
	Account                   *admin.AccountHandler
	Announcement              *admin.AnnouncementHandler
	DataManagement            *admin.DataManagementHandler
	Backup                    *admin.BackupHandler
	OAuth                     *admin.OAuthHandler
	OpenAIOAuth               *admin.OpenAIOAuthHandler
	GeminiOAuth               *admin.GeminiOAuthHandler
	AntigravityOAuth          *admin.AntigravityOAuthHandler
	KiroOAuth                 *admin.KiroOAuthHandler
	GrokOAuth                 *admin.GrokOAuthHandler
	Proxy                     *admin.ProxyHandler
	Redeem                    *admin.RedeemHandler
	Promo                     *admin.PromoHandler
	Setting                   *admin.SettingHandler
	Ops                       *admin.OpsHandler
	System                    *admin.SystemHandler
	Subscription              *admin.SubscriptionHandler
	Usage                     *admin.UsageHandler
	UserAttribute             *admin.UserAttributeHandler
	ErrorPassthrough          *admin.ErrorPassthroughHandler
	TLSFingerprintProfile     *admin.TLSFingerprintProfileHandler
	APIKey                    *admin.AdminAPIKeyHandler
	ScheduledTest             *admin.ScheduledTestHandler
	Channel                   *admin.ChannelHandler
	ChannelMonitor            *admin.ChannelMonitorHandler
	ChannelMonitorTemplate    *admin.ChannelMonitorRequestTemplateHandler
	ContentModeration         *admin.ContentModerationHandler
	PromptAudit               *securityaudit.PromptAdminHandler
	Payment                   *admin.PaymentHandler
	RechargePromo             *admin.RechargePromoHandler
	ModelIntro                *admin.ModelIntroHandler
	Affiliate                 *admin.AffiliateHandler
	SupportTicket             *admin.SupportTicketHandler
	SupportTicketNotification *admin.SupportTicketNotificationHandler
	SupportFaq                *admin.SupportFaqHandler
	SupportDocIndex           *admin.SupportDocIndexHandler
	SupportChatLog            *admin.SupportChatLogHandler
	OidcClient                *admin.OidcClientHandler
	OidcSigningKey            *admin.OidcSigningKeyHandler
	OidcProviderSettings      *admin.OidcProviderSettingsHandler
	BillingApp                *admin.BillingAppHandler
	Compliance                *admin.ComplianceHandler
	CostCenter                *admin.CostCenterHandler
	COSImage                  *admin.COSImageHandler
	// File 管理员文件管理：直接管理图片转存桶里的对象（依赖 COSImage 已启用）。
	File             *admin.FileHandler
	AsyncMediaConfig *admin.AsyncMediaConfigHandler
	AuditLog         *admin.AuditLogHandler
}

// Handlers contains all HTTP handlers
type Handlers struct {
	Auth                      *AuthHandler
	User                      *UserHandler
	APIKey                    *APIKeyHandler
	Usage                     *UsageHandler
	Redeem                    *RedeemHandler
	Subscription              *SubscriptionHandler
	Announcement              *AnnouncementHandler
	ChannelMonitor            *ChannelMonitorUserHandler
	ChannelMonitorV2          *ChannelMonitorV2Handler
	Admin                     *AdminHandlers
	Gateway                   *GatewayHandler
	OpenAIGateway             *OpenAIGatewayHandler
	Setting                   *SettingHandler
	Totp                      *TotpHandler
	Passkey                   *PasskeyHandler
	Payment                   *PaymentHandler
	PaymentWebhook            *PaymentWebhookHandler
	AvailableChannel          *AvailableChannelHandler
	ModelPlaza                *ModelPlazaHandler
	AsyncImage                *AsyncImageHandler
	BatchImage                *BatchImageHandler
	FalGateway                *FalGatewayHandler
	FalVideoGateway           *FalVideoGatewayHandler
	Plaza                     *PlazaHandler
	SupportTicket             *SupportTicketHandler
	SupportTicketAttachment   *SupportTicketAttachmentHandler
	SupportTicketNotification *SupportTicketNotificationHandler
	SupportChat               *SupportChatHandler
	OidcProvider              *OidcProviderHandler
	Organization              *OrganizationHandler
	VideoModel                *VideoModelHandler
	UserMaterial              *UserMaterialHandler
}

// BuildInfo contains build-time information
type BuildInfo struct {
	Version   string
	BuildType string // "source" for manual builds, "release" for CI builds
}
