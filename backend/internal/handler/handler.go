package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
)

// AdminHandlers contains all admin-related HTTP handlers
type AdminHandlers struct {
	Dashboard        *admin.DashboardHandler
	User             *admin.UserHandler
	Group            *admin.GroupHandler
	Account          *admin.AccountHandler
	Announcement     *admin.AnnouncementHandler
	DataManagement   *admin.DataManagementHandler
	Backup           *admin.BackupHandler
	OAuth            *admin.OAuthHandler
	OpenAIOAuth      *admin.OpenAIOAuthHandler
	GeminiOAuth      *admin.GeminiOAuthHandler
	AntigravityOAuth *admin.AntigravityOAuthHandler
	GrokOAuth        *admin.GrokOAuthHandler
	CNProvider       *admin.CNProviderHandler
	Proxy            *admin.ProxyHandler
	Redeem           *admin.RedeemHandler
	Promo            *admin.PromoHandler
	Setting          *admin.SettingHandler
	Ops              *admin.OpsHandler
	System           *admin.SystemHandler
	Subscription     *admin.SubscriptionHandler
	Usage            *admin.UsageHandler
	// UpstreamCostSettings 管的是「后台按请求 ID 反查上游真实成本」用的 A6 凭据。
	// 它和使用记录页面是一体的：成本列为空时，答案就在这个 handler 管的那份配置里。
	UpstreamCostSettings *admin.UpstreamCostSettingsHandler
	// UsageProfitExclusion 管的是「谁的收入不计入盈亏」这份名单。
	// 与 UpstreamCostSettings 同属使用记录页的经营口径，但两者的失败模式不同，
	// 因此各自一个 handler、各自一个接口，保存时互不牵连。
	UsageProfitExclusion   *admin.UsageProfitExclusionHandler
	UserAttribute          *admin.UserAttributeHandler
	ErrorPassthrough       *admin.ErrorPassthroughHandler
	TLSFingerprintProfile  *admin.TLSFingerprintProfileHandler
	Plugin                 *admin.PluginHandler
	APIKey                 *admin.AdminAPIKeyHandler
	ScheduledTest          *admin.ScheduledTestHandler
	IntelligenceCheck      *admin.IntelligenceCheckHandler
	Channel                *admin.ChannelHandler
	ChannelMonitor         *admin.ChannelMonitorHandler
	ChannelMonitorTemplate *admin.ChannelMonitorRequestTemplateHandler
	ContentModeration      *admin.ContentModerationHandler
	PromptAudit            *securityaudit.PromptAdminHandler
	Payment                *admin.PaymentHandler
	Affiliate              *admin.AffiliateHandler
	Compliance             *admin.ComplianceHandler
	AuditLog               *admin.AuditLogHandler
}

// Handlers contains all HTTP handlers
type Handlers struct {
	Auth             *AuthHandler
	User             *UserHandler
	MCPOAuth         *MCPOAuthHandler
	APIKey           *APIKeyHandler
	Usage            *UsageHandler
	Redeem           *RedeemHandler
	Subscription     *SubscriptionHandler
	Announcement     *AnnouncementHandler
	ChannelMonitor   *ChannelMonitorUserHandler
	ChannelMonitorV2 *ChannelMonitorV2Handler
	// ChannelMonitorV1Matrix V1 主动探测的模型广场 Pro 矩阵（与 V2 matrix 同形，只读）。
	ChannelMonitorV1Matrix *ChannelMonitorV1MatrixHandler
	// IntelligenceCheck 智力检测的用户侧只读脱敏接口（管理员走 Admin.IntelligenceCheck）。
	IntelligenceCheck *IntelligenceCheckPublicHandler

	Admin            *AdminHandlers
	Gateway          *GatewayHandler
	OpenAIGateway    *OpenAIGatewayHandler
	Setting          *SettingHandler
	Totp             *TotpHandler
	Passkey          *PasskeyHandler
	Payment          *PaymentHandler
	PaymentWebhook   *PaymentWebhookHandler
	AvailableChannel *AvailableChannelHandler
	ModelPlaza       *ModelPlazaHandler
	AsyncImage       *AsyncImageHandler
	BatchImage       *BatchImageHandler
	// ProviderPricing 对外公开的价格接口（无鉴权，挂在 /api/v1 之外）
	ProviderPricing *ProviderPricingHandler
}

// BuildInfo contains build-time information
type BuildInfo struct {
	Version   string
	BuildType string // "source" for manual builds, "release" for CI builds
}
