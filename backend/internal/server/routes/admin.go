// Package routes provides HTTP route registration and handlers.
package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes æ³¨åç®¡çåè·¯ç±
func RegisterAdminRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	adminAuth middleware.AdminAuthMiddleware,
	settingService *service.SettingService,
) {
	admin := v1.Group("/admin")
	admin.Use(gin.HandlerFunc(adminAuth))
	admin.Use(middleware.AdminComplianceGuard(settingService))
	{
		// 部署与运营合规确认
		registerAdminComplianceRoutes(admin, h)

		// 仪表盘
		registerDashboardRoutes(admin, h)

		// ç¨æ·ç®¡ç
		registerUserManagementRoutes(admin, h)

		// åç»ç®¡ç
		registerGroupRoutes(admin, h)

		// è´¦å·ç®¡ç
		registerAccountRoutes(admin, h)

		// å¬åç®¡ç
		registerAnnouncementRoutes(admin, h)

		// ä»£çç®¡ç
		registerProxyRoutes(admin, h)

		// å¡å¯ç®¡ç
		registerRedeemCodeRoutes(admin, h)

		// ä¼æ ç ç®¡ç
		registerPromoCodeRoutes(admin, h)

		// ç³»ç»è®¾ç½®
		registerSettingsRoutes(admin, h)

		// æ°æ®ç®¡ç
		registerDataManagementRoutes(admin, h)

		// æ°æ®åºå¤ä»½æ¢å¤
		registerBackupRoutes(admin, h)

		// è¿ç»´çæ§ï¼Opsï¼
		registerOpsRoutes(admin, h)

		// ç³»ç»ç®¡ç
		registerSystemRoutes(admin, h)

		// è®¢éç®¡ç
		registerSubscriptionRoutes(admin, h)

		// ä½¿ç¨è®°å½ç®¡ç
		registerUsageRoutes(admin, h)

		// ç¨æ·å±æ§ç®¡ç
		registerUserAttributeRoutes(admin, h)

		// éè¯¯éä¼ è§åç®¡ç
		registerErrorPassthroughRoutes(admin, h)

		// TLS æçº¹æ¨¡æ¿ç®¡ç
		registerTLSFingerprintProfileRoutes(admin, h)

		// API Key ç®¡ç
		registerAdminAPIKeyRoutes(admin, h)

		// å®æ¶æµè¯è®¡å
		registerScheduledTestRoutes(admin, h)

		// æ¸ éç®¡ç
		// 渠道管理已迁移到 plugins/channel-management/，路由由插件注册

		// 渠道监控路由已迁移到 plugins/channel-management/

		// 插件管理
		registerPluginRoutes(admin, h)
		registerPluginSettingsRoutes(admin, h)
		// 平台管理
		registerPlatformRoutes(admin, h)

		// 服务限额（Service Quota）
		registerServiceQuotaRoutes(admin, h)

		// 风控中心（content moderation）已迁移到 plugins/content-moderation/

		// 邀请返利（专属用户管理）
		registerAffiliateRoutes(admin, h)

		// 文件上传（图片）— admin auth, 写入 UploadService.UploadDir
		registerUploadRoutes(admin, h)
	}
}

func registerAdminComplianceRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	compliance := admin.Group("/compliance")
	{
		compliance.GET("", h.Admin.Compliance.GetStatus)
		compliance.POST("/accept", h.Admin.Compliance.Accept)
	}
}

// registerUploadRoutes mounts the admin image upload endpoint. The matching
// public download route lives in uploads.go (RegisterUploadRoutes) and is
// registered without auth at the engine root.
func registerUploadRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	if h.Admin.Upload == nil {
		return
	}
	uploads := admin.Group("/uploads")
	{
		uploads.POST("/image", h.Admin.Upload.UploadImage)
	}
}

func registerServiceQuotaRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	if h.Admin.ServiceQuota == nil {
		return
	}
	quotas := admin.Group("/service-quotas")
	{
		quotas.GET("", h.Admin.ServiceQuota.List)
		quotas.POST("", h.Admin.ServiceQuota.Create)
		quotas.PUT("/:id", h.Admin.ServiceQuota.Update)
		quotas.DELETE("/:id", h.Admin.ServiceQuota.Delete)
		// 手动重置 Redis 计数器（按 rule_id + path_id + limiter_type [+ scope_user_id]）
		quotas.POST("/reset", h.Admin.ServiceQuota.ResetCounter)
		// 运行时监控（独立 handler，规则 CRUD 之外的只读快照入口）
		if h.Admin.ServiceQuotaMonitor != nil {
			quotas.GET("/monitor", h.Admin.ServiceQuotaMonitor.Snapshot)
		}
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

		// Service quota metrics (in-memory atomic counters; not gated by monitoring switch)
		ops.GET("/service-quota-metrics", h.Admin.Ops.GetServiceQuotaMetrics)
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
		accounts.POST("/:id/revert-proxy-fallback", h.Admin.Account.RevertProxyFallback)
		accounts.GET("/:id/usage", h.Admin.Account.GetUsage)
		accounts.GET("/:id/today-stats", h.Admin.Account.GetTodayStats)
		accounts.POST("/today-stats/batch", h.Admin.Account.GetBatchTodayStats)
		accounts.POST("/:id/clear-rate-limit", h.Admin.Account.ClearRateLimit)
		accounts.POST("/:id/reset-quota", h.Admin.Account.ResetQuota)
		accounts.GET("/:id/temp-unschedulable", h.Admin.Account.GetTempUnschedulable)
		accounts.DELETE("/:id/temp-unschedulable", h.Admin.Account.ClearTempUnschedulable)
		accounts.POST("/:id/schedulable", h.Admin.Account.SetSchedulable)
		accounts.POST("/models/sync-upstream-preview", h.Admin.Account.SyncUpstreamModelsPreview)
		accounts.GET("/:id/models", h.Admin.Account.GetAvailableModels)
		accounts.POST("/batch", h.Admin.Account.BatchCreate)
		accounts.GET("/data", h.Admin.Account.ExportData)
		accounts.POST("/data", h.Admin.Account.ImportData)
		accounts.POST("/batch-update-credentials", h.Admin.Account.BatchUpdateCredentials)
		accounts.POST("/batch-refresh-tier", h.Admin.Account.BatchRefreshTier)
		accounts.POST("/bulk-update", h.Admin.Account.BulkUpdate)
		accounts.POST("/batch-clear-error", h.Admin.Account.BatchClearError)
		accounts.POST("/batch-refresh", h.Admin.Account.BatchRefresh)

		// Antigravity é»è®¤æ¨¡åæ å°
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
		// Admin API Key ç®¡ç
		adminSettings.GET("/admin-api-key", h.Admin.Setting.GetAdminAPIKey)
		adminSettings.POST("/admin-api-key/regenerate", h.Admin.Setting.RegenerateAdminAPIKey)
		adminSettings.DELETE("/admin-api-key", h.Admin.Setting.DeleteAdminAPIKey)
		// 529è¿è½½å·å´éç½®
		adminSettings.GET("/overload-cooldown", h.Admin.Setting.GetOverloadCooldownSettings)
		adminSettings.PUT("/overload-cooldown", h.Admin.Setting.UpdateOverloadCooldownSettings)
		// 429默认回避配置
		adminSettings.GET("/rate-limit-429-cooldown", h.Admin.Setting.GetRateLimit429CooldownSettings)
		adminSettings.PUT("/rate-limit-429-cooldown", h.Admin.Setting.UpdateRateLimit429CooldownSettings)
		// 流超时处理配置
		adminSettings.GET("/stream-timeout", h.Admin.Setting.GetStreamTimeoutSettings)
		adminSettings.PUT("/stream-timeout", h.Admin.Setting.UpdateStreamTimeoutSettings)
		// è¯·æ±æ´æµå¨éç½®
		adminSettings.GET("/rectifier", h.Admin.Setting.GetRectifierSettings)
		adminSettings.PUT("/rectifier", h.Admin.Setting.UpdateRectifierSettings)
		// Beta ç­ç¥éç½®
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
		// S3 å­å¨éç½®
		backup.GET("/s3-config", h.Admin.Backup.GetS3Config)
		backup.PUT("/s3-config", h.Admin.Backup.UpdateS3Config)
		backup.POST("/s3-config/test", h.Admin.Backup.TestS3Connection)

		// å®æ¶å¤ä»½éç½®
		backup.GET("/schedule", h.Admin.Backup.GetSchedule)
		backup.PUT("/schedule", h.Admin.Backup.UpdateSchedule)

		// å¤ä»½æä½
		backup.POST("", h.Admin.Backup.CreateBackup)
		backup.GET("", h.Admin.Backup.ListBackups)
		backup.GET("/:id", h.Admin.Backup.GetBackup)
		backup.DELETE("/:id", h.Admin.Backup.DeleteBackup)
		backup.GET("/:id/download-url", h.Admin.Backup.GetDownloadURL)

		// æ¢å¤æä½
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

	// åç»ä¸çè®¢éåè¡¨
	admin.GET("/groups/:id/subscriptions", h.Admin.Subscription.ListByGroup)

	// ç¨æ·ä¸çè®¢éåè¡¨
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

func registerPluginRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	plugins := admin.Group("/plugins")
	{
		plugins.GET("", h.Admin.Plugin.List)
		plugins.GET("/:name", h.Admin.Plugin.Get)
		plugins.POST("/:name/enable", h.Admin.Plugin.Enable)
		plugins.POST("/:name/disable", h.Admin.Plugin.Disable)
		plugins.POST("/:name/restart", h.Admin.Plugin.Restart)
		plugins.PUT("/:name/config", h.Admin.Plugin.UpdateConfig)
		// P13/C-1 软卸载: uninstall 关进程 + 标记 uninstalled_at (数据保留),
		// install 撤回软卸载。两个 endpoint 都走 admin auth middleware。
		plugins.POST("/:name/uninstall", h.Admin.Plugin.Uninstall)
		plugins.POST("/:name/install", h.Admin.Plugin.Install)
		// P13/C-2 硬卸载: 物理清除已经软卸载的插件全部数据。 不可逆, 因此要求
		// purge=true query + 请求体携带 name 二次确认; 仍 active 的插件返回 409,
		// 强制走完 "先 Uninstall 再 Purge" 的流程。
		plugins.DELETE("/:name", h.Admin.Plugin.Delete)
		// 删除插件文件: 清除磁盘二进制但保留数据库数据。插件必须先禁用。
		plugins.POST("/:name/remove-files", h.Admin.Plugin.RemoveFiles)
	}
}

// registerPluginSettingsRoutes wires the V5/W3 plugin settings admin REST
// surface. Endpoints:
//
//	GET  /api/v1/admin/plugin-settings                       — list namespaces
//	GET  /api/v1/admin/plugin-settings/:plugin               — schema + values
//	PUT  /api/v1/admin/plugin-settings/:plugin/:key          — update value
func registerPluginSettingsRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	settings := admin.Group("/plugin-settings")
	{
		settings.GET("", h.Admin.PluginSettings.List)
		settings.GET("/:plugin", h.Admin.PluginSettings.Get)
		settings.PUT("/:plugin/:key", h.Admin.PluginSettings.Update)
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

// registerPlatformRoutes wires the platforms API endpoints.
//
//	GET /api/v1/admin/platforms             — all registered platforms
//	GET /api/v1/admin/platforms/:platform   — single platform details
func registerPlatformRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	platforms := admin.Group("/platforms")
	{
		platforms.GET("", h.Admin.Platform.List)
		platforms.GET("/:platform", h.Admin.Platform.Get)
		platforms.GET("/:platform/models", h.Admin.Platform.GetModels)
	}
}
