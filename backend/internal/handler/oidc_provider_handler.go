package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OIDCProviderHandler handles OIDC Provider endpoints
// This handler implements the OIDC Provider functionality for the application
// to act as an OIDC Provider for other applications

type OIDCProviderHandler struct {
	cfg               *config.Config
	providerService   *service.OIDCProviderService
	clientService     *service.OidcClientService
	consentService    *service.OidcConsentService
}

// NewOIDCProviderHandler creates a new OIDCProviderHandler
func NewOIDCProviderHandler(
	cfg *config.Config,
	providerService *service.OIDCProviderService,
	clientService *service.OidcClientService,
	consentService *service.OidcConsentService,
) *OIDCProviderHandler {
	return &OIDCProviderHandler{
		cfg:             cfg,
		providerService: providerService,
		clientService:   clientService,
		consentService:  consentService,
	}
}

// AuthorizeRequest represents the OIDC authorization request parameters
type AuthorizeRequest struct {
	ClientID     string `form:"client_id" binding:"required"`
	RedirectURI  string `form:"redirect_uri" binding:"required"`
	ResponseType string `form:"response_type" binding:"required"`
	Scope        string `form:"scope"`
	State        string `form:"state"`
	Nonce        string `form:"nonce"`
	CodeChallenge       string `form:"code_challenge"`
	CodeChallengeMethod string `form:"code_challenge_method"`
	ResponseMode string `form:"response_mode"`
	Display      string `form:"display"`
	Prompt       string `form:"prompt"`
	MaxAge       string `form:"max_age"`
	UILocales    string `form:"ui_locales"`
	IDTokenHint  string `form:"id_token_hint"`
	LoginHint    string `form:"login_hint"`
	ACRValues    string `form:"acr_values"`
}

// AuthorizeResponse represents the OIDC authorization response
type AuthorizeResponse struct {
	Code  string `json:"code,omitempty"`
	State string `json:"state,omitempty"`
	Error string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// TokenRequest represents the OIDC token request parameters
type TokenRequest struct {
	GrantType    string `form:"grant_type" binding:"required"`
	Code         string `form:"code"`
	RedirectURI  string `form:"redirect_uri"`
	ClientID     string `form:"client_id"`
	ClientSecret string `form:"client_secret"`
	CodeVerifier string `form:"code_verifier"`
	RefreshToken string `form:"refresh_token"`
	Scope        string `form:"scope"`
}

// TokenResponse represents the OIDC token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// UserInfoResponse represents the OIDC userinfo response
type UserInfoResponse struct {
	Sub               string `json:"sub"`
	Name              string `json:"name,omitempty"`
	GivenName         string `json:"given_name,omitempty"`
	FamilyName        string `json:"family_name,omitempty"`
	MiddleName        string `json:"middle_name,omitempty"`
	Nickname          string `json:"nickname,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Profile           string `json:"profile,omitempty"`
	Picture           string `json:"picture,omitempty"`
	Website           string `json:"website,omitempty"`
	Email             string `json:"email,omitempty"`
	EmailVerified     *bool  `json:"email_verified,omitempty"`
	Gender            string `json:"gender,omitempty"`
	Birthdate         string `json:"birthdate,omitempty"`
	Zoneinfo          string `json:"zoneinfo,omitempty"`
	Locale            string `json:"locale,omitempty"`
	PhoneNumber       string `json:"phone_number,omitempty"`
	PhoneNumberVerified *bool `json:"phone_number_verified,omitempty"`
	Address           map[string]interface{} `json:"address,omitempty"`
	UpdatedAt         int64  `json:"updated_at,omitempty"`
}

// DiscoveryResponse represents the OIDC discovery document
type DiscoveryResponse struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
	JwksURI                           string   `json:"jwks_uri"`
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	ResponseModesSupported            []string `json:"response_modes_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
}

// JWKSResponse represents the JSON Web Key Set response
type JWKSResponse struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
}

