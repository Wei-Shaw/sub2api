// Package routes provides HTTP route registration and handlers.
package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes 注册管理员路由
func RegisterAdminRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	adminAuth middleware.AdminAuthMiddleware,
) {
	admin := v1.Group("/admin")
	admin.Use(gin.HandlerFunc(adminAuth))
	{
		// 仪表盘
		registerDashboardRoutes(admin, h)

		// 用户管理
		registerUserManagementRoutes(admin, h)

		// 分组管理
		registerGroupRoutes(admin, h)

		// 账号管理
		registerAccountRoutes(admin, h)

		// 公告管理
		registerAnnouncementRoutes(admin, h)

		// OpenAI OAuth
		registerOpenAIOAuthRoutes(admin, h)

		// Gemini OAuth
		registerGeminiOAuthRoutes(admin, h)

		// Antigravity OAuth
		registerAntigravityOAuthRoutes(admin, h)

		// 代理管理
		registerProxyRoutes(admin, h)

		// 卡密管理
		registerRedeemCodeRoutes(admin, h)

		// 优惠码管理
		registerPromoCodeRoutes(admin, h)

		// 系统设置
		registerSettingsRoutes(admin, h)

		// 数据管理
		registerDataManagementRoutes(admin, h)

		// 数据库备份恢复
		registerBackupRoutes(admin, h)

		// 运维监控（Ops）
		registerOpsRoutes(admin, h)

		// 系统管理
		registerSystemRoutes(admin, h)

		// 订阅管理
		registerSubscriptionRoutes(admin, h)

		// 使用记录管理
		registerUsageRoutes(admin, h)

		// 用户属性管理
		registerUserAttributeRoutes(admin, h)

		// 错误透传规则管理
		registerErrorPassthroughRoutes(admin, h)

		// TLS 指纹模板管理
		registerTLSFingerprintProfileRoutes(admin, h)

		// API Key 管理
		registerAdminAPIKeyRoutes(admin, h)

		// 定时测试计划
		registerScheduledTestRoutes(admin, h)

		// 渠道管理
		registerChannelRoutes(admin, h)

		// 渠道监控
		registerChannelMonitorRoutes(admin, h)

		// 风控中心
		registerContentModerationRoutes(admin, h)

		// 邀请返利（专属用户管理）
		registerAffiliateRoutes(admin, h)
	}
}

func registerContentModerationRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	risk := admin.Group("/risk-control")
	{
		risk.GET("/config", h.Admin.ContentModeration.GetConfig)
		risk.PUT("/config", h.Admin.ContentModeration.UpdateConfig)
		risk.POST("/api-keys/test", h.Admin.ContentModeration.TestAPIKeys)
		risk.GET("/status", h.Admin.ContentModeration.GetStatus)
		risk.GET("/logs", h.Admin.ContentModeration.ListLogs)
		risk.POST("/users/:user_id/unban", h.Admin.ContentModeration.UnbanUser)
		risk.DELETE("/hashes", h.Admin.ContentModeration.DeleteFlaggedHash)
		risk.DELETE("/hashes/all", h.Admin.ContentModeration.ClearFlaggedHashes)
	}
}

func registerAdminAPIKeyRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	apiKeys := admin.Group("/api-keys")
	{
		apiKeys.PUT("/:id", h.Admin.APIKey.UpdateGroup)
	}
}

func registerOpsRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	ops := admin.Group("/ops")
	{
		// Realtime ops signals
		ops.GET("/concurrency", h.Admin.Ops.GetConcurrencyStats)
		ops.GET("/user-concurrency", h.Admin.Ops.GetUserConcurrencyStats)
		ops.GET("/account-availability", h.Admin.Ops.GetAccountAvailability)
		ops.GET("/realtime-traffic", h.Admin.Ops.GetRealtimeTrafficSummary)

		// Alerts (rules + events)
		ops.GET("/alert-rules", h.Admin.Ops.ListAlertRules)
		ops.POST("/alert-rules", h.Admin.Ops.CreateAlertRule)
		ops.PUT("/alert-rules/:id", h.Admin.Ops.UpdateAlertRule)
		ops.DELETE("/alert-rules/:id", h.Admin.Ops.DeleteAlertRule)
		ops.GET("/alert-events", h.Admin.Ops.ListAlertEvents)
		ops.GET("/alert-events/:id", h.Admin.Ops.GetAlertEvent)
		ops.PUT("/alert-events/:id/status", h.Admin.Ops.UpdateAlertEventStatus)
		ops.POST("/alert-silences", h.Admin.Ops.CreateAlertSilence)

		// Email notification config (DB-backed)
		ops.GET("/email-notification/config", h.Admin.Ops.GetEmailNotificationConfig)
		ops.PUT("/email-notification/config", h.Admin.Ops.UpdateEmailNotificationConfig)

		// Runtime settings (DB-backed)
		runtime := ops.Group("/runtime")
		{
			runtime.GET("/alert", h.Admin.Ops.GetAlertRuntimeSettings)
			runtime.PUT("/alert", h.Admin.Ops.UpdateAlertRuntimeSettings)
			runtime.GET("/logging", h.Admin.Ops.GetRuntimeLogConfig)
			runtime.PUT("/logging", h.Admin.Ops.UpdateRuntimeLogConfig)
			runtime.POST("/logging/reset", h.Admin.Ops.ResetRuntimeLogConfig)
		}

		// Advanced settings (DB-backed)
		ops.GET("/advanced-settings", h.Admin.Ops.GetAdvancedSettings)
		ops.PUT("/advanced-settings", h.Admin.Ops.UpdateAdvancedSettings)

		// Settings group (DB-backed)
		settings := ops.Group("/settings")
		{
			settings.GET("/metric-thresholds", h.Admin.Ops.GetMetricThresholds)
			settings.PUT("/metric-thresholds", h.Admin.Ops.UpdateMetricThresholds)
		}

		// WebSocket realtime (QPS/TPS)
		ws := ops.Group("/ws")
		{
			ws.GET("/qps", h.Admin.Ops.QPSWSHandler)
		}

		// Error logs (legacy)
		ops.GET("/errors", h.Admin.Ops.GetErrorLogs)
		ops.GET("/errors/:id", h.Admin.Ops.GetErrorLogByID)
		ops.GET("/errors/:id/retries", h.Admin.Ops.ListRetryAttempts)
		ops.POST("/errors/:id/retry", h.Admin.Ops.RetryErrorRequest)
		ops.PUT("/errors/:id/resolve", h.Admin.Ops.UpdateErrorResolution)

		// Request errors (client-visible failures)
		ops.GET("/request-errors", h.Admin.Ops.ListRequestErrors)
		ops.GET("/request-errors/:id", h.Admin.Ops.GetRequestError)
		ops.GET("/request-errors/:id/upstream-errors", h.Admin.Ops.ListRequestErrorUpstreamErrors)
		ops.POST("/request-errors/:id/retry-client", h.Admin.Ops.RetryRequestErrorClient)
		ops.POST("/request-errors/:id/upstream-errors/:idx/retry", h.Admin.Ops.RetryRequestErrorUpstreamEvent)
		ops.PUT("/request-errors/:id/resolve", h.Admin.Ops.ResolveRequestError)

		// Upstream errors (independent upstream failures)
		ops.GET("/upstream-errors", h.Admin.Ops.ListUpstreamErrors)
		ops.GET("/upstream-errors/:id", h.Admin.Ops.GetUpstreamError)
		ops.POST("/upstream-errors/:id/retry", h.Admin.Ops.RetryUpstreamError)
		ops.PUT("/upstream-errors/:id/resolve", h.Admin.Ops.ResolveUpstreamError)

		// Request drilldown (success + error)
		ops.GET("/requests", h.Admin.Ops.ListRequestDetails)

		// Indexed system logs
		ops.GET("/system-logs", h.Admin.Ops.ListSystemLogs)
		ops.POST("/system-logs/cleanup", h.Admin.Ops.CleanupSystemLogs)
		ops.GET("/system-logs/health", h.Admin.Ops.GetSystemLogIngestionHealth)

		// Dashboard (vNext - raw path for MVP)
		ops.GET("/dashboard/snapshot-v2", h.Admin.Ops.GetDashboardSnapshotV2)
		ops.GET("/dashboard/overview", h.Admin.Ops.GetDashboardOverview)
		ops.GET("/dashboard/throughput-trend", h.Admin.Ops.GetDashboardThroughputTrend)
		ops.GET("/dashboard/latency-histogram", h.Admin.Ops.GetDashboardLatencyHistogram)
		ops.GET("/dashboard/error-trend", h.Admin.Ops.GetDashboardErrorTrend)
		ops.GET("/dashboard/error-distribution", h.Admin.Ops.GetDashboardErrorDistribution)
		ops.GET("/dashboard/openai-token-stats", h.Admin.Ops.GetDashboardOpenAITokenStats)
	}
}

func registerDashboardRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	dashboard := admin.Group("/dashboard")
	{
		dashboard.GET("/snapshot-v2", h.Admin.Dashboard.GetSnapshotV2)
		dashboard.GET("/stats", h.Admin.Dashboard.GetStats)
		dashboard.GET("/realtime", h.Admin.Dashboard.GetRealtimeMetrics)
		dashboard.GET("/trend", h.Admin.Dashboard.GetUsageTrend)
		dashboard.GET("/models", h.Admin.Dashboard.GetModelStats)
		dashboard.GET("/groups", h.Admin.Dashboard.GetGroupStats)
		dashboard.GET("/api-keys-trend", h.Admin.Dashboard.GetAPIKeyUsageTrend)
		dashboard.GET("/users-trend", h.Admin.Dashboard.GetUserUsageTrend)
		dashboard.GET("/users-ranking", h.Admin.Dashboard.GetUserSpendingRanking)
		dashboard.POST("/users-usage", h.Admin.Dashboard.GetBatchUsersUsage)
		dashboard.POST("/api-keys-usage", h.Admin.Dashboard.GetBatchAPIKeysUsage)
		dashboard.GET("/user-breakdown", h.Admin.Dashboard.GetUserBreakdown)
		dashboard.POST("/aggregation/backfill", h.Admin.Dashboard.BackfillAggregation)
	}
}

func registerUserManagementRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	users := admin.Group("/users")
	{
		users.GET("", h.Admin.User.List)
		users.GET("/:id", h.Admin.User.GetByID)
		users.POST("/:id/auth-identities", h.Admin.User.BindAuthIdentity)
		users.POST("", h.Admin.User.Create)
		users.PUT("/:id", h.Admin.User.Update)
		users.DELETE("/:id", h.Admin.User.Delete)
		users.POST("/:id/balance", h.Admin.User.UpdateBalance)
		users.GET("/:id/api-keys", h.Admin.User.GetUserAPIKeys)
		users.GET("/:id/usage", h.Admin.User.GetUserUsage)
		users.GET("/:id/balance-history", h.Admin.User.GetBalanceHistory)
		users.POST("/:id/replace-group", h.Admin.User.ReplaceGroup)
		users.GET("/:id/rpm-status", h.Admin.User.GetUserRPMStatus)
		users.POST("/batch-concurrency", h.Admin.User.BatchUpdateConcurrency)

		// User attribute values
		users.GET("/:id/attributes", h.Admin.UserAttribute.GetUserAttributes)
		users.PUT("/:id/attributes", h.Admin.UserAttribute.UpdateUserAttributes)
	}
}

func registerGroupRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	groups := admin.Group("/groups")
	{
		groups.GET("", h.Admin.Group.List)
		groups.GET("/all", h.Admin.Group.GetAll)
		groups.GET("/usage-summary", h.Admin.Group.GetUsageSummary)
		groups.GET("/capacity-summary", h.Admin.Group.GetCapacitySummary)
		groups.PUT("/sort-order", h.Admin.Group.UpdateSortOrder)
		groups.GET("/:id", h.Admin.Group.GetByID)
		groups.POST("", h.Admin.Group.Create)
		groups.PUT("/:id", h.Admin.Group.Update)
		groups.DELETE("/:id", h.Admin.Group.Delete)
		groups.GET("/:id/stats", h.Admin.Group.GetStats)
		groups.GET("/:id/rate-multipliers", h.Admin.Group.GetGroupRateMultipliers)
		groups.PUT("/:id/rate-multipliers", h.Admin.Group.BatchSetGroupRateMultipliers)
		groups.DELETE("/:id/rate-multipliers", h.Admin.Group.ClearGroupRateMultipliers)
		groups.PUT("/:id/rpm-overrides", h.Admin.Group.BatchSetGroupRPMOverrides)
		groups.DELETE("/:id/rpm-overrides", h.Admin.Group.ClearGroupRPMOverrides)
		groups.GET("/:id/api-keys", h.Admin.Group.GetGroupAPIKeys)
	}
}

func registerAccountRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	accounts := admin.Group("/accounts")
	{
		accounts.GET("", h.Admin.Account.List)
		accounts.GET("/:id", h.Admin.Account.GetByID)
		accounts.POST("", h.Admin.Account.Create)
		accounts.POST("/check-mixed-channel", h.Admin.Account.CheckMixedChannel)
		accounts.POST("/import/codex-session", h.Admin.Account.ImportCodexSession)
		accounts.POST("/sync/crs", h.Admin.Account.SyncFromCRS)
		accounts.POST("/sync/crs/preview", h.Admin.Account.PreviewFromCRS)
		accounts.PUT("/:id", h.Admin.Account.Update)
		accounts.DELETE("/:id", h.Admin.Account.Delete)
		accounts.POST("/:id/test", h.Admin.Account.Test)
		accounts.POST("/:id/recover-state", h.Admin.Account.RecoverState)
		accounts.POST("/:id/refresh", h.Admin.Account.Refresh)
		accounts.POST("/:id/set-privacy", h.Admin.Account.SetPrivacy)
		accounts.POST("/:id/refresh-tier", h.Admin.Account.RefreshTier)
		accounts.GET("/:id/stats", h.Admin.Account.GetStats)
		accounts.POST("/:id/clear-error", h.Admin.Account.ClearError)
		accounts.GET("/:id/usage", h.Admin.Account.GetUsage)
		accounts.GET("/:id/today-stats", h.Admin.Account.GetTodayStats)
		accounts.POST("/today-stats/batch", h.Admin.Account.GetBatchTodayStats)
		accounts.POST("/:id/clear-rate-limit", h.Admin.Account.ClearRateLimit)
		accounts.POST("/:id/reset-quota", h.Admin.Account.ResetQuota)
		accounts.GET("/:id/temp-unschedulable", h.Admin.Account.GetTempUnschedulable)
		accounts.DELETE("/:id/temp-unschedulable", h.Admin.Account.ClearTempUnschedulable)
		accounts.POST("/:id/schedulable", h.Admin.Account.SetSchedulable)
		accounts.GET("/:id/models", h.Admin.Account.GetAvailableModels)
		accounts.POST("/batch", h.Admin.Account.BatchCreate)
		accounts.GET("/data", h.Admin.Account.ExportData)
		accounts.POST("/data", h.Admin.Account.ImportData)
		accounts.POST("/batch-update-credentials", h.Admin.Account.BatchUpdateCredentials)
		accounts.POST("/batch-refresh-tier", h.Admin.Account.BatchRefreshTier)
		accounts.POST("/bulk-update", h.Admin.Account.BulkUpdate)
		accounts.POST("/batch-clear-error", h.Admin.Account.BatchClearError)
		accounts.POST("/batch-refresh", h.Admin.Account.BatchRefresh)

		// Antigravity 默认模型映射
		accounts.GET("/antigravity/default-model-mapping", h.Admin.Account.GetAntigravityDefaultModelMapping)

		// Claude OAuth routes
		accounts.POST("/generate-auth-url", h.Admin.OAuth.GenerateAuthURL)
		accounts.POST("/generate-setup-token-url", h.Admin.OAuth.GenerateSetupTokenURL)
		accounts.POST("/exchange-code", h.Admin.OAuth.ExchangeCode)
		accounts.POST("/exchange-setup-token-code", h.Admin.OAuth.ExchangeSetupTokenCode)
		accounts.POST("/cookie-auth", h.Admin.OAuth.CookieAuth)
		accounts.POST("/setup-token-cookie-auth", h.Admin.OAuth.SetupTokenCookieAuth)
	}
}

func registerAnnouncementRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	announcements := admin.Group("/announcements")
	{
		announcements.GET("", h.Admin.Announcement.List)
		announcements.POST("", h.Admin.Announcement.Create)
		announcements.GET("/:id", h.Admin.Announcement.GetByID)
		announcements.PUT("/:id", h.Admin.Announcement.Update)
		announcements.DELETE("/:id", h.Admin.Announcement.Delete)
		announcements.GET("/:id/read-status", h.Admin.Announcement.ListReadStatus)
	}
}

func registerOpenAIOAuthRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	openai := admin.Group("/openai")
	{
		openai.POST("/generate-auth-url", h.Admin.OpenAIOAuth.GenerateAuthURL)
		openai.POST("/exchange-code", h.Admin.OpenAIOAuth.ExchangeCode)
		openai.POST("/refresh-token", h.Admin.OpenAIOAuth.RefreshToken)
		openai.POST("/accounts/:id/refresh", h.Admin.OpenAIOAuth.RefreshAccountToken)
		openai.POST("/create-from-oauth", h.Admin.OpenAIOAuth.CreateAccountFromOAuth)
	}
}

func registerGeminiOAuthRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	gemini := admin.Group("/gemini")
	{
		gemini.POST("/oauth/auth-url", h.Admin.GeminiOAuth.GenerateAuthURL)
		gemini.POST("/oauth/exchange-code", h.Admin.GeminiOAuth.ExchangeCode)
		gemini.GET("/oauth/capabilities", h.Admin.GeminiOAuth.GetCapabilities)
	}
}

func registerAntigravityOAuthRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	antigravity := admin.Group("/antigravity")
	{
		antigravity.POST("/oauth/auth-url", h.Admin.AntigravityOAuth.GenerateAuthURL)
		antigravity.POST("/oauth/exchange-code", h.Admin.AntigravityOAuth.ExchangeCode)
		antigravity.POST("/oauth/refresh-token", h.Admin.AntigravityOAuth.RefreshToken)
	}
}

func registerProxyRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	proxies := admin.Group("/proxies")
	{
		proxies.GET("", h.Admin.Proxy.List)
		proxies.GET("/all", h.Admin.Proxy.GetAll)
		proxies.GET("/data", h.Admin.Proxy.ExportData)
		proxies.POST("/data", h.Admin.Proxy.ImportData)
		proxies.GET("/:id", h.Admin.Proxy.GetByID)
		proxies.POST("", h.Admin.Proxy.Create)
		proxies.PUT("/:id", h.Admin.Proxy.Update)
		proxies.DELETE("/:id", h.Admin.Proxy.Delete)
		proxies.POST("/:id/test", h.Admin.Proxy.Test)
		proxies.POST("/:id/quality-check", h.Admin.Proxy.CheckQuality)
		proxies.GET("/:id/stats", h.Admin.Proxy.GetStats)
		proxies.GET("/:id/accounts", h.Admin.Proxy.GetProxyAccounts)
		proxies.POST("/batch-delete", h.Admin.Proxy.BatchDelete)
		proxies.POST("/batch", h.Admin.Proxy.BatchCreate)
	}
}

func registerRedeemCodeRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	codes := admin.Group("/redeem-codes")
	{
		codes.GET("", h.Admin.Redeem.List)
		codes.GET("/stats", h.Admin.Redeem.GetStats)
		codes.GET("/export", h.Admin.Redeem.Export)
		codes.GET("/:id", h.Admin.Redeem.GetByID)
		codes.POST("/create-and-redeem", h.Admin.Redeem.CreateAndRedeem)
		codes.POST("/generate", h.Admin.Redeem.Generate)
		codes.DELETE("/:id", h.Admin.Redeem.Delete)
		codes.POST("/batch-delete", h.Admin.Redeem.BatchDelete)
		codes.POST("/:id/expire", h.Admin.Redeem.Expire)
	}
}

func registerPromoCodeRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	promoCodes := admin.Group("/promo-codes")
	{
		promoCodes.GET("", h.Admin.Promo.List)
		promoCodes.GET("/:id", h.Admin.Promo.GetByID)
		promoCodes.POST("", h.Admin.Promo.Create)
		promoCodes.PUT("/:id", h.Admin.Promo.Update)
		promoCodes.DELETE("/:id", h.Admin.Promo.Delete)
		promoCodes.GET("/:id/usages", h.Admin.Promo.GetUsages)
	}
}

func registerSettingsRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	adminSettings := admin.Group("/settings")
	{
		adminSettings.GET("", h.Admin.Setting.GetSettings)
		adminSettings.PUT("", h.Admin.Setting.UpdateSettings)
		adminSettings.POST("/test-smtp", h.Admin.Setting.TestSMTPConnection)
		adminSettings.POST("/send-test-email", h.Admin.Setting.SendTestEmail)
		// Admin API Key 管理
		adminSettings.GET("/admin-api-key", h.Admin.Setting.GetAdminAPIKey)
		adminSettings.POST("/admin-api-key/regenerate", h.Admin.Setting.RegenerateAdminAPIKey)
		adminSettings.DELETE("/admin-api-key", h.Admin.Setting.DeleteAdminAPIKey)
		// 529过载冷却配置
		adminSettings.GET("/overload-cooldown", h.Admin.Setting.GetOverloadCooldownSettings)
		adminSettings.PUT("/overload-cooldown", h.Admin.Setting.UpdateOverloadCooldownSettings)
		// 429默认回避配置
		adminSettings.GET("/rate-limit-429-cooldown", h.Admin.Setting.GetRateLimit429CooldownSettings)
		adminSettings.PUT("/rate-limit-429-cooldown", h.Admin.Setting.UpdateRateLimit429CooldownSettings)
		// 流超时处理配置
		adminSettings.GET("/stream-timeout", h.Admin.Setting.GetStreamTimeoutSettings)
		adminSettings.PUT("/stream-timeout", h.Admin.Setting.UpdateStreamTimeoutSettings)
		// 请求整流器配置
		adminSettings.GET("/rectifier", h.Admin.Setting.GetRectifierSettings)
		adminSettings.PUT("/rectifier", h.Admin.Setting.UpdateRectifierSettings)
		// Beta 策略配置
		adminSettings.GET("/beta-policy", h.Admin.Setting.GetBetaPolicySettings)
		adminSettings.PUT("/beta-policy", h.Admin.Setting.UpdateBetaPolicySettings)
		// Web Search 模拟配置
		adminSettings.GET("/web-search-emulation", h.Admin.Setting.GetWebSearchEmulationConfig)
		adminSettings.PUT("/web-search-emulation", h.Admin.Setting.UpdateWebSearchEmulationConfig)
		adminSettings.POST("/web-search-emulation/test", h.Admin.Setting.TestWebSearchEmulation)
		adminSettings.POST("/web-search-emulation/reset-usage", h.Admin.Setting.ResetWebSearchUsage)
	}
}

func registerDataManagementRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	dataManagement := admin.Group("/data-management")
	{
		dataManagement.GET("/agent/health", h.Admin.DataManagement.GetAgentHealth)
		dataManagement.GET("/config", h.Admin.DataManagement.GetConfig)
		dataManagement.PUT("/config", h.Admin.DataManagement.UpdateConfig)
		dataManagement.GET("/sources/:source_type/profiles", h.Admin.DataManagement.ListSourceProfiles)
		dataManagement.POST("/sources/:source_type/profiles", h.Admin.DataManagement.CreateSourceProfile)
		dataManagement.PUT("/sources/:source_type/profiles/:profile_id", h.Admin.DataManagement.UpdateSourceProfile)
		dataManagement.DELETE("/sources/:source_type/profiles/:profile_id", h.Admin.DataManagement.DeleteSourceProfile)
		dataManagement.POST("/sources/:source_type/profiles/:profile_id/activate", h.Admin.DataManagement.SetActiveSourceProfile)
		dataManagement.POST("/s3/test", h.Admin.DataManagement.TestS3)
		dataManagement.GET("/s3/profiles", h.Admin.DataManagement.ListS3Profiles)
		dataManagement.POST("/s3/profiles", h.Admin.DataManagement.CreateS3Profile)
		dataManagement.PUT("/s3/profiles/:profile_id", h.Admin.DataManagement.UpdateS3Profile)
		dataManagement.DELETE("/s3/profiles/:profile_id", h.Admin.DataManagement.DeleteS3Profile)
		dataManagement.POST("/s3/profiles/:profile_id/activate", h.Admin.DataManagement.SetActiveS3Profile)
		dataManagement.POST("/backups", h.Admin.DataManagement.CreateBackupJob)
		dataManagement.GET("/backups", h.Admin.DataManagement.ListBackupJobs)
		dataManagement.GET("/backups/:job_id", h.Admin.DataManagement.GetBackupJob)
	}
}

func registerBackupRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	backup := admin.Group("/backups")
	{
		// S3 存储配置
		backup.GET("/s3-config", h.Admin.Backup.GetS3Config)
		backup.PUT("/s3-config", h.Admin.Backup.UpdateS3Config)
		backup.POST("/s3-config/test", h.Admin.Backup.TestS3Connection)

		// 定时备份配置
		backup.GET("/schedule", h.Admin.Backup.GetSchedule)
		backup.PUT("/schedule", h.Admin.Backup.UpdateSchedule)

		// 备份操作
		backup.POST("", h.Admin.Backup.CreateBackup)
		backup.GET("", h.Admin.Backup.ListBackups)
		backup.GET("/:id", h.Admin.Backup.GetBackup)
		backup.DELETE("/:id", h.Admin.Backup.DeleteBackup)
		backup.GET("/:id/download-url", h.Admin.Backup.GetDownloadURL)

		// 恢复操作
		backup.POST("/:id/restore", h.Admin.Backup.RestoreBackup)
	}
}

func registerSystemRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	system := admin.Group("/system")
	{
		system.GET("/version", h.Admin.System.GetVersion)
		system.GET("/check-updates", h.Admin.System.CheckUpdates)
		system.POST("/update", h.Admin.System.PerformUpdate)
		system.POST("/rollback", h.Admin.System.Rollback)
		system.POST("/restart", h.Admin.System.RestartService)
	}
}

func registerSubscriptionRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	subscriptions := admin.Group("/subscriptions")
	{
		subscriptions.GET("", h.Admin.Subscription.List)
		subscriptions.GET("/:id", h.Admin.Subscription.GetByID)
		subscriptions.GET("/:id/progress", h.Admin.Subscription.GetProgress)
		subscriptions.POST("/assign", h.Admin.Subscription.Assign)
		subscriptions.POST("/bulk-assign", h.Admin.Subscription.BulkAssign)
		subscriptions.POST("/:id/extend", h.Admin.Subscription.Extend)
		subscriptions.POST("/:id/reset-quota", h.Admin.Subscription.ResetQuota)
		subscriptions.DELETE("/:id", h.Admin.Subscription.Revoke)
	}

	// 分组下的订阅列表
	admin.GET("/groups/:id/subscriptions", h.Admin.Subscription.ListByGroup)

	// 用户下的订阅列表
	admin.GET("/users/:id/subscriptions", h.Admin.Subscription.ListByUser)
}

func registerUsageRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	usage := admin.Group("/usage")
	{
		usage.GET("", h.Admin.Usage.List)
		usage.GET("/stats", h.Admin.Usage.Stats)
		usage.GET("/search-users", h.Admin.Usage.SearchUsers)
		usage.GET("/search-api-keys", h.Admin.Usage.SearchAPIKeys)
		usage.GET("/cleanup-tasks", h.Admin.Usage.ListCleanupTasks)
		usage.POST("/cleanup-tasks", h.Admin.Usage.CreateCleanupTask)
		usage.POST("/cleanup-tasks/:id/cancel", h.Admin.Usage.CancelCleanupTask)
	}
}

func registerUserAttributeRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	attrs := admin.Group("/user-attributes")
	{
		attrs.GET("", h.Admin.UserAttribute.ListDefinitions)
		attrs.POST("", h.Admin.UserAttribute.CreateDefinition)
		attrs.POST("/batch", h.Admin.UserAttribute.GetBatchUserAttributes)
		attrs.PUT("/reorder", h.Admin.UserAttribute.ReorderDefinitions)
		attrs.PUT("/:id", h.Admin.UserAttribute.UpdateDefinition)
		attrs.DELETE("/:id", h.Admin.UserAttribute.DeleteDefinition)
	}
}

func registerScheduledTestRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	plans := admin.Group("/scheduled-test-plans")
	{
		plans.POST("", h.Admin.ScheduledTest.Create)
		plans.PUT("/:id", h.Admin.ScheduledTest.Update)
		plans.DELETE("/:id", h.Admin.ScheduledTest.Delete)
		plans.GET("/:id/results", h.Admin.ScheduledTest.ListResults)
	}
	// Nested under accounts
	admin.GET("/accounts/:id/scheduled-test-plans", h.Admin.ScheduledTest.ListByAccount)
}

func registerErrorPassthroughRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	rules := admin.Group("/error-passthrough-rules")
	{
		rules.GET("", h.Admin.ErrorPassthrough.List)
		rules.GET("/:id", h.Admin.ErrorPassthrough.GetByID)
		rules.POST("", h.Admin.ErrorPassthrough.Create)
		rules.PUT("/:id", h.Admin.ErrorPassthrough.Update)
		rules.DELETE("/:id", h.Admin.ErrorPassthrough.Delete)
	}
}

func registerTLSFingerprintProfileRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	profiles := admin.Group("/tls-fingerprint-profiles")
	{
		profiles.GET("", h.Admin.TLSFingerprintProfile.List)
		profiles.GET("/:id", h.Admin.TLSFingerprintProfile.GetByID)
		profiles.POST("", h.Admin.TLSFingerprintProfile.Create)
		profiles.PUT("/:id", h.Admin.TLSFingerprintProfile.Update)
		profiles.DELETE("/:id", h.Admin.TLSFingerprintProfile.Delete)
	}
}

func registerChannelRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	channels := admin.Group("/channels")
	{
		channels.GET("", h.Admin.Channel.List)
		channels.GET("/model-pricing", h.Admin.Channel.GetModelDefaultPricing)
		channels.GET("/:id", h.Admin.Channel.GetByID)
		channels.POST("", h.Admin.Channel.Create)
		channels.PUT("/:id", h.Admin.Channel.Update)
		channels.DELETE("/:id", h.Admin.Channel.Delete)
	}
}

func registerChannelMonitorRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	monitors := admin.Group("/channel-monitors")
	{
		monitors.GET("", h.Admin.ChannelMonitor.List)
		monitors.POST("", h.Admin.ChannelMonitor.Create)
		monitors.GET("/:id", h.Admin.ChannelMonitor.Get)
		monitors.PUT("/:id", h.Admin.ChannelMonitor.Update)
		monitors.DELETE("/:id", h.Admin.ChannelMonitor.Delete)
		monitors.POST("/:id/run", h.Admin.ChannelMonitor.Run)
		monitors.GET("/:id/history", h.Admin.ChannelMonitor.History)
	}

	templates := admin.Group("/channel-monitor-templates")
	{
		templates.GET("", h.Admin.ChannelMonitorTemplate.List)
		templates.POST("", h.Admin.ChannelMonitorTemplate.Create)
		templates.GET("/:id", h.Admin.ChannelMonitorTemplate.Get)
		templates.PUT("/:id", h.Admin.ChannelMonitorTemplate.Update)
		templates.DELETE("/:id", h.Admin.ChannelMonitorTemplate.Delete)
		templates.GET("/:id/monitors", h.Admin.ChannelMonitorTemplate.AssociatedMonitors)
		templates.POST("/:id/apply", h.Admin.ChannelMonitorTemplate.Apply)
	}
}

// registerAffiliateRoutes 注册邀请返利的管理端路由（专属用户配置）
func registerAffiliateRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	affiliates := admin.Group("/affiliates")
	{
		affiliates.GET("/invites", h.Admin.Affiliate.ListInviteRecords)
		affiliates.GET("/rebates", h.Admin.Affiliate.ListRebateRecords)
		affiliates.GET("/transfers", h.Admin.Affiliate.ListTransferRecords)

		users := affiliates.Group("/users")
		{
			users.GET("", h.Admin.Affiliate.ListUsers)
			users.GET("/lookup", h.Admin.Affiliate.LookupUsers)
			users.POST("/batch-rate", h.Admin.Affiliate.BatchSetRate)
			users.GET("/:user_id/overview", h.Admin.Affiliate.GetUserOverview)
			users.PUT("/:user_id", h.Admin.Affiliate.UpdateUserSettings)
			users.DELETE("/:user_id", h.Admin.Affiliate.ClearUserSettings)
		}
	}
}
