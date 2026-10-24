package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const oauthTransactionCookie = "sub2api_oauth_transaction"

type OAuthProviderHandler struct {
	provider    *service.OAuthProviderService
	userService *service.UserService
	settingSvc  *service.SettingService
}

func NewOAuthProviderHandler(provider *service.OAuthProviderService, userService *service.UserService, settingSvc *service.SettingService) *OAuthProviderHandler {
	return &OAuthProviderHandler{provider: provider, userService: userService, settingSvc: settingSvc}
}

func (h *OAuthProviderHandler) AuthMiddleware(jwtAuth middleware.JWTAuthMiddleware) middleware.JWTAuthMiddleware {
	return middleware.NewOAuthAwareAuthMiddleware(jwtAuth, h.provider, h.userService)
}

func (h *OAuthProviderHandler) Discovery(c *gin.Context) {
	base := requestBaseURL(c) + "/api/v1/oauth"
	c.JSON(http.StatusOK, gin.H{
		"issuer":                           base,
		"authorization_endpoint":           base + "/authorize",
		"token_endpoint":                   base + "/token",
		"revocation_endpoint":              base + "/revoke",
		"userinfo_endpoint":                requestBaseURL(c) + "/api/v1/auth/me",
		"groups_endpoint":                  requestBaseURL(c) + "/api/v1/groups/available",
		"keys_endpoint":                    requestBaseURL(c) + "/api/v1/keys",
		"code_challenge_methods_supported": []string{"S256"},
		"grant_types_supported":            []string{"authorization_code", "refresh_token"},
		"response_types_supported":         []string{"code"},
		"scopes_supported":                 []string{"openid", "profile", "groups:read", "keys:read", "keys:create", "keys:revoke"},
	})
}

func (h *OAuthProviderHandler) Authorize(c *gin.Context) {
	if h == nil || h.provider == nil {
		oauthJSONError(c, http.StatusServiceUnavailable, "temporarily_unavailable", "OAuth provider is unavailable")
		return
	}
	if strings.TrimSpace(c.Query("response_type")) != "code" {
		oauthJSONError(c, http.StatusBadRequest, "unsupported_response_type", "response_type must be code")
		return
	}
	clientID := strings.TrimSpace(c.Query("client_id"))
	redirectURI := strings.TrimSpace(c.Query("redirect_uri"))
	state := c.Query("state")
	challenge := strings.TrimSpace(c.Query("code_challenge"))
	method := strings.TrimSpace(c.Query("code_challenge_method"))
	if strings.TrimSpace(state) == "" {
		oauthJSONError(c, http.StatusBadRequest, "invalid_request", "state is required")
		return
	}
	scopes := strings.Fields(strings.TrimSpace(c.Query("scope")))
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile"}
	}
	client, err := h.provider.ValidateAuthorizationRequest(c.Request.Context(), clientID, redirectURI, scopes, challenge, method)
	if err != nil {
		oauthJSONError(c, http.StatusBadRequest, oauthErrorCode(err), "Invalid OAuth authorization request")
		return
	}
	tx, err := h.provider.CreateAuthorizationTransaction(c.Request.Context(), service.OAuthAuthorizationTransaction{
		ClientID: client.ClientID, ClientName: client.Name, ClientDescription: client.Description,
		ClientLogoURL: client.LogoURL, RedirectURI: redirectURI, Scope: scopes, State: state,
		CodeChallenge: challenge, CodeChallengeMethod: method,
	})
	if err != nil {
		oauthJSONError(c, http.StatusServiceUnavailable, "temporarily_unavailable", "Failed to create authorization transaction")
		return
	}
	secure := requestIsHTTPS(c)
	http.SetCookie(c.Writer, &http.Cookie{
		Name: oauthTransactionCookie, Value: tx.ID, Path: "/api/v1/oauth", MaxAge: 600,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	redirect := h.authorizationPageURL(c, tx.ID)
	c.Redirect(http.StatusFound, redirect)
}

// authorizationPageURL points the browser at the SPA authorization page. The
// API and the frontend often run on different origins during development.
func (h *OAuthProviderHandler) authorizationPageURL(c *gin.Context, transactionID string) string {
	base := ""
	if h.settingSvc != nil {
		base = strings.TrimRight(h.settingSvc.GetFrontendURL(c.Request.Context()), "/")
	}
	if base == "" {
		// The Vite development server is the only supported cross-origin setup.
		// Production deployments serve the SPA from the API origin.
		if strings.HasPrefix(c.Request.Host, "127.0.0.1:") || strings.HasPrefix(c.Request.Host, "localhost:") {
			base = "http://localhost:3000"
		}
	}
	path := "/oauth/authorize?transaction_id=" + url.QueryEscape(transactionID)
	if base == "" {
		return path
	}
	return base + path
}

func (h *OAuthProviderHandler) AuthorizationTransaction(c *gin.Context) {
	tx, err := h.provider.GetAuthorizationTransaction(c.Request.Context(), c.Param("id"))
	if err != nil {
		oauthJSONError(c, http.StatusBadRequest, "invalid_request", "Authorization transaction is invalid or expired")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": tx.ID, "client_id": tx.ClientID, "client_name": tx.ClientName,
		"client_description": tx.ClientDescription, "client_logo_url": tx.ClientLogoURL,
		"scope": tx.Scope, "expires_at": tx.ExpiresAt,
	})
}

