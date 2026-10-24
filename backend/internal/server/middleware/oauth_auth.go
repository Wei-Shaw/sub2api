package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type OAuthAuthContext struct {
	ClientID string
	TokenID  int64
	Scopes   map[string]struct{}
}

func GetOAuthAuthContext(c *gin.Context) (OAuthAuthContext, bool) {
	if c == nil || c.GetString(string(ContextKeyAuthKind)) != "oauth" {
		return OAuthAuthContext{}, false
	}
	clientID, ok := c.Get(string(ContextKeyOAuthClientID))
	if !ok {
		return OAuthAuthContext{}, false
	}
	scopes, _ := c.Get(string(ContextKeyOAuthScopes))
	tokenID, _ := c.Get(string(ContextKeyOAuthTokenID))
	client, _ := clientID.(string)
	scopeSet, _ := scopes.(map[string]struct{})
	id, _ := tokenID.(int64)
	return OAuthAuthContext{ClientID: client, TokenID: id, Scopes: scopeSet}, client != ""
}

func OAuthHasScope(c *gin.Context, scope string) bool {
	auth, ok := GetOAuthAuthContext(c)
	if !ok {
		return false
	}
	_, ok = auth.Scopes[scope]
	return ok
}

// NewOAuthAwareAuthMiddleware preserves the existing JWT behavior and accepts
// opaque OAuth access tokens only on the explicitly scoped user resource paths.
func NewOAuthAwareAuthMiddleware(
	jwtAuth JWTAuthMiddleware,
	oauthService *service.OAuthProviderService,
	userService *service.UserService,
) JWTAuthMiddleware {
	return JWTAuthMiddleware(func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			jwtAuth(c)
			return
		}
		token := strings.TrimSpace(parts[1])
		// Existing panel JWTs are three-part tokens. Delegate them without
		// changing session binding, token version checks, or refresh semantics.
		if strings.Count(token, ".") == 2 {
			jwtAuth(c)
			return
		}
		if oauthService == nil || userService == nil {
			AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
			return
		}
		info, err := oauthService.ValidateAccessToken(c.Request.Context(), token)
		if err != nil {
			AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
			return
		}
		required := requiredOAuthScope(c.Request.Method, c.Request.URL.Path)
		if required == "" {
			AbortWithError(c, 403, "INSUFFICIENT_SCOPE", "OAuth token is not allowed for this resource")
			return
		}
		if _, ok := info.Scopes[required]; !ok && (required != "profile" || !hasScope(info.Scopes, "openid")) {
			AbortWithError(c, 403, "INSUFFICIENT_SCOPE", "OAuth token does not grant the required scope")
			return
		}
		user, err := userService.GetByID(c.Request.Context(), info.UserID)
		if err != nil || user == nil || !user.IsActive() {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
			return
		}
		c.Set(string(ContextKeyUser), AuthSubject{UserID: user.ID, Concurrency: user.Concurrency})
		c.Set(string(ContextKeyUserRole), user.Role)
		c.Set(ContextKeyAuthEmail, user.Email)
		c.Set(string(ContextKeyAuthKind), "oauth")
		c.Set(string(ContextKeyOAuthClientID), info.ClientID)
		c.Set(string(ContextKeyOAuthScopes), info.Scopes)
		c.Set(string(ContextKeyOAuthTokenID), info.TokenID)
		c.Next()
	})
}

func hasScope(scopes map[string]struct{}, scope string) bool {
	_, ok := scopes[scope]
	return ok
}

func requiredOAuthScope(method, path string) string {
	path = strings.TrimPrefix(path, "/api/v1")
	switch {
	case method == "GET" && path == "/auth/me":
		return "profile"
	case path == "/groups/available" || path == "/groups/rates":
		return "groups:read"
	case path == "/keys" && method == "GET":
		return "keys:read"
	case path == "/keys" && method == "POST":
		return "keys:create"
	case strings.HasPrefix(path, "/keys/") && strings.HasSuffix(path, "/reveal") && method == "POST":
		return "keys:read"
	case strings.HasPrefix(path, "/keys/") && method == "GET":
		return "keys:read"
	case strings.HasPrefix(path, "/keys/") && method == "DELETE":
		return "keys:revoke"
	default:
		return ""
	}
}
