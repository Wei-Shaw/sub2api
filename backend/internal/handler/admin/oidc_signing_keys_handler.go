package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// OIDCSigningKeysHandler handles OIDC signing keys management operations
type OIDCSigningKeysHandler struct {
	signingKeyService *service.OIDCSigningKeyService
}

// NewOIDCSigningKeysHandler creates a new OIDC signing keys handler
func NewOIDCSigningKeysHandler(signingKeyService *service.OIDCSigningKeyService) *OIDCSigningKeysHandler {
	return &OIDCSigningKeysHandler{signingKeyService: signingKeyService}
}

// ListSigningKeys lists all OIDC signing keys
// GET /api/v1/admin/oidc/signing-keys
func (h *OIDCSigningKeysHandler) ListSigningKeys(c *gin.Context) {
	keys, err := h.signingKeyService.ListSigningKeys(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, "获取签名密钥列表失败: "+err.Error())
		return
	}

	response.OK(c, keys)
}

// GenerateSigningKey generates a new OIDC signing key
// POST /api/v1/admin/oidc/signing-keys/generate
func (h *OIDCSigningKeysHandler) GenerateSigningKey(c *gin.Context) {
	kid, err := h.signingKeyService.GenerateSigningKey(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, "生成签名密钥失败: "+err.Error())
		return
	}

	response.Created(c, gin.H{
		"kid": kid,
	})
}

// RotateSigningKey rotates the active signing key
// POST /api/v1/admin/oidc/signing-keys/rotate
func (h *OIDCSigningKeysHandler) RotateSigningKey(c *gin.Context) {
	kid, err := h.signingKeyService.RotateSigningKey(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, "轮换签名密钥失败: "+err.Error())
		return
	}

	response.Created(c, gin.H{
		"kid": kid,
	})
}

// DeleteSigningKey deletes a signing key
// DELETE /api/v1/admin/oidc/signing-keys/:kid
func (h *OIDCSigningKeysHandler) DeleteSigningKey(c *gin.Context) {
	kid := c.Param("kid")
	if kid == "" {
		response.BadRequest(c, "密钥ID不能为空")
		return
	}

	err := h.signingKeyService.DeleteSigningKey(c.Request.Context(), kid)
	if err != nil {
		if err.Error() == "cannot delete active signing key" {
			response.BadRequest(c, "不能删除活跃的签名密钥")
			return
		}
		response.InternalServerError(c, "删除签名密钥失败: "+err.Error())
		return
	}

	response.NoContent(c)
}