package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterOrganizationRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	auditLog middleware.AuditLogMiddleware,
	settingService *service.SettingService,
) {
	if h == nil || h.Organization == nil {
		return
	}
	authenticated := v1.Group("/organization")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	authenticated.Use(gin.HandlerFunc(auditLog))
	{
		authenticated.GET("/applications/current", h.Organization.CurrentApplication)
		authenticated.GET("/applications/eligibility", h.Organization.UpgradeEligibility)
		authenticated.POST("/applications", h.Organization.SubmitApplication)
		authenticated.POST("/applications/:application_id/withdraw", h.Organization.WithdrawApplication)
	}

	organizationScoped := authenticated.Group("")
	organizationScoped.Use(h.Organization.RequireOrganization)
	{
		organizationScoped.GET("/context", h.Organization.Context)
		organizationScoped.POST("/name-change-requests", h.Organization.RequestNameChange)
		organizationScoped.GET("/members", h.Organization.ListMembers)
		organizationScoped.POST("/members", h.Organization.CreateMember)
		organizationScoped.GET("/members/:member_id", h.Organization.GetMember)
		organizationScoped.PATCH("/members/:member_id/status", h.Organization.SetMemberStatus)
		organizationScoped.POST("/members/:member_id/reset-password", h.Organization.ResetMemberPassword)
		organizationScoped.PUT("/password", h.Organization.ChangePassword)
		organizationScoped.POST("/recovery-email/send-code", h.Organization.SendRecoveryEmailCode)
		organizationScoped.POST("/recovery-email/verify", h.Organization.VerifyRecoveryEmail)
		organizationScoped.GET("/policies", h.Organization.ListPolicies)
		organizationScoped.GET("/members/:member_id/policies", h.Organization.ListMemberPolicies)
		organizationScoped.PUT("/members/:member_id/policies", h.Organization.SetPolicy)
		organizationScoped.GET("/spend-limits", h.Organization.ListSpendLimits)
		organizationScoped.PUT("/spend-limits", h.Organization.UpsertSpendLimits)
		organizationScoped.DELETE("/spend-limits", h.Organization.DeleteSpendLimit)
		organizationScoped.GET("/spend-limits/usage", h.Organization.SpendLimitUsage)
		organizationScoped.POST("/members/:member_id/balance", h.Organization.TransferBalance)
		organizationScoped.POST("/company-balance", h.Organization.CompanyBalanceTransfer)
		organizationScoped.GET("/subscriptions", h.Organization.ListSubscriptions)
		organizationScoped.GET("/subscription-groups", h.Organization.SubscriptionGroups)
		organizationScoped.GET("/subscriptions/fallback", h.Organization.SubscriptionFallback)
		organizationScoped.POST("/subscriptions", h.Organization.CreateSubscription)
		organizationScoped.POST("/subscription-orders", h.Organization.CreateSubscriptionOrder)
		organizationScoped.DELETE("/subscriptions/:subscription_id", h.Organization.CancelSubscription)
		organizationScoped.GET("/finance", h.Organization.Finance)
		organizationScoped.GET("/dashboard", h.Organization.Dashboard)
		organizationScoped.GET("/dashboard/spending-ranking", h.Organization.SpendingRanking)
		organizationScoped.GET("/dashboard/user-breakdown", h.Organization.UserBreakdown)
		organizationScoped.GET("/dashboard/users-trend", h.Organization.UsersTrend)
		organizationScoped.GET("/usage", h.Organization.Usage)
		organizationScoped.GET("/usage/api-keys/search", h.Organization.SearchAPIKeys)
		organizationScoped.GET("/usage/stats", h.Organization.UsageStats)
		organizationScoped.GET("/usage/trend", h.Organization.UsageTrend)
		organizationScoped.GET("/usage/charts", h.Organization.UsageCharts)
		organizationScoped.GET("/usage/errors", h.Organization.UsageErrors)
		organizationScoped.GET("/usage/errors/:error_id", h.Organization.UsageErrorDetail)
		organizationScoped.GET("/audit-events", h.Organization.AuditEvents)
		organizationScoped.GET("/settings", h.Organization.Settings)
		organizationScoped.PUT("/settings", h.Organization.UpdateSettings)
	}
}
