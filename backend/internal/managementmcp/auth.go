package managementmcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const AuthMethod = service.AuditAuthMethodMCPOAuth

type TokenAuthenticator interface {
	Authenticate(context.Context, string) (*service.MCPPrincipal, error)
}

type requestKey struct{}
type dispatchKey struct{}

type authenticatedRequest struct {
	principal *service.MCPPrincipal
	request   *http.Request
}

// Authenticate accepts only resource-bound OAuth access tokens. Browser sessions and
// inference keys never become management credentials through this endpoint.
func Authenticate(tokens TokenAuthenticator, allowedOrigins []string, resourceURL func(context.Context) (string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		resource, err := resourceURL(c.Request.Context())
		if err != nil {
			middleware.AbortWithError(c, http.StatusServiceUnavailable, "MCP_OAUTH_NOT_CONFIGURED", "Configure the public MCP URL to enable OAuth")
			return
		}
		u, err := url.Parse(resource)
		if err != nil || u.Host == "" {
			middleware.AbortWithError(c, http.StatusServiceUnavailable, "MCP_OAUTH_NOT_CONFIGURED", "MCP public URL is invalid")
			return
		}
		challenge := fmt.Sprintf(`Bearer resource_metadata="%s://%s/.well-known/oauth-protected-resource/api/v1/mcp", scope="mcp"`, u.Scheme, u.Host)
		if !originAllowed(c.Request, allowedOrigins) {
			middleware.AbortWithError(c, http.StatusForbidden, "MCP_ORIGIN_DENIED", "Origin is not allowed for MCP")
			return
		}
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.Header("WWW-Authenticate", challenge)
			middleware.AbortWithError(c, http.StatusUnauthorized, "INVALID_MCP_TOKEN", "An OAuth access token is required")
			return
		}
		principal, err := tokens.Authenticate(c.Request.Context(), parts[1])
		if err != nil || principal == nil || principal.Resource != resource {
			if err != nil && !errors.Is(err, service.ErrMCPOAuthInvalidToken) {
				middleware.AbortWithError(c, http.StatusServiceUnavailable, "MCP_AUTH_UNAVAILABLE", "MCP authentication is temporarily unavailable")
				return
			}
			c.Header("WWW-Authenticate", challenge+`, error="invalid_token"`)
			middleware.AbortWithError(c, http.StatusUnauthorized, "INVALID_MCP_TOKEN", "MCP OAuth access token is invalid or expired")
			return
		}
		identity := &authenticatedRequest{principal: principal, request: c.Request}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestKey{}, identity))
		setSubject(c, principal)
		c.Next()
	}
}

// DispatchAuth is used exclusively by the private in-process management router.
// No HTTP header can supply this context. Public REST auth remains JWT-only.
func DispatchAuth(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, _ := c.Request.Context().Value(requestKey{}).(*authenticatedRequest)
		spec, ok := c.Request.Context().Value(dispatchKey{}).(ToolSpec)
		if identity == nil || !ok || spec.Method != c.Request.Method || spec.Path != c.FullPath() ||
			!toolAllowed(identity.principal, spec) || (admin && identity.principal.Scope != service.MCPScopeAdmin) {
			middleware.AbortWithError(c, http.StatusForbidden, "MCP_FORBIDDEN", "MCP operation is not allowed")
			return
		}
		setSubject(c, identity.principal)
		middleware.SetAuditExtra(c, map[string]any{"mcp_grant_id": identity.principal.GrantID, "mcp_tool": spec.Name})
		c.Next()
	}
}

func setSubject(c *gin.Context, principal *service.MCPPrincipal) {
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: principal.UserID, Concurrency: principal.Concurrency})
	c.Set(string(middleware.ContextKeyUserRole), principal.Role)
	c.Set(middleware.ContextKeyAuthEmail, principal.Email)
	c.Set("auth_method", AuthMethod)
	// Never share a legacy user's step-up grant with a machine credential.
	c.Set(middleware.ContextKeySessionID, "mcp:"+principal.GrantID)
}

func toolAllowed(p *service.MCPPrincipal, spec ToolSpec) bool {
	return p != nil && p.UserID > 0 &&
		(p.Scope == service.MCPScopeUser || p.Scope == service.MCPScopeAdmin) &&
		(!spec.Admin || (p.Scope == service.MCPScopeAdmin && p.Role == service.RoleAdmin)) &&
		(!p.ReadOnly || spec.ReadOnly)
}

func originAllowed(r *http.Request, origins []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Native MCP clients do not send an Origin.
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	// Same-host browser access also works behind an HTTPS terminating proxy.
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, allowed := range origins {
		// Wildcard CORS does not disable MCP's Origin validation.
		if allowed != "*" && origin == strings.TrimSpace(allowed) {
			return true
		}
	}
	return false
}
