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

		// OpenAI OAuth
		registerOpenAIOAuthRoutes(admin, h)

		// 代理管理
		registerProxyRoutes(admin, h)

		// 卡密管理
		registerRedeemCodeRoutes(admin, h)

		// 系统设置
		registerSettingsRoutes(admin, h)

		// 系统管理
		registerSystemRoutes(admin, h)

		// 订阅管理
		registerSubscriptionRoutes(admin, h)

		// 使用记录管理
		registerUsageRoutes(admin, h)
REDACTED
REDACTED

func registerDashboardRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	dashboard := admin.Group("/dashboard")
	{
		dashboard.GET("/stats", h.Admin.Dashboard.GetStats)
		dashboard.GET("/realtime", h.Admin.Dashboard.GetRealtimeMetrics)
		dashboard.GET("/trend", h.Admin.Dashboard.GetUsageTrend)
		dashboard.GET("/models", h.Admin.Dashboard.GetModelStats)
		dashboard.GET("/api-keys-trend", h.Admin.Dashboard.GetApiKeyUsageTrend)
		dashboard.GET("/users-trend", h.Admin.Dashboard.GetUserUsageTrend)
		dashboard.POST("/users-usage", h.Admin.Dashboard.GetBatchUsersUsage)
		dashboard.POST("/api-keys-usage", h.Admin.Dashboard.GetBatchApiKeysUsage)
REDACTED
REDACTED

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
REDACTED
REDACTED

func registerGroupRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	groups := admin.Group("/groups")
	{
		groups.GET("", h.Admin.Group.List)
		groups.GET("/all", h.Admin.Group.GetAll)
		groups.GET("/:id", h.Admin.Group.GetByID)
		groups.POST("", h.Admin.Group.Create)
		groups.PUT("/:id", h.Admin.Group.Update)
		groups.DELETE("/:id", h.Admin.Group.Delete)
		groups.GET("/:id/stats", h.Admin.Group.GetStats)
		groups.GET("/:id/api-keys", h.Admin.Group.GetGroupAPIKeys)
REDACTED
REDACTED

func registerAccountRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	accounts := admin.Group("/accounts")
	{
		accounts.GET("", h.Admin.Account.List)
		accounts.GET("/:id", h.Admin.Account.GetByID)
		accounts.POST("", h.Admin.Account.Create)
		accounts.POST("/sync/crs", h.Admin.Account.SyncFromCRS)
		accounts.PUT("/:id", h.Admin.Account.Update)
		accounts.DELETE("/:id", h.Admin.Account.Delete)
		accounts.POST("/:id/test", h.Admin.Account.Test)
		accounts.POST("/:id/refresh", h.Admin.Account.Refresh)
		accounts.GET("/:id/stats", h.Admin.Account.GetStats)
		accounts.POST("/:id/clear-error", h.Admin.Account.ClearError)
		accounts.GET("/:id/usage", h.Admin.Account.GetUsage)
		accounts.GET("/:id/today-stats", h.Admin.Account.GetTodayStats)
		accounts.POST("/:id/clear-rate-limit", h.Admin.Account.ClearRateLimit)
		accounts.POST("/:id/schedulable", h.Admin.Account.SetSchedulable)
		accounts.GET("/:id/models", h.Admin.Account.GetAvailableModels)
		accounts.POST("/batch", h.Admin.Account.BatchCreate)
		accounts.POST("/batch-update-credentials", h.Admin.Account.BatchUpdateCredentials)
		accounts.POST("/bulk-update", h.Admin.Account.BulkUpdate)

		// Claude OAuth routes
		accounts.POST("/generate-auth-url", h.Admin.OAuth.GenerateAuthURL)
		accounts.POST("/generate-setup-token-url", h.Admin.OAuth.GenerateSetupTokenURL)
		accounts.POST("/exchange-code", h.Admin.OAuth.ExchangeCode)
		accounts.POST("/exchange-setup-token-code", h.Admin.OAuth.ExchangeSetupTokenCode)
		accounts.POST("/cookie-auth", h.Admin.OAuth.CookieAuth)
		accounts.POST("/setup-token-cookie-auth", h.Admin.OAuth.SetupTokenCookieAuth)
REDACTED
REDACTED

func registerOpenAIOAuthRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	openai := admin.Group("/openai")
	{
		openai.POST("/generate-auth-url", h.Admin.OpenAIOAuth.GenerateAuthURL)
		openai.POST("/exchange-code", h.Admin.OpenAIOAuth.ExchangeCode)
		openai.POST("/refresh-token", h.Admin.OpenAIOAuth.RefreshToken)
		openai.POST("/accounts/:id/refresh", h.Admin.OpenAIOAuth.RefreshAccountToken)
		openai.POST("/create-from-oauth", h.Admin.OpenAIOAuth.CreateAccountFromOAuth)
REDACTED
REDACTED

func registerProxyRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	proxies := admin.Group("/proxies")
	{
		proxies.GET("", h.Admin.Proxy.List)
		proxies.GET("/all", h.Admin.Proxy.GetAll)
		proxies.GET("/:id", h.Admin.Proxy.GetByID)
		proxies.POST("", h.Admin.Proxy.Create)
		proxies.PUT("/:id", h.Admin.Proxy.Update)
		proxies.DELETE("/:id", h.Admin.Proxy.Delete)
		proxies.POST("/:id/test", h.Admin.Proxy.Test)
		proxies.GET("/:id/stats", h.Admin.Proxy.GetStats)
		proxies.GET("/:id/accounts", h.Admin.Proxy.GetProxyAccounts)
		proxies.POST("/batch", h.Admin.Proxy.BatchCreate)
REDACTED
REDACTED

func registerRedeemCodeRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	codes := admin.Group("/redeem-codes")
	{
		codes.GET("", h.Admin.Redeem.List)
		codes.GET("/stats", h.Admin.Redeem.GetStats)
		codes.GET("/export", h.Admin.Redeem.Export)
		codes.GET("/:id", h.Admin.Redeem.GetByID)
		codes.POST("/generate", h.Admin.Redeem.Generate)
		codes.DELETE("/:id", h.Admin.Redeem.Delete)
		codes.POST("/batch-delete", h.Admin.Redeem.BatchDelete)
		codes.POST("/:id/expire", h.Admin.Redeem.Expire)
REDACTED
REDACTED

func registerSettingsRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	adminSettings := admin.Group("/settings")
	{
		adminSettings.GET("", h.Admin.Setting.GetSettings)
		adminSettings.PUT("", h.Admin.Setting.UpdateSettings)
		adminSettings.POST("/test-smtp", h.Admin.Setting.TestSmtpConnection)
		adminSettings.POST("/send-test-email", h.Admin.Setting.SendTestEmail)
		// Admin API Key 管理
		adminSettings.GET("/admin-api-key", h.Admin.Setting.GetAdminApiKey)
		adminSettings.POST("/admin-api-key/regenerate", h.Admin.Setting.RegenerateAdminApiKey)
		adminSettings.DELETE("/admin-api-key", h.Admin.Setting.DeleteAdminApiKey)
REDACTED
REDACTED

func registerSystemRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	system := admin.Group("/system")
	{
		system.GET("/version", h.Admin.System.GetVersion)
		system.GET("/check-updates", h.Admin.System.CheckUpdates)
		system.POST("/update", h.Admin.System.PerformUpdate)
		system.POST("/rollback", h.Admin.System.Rollback)
		system.POST("/restart", h.Admin.System.RestartService)
REDACTED
REDACTED

func registerSubscriptionRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	subscriptions := admin.Group("/subscriptions")
	{
		subscriptions.GET("", h.Admin.Subscription.List)
		subscriptions.GET("/:id", h.Admin.Subscription.GetByID)
		subscriptions.GET("/:id/progress", h.Admin.Subscription.GetProgress)
		subscriptions.POST("/assign", h.Admin.Subscription.Assign)
		subscriptions.POST("/bulk-assign", h.Admin.Subscription.BulkAssign)
		subscriptions.POST("/:id/extend", h.Admin.Subscription.Extend)
		subscriptions.DELETE("/:id", h.Admin.Subscription.Revoke)
REDACTED

	// 分组下的订阅列表
	admin.GET("/groups/:id/subscriptions", h.Admin.Subscription.ListByGroup)

	// 用户下的订阅列表
	admin.GET("/users/:id/subscriptions", h.Admin.Subscription.ListByUser)
REDACTED

func registerUsageRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	usage := admin.Group("/usage")
	{
		usage.GET("", h.Admin.Usage.List)
		usage.GET("/stats", h.Admin.Usage.Stats)
		usage.GET("/search-users", h.Admin.Usage.SearchUsers)
		usage.GET("/search-api-keys", h.Admin.Usage.SearchApiKeys)
REDACTED
REDACTED
