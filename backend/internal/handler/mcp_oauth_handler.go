package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

type MCPOAuthHandler struct {
	oauth    *service.MCPOAuthService
	totp     *service.TotpService
	users    *service.UserService
	settings *service.SettingService
	cfg      *config.Config
}

func NewMCPOAuthHandler(oauth *service.MCPOAuthService, totp *service.TotpService, users *service.UserService, settings *service.SettingService, cfg *config.Config) *MCPOAuthHandler {
	return &MCPOAuthHandler{oauth: oauth, totp: totp, users: users, settings: settings, cfg: cfg}
}

func (h *MCPOAuthHandler) Service() *service.MCPOAuthService { return h.oauth }

// ResourceURL is anchored to operator configuration, never the request Host or
// forwarded headers. MCP stays disabled until MCP_PUBLIC_URL is set explicitly,
// so upgrading never exposes client registration or management tools.
func (h *MCPOAuthHandler) ResourceURL(_ context.Context) (string, error) {
	if h.cfg == nil || !h.cfg.MCP.Enabled() {
		return "", errors.New("MCP is disabled; set MCP_PUBLIC_URL to enable it")
	}
	configured := config.MCPConfig{PublicURL: h.cfg.MCP.PublicURL}
	if err := configured.Validate(); err != nil || configured.PublicURL == "" {
		return "", errors.New("MCP public URL is not configured correctly")
	}
	return configured.PublicURL, nil
}

func (h *MCPOAuthHandler) frontendURL(ctx context.Context) string {
	if h.settings != nil {
		return strings.TrimRight(strings.TrimSpace(h.settings.GetFrontendURL(ctx)), "/")
	}
	if h.cfg != nil {
		return strings.TrimRight(strings.TrimSpace(h.cfg.Server.FrontendURL), "/")
	}
	return ""
}

func (h *MCPOAuthHandler) ResourceMetadata(c *gin.Context) {
	resource, ok := h.resource(c, true)
	if !ok {
		return
	}
	u, _ := url.Parse(resource)
	issuer := u.Scheme + "://" + u.Host
	c.JSON(http.StatusOK, oauthex.ProtectedResourceMetadata{
		Resource: resource, AuthorizationServers: []string{issuer},
		ScopesSupported: []string{service.MCPOAuthScope}, BearerMethodsSupported: []string{"header"},
		ResourceName: "Sub2API management",
	})
}

func (h *MCPOAuthHandler) AuthorizationMetadata(c *gin.Context) {
	resource, ok := h.resource(c, true)
	if !ok {
		return
	}
	u, _ := url.Parse(resource)
	issuer := u.Scheme + "://" + u.Host
	frontend := h.frontendURL(c.Request.Context())
	if frontend == "" {
		frontend = issuer
	}
	// The SPA restores the browser login and obtains explicit user consent.
	authorization, err := mcpAuthorizationURL(frontend)
	if err != nil {
		mcpOAuthErrorResponse(c, http.StatusServiceUnavailable, "server_error", "MCP authorization page is not configured correctly")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"issuer": issuer, "authorization_endpoint": authorization,
		"token_endpoint":           issuer + "/api/v1/mcp/oauth/token",
		"registration_endpoint":    issuer + "/api/v1/mcp/oauth/register",
		"revocation_endpoint":      issuer + "/api/v1/mcp/oauth/revoke",
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"},
		"revocation_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{service.MCPOAuthScope},
	})
}

func mcpAuthorizationURL(frontend string) (string, error) {
	u, err := url.Parse(frontend)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(frontend, "#") {
		return "", errors.New("invalid frontend URL")
	}
	origin := config.MCPConfig{PublicURL: u.Scheme + "://" + u.Host + "/api/v1/mcp"}
	if err := origin.Validate(); err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/oauth/mcp/authorize"
	u.RawPath = ""
	return u.String(), nil
}

