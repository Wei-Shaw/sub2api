package admin

import (
	"errors"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// auditOidcAdmin 以结构化日志方式记录 OIDC admin 操作审计 (与既有 setting/user
// handler 的 slog "audit"=true 约定一致；OIDC 为低频管理操作，无需独立审计表)。
// fields 为成对的 key/value 附加字段。
func auditOidcAdmin(c *gin.Context, action string, fields ...any) {
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	args := make([]any, 0, 8+len(fields))
	args = append(args,
		"audit", true,
		"component", "audit.oidc_provider",
		"action", action,
		"operator_id", subject.UserID,
		"role", role,
	)
	args = append(args, fields...)
	slog.Info("oidc provider admin operation", args...)
}

// OidcClientHandler 暴露 admin 端的 OIDC RP (第三方客户端) CRUD 接口。
type OidcClientHandler struct {
	svc *service.OidcClientService
}

// NewOidcClientHandler 构造 OidcClientHandler。
func NewOidcClientHandler(svc *service.OidcClientService) *OidcClientHandler {
	return &OidcClientHandler{svc: svc}
}

// adminOidcClient 是 admin 列表/详情返回的安全视图 DTO (永远不含 secret)。
type adminOidcClient struct {
	ID              int64     `json:"id"`
	ClientID        string    `json:"client_id"`
	ClientName      string    `json:"client_name"`
	RedirectURIs    []string  `json:"redirect_uris"`
	AllowedScopes   []string  `json:"allowed_scopes"`
	GrantTypes      []string  `json:"grant_types"`
	ConsentRequired bool      `json:"consent_required"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func toAdminOidcClient(v *service.OidcClientView) *adminOidcClient {
	if v == nil {
		return nil
	}
	return &adminOidcClient{
		ID:              v.ID,
		ClientID:        v.ClientID,
		ClientName:      v.ClientName,
		RedirectURIs:    v.RedirectURIs,
		AllowedScopes:   v.AllowedScopes,
		GrantTypes:      v.GrantTypes,
		ConsentRequired: v.ConsentRequired,
		Enabled:         v.Enabled,
		CreatedAt:       v.CreatedAt,
		UpdatedAt:       v.UpdatedAt,
	}
}

// mapOidcClientError 把 service 层哨兵错误映射为合适的 HTTP 响应；返回是否已处理。
func mapOidcClientError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, service.ErrOidcClientNotFound):
		response.NotFound(c, "oidc client not found")
	case errors.Is(err, service.ErrOidcClientNameRequired),
		errors.Is(err, service.ErrOidcClientNameTooLong),
		errors.Is(err, service.ErrOidcClientRedirectURIsRequired),
		errors.Is(err, service.ErrOidcClientInvalidRedirectURI),
		errors.Is(err, service.ErrOidcClientInvalidScope):
		response.BadRequest(c, err.Error())
	default:
		response.ErrorFrom(c, err)
	}
	return true
}

// List GET /api/v1/admin/oidc/clients
func (h *OidcClientHandler) List(c *gin.Context) {
	filters := service.OidcClientListFilters{
		OnlyEnabled: c.Query("only_enabled") == "true",
		NameLike:    c.Query("name"),
	}
	rows, err := h.svc.List(c.Request.Context(), filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*adminOidcClient, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAdminOidcClient(r))
	}
	response.Success(c, out)
}

// Get GET /api/v1/admin/oidc/clients/:id
func (h *OidcClientHandler) Get(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	row, err := h.svc.Get(c.Request.Context(), id)
	if mapOidcClientError(c, err) {
		return
	}
	response.Success(c, toAdminOidcClient(row))
}

// createOidcClientRequest Create 请求体。
type createOidcClientRequest struct {
	ClientName      string   `json:"client_name"`
	RedirectURIs    []string `json:"redirect_uris"`
	AllowedScopes   []string `json:"allowed_scopes"`
	ConsentRequired bool     `json:"consent_required"`
	Enabled         bool     `json:"enabled"`
}

// createOidcClientResponse Create 返回体：一次性附带明文 secret。
type createOidcClientResponse struct {
	*adminOidcClient
	ClientSecret string `json:"client_secret"`
}

// Create POST /api/v1/admin/oidc/clients
func (h *OidcClientHandler) Create(c *gin.Context) {
	var req createOidcClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	view, plaintext, err := h.svc.Create(c.Request.Context(), service.CreateOidcClientRequest{
		ClientName:      req.ClientName,
		RedirectURIs:    req.RedirectURIs,
		AllowedScopes:   req.AllowedScopes,
		ConsentRequired: req.ConsentRequired,
		Enabled:         req.Enabled,
	})
	if mapOidcClientError(c, err) {
		return
	}
	auditOidcAdmin(c, "client.create",
		"client_id", view.ClientID,
		"client_name", view.ClientName,
		"allowed_scopes", view.AllowedScopes,
		"enabled", view.Enabled,
	)
	response.Created(c, createOidcClientResponse{
		adminOidcClient: toAdminOidcClient(view),
		ClientSecret:    plaintext,
	})
}

// updateOidcClientRequest Update 请求体；指针字段为 nil 表示不修改。
type updateOidcClientRequest struct {
	ClientName      *string   `json:"client_name"`
	RedirectURIs    *[]string `json:"redirect_uris"`
	AllowedScopes   *[]string `json:"allowed_scopes"`
	ConsentRequired *bool     `json:"consent_required"`
	Enabled         *bool     `json:"enabled"`
}

// Update PATCH /api/v1/admin/oidc/clients/:id
func (h *OidcClientHandler) Update(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req updateOidcClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	row, err := h.svc.Update(c.Request.Context(), id, service.UpdateOidcClientPatch{
		ClientName:      req.ClientName,
		RedirectURIs:    req.RedirectURIs,
		AllowedScopes:   req.AllowedScopes,
		ConsentRequired: req.ConsentRequired,
		Enabled:         req.Enabled,
	})
	if mapOidcClientError(c, err) {
		return
	}
	auditOidcAdmin(c, "client.update",
		"id", id,
		"client_id", row.ClientID,
		"allowed_scopes", row.AllowedScopes,
		"enabled", row.Enabled,
	)
	response.Success(c, toAdminOidcClient(row))
}

// Delete DELETE /api/v1/admin/oidc/clients/:id
func (h *OidcClientHandler) Delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); mapOidcClientError(c, err) {
		return
	}
	auditOidcAdmin(c, "client.delete", "id", id)
	response.Success(c, gin.H{"message": "deleted"})
}

// resetSecretResponse ResetSecret 返回体：一次性新明文 secret。
type resetSecretResponse struct {
	ClientSecret string `json:"client_secret"`
}

// ResetSecret POST /api/v1/admin/oidc/clients/:id/reset-secret
func (h *OidcClientHandler) ResetSecret(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	plaintext, err := h.svc.ResetSecret(c.Request.Context(), id)
	if mapOidcClientError(c, err) {
		return
	}
	auditOidcAdmin(c, "client.reset_secret", "id", id)
	response.Success(c, resetSecretResponse{ClientSecret: plaintext})
}
