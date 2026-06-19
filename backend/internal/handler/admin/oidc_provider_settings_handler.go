package admin

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OidcProviderSettingsHandler 暴露 admin 端的 OIDC Provider 全局设置读写。
type OidcProviderSettingsHandler struct {
	svc *service.OidcProviderService
}

// NewOidcProviderSettingsHandler 构造 handler。
func NewOidcProviderSettingsHandler(svc *service.OidcProviderService) *OidcProviderSettingsHandler {
	return &OidcProviderSettingsHandler{svc: svc}
}

// Get GET /api/v1/admin/oidc/settings
func (h *OidcProviderSettingsHandler) Get(c *gin.Context) {
	view, err := h.svc.GetProviderSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// Update PUT /api/v1/admin/oidc/settings
func (h *OidcProviderSettingsHandler) Update(c *gin.Context) {
	var in service.OidcProviderSettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.svc.UpdateProviderSettings(c.Request.Context(), in); err != nil {
		// issuer_url 格式 / TTL 校验失败统一返回 400。
		if errors.Is(err, service.ErrOidcProviderIssuerURLEmpty) ||
			errors.Is(err, service.ErrOidcProviderIssuerURLNotHTTPS) ||
			errors.Is(err, service.ErrOidcProviderIssuerURLTrailingSlash) ||
			errors.Is(err, service.ErrOidcProviderIssuerURLContainsQueryOrFragment) ||
			errors.Is(err, service.ErrOidcProviderInvalidTTL) {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	view, err := h.svc.GetProviderSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditOidcAdmin(c, "settings.update",
		"enabled", view.Enabled,
		"issuer_url", view.IssuerURL,
	)
	response.Success(c, view)
}
