package handler

import (
	"log/slog"
	"strconv"

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

type createCustomDomainRequest struct {
	Domain string `json:"domain" binding:"required"`
}

func (h *CustomDomainHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	domains, err := h.customDomainService.ListForUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"enabled":      h.customDomainService.IsEnabled(c.Request.Context()),
		"cname_target": h.customDomainService.GatewayTarget(c.Request.Context()),
		"domains":      dto.CustomDomainsForUserFromService(domains, subject.UserID),
	})
}

func (h *CustomDomainHandler) Create(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req createCustomDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	domain, err := h.customDomainService.CreateForUserWithAccess(c.Request.Context(), subject.UserID, req.Domain, false, []int64{subject.UserID})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	slog.Info("custom domain created",
		"audit", true,
		"user_id", subject.UserID,
		"domain_id", domain.ID,
		"domain", domain.Domain,
	)
	response.Success(c, dto.CustomDomainForUserFromService(domain, subject.UserID))
}

func (h *CustomDomainHandler) Verify(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := parseCustomDomainID(c)
	if err != nil {
		response.BadRequest(c, "Invalid custom domain id")
		return
	}
	domain, err := h.customDomainService.VerifyForUser(c.Request.Context(), subject.UserID, id)
	if err != nil {
		if domain == nil {
			response.ErrorFrom(c, err)
			return
		}
		slog.Info("custom domain verification pending",
			"audit", true,
			"user_id", subject.UserID,
			"domain_id", domain.ID,
			"domain", domain.Domain,
			"status", domain.Status,
		)
		response.Success(c, dto.CustomDomainForUserFromService(domain, subject.UserID))
		return
	}
	slog.Info("custom domain verified",
		"audit", true,
		"user_id", subject.UserID,
		"domain_id", domain.ID,
		"domain", domain.Domain,
		"status", domain.Status,
	)
	response.Success(c, dto.CustomDomainForUserFromService(domain, subject.UserID))
}

func (h *CustomDomainHandler) Delete(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := parseCustomDomainID(c)
	if err != nil {
		response.BadRequest(c, "Invalid custom domain id")
		return
	}
	if err := h.customDomainService.DeleteForUser(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	slog.Info("custom domain deleted",
		"audit", true,
		"user_id", subject.UserID,
		"domain_id", id,
	)
	response.Success(c, gin.H{"message": "deleted"})
}

func parseCustomDomainID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}
