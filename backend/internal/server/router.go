package server

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/web"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetupRouter 配置路由器中间件和路由
func SetupRouter(r *gin.Engine, cfg *config.Config, handlers *handler.Handlers, services *service.Services, repos *repository.Repositories) *gin.Engine {
	// 显式设置可信代理，避免默认信任所有代理导致 ClientIP 被伪造。
	if len(cfg.Server.TrustedProxies) == 0 {
		if err := r.SetTrustedProxies(nil); err != nil {
			log.Printf("failed to clear trusted proxies: %v", err)
		}
	} else if err := r.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		log.Printf("failed to set trusted proxies: %v", err)
	}

	// 应用中间件
	r.Use(middleware.Logger())
	r.Use(middleware.CORS(&cfg.CORS))

	// 注册路由
	registerRoutes(r, handlers, services, repos)

	// Serve embedded frontend if available
	if web.HasEmbeddedFrontend() {
		r.Use(web.ServeEmbeddedFrontend())
	}

	return r
}

// registerRoutes 注册所有 HTTP 路由
func registerRoutes(r *gin.Engine, h *handler.Handlers, s *service.Services, repos *repository.Repositories) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Claude Code 遥测日志（忽略，直接返回200）
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 公开接口
		auth := v1.Group("/auth")
		{
			auth.POST("/register", h.Auth.Register)
			auth.POST("/login", h.Auth.Login)
			auth.POST("/send-verify-code", h.Auth.SendVerifyCode)
			auth.POST("/logout", h.Auth.Logout)
		}

		// 公开设置（无需认证）
		settings := v1.Group("/settings")
		{
			settings.GET("/public", h.Setting.GetPublicSettings)
		}

		// 需要认证的接口
		authenticated := v1.Group("")
		authenticated.Use(middleware.JWTAuth(s.Auth, repos.User))
		{
			// 当前用户信息
			authenticated.GET("/auth/me", h.Auth.GetCurrentUser)

			// 用户接口
			user := authenticated.Group("/user")
			{
				user.GET("/profile", h.User.GetProfile)
				user.PUT("/password", h.User.ChangePassword)
				user.PUT("", h.User.UpdateProfile)
			}

			// API Key管理
			keys := authenticated.Group("/keys")
			{
				keys.GET("", h.APIKey.List)
				keys.GET("/:id", h.APIKey.GetByID)
				keys.POST("", h.APIKey.Create)
				keys.PUT("/:id", h.APIKey.Update)
				keys.DELETE("/:id", h.APIKey.Delete)
			}

			// 用户可用分组（非管理员接口）
			groups := authenticated.Group("/groups")
			{
				groups.GET("/available", h.APIKey.GetAvailableGroups)
			}

			// 使用记录
			usage := authenticated.Group("/usage")
			{
				usage.GET("", h.Usage.List)
				usage.GET("/:id", h.Usage.GetByID)
				usage.GET("/stats", h.Usage.Stats)
				// User dashboard endpoints
				usage.GET("/dashboard/stats", h.Usage.DashboardStats)
				usage.GET("/dashboard/trend", h.Usage.DashboardTrend)
				usage.GET("/dashboard/models", h.Usage.DashboardModels)
				usage.POST("/dashboard/api-keys-usage", h.Usage.DashboardApiKeysUsage)
			}

			// 卡密兑换
			redeem := authenticated.Group("/redeem")
			{
				redeem.POST("", h.Redeem.Redeem)
				redeem.GET("/history", h.Redeem.GetHistory)
			}

			// 用户订阅
			subscriptions := authenticated.Group("/subscriptions")
			{
				subscriptions.GET("", h.Subscription.List)
				subscriptions.GET("/active", h.Subscription.GetActive)
				subscriptions.GET("/progress", h.Subscription.GetProgress)
				subscriptions.GET("/summary", h.Subscription.GetSummary)
			}
		}

		// 管理员接口
		admin := v1.Group("/admin")
		admin.Use(middleware.AdminAuth(s.Auth, repos.User, s.Setting))
		{
			// 仪表盘
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
			}

			// 用户管理
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
			}

			// 分组管理
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
			}

			// 账号管理
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
			}

			// OpenAI OAuth routes
			openai := admin.Group("/openai")
			{
				openai.POST("/generate-auth-url", h.Admin.OpenAIOAuth.GenerateAuthURL)
				openai.POST("/exchange-code", h.Admin.OpenAIOAuth.ExchangeCode)
				openai.POST("/refresh-token", h.Admin.OpenAIOAuth.RefreshToken)
				openai.POST("/accounts/:id/refresh", h.Admin.OpenAIOAuth.RefreshAccountToken)
				openai.POST("/create-from-oauth", h.Admin.OpenAIOAuth.CreateAccountFromOAuth)
			}

			// 代理管理
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
			}

			// 卡密管理
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
			}

			// 系统设置
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
			}

			// 系统管理
			system := admin.Group("/system")
			{
				system.GET("/version", h.Admin.System.GetVersion)
				system.GET("/check-updates", h.Admin.System.CheckUpdates)
				system.POST("/update", h.Admin.System.PerformUpdate)
				system.POST("/rollback", h.Admin.System.Rollback)
				system.POST("/restart", h.Admin.System.RestartService)
			}

			// 订阅管理
			subscriptions := admin.Group("/subscriptions")
			{
				subscriptions.GET("", h.Admin.Subscription.List)
				subscriptions.GET("/:id", h.Admin.Subscription.GetByID)
				subscriptions.GET("/:id/progress", h.Admin.Subscription.GetProgress)
				subscriptions.POST("/assign", h.Admin.Subscription.Assign)
				subscriptions.POST("/bulk-assign", h.Admin.Subscription.BulkAssign)
				subscriptions.POST("/:id/extend", h.Admin.Subscription.Extend)
				subscriptions.DELETE("/:id", h.Admin.Subscription.Revoke)
			}

			// 分组下的订阅列表
			admin.GET("/groups/:id/subscriptions", h.Admin.Subscription.ListByGroup)

			// 用户下的订阅列表
			admin.GET("/users/:id/subscriptions", h.Admin.Subscription.ListByUser)

			// 使用记录管理
			usage := admin.Group("/usage")
			{
				usage.GET("", h.Admin.Usage.List)
				usage.GET("/stats", h.Admin.Usage.Stats)
				usage.GET("/search-users", h.Admin.Usage.SearchUsers)
				usage.GET("/search-api-keys", h.Admin.Usage.SearchApiKeys)
			}
		}
	}

	// API网关（Claude API兼容）
	gateway := r.Group("/v1")
	gateway.Use(middleware.ApiKeyAuthWithSubscription(s.ApiKey, s.Subscription))
	{
		gateway.POST("/messages", h.Gateway.Messages)
		gateway.POST("/messages/count_tokens", h.Gateway.CountTokens)
		gateway.GET("/models", h.Gateway.Models)
		gateway.GET("/usage", h.Gateway.Usage)
		// OpenAI Responses API
		gateway.POST("/responses", h.OpenAIGateway.Responses)
	}

	// OpenAI Responses API（不带v1前缀的别名）
	r.POST("/responses", middleware.ApiKeyAuthWithSubscription(s.ApiKey, s.Subscription), h.OpenAIGateway.Responses)
}