func (h *MCPOAuthHandler) Register(c *gin.Context) {
	mcpOAuthNoStore(c)
	if _, ok := h.resource(c, true); !ok {
		return
	}
	var input service.MCPOAuthRegisterInput
	if !mcpOAuthReadJSON(c, &input, false) {
		return
	}
	client, err := h.oauth.Register(c.Request.Context(), input)
	if mcpOAuthHandleError(c, err, true) {
		return
	}
	c.JSON(http.StatusCreated, client)
}

func (h *MCPOAuthHandler) Token(c *gin.Context) {
	mcpOAuthNoStore(c)
	resource, ok := h.resource(c, true)
	if !ok {
		return
	}
	form, ok := mcpOAuthReadForm(c)
	if !ok {
		return
	}
	if c.GetHeader("Authorization") != "" || form.Get("client_secret") != "" {
		mcpOAuthErrorResponse(c, http.StatusBadRequest, "invalid_client", "Only public clients using token_endpoint_auth_method=none are supported")
		return
	}
	input := service.MCPOAuthTokenInput{
		GrantType: form.Get("grant_type"), ClientID: form.Get("client_id"), Code: form.Get("code"),
		RedirectURI: form.Get("redirect_uri"), CodeVerifier: form.Get("code_verifier"),
		RefreshToken: form.Get("refresh_token"), Resource: form.Get("resource"), Scope: form.Get("scope"),
	}
	result, err := h.oauth.Token(c.Request.Context(), input, resource)
	if mcpOAuthHandleError(c, err, true) {
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *MCPOAuthHandler) RevokeToken(c *gin.Context) {
	mcpOAuthNoStore(c)
	form, ok := mcpOAuthReadForm(c)
	if !ok {
		return
	}
	if c.GetHeader("Authorization") != "" || form.Get("client_secret") != "" {
		mcpOAuthErrorResponse(c, http.StatusBadRequest, "invalid_client", "Only public clients are supported")
		return
	}
	if mcpOAuthHandleError(c, h.oauth.RevokeToken(c.Request.Context(), form.Get("client_id"), form.Get("token")), true) {
		return
	}
	c.Status(http.StatusOK)
}

func (h *MCPOAuthHandler) Preview(c *gin.Context) {
	mcpOAuthNoStore(c)
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	resource, ok := h.resource(c, false)
	if !ok {
		return
	}
	query := c.Request.URL.Query()
	for _, values := range query {
		if len(values) != 1 {
			response.BadRequest(c, "Repeated OAuth parameters are not supported")
			return
		}
	}
	input := service.MCPOAuthAuthorizeInput{
		ResponseType: query.Get("response_type"), ClientID: query.Get("client_id"), RedirectURI: query.Get("redirect_uri"),
		Scope: query.Get("scope"), State: query.Get("state"), CodeChallenge: query.Get("code_challenge"),
		CodeChallengeMethod: query.Get("code_challenge_method"), Resource: query.Get("resource"),
	}
	consent, err := h.oauth.Preview(c.Request.Context(), subject.UserID, input, resource)
	if mcpOAuthHandleError(c, err, false) {
		return
	}
	response.Success(c, consent)
}

func (h *MCPOAuthHandler) Decide(c *gin.Context) {
	mcpOAuthNoStore(c)
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	resource, ok := h.resource(c, false)
	if !ok {
		return
	}
	var input service.MCPOAuthConsentInput
	if !mcpOAuthReadJSON(c, &input, true) {
		return
	}
	if input.Approved && input.Scope == service.MCPScopeAdmin {
		role, _ := middleware.GetUserRoleFromContext(c)
		if role != service.RoleAdmin {
			response.Forbidden(c, "Only administrators can authorize admin MCP access")
			return
		}
		// Administrator write grants outlive browser sessions, so they always
		// need a fresh TOTP verification even when the global switch is off.
		var verified bool
		if input.ReadOnly != nil && !*input.ReadOnly {
			verified = middleware.EnforceStepUpAlways(c, h.totp, h.users)
		} else {
			verified = middleware.EnforceStepUp(c, h.totp, h.users, h.settings)
		}
		if !verified {
			return
		}
	}
	redirect, err := h.oauth.Decide(c.Request.Context(), subject.UserID, input, resource)
	if mcpOAuthHandleError(c, err, false) {
		return
	}
	middleware.SetAuditAction(c, "user.mcp.authorize")
	response.Success(c, gin.H{"redirect_url": redirect})
}

func (h *MCPOAuthHandler) ListGrants(c *gin.Context) {
	mcpOAuthNoStore(c)
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	grants, err := h.oauth.ListGrants(c.Request.Context(), subject.UserID)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, grants)
}

