package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// OidcClientHandler handles OIDC client management operations
type OidcClientHandler struct {
	clientService *service.OidcClientService
}

// NewOidcClientHandler creates a new OIDC client handler
func NewOidcClientHandler(clientService *service.OidcClientService) *OidcClientHandler {
	return &OidcClientHandler{clientService: clientService}
}

// ListOidcClientsRequest represents list OIDC clients request
type ListOidcClientsRequest struct {
	OnlyEnabled bool   `form:"only_enabled"`
	NameLike    string `form:"name_like"`
}

// ListOidcClients lists OIDC clients
// GET /api/v1/admin/oidc/clients
func (h *OidcClientHandler) ListOidcClients(c *gin.Context) {
	var req ListOidcClientsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	filters := service.OidcClientListFilters{
		OnlyEnabled: req.OnlyEnabled,
		NameLike:    req.NameLike,
	}

	clients, err := h.clientService.List(c.Request.Context(), filters)
	if err != nil {
		response.InternalServerError(c, "获取OIDC客户端列表失败: "+err.Error())
		return
	}

	response.OK(c, clients)
}

// CreateOidcClientRequest represents create OIDC client request
type CreateOidcClientRequest struct {
	ClientName      string   `json:"client_name" binding:"required"`
	RedirectURIs    []string `json:"redirect_uris" binding:"required"`
	AllowedScopes   []string `json:"allowed_scopes"`
	ConsentRequired bool     `json:"consent_required"`
	Enabled         bool     `json:"enabled"`
}

// CreateOidcClient creates a new OIDC client
// POST /api/v1/admin/oidc/clients
func (h *OidcClientHandler) CreateOidcClient(c *gin.Context) {
	var req CreateOidcClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	serviceReq := service.CreateOidcClientRequest{
		ClientName:      req.ClientName,
		RedirectURIs:    req.RedirectURIs,
		AllowedScopes:   req.AllowedScopes,
		ConsentRequired: req.ConsentRequired,
		Enabled:         req.Enabled,
	}

	client, secret, err := h.clientService.Create(c.Request.Context(), serviceReq)
	if err != nil {
		response.InternalServerError(c, "创建OIDC客户端失败: "+err.Error())
		return
	}

	response.Created(c, gin.H{
		"client": client,
		"secret": secret, // 仅在创建时返回一次
	})
}

// GetOidcClient gets an OIDC client by ID
// GET /api/v1/admin/oidc/clients/:id
func (h *OidcClientHandler) GetOidcClient(c *gin.Context) {
	id, err := response.ParseInt64Param(c, "id")
	if err != nil {
		response.BadRequest(c, "ID参数无效")
		return
	}

	client, err := h.clientService.Get(c.Request.Context(), id)
	if err != nil {
		if err == service.ErrOidcClientNotFound {
			response.NotFound(c, "OIDC客户端不存在")
			return
		}
		response.InternalServerError(c, "获取OIDC客户端失败: "+err.Error())
		return
	}

	response.OK(c, client)
}

// UpdateOidcClientRequest represents update OIDC client request
type UpdateOidcClientRequest struct {
	ClientName      *string   `json:"client_name"`
	RedirectURIs    *[]string `json:"redirect_uris"`
	AllowedScopes   *[]string `json:"allowed_scopes"`
	ConsentRequired *bool     `json:"consent_required"`
	Enabled         *bool     `json:"enabled"`
}

// UpdateOidcClient updates an OIDC client
// PUT /api/v1/admin/oidc/clients/:id
func (h *OidcClientHandler) UpdateOidcClient(c *gin.Context) {
	id, err := response.ParseInt64Param(c, "id")
	if err != nil {
		response.BadRequest(c, "ID参数无效")
		return
	}

	var req UpdateOidcClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求无效: "+err.Error())
		return
	}

	patch := service.UpdateOidcClientPatch{
		ClientName:      req.ClientName,
		RedirectURIs:    req.RedirectURIs,
		AllowedScopes:   req.AllowedScopes,
		ConsentRequired: req.ConsentRequired,
		Enabled:         req.Enabled,
	}

	client, err := h.clientService.Update(c.Request.Context(), id, patch)
	if err != nil {
		if err == service.ErrOidcClientNotFound {
			response.NotFound(c, "OIDC客户端不存在")
			return
		}
		response.InternalServerError(c, "更新OIDC客户端失败: "+err.Error())
		return
	}

	response.OK(c, client)
}

// DeleteOidcClient deletes an OIDC client
// DELETE /api/v1/admin/oidc/clients/:id
func (h *OidcClientHandler) DeleteOidcClient(c *gin.Context) {
	id, err := response.ParseInt64Param(c, "id")
	if err != nil {
		response.BadRequest(c, "ID参数无效")
		return
	}

	err = h.clientService.Delete(c.Request.Context(), id)
	if err != nil {
		if err == service.ErrOidcClientNotFound {
			response.NotFound(c, "OIDC客户端不存在")
			return
		}
		response.InternalServerError(c, "删除OIDC客户端失败: "+err.Error())
		return
	}

	response.NoContent(c)
}

// ResetOidcClientSecret resets an OIDC client secret
// POST /api/v1/admin/oidc/clients/:id/reset-secret
func (h *OidcClientHandler) ResetOidcClientSecret(c *gin.Context) {
	id, err := response.ParseInt64Param(c, "id")
	if err != nil {
		response.BadRequest(c, "ID参数无效")
		return
	}

	secret, err := h.clientService.ResetSecret(c.Request.Context(), id)
	if err != nil {
		if err == service.ErrOidcClientNotFound {
			response.NotFound(c, "OIDC客户端不存在")
			return
		}
		response.InternalServerError(c, "重置客户端密钥失败: "+err.Error())
		return
	}

	response.OK(c, gin.H{
		"secret": secret, // 仅在重置时返回一次
	})
}