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
		organizationScoped.POST("/members/:member_id/balance", h.Organization.TransferBalance)
		organizationScoped.GET("/finance", h.Organization.Finance)
		organizationScoped.GET("/usage", h.Organization.Usage)
		organizationScoped.GET("/usage/stats", h.Organization.UsageStats)
		organizationScoped.GET("/usage/trend", h.Organization.UsageTrend)
	}
}