func (h *MCPOAuthHandler) ConnectionConfig(c *gin.Context) {
	mcpOAuthNoStore(c)
	if _, ok := middleware.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	resource, ok := h.resource(c, false)
	if !ok {
		return
	}
	response.Success(c, gin.H{"resource": resource})
}

func (h *MCPOAuthHandler) RevokeGrant(c *gin.Context) {
	mcpOAuthNoStore(c)
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if response.ErrorFrom(c, h.oauth.RevokeGrant(c.Request.Context(), subject.UserID, c.Param("id"))) {
		return
	}
	middleware.SetAuditAction(c, "user.mcp.revoke")
	response.Success(c, gin.H{"revoked": true})
}

func (h *MCPOAuthHandler) resource(c *gin.Context, protocol bool) (string, bool) {
	resource, err := h.ResourceURL(c.Request.Context())
	if err != nil {
		if protocol {
			mcpOAuthErrorResponse(c, http.StatusServiceUnavailable, "server_error", err.Error())
		} else {
			response.Error(c, http.StatusServiceUnavailable, err.Error())
		}
		return "", false
	}
	return resource, true
}

func mcpOAuthNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
}

func mcpOAuthErrorResponse(c *gin.Context, status int, code, description string) {
	mcpOAuthNoStore(c)
	c.JSON(status, service.MCPOAuthError{Code: code, Description: description})
}

func mcpOAuthHandleError(c *gin.Context, err error, protocol bool) bool {
	if err == nil {
		return false
	}
	var oauthErr *service.MCPOAuthError
	if errors.As(err, &oauthErr) {
		if protocol {
			mcpOAuthErrorResponse(c, http.StatusBadRequest, oauthErr.Code, oauthErr.Description)
		} else {
			response.BadRequest(c, oauthErr.Description)
		}
	} else if protocol {
		mcpOAuthErrorResponse(c, http.StatusInternalServerError, "server_error", "OAuth request could not be completed")
	} else {
		response.ErrorFrom(c, err)
	}
	return true
}

func mcpOAuthReadJSON(c *gin.Context, out any, browser bool) bool {
	mediaType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mediaType == "application/json" {
		// Unauthenticated registrations are embedded in client IDs, so they get a tighter bound.
		limit := int64(4 << 10)
		if browser {
			limit = 16 << 10
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		decoder := json.NewDecoder(c.Request.Body)
		if browser {
			decoder.DisallowUnknownFields()
		}
		if err := decoder.Decode(out); err == nil {
			var extra any
			if errors.Is(decoder.Decode(&extra), io.EOF) {
				return true
			}
		}
	}
	if browser {
		response.BadRequest(c, "Invalid OAuth authorization request")
	} else {
		mcpOAuthErrorResponse(c, http.StatusBadRequest, "invalid_request", "A bounded JSON request body is required")
	}
	return false
}

func mcpOAuthReadForm(c *gin.Context) (url.Values, bool) {
	mediaType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mediaType != "application/x-www-form-urlencoded" {
		mcpOAuthErrorResponse(c, http.StatusBadRequest, "invalid_request", "Use application/x-www-form-urlencoded")
		return nil, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := c.Request.ParseForm(); err != nil {
		mcpOAuthErrorResponse(c, http.StatusBadRequest, "invalid_request", "Cannot read OAuth form")
		return nil, false
	}
	for _, values := range c.Request.PostForm {
		if len(values) != 1 {
			mcpOAuthErrorResponse(c, http.StatusBadRequest, "invalid_request", "Repeated OAuth parameters are not supported")
			return nil, false
		}
	}
	return c.Request.PostForm, true
}
