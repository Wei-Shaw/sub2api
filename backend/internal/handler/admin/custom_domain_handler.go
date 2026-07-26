package admin

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
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
		Enabled:     h.customDomainService.IsEnabled(c.Request.Context()),
		CNAMETarget: h.customDomainService.GatewayTarget(c.Request.Context()),
	})
}

func (h *CustomDomainHandler) List(c *gin.Context) {
	filters := service.CustomDomainListFilters{
		Domain: strings.TrimSpace(c.Query("domain")),
		Status: strings.TrimSpace(c.Query("status")),
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		if userID, err := strconv.ParseInt(raw, 10, 64); err == nil && userID > 0 {
			filters.UserID = userID
		}
	}
	if raw := strings.TrimSpace(c.Query("all_users")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err == nil {
			filters.AllUsers = &value
		}
	}
	domains, err := h.customDomainService.ListAll(c.Request.Context(), filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.CustomDomainsFromService(domains))
}

func (h *CustomDomainHandler) Create(c *gin.Context) {
	var req createAdminCustomDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	domain, err := h.customDomainService.CreateForUserWithAccess(c.Request.Context(), req.UserID, req.Domain, req.AllUsers, req.UserIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "custom domain created", domain.ID, domain.Domain,
		"owner_user_id", req.UserID,
		"all_users", req.AllUsers,
		"user_ids", req.UserIDs,
	)
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
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	domain, err := h.customDomainService.UpdateAccessAsAdmin(c.Request.Context(), id, req.AllUsers, req.UserIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.audit(c, "custom domain access updated", id, domain.Domain,
		"all_users", req.AllUsers,
		"user_ids", req.UserIDs,
	)
	response.Success(c, dto.CustomDomainFromService(domain))
}

func (h *CustomDomainHandler) Verify(c *gin.Context) {
	id, err := parseAdminCustomDomainID(c)
	if err != nil {
		response.BadRequest(c, "Invalid custom domain id")
		return
	}
	domain, err := h.customDomainService.VerifyAsAdmin(c.Request.Context(), id)
	if err != nil && !errors.Is(err, service.ErrCustomDomainVerificationPending) {
		response.ErrorFrom(c, err)
		return
	}
	message := "custom domain verified"
	if errors.Is(err, service.ErrCustomDomainVerificationPending) {
		message = "custom domain verification pending"
	}
	h.audit(c, message, id, domain.Domain, "status", domain.Status)
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
	h.audit(c, "custom domain disabled", id, domain.Domain, "reason", req.Reason)
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
	h.audit(c, "custom domain enabled", id, domain.Domain, "status", domain.Status)
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
	response.Success(c, gin.H{"message": "ok"})
}

func parseAdminCustomDomainID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

func (h *CustomDomainHandler) audit(c *gin.Context, message string, domainID int64, domain string, fields ...any) {
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	role, _ := middleware2.GetUserRoleFromContext(c)
	attrs := []any{"audit", true, "user_id", subject.UserID, "role", role}
	if domainID > 0 {
		attrs = append(attrs, "domain_id", domainID)
	}
	if domain != "" {
		attrs = append(attrs, "domain", domain)
	}
	attrs = append(attrs, fields...)
	slog.Info(message, attrs...)
}