// Authorize handles OIDC authorization endpoint
// GET /oidc/authorize
func (h *OIDCProviderHandler) Authorize(c *gin.Context) {
	var req AuthorizeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.handleAuthorizationError(c, "invalid_request", "Invalid request parameters", req.State)
		return
	}

	// Validate required parameters
	if req.ClientID == "" {
		h.handleAuthorizationError(c, "invalid_request", "Missing client_id", req.State)
		return
	}

	if req.RedirectURI == "" {
		h.handleAuthorizationError(c, "invalid_request", "Missing redirect_uri", req.State)
		return
	}

	if req.ResponseType == "" {
		h.handleAuthorizationError(c, "invalid_request", "Missing response_type", req.State)
		return
	}

	// Check if user is authenticated
	subject, ok := getAuthSubjectFromContext(c)
	if !ok {
		// Redirect to login page with OIDC parameters
		h.redirectToLogin(c, req)
		return
	}

	// Check consent
	ctx := c.Request.Context()
	consentRequired, err := h.consentService.CheckConsentRequired(ctx, subject.UserID, req.ClientID, req.Scope)
	if err != nil {
		h.handleAuthorizationError(c, "server_error", "Internal server error", req.State)
		return
	}

	if consentRequired {
		// Redirect to consent page
		h.redirectToConsent(c, req, subject.UserID)
		return
	}

	// Handle authorization
	authResult, err := h.providerService.HandleAuthorize(ctx, service.AuthorizeParams{
		ClientID:            req.ClientID,
		RedirectURI:         req.RedirectURI,
		ResponseType:        req.ResponseType,
		Scope:               req.Scope,
		State:               req.State,
		Nonce:               req.Nonce,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		UserID:              subject.UserID,
	})

	if err != nil {
		h.handleAuthorizationErrorFromService(c, err, req.State)
		return
	}

	// Redirect with authorization code
	redirectURL, err := url.Parse(req.RedirectURI)
	if err != nil {
		h.handleAuthorizationError(c, "invalid_request", "Invalid redirect_uri", req.State)
		return
	}

	query := redirectURL.Query()
	query.Set("code", authResult.Code)
	if req.State != "" {
		query.Set("state", req.State)
	}
	redirectURL.RawQuery = query.Encode()

	c.Redirect(http.StatusFound, redirectURL.String())
}

// Token handles OIDC token endpoint
// POST /oidc/token
func (h *OIDCProviderHandler) Token(c *gin.Context) {
	var req TokenRequest
	
	// Support both form-urlencoded and JSON
	contentType := c.ContentType()
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if err := c.ShouldBind(&req); err != nil {
			h.handleTokenError(c, "invalid_request", "Invalid request parameters")
			return
		}
	} else if strings.Contains(contentType, "application/json") {
		if err := c.ShouldBindJSON(&req); err != nil {
			h.handleTokenError(c, "invalid_request", "Invalid request parameters")
			return
		}
	} else {
		h.handleTokenError(c, "invalid_request", "Unsupported content type")
		return
	}

	// Validate grant type
	if req.GrantType == "" {
		h.handleTokenError(c, "invalid_request", "Missing grant_type")
		return
	}

	ctx := c.Request.Context()
	var tokenResp *service.TokenResponse
	var err error

	switch req.GrantType {
	case "authorization_code":
		if req.Code == "" {
			h.handleTokenError(c, "invalid_request", "Missing code")
			return
		}
		if req.RedirectURI == "" {
			h.handleTokenError(c, "invalid_request", "Missing redirect_uri")
			return
		}
		
		tokenResp, err = h.providerService.ExchangeCode(ctx, req.Code, req.RedirectURI, req.CodeVerifier)
		
	case "refresh_token":
		if req.RefreshToken == "" {
			h.handleTokenError(c, "invalid_request", "Missing refresh_token")
			return
		}
		
		tokenResp, err = h.providerService.RefreshToken(ctx, req.RefreshToken, req.Scope)
		
	default:
		h.handleTokenError(c, "unsupported_grant_type", "Unsupported grant type")
		return
	}

	if err != nil {
		h.handleTokenErrorFromService(c, err)
		return
	}

	response.Success(c, TokenResponse{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		Scope:        tokenResp.Scope,
	})
}

