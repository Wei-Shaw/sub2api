package admin

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type CustomDomainHandler struct {
	customDomainService *service.CustomDomainService
}

func NewCustomDomainHandler(customDomainService *service.CustomDomainService) *CustomDomainHandler {
	return &CustomDomainHandler{customDomainService: customDomainService}
}

type updateCustomDomainConfigRequest struct {
	Enabled bool `json:"enabled"`
}

type createAdminCustomDomainRequest struct {
	UserID   int64   `json:"user_id" binding:"required"`
	Domain   string  `json:"domain" binding:"required"`
	AllUsers bool    `json:"all_users"`
	UserIDs  []int64 `json:"user_ids"`
}

type updateCustomDomainAccessRequest struct {
	AllUsers bool    `json:"all_users"`
	UserIDs  []int64 `json:"user_ids"`
}

type disableCustomDomainRequest struct {
	Reason string `json:"reason"`
}

func (h *CustomDomainHandler) GetConfig(c *gin.Context) {
	response.Success(c, dto.CustomDomainConfig{
		Enabled:     h.customDomainService.IsEnabled(c.Request.Context()),
		CNAMETarget: h.customDomainService.GatewayTarget(c.Request.Context()),
	})
}

func (h *CustomDomainHandler) UpdateConfig(c *gin.Context) {
	var req updateCustomDomainConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if err := h.customDomainService.SetEnabled(c.Request.Context(), req.Enabled); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "custom domain feature updated", 0, "", "enabled", req.Enabled)
	response.Success(c, dto.CustomDomainConfig{
		Enabled:     req.Enabled,
		CNAMETarget: h.customDomainService.GatewayTarget(c.Request.Context()),
	})
}

func (h *CustomDomainHandler) List(c *gin.Context) {
	var userID *int64
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		userID = &id
	}
	var allUsers *bool
	if raw := strings.TrimSpace(c.Query("all_users")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "Invalid all_users")
			return
		}
		allUsers = &parsed
	}
	domains, err := h.customDomainService.ListAll(c.Request.Context(), service.CustomDomainListFilters{
		Domain:   strings.TrimSpace(c.Query("domain")),
		Status:   strings.TrimSpace(c.Query("status")),
		UserID:   userID,
		AllUsers: allUsers,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.CustomDomainsFromService(domains))
}

func (h *CustomDomainHandler) Create(c *gin.Context) {
	var req createAdminCustomDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID <= 0 {
		response.BadRequest(c, "Invalid request body")
		return
	}
	access := service.CustomDomainAccessInput{AllUsers: req.AllUsers, UserIDs: req.UserIDs}
	domain, err := h.customDomainService.CreateForUserWithAccess(c.Request.Context(), req.UserID, strings.TrimSpace(req.Domain), access)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "custom domain created", domain.ID, domain.Domain, "owner_user_id", domain.UserID, "all_users", domain.AllUsers, "user_ids", domain.UserIDs)
	response.Success(c, dto.CustomDomainFromService(domain))
}

func (h *CustomDomainHandler) UpdateAccess(c *gin.Context) {
	id, err := parseAdminCustomDomainID(c)
	if err != nil {
		response.BadRequest(c, "Invalid custom domain id")
		return
	}
	var req updateCustomDomainAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	domain, err := h.customDomainService.UpdateAccessAsAdmin(c.Request.Context(), id, service.CustomDomainAccessInput{
		AllUsers: req.AllUsers,
		UserIDs:  req.UserIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "custom domain access updated", domain.ID, domain.Domain, "all_users", domain.AllUsers, "user_ids", domain.UserIDs)
	response.Success(c, dto.CustomDomainFromService(domain))
}

func (h *CustomDomainHandler) Verify(c *gin.Context) {
	id, err := parseAdminCustomDomainID(c)
	if err != nil {
		response.BadRequest(c, "Invalid custom domain id")
		return
	}
	domain, err := h.customDomainService.VerifyAsAdmin(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "custom domain verified", domain.ID, domain.Domain, "status", domain.Status)
	response.Success(c, dto.CustomDomainFromService(domain))
}

func (h *CustomDomainHandler) Disable(c *gin.Context) {
	id, err := parseAdminCustomDomainID(c)
	if err != nil {
		response.BadRequest(c, "Invalid custom domain id")
		return
	}
	var req disableCustomDomainRequest
	_ = c.ShouldBindJSON(&req)
	domain, err := h.customDomainService.DisableAsAdmin(c.Request.Context(), id, req.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "custom domain disabled", domain.ID, domain.Domain, "reason", req.Reason)
	response.Success(c, dto.CustomDomainFromService(domain))
}

func (h *CustomDomainHandler) Enable(c *gin.Context) {
	id, err := parseAdminCustomDomainID(c)
	if err != nil {
		response.BadRequest(c, "Invalid custom domain id")
		return
	}
	domain, err := h.customDomainService.EnableAsAdmin(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "custom domain enabled", domain.ID, domain.Domain, "status", domain.Status)
	response.Success(c, dto.CustomDomainFromService(domain))
}

func (h *CustomDomainHandler) Delete(c *gin.Context) {
	id, err := parseAdminCustomDomainID(c)
	if err != nil {
		response.BadRequest(c, "Invalid custom domain id")
		return
	}
	if err := h.customDomainService.DeleteAsAdmin(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "custom domain deleted", id, "")
	response.Success(c, gin.H{"deleted": true})
}

func (h *CustomDomainHandler) audit(c *gin.Context, message string, domainID int64, domain string, extra ...any) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	args := []any{
		"audit", true,
		"user_id", subject.UserID,
		"role", role,
	}
	if domainID > 0 {
		args = append(args, "domain_id", domainID)
	}
	if domain != "" {
		args = append(args, "domain", domain)
	}
	args = append(args, extra...)
	slog.Info(message, args...)
}

func parseAdminCustomDomainID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
}
