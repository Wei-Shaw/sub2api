package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OidcSigningKeyHandler 暴露 admin 端的 OIDC 签名密钥生命周期接口 (rotate / delete / list)。
type OidcSigningKeyHandler struct {
	svc *service.OidcSigningService
}

// NewOidcSigningKeyHandler 构造 OidcSigningKeyHandler。
func NewOidcSigningKeyHandler(svc *service.OidcSigningService) *OidcSigningKeyHandler {
	return &OidcSigningKeyHandler{svc: svc}
}

// adminOidcSigningKey 是 admin 列表返回的 kid 元数据 DTO。
type adminOidcSigningKey struct {
	Kid       string     `json:"kid"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	RetiredAt *time.Time `json:"retired_at,omitempty"`
	Removable bool       `json:"removable"`
}

// List GET /api/v1/admin/oidc/signing-keys
func (h *OidcSigningKeyHandler) List(c *gin.Context) {
	keys := h.svc.ListKeys()
	out := make([]adminOidcSigningKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, adminOidcSigningKey{
			Kid:       k.Kid,
			IsActive:  k.IsActive,
			CreatedAt: k.CreatedAt,
			RetiredAt: k.RetiredAt,
			Removable: k.Removable,
		})
	}
	response.Success(c, out)
}

// Rotate POST /api/v1/admin/oidc/signing-keys/rotate
func (h *OidcSigningKeyHandler) Rotate(c *gin.Context) {
	kid, err := h.svc.RotateKey(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditOidcAdmin(c, "signing_key.rotate", "active_kid", kid)
	response.Success(c, gin.H{"active_kid": kid})
}

// Delete DELETE /api/v1/admin/oidc/signing-keys/:kid
func (h *OidcSigningKeyHandler) Delete(c *gin.Context) {
	kid := c.Param("kid")
	err := h.svc.DeleteKey(c.Request.Context(), kid)
	switch {
	case err == nil:
		auditOidcAdmin(c, "signing_key.delete", "kid", kid)
		response.Success(c, gin.H{"message": "deleted"})
	case errors.Is(err, service.ErrOidcSigningKeyNotFound):
		response.NotFound(c, "signing key not found")
	case errors.Is(err, service.ErrOidcSigningActiveKeyDeletion):
		response.Error(c, http.StatusConflict, "active signing key cannot be deleted")
	default:
		response.ErrorFrom(c, err)
	}
}