// UserInfo handles OIDC userinfo endpoint
// GET /oidc/userinfo
func (h *OIDCProviderHandler) UserInfo(c *gin.Context) {
	// Extract access token from Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		response.Unauthorized(c, "Missing Authorization header")
		return
	}

	// Parse Bearer token
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		response.Unauthorized(c, "Invalid Authorization header format")
		return
	}

	accessToken := parts[1]
	if accessToken == "" {
		response.Unauthorized(c, "Missing access token")
		return
	}

	ctx := c.Request.Context()
	userInfo, err := h.providerService.BuildUserInfo(ctx, accessToken)
	if err != nil {
		h.handleUserInfoError(c, err)
		return
	}

	// Convert to UserInfoResponse
	var resp UserInfoResponse
	if sub, ok := userInfo["sub"].(string); ok {
		resp.Sub = sub
	}
	if name, ok := userInfo["name"].(string); ok {
		resp.Name = name
	}
	if email, ok := userInfo["email"].(string); ok {
		resp.Email = email
	}
	if emailVerified, ok := userInfo["email_verified"].(bool); ok {
		resp.EmailVerified = &emailVerified
	}
	// Add other claims as needed

	response.Success(c, resp)
}

// Discovery handles OIDC discovery endpoint
// GET /.well-known/openid-configuration
func (h *OIDCProviderHandler) Discovery(c *gin.Context) {
	issuer := h.cfg.OIDC.Issuer
	if issuer == "" {
		issuer = h.cfg.BaseURL
	}

	resp := DiscoveryResponse{
		Issuer:                            issuer,
		AuthorizationEndpoint:             issuer + "/oidc/authorize",
		TokenEndpoint:                     issuer + "/oidc/token",
		UserinfoEndpoint:                  issuer + "/oidc/userinfo",
		JwksURI:                           issuer + "/oidc/jwks",
		ScopesSupported:                   []string{"openid", "profile", "email", "phone", "address"},
		ResponseTypesSupported:            []string{"code"},
		ResponseModesSupported:            []string{"query", "fragment"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		SubjectTypesSupported:             []string{"public"},
		IDTokenSigningAlgValuesSupported:  []string{"RS256"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "client_secret_basic"},
		ClaimsSupported:                   []string{"sub", "name", "email", "email_verified", "profile", "picture", "updated_at"},
		CodeChallengeMethodsSupported:     []string{"S256", "plain"},
	}

	response.Success(c, resp)
}

// JWKS handles OIDC JWKS endpoint
// GET /oidc/jwks
func (h *OIDCProviderHandler) JWKS(c *gin.Context) {
	// TODO: Implement JWKS endpoint
	// This should return the current signing keys
	response.Success(c, JWKSResponse{
		Keys: []JWK{},
	})
}

// Revocation handles OIDC token revocation endpoint
// POST /oidc/revoke
func (h *OIDCProviderHandler) Revocation(c *gin.Context) {
	var req struct {
		Token string `form:"token" binding:"required"`
		TokenTypeHint string `form:"token_type_hint"`
	}

	if err := c.ShouldBind(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}

	ctx := c.Request.Context()
	err := h.providerService.RevokeToken(ctx, req.Token)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Token revoked successfully"})
}

// Helper methods

func (h *OIDCProviderHandler) handleAuthorizationError(c *gin.Context, errorCode, errorDescription, state string) {
	// If redirect_uri is provided, redirect with error
	redirectURI := c.Query("redirect_uri")
	if redirectURI != "" && state != "" {
		redirectURL, err := url.Parse(redirectURI)
		if err == nil {
			query := redirectURL.Query()
			query.Set("error", errorCode)
			query.Set("error_description", errorDescription)
			if state != "" {
				query.Set("state", state)
			}
			redirectURL.RawQuery = query.Encode()
			c.Redirect(http.StatusFound, redirectURL.String())
			return
		}
	}

	// Otherwise return JSON error
	response.Error(c, http.StatusBadRequest, errorCode, errorDescription)
}

func (h *OIDCProviderHandler) handleAuthorizationErrorFromService(c *gin.Context, err error, state string) {
	// Map service errors to OIDC error codes
	switch err {
	case service.ErrOIDCClientNotFound:
		h.handleAuthorizationError(c, "unauthorized_client", "Client not found", state)
	case service.ErrOIDCClientDisabled:
		h.handleAuthorizationError(c, "unauthorized_client", "Client disabled", state)
	case service.ErrOIDCInvalidRedirectURI:
		h.handleAuthorizationError(c, "invalid_request", "Invalid redirect URI", state)
	case service.ErrOIDCInvalidResponseType:
		h.handleAuthorizationError(c, "unsupported_response_type", "Unsupported response type", state)
	case service.ErrOIDCInvalidScope:
		h.handleAuthorizationError(c, "invalid_scope", "Invalid scope", state)
	default:
		h.handleAuthorizationError(c, "server_error", "Internal server error", state)
	}
}

func (h *OIDCProviderHandler) handleTokenError(c *gin.Context, errorCode, errorDescription string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	
	response.Error(c, http.StatusBadRequest, errorCode, errorDescription)
}

func (h *OIDCProviderHandler) handleTokenErrorFromService(c *gin.Context, err error) {
	// Map service errors to OIDC error codes
	switch err {
	case service.ErrOIDCCodeNotFound:
		h.handleTokenError(c, "invalid_grant", "Invalid authorization code")
	case service.ErrOIDCRefreshTokenRevoked:
		h.handleTokenError(c, "invalid_grant", "Refresh token revoked")
	case service.ErrOIDCTokenNotFound:
		h.handleTokenError(c, "invalid_grant", "Invalid token")
	default:
		h.handleTokenError(c, "server_error", "Internal server error")
	}
}

func (h *OIDCProviderHandler) handleUserInfoError(c *gin.Context, err error) {
	switch err {
	case service.ErrOIDCTokenNotFound:
		response.Unauthorized(c, "Invalid access token")
	default:
		response.InternalError(c, "Failed to fetch user info")
	}
}

func (h *OIDCProviderHandler) redirectToLogin(c *gin.Context, req AuthorizeRequest) {
	// Build login URL with OIDC parameters
	loginURL := "/login?"
	params := url.Values{}
	params.Set("oidc", "true")
	params.Set("client_id", req.ClientID)
	params.Set("redirect_uri", req.RedirectURI)
	params.Set("response_type", req.ResponseType)
	if req.State != "" {
		params.Set("state", req.State)
	}
	if req.Scope != "" {
		params.Set("scope", req.Scope)
	}
	
	c.Redirect(http.StatusFound, loginURL+params.Encode())
}

func (h *OIDCProviderHandler) redirectToConsent(c *gin.Context, req AuthorizeRequest, userID int64) {
	// Build consent URL with OIDC parameters
	consentURL := "/consent?"
	params := url.Values{}
	params.Set("client_id", req.ClientID)
	params.Set("redirect_uri", req.RedirectURI)
	params.Set("response_type", req.ResponseType)
	if req.State != "" {
		params.Set("state", req.State)
	}
	if req.Scope != "" {
		params.Set("scope", req.Scope)
	}
	
	c.Redirect(http.StatusFound, consentURL+params.Encode())
}

// getAuthSubjectFromContext extracts authentication subject from context
// This should be implemented based on your authentication middleware
func getAuthSubjectFromContext(c *gin.Context) (*AuthSubject, bool) {
	// TODO: Implement based on your authentication middleware
	// This is a placeholder implementation
	return nil, false
}

// AuthSubject represents the authenticated user subject
type AuthSubject struct {
	UserID int64
	Email  string
	Roles  []string
}