type oauthApprovalRequest struct {
	TransactionID string `json:"transaction_id" binding:"required"`
	Decision      string `json:"decision" binding:"required"`
}

func (h *OAuthProviderHandler) Approve(c *gin.Context) {
	var req oauthApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		oauthJSONError(c, http.StatusBadRequest, "invalid_request", "Invalid approval request")
		return
	}
	tx, err := h.provider.GetAuthorizationTransaction(c.Request.Context(), req.TransactionID)
	if err != nil {
		oauthJSONError(c, http.StatusBadRequest, "invalid_request", "Authorization transaction is invalid or expired")
		return
	}
	cookie, cookieErr := c.Request.Cookie(oauthTransactionCookie)
	if cookieErr != nil || cookie.Value != tx.ID {
		oauthJSONError(c, http.StatusForbidden, "access_denied", "Authorization transaction does not belong to this browser")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		oauthJSONError(c, http.StatusUnauthorized, "login_required", "Login is required")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(req.Decision), "approve") {
		redirect := oauthRedirectError(tx.RedirectURI, "access_denied", tx.State)
		if acceptsJSON(c) {
			c.JSON(http.StatusOK, gin.H{"redirect_uri": redirect})
			return
		}
		c.Redirect(http.StatusFound, redirect)
		return
	}
	code, tx, err := h.provider.IssueAuthorizationCode(c.Request.Context(), tx.ID, subject.UserID)
	if err != nil {
		oauthJSONError(c, http.StatusBadRequest, "invalid_request", "Authorization transaction is invalid or expired")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name: oauthTransactionCookie, Value: "", Path: "/api/v1/oauth", MaxAge: -1,
		HttpOnly: true, Secure: requestIsHTTPS(c), SameSite: http.SameSiteLaxMode,
	})
	redirect := tx.RedirectURI + "?" + url.Values{"code": []string{code}, "state": []string{tx.State}}.Encode()
	if acceptsJSON(c) {
		c.JSON(http.StatusOK, gin.H{"redirect_uri": redirect})
		return
	}
	c.Redirect(http.StatusFound, redirect)
}

type oauthTokenRequest struct {
	GrantType    string `form:"grant_type" json:"grant_type"`
	ClientID     string `form:"client_id" json:"client_id"`
	Code         string `form:"code" json:"code"`
	RedirectURI  string `form:"redirect_uri" json:"redirect_uri"`
	CodeVerifier string `form:"code_verifier" json:"code_verifier"`
	RefreshToken string `form:"refresh_token" json:"refresh_token"`
}

