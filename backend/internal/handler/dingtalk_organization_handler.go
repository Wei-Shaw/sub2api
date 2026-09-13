package handler

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type DingTalkOrganizationHandler struct {
	settings     *service.SettingService
	organization *service.DingTalkOrganizationService
	auth         *AuthHandler
}

func NewDingTalkOrganizationHandler(db *sql.DB, settings *service.SettingService, users *service.UserService, auth *AuthHandler) *DingTalkOrganizationHandler {
	return &DingTalkOrganizationHandler{settings: settings, organization: service.NewDingTalkOrganizationService(db, users), auth: auth}
}
func dingTalkActor(c *gin.Context) (int64, bool, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Authentication required")
		return 0, false, false
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	return subject.UserID, role == service.RoleAdmin, true
}
func dingTalkRequireAdmin(c *gin.Context) bool {
	_, admin, ok := dingTalkActor(c)
	if !ok {
		return false
	}
	if !admin {
		response.Forbidden(c, "Global administrator access required")
		return false
	}
	return true
}

func (h *DingTalkOrganizationHandler) Apps(c *gin.Context) {
	_, admin, ok := dingTalkActor(c)
	if !ok {
		return
	}
	apps, err := h.settings.GetDingTalkApps(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if admin {
		response.Success(c, service.RedactDingTalkApps(apps))
		return
	}
	managers, err := h.organization.Managers(c.Request.Context(), mustDingTalkActorID(c), false)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	allowed := map[string]bool{}
	for _, m := range managers {
		if m.Enabled {
			for _, d := range m.Departments {
				allowed[d.AppID] = true
			}
		}
	}
	result := []gin.H{}
	if allowed["default"] {
		if _, err := h.settings.GetDingTalkConnectOAuthConfig(c.Request.Context()); err == nil {
			result = append(result, gin.H{"id": "default", "name": "DingTalk", "enabled": true})
		}
	}
	for _, a := range apps {
		if a.Enabled && allowed[a.ID] {
			result = append(result, gin.H{"id": a.ID, "name": a.Name, "enabled": true})
		}
	}
	response.Success(c, result)
}
func mustDingTalkActorID(c *gin.Context) int64 {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	return subject.UserID
}
func (h *DingTalkOrganizationHandler) SaveApps(c *gin.Context) {
	if !dingTalkRequireAdmin(c) {
		return
	}
	var req struct {
		Apps []config.DingTalkAppConfig `json:"apps"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Apps == nil {
		response.BadRequest(c, "Applications array is required")
		return
	}
	if err := h.organization.SaveApps(c.Request.Context(), h.settings, req.Apps); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, service.RedactDingTalkApps(req.Apps))
}
func (h *DingTalkOrganizationHandler) PublicApps(c *gin.Context) {
	apps, err := h.settings.GetDingTalkApps(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result := []gin.H{}
	if _, err := h.settings.GetDingTalkConnectOAuthConfig(c.Request.Context()); err == nil {
		result = append(result, gin.H{"id": "default", "name": "DingTalk"})
	}
	for _, a := range apps {
		if a.Enabled {
			result = append(result, gin.H{"id": a.ID, "name": a.Name})
		}
	}
	response.Success(c, result)
}
func (h *DingTalkOrganizationHandler) Directory(c *gin.Context) {
	actor, admin, ok := dingTalkActor(c)
	if !ok {
		return
	}
	if _, err := h.settings.GetDingTalkOAuthConfigForApp(c.Request.Context(), c.Param("app")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result, err := h.organization.Directory(c.Request.Context(), c.Param("app"), actor, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
func (h *DingTalkOrganizationHandler) Sync(c *gin.Context) {
	if !dingTalkRequireAdmin(c) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	cfg, err := h.settings.GetDingTalkOAuthConfigForApp(ctx, c.Param("app"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	ds, ms, err := h.auth.dingTalkClient(cfg).ReadOrganization(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err = h.organization.ReplaceDirectory(ctx, c.Param("app"), ds, ms); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"departments": len(ds), "members": len(ms)})
}
func (h *DingTalkOrganizationHandler) Managers(c *gin.Context) {
	actor, admin, ok := dingTalkActor(c)
	if !ok {
		return
	}
	result, err := h.organization.Managers(c.Request.Context(), actor, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
func (h *DingTalkOrganizationHandler) SaveManager(c *gin.Context) {
	if !dingTalkRequireAdmin(c) {
		return
	}
	var req service.DingTalkManager
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid manager configuration")
		return
	}
	if err := h.organization.SaveManager(c.Request.Context(), req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}
func (h *DingTalkOrganizationHandler) Grant(c *gin.Context) {
	actor, admin, ok := dingTalkActor(c)
	if !ok {
		return
	}
	var req struct {
		AppID        string  `json:"app_id"`
		DepartmentID int64   `json:"department_id"`
		TargetID     int64   `json:"target_id"`
		Amount       float64 `json:"amount"`
		RequestID    string  `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid quota allocation")
		return
	}
	// Re-read configuration for each write so disabling an application takes effect.
	if _, err := h.settings.GetDingTalkOAuthConfigForApp(c.Request.Context(), req.AppID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	cents, err := service.DingTalkQuotaCents(req.Amount)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result, err := h.organization.Grant(c.Request.Context(), service.DingTalkQuotaGrant{ActorID: actor, TargetID: req.TargetID, AppID: req.AppID, DepartmentID: req.DepartmentID, AmountCents: cents, RequestID: req.RequestID}, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
func (h *DingTalkOrganizationHandler) Grants(c *gin.Context) {
	actor, admin, ok := dingTalkActor(c)
	if !ok {
		return
	}
	result, err := h.organization.Grants(c.Request.Context(), actor, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