func (h *OAuthProviderHandler) Token(c *gin.Context) {
	var req oauthTokenRequest
	if err := c.ShouldBind(&req); err != nil {
		oauthTokenError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.ClientID == "" {
		req.ClientID = basicClientID(c)
	}
	client, err := h.provider.GetClient(c.Request.Context(), req.ClientID)
	if err != nil || client.ClientType == "confidential" {
		oauthTokenError(c, http.StatusUnauthorized, "invalid_client")
		return
	}
	var result service.OAuthTokenResponse
	switch strings.TrimSpace(req.GrantType) {
	case "authorization_code":
		result, err = h.provider.ExchangeAuthorizationCode(c.Request.Context(), req.ClientID, req.Code, req.RedirectURI, req.CodeVerifier)
	case "refresh_token":
		result, err = h.provider.RefreshAccessToken(c.Request.Context(), req.ClientID, req.RefreshToken)
	default:
		oauthTokenError(c, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	if err != nil {
		oauthTokenError(c, http.StatusBadRequest, oauthErrorCode(err))
		return
	}
	// RFC 6749 §5.1: token responses must not be cached.
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	// RFC 9207: echo the issuer so clients can defend against mix-up attacks.
	result.Iss = requestBaseURL(c) + "/api/v1/oauth"
	c.JSON(http.StatusOK, result)
}

type oauthRevokeRequest struct {
	Token string `form:"token" json:"token" binding:"required"`
}

func (h *OAuthProviderHandler) Revoke(c *gin.Context) {
	var req oauthRevokeRequest
	if err := c.ShouldBind(&req); err != nil {
		oauthJSONError(c, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}
	if err := h.provider.RevokeToken(c.Request.Context(), req.Token); err != nil {
		oauthJSONError(c, http.StatusInternalServerError, "temporarily_unavailable", "Failed to revoke token")
		return
	}
	c.Status(http.StatusOK)
}

func (h *OAuthProviderHandler) GrantsCurrent(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		oauthJSONError(c, http.StatusUnauthorized, "login_required", "Login is required")
		return
	}
	clientID := strings.TrimSpace(c.Query("client_id"))
	if _, err := h.provider.GetClient(c.Request.Context(), clientID); err != nil {
		oauthJSONError(c, http.StatusBadRequest, "invalid_client", "Invalid client")
		return
	}
	approved, scopes, err := h.provider.GrantStatus(c.Request.Context(), subject.UserID, clientID)
	if err != nil {
		oauthJSONError(c, http.StatusInternalServerError, "temporarily_unavailable", "Failed to load grant")
		return
	}
	c.JSON(http.StatusOK, gin.H{"client_id": clientID, "approved": approved, "scopes": scopes})
}

// GrantsList returns the signed-in panel user's connected apps (active grants).
func (h *OAuthProviderHandler) GrantsList(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		oauthJSONError(c, http.StatusUnauthorized, "login_required", "Login is required")
		return
	}
	grants, err := h.provider.ListGrants(c.Request.Context(), subject.UserID)
	if err != nil {
		oauthJSONError(c, http.StatusInternalServerError, "temporarily_unavailable", "Failed to load grants")
		return
	}
	c.JSON(http.StatusOK, gin.H{"grants": grants})
}

// RevokeGrantByID disconnects one connected app for the signed-in panel user;
// the grant and every token issued under it are revoked immediately.
func (h *OAuthProviderHandler) RevokeGrantByID(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		oauthJSONError(c, http.StatusUnauthorized, "login_required", "Login is required")
		return
	}
	clientID := strings.TrimSpace(c.Param("clientId"))
	if clientID == "" {
		oauthJSONError(c, http.StatusBadRequest, "invalid_request", "client id is required")
		return
	}
	if err := h.provider.RevokeGrant(c.Request.Context(), subject.UserID, clientID); err != nil {
		oauthJSONError(c, http.StatusInternalServerError, "temporarily_unavailable", "Failed to revoke grant")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "grant revoked"})
}

// ==================== Admin: OAuth client management ====================
// Endpoints live under /api/v1/admin/oauth-clients behind adminAuth + audit.
// The first release manages public clients only; PKCE is always mandatory and
// confidential secrets are reserved for a later release (see service docs).

type oauthClientRequest struct {
	Name          string   `json:"name" binding:"required"`
	Description   string   `json:"description"`
	LogoURL       string   `json:"logo_url"`
	RedirectURIs  []string `json:"redirect_uris" binding:"required"`
	AllowedScopes []string `json:"allowed_scopes" binding:"required"`
	Status        string   `json:"status"`
}

// AdminListClients returns every registered OAuth client.
func (h *OAuthProviderHandler) AdminListClients(c *gin.Context) {
	if h.provider == nil {
		response.InternalError(c, "OAuth provider is not ready")
		return
	}
	clients, err := h.provider.ListClients(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"clients": clients})
}

// AdminUpsertClient creates or updates one client identified by the URL param.
// The client_id is immutable: changing it means registering a new client.
func (h *OAuthProviderHandler) AdminUpsertClient(c *gin.Context) {
	if h.provider == nil {
		response.InternalError(c, "OAuth provider is not ready")
		return
	}
	var req oauthClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: name, redirect_uris and allowed_scopes are required")
		return
	}
	client, err := h.provider.UpsertClient(c.Request.Context(), c.Param("clientId"), service.OAuthClientInput{
		Name:          req.Name,
		Description:   req.Description,
		LogoURL:       req.LogoURL,
		RedirectURIs:  req.RedirectURIs,
		AllowedScopes: req.AllowedScopes,
		Status:        req.Status,
	})
	if err != nil {
		if errors.Is(err, service.ErrOAuthInvalidClient) {
			response.NotFound(c, "OAuth client not found")
			return
		}
		if errors.Is(err, service.ErrOAuthInvalidClientInput) {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, client)
}

// AdminDeleteClient removes one client; its grants and tokens stop working.
func (h *OAuthProviderHandler) AdminDeleteClient(c *gin.Context) {
	if h.provider == nil {
		response.InternalError(c, "OAuth provider is not ready")
		return
	}
	err := h.provider.DeleteClient(c.Request.Context(), c.Param("clientId"))
	if err != nil {
		if errors.Is(err, service.ErrOAuthInvalidClient) {
			response.NotFound(c, "OAuth client not found")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "OAuth client deleted"})
}

func requestBaseURL(c *gin.Context) string {
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}
	return scheme + "://" + c.Request.Host
}

func requestIsHTTPS(c *gin.Context) bool {
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") || c.Request.TLS != nil
}

func oauthRedirectError(redirectURI, code, state string) string {
	values := url.Values{"error": []string{code}}
	if state != "" {
		values.Set("state", state)
	}
	return redirectURI + "?" + values.Encode()
}

func basicClientID(c *gin.Context) string {
	username, _, ok := c.Request.BasicAuth()
	if !ok {
		return ""
	}
	return username
}

func oauthErrorCode(err error) string {
	switch err {
	case service.ErrOAuthInvalidClient:
		return "invalid_client"
	case service.ErrOAuthInvalidScope:
		return "invalid_scope"
	case service.ErrOAuthRedirectMismatch:
		return "invalid_request"
	case service.ErrOAuthAuthorizationDenied:
		return "access_denied"
	default:
		return "invalid_grant"
	}
}

func oauthJSONError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": code, "error_description": message})
}

func oauthTokenError(c *gin.Context, status int, code string) {
	c.JSON(status, gin.H{"error": code})
}

func acceptsJSON(c *gin.Context) bool {
	return strings.Contains(strings.ToLower(c.GetHeader("Accept")), "application/json")
}
