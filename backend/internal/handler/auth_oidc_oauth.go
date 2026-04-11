package handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
)

const (
	oidcOAuthCookiePath        = "/api/v1/auth/oauth/oidc"
	oidcOAuthStateCookieName   = "oidc_oauth_state"
	oidcOAuthVerifierCookie    = "oidc_oauth_verifier"
	oidcOAuthRedirectCookie    = "oidc_oauth_redirect"
	oidcOAuthNonceCookie       = "oidc_oauth_nonce"
	oidcOAuthCookieMaxAgeSec   = 10 * 60 // 10 minutes
	oidcOAuthDefaultRedirectTo = "/dashboard"
	oidcOAuthDefaultFrontendCB = "/auth/oidc/callback"
)

type oidcTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

type oidcTokenExchangeError struct {
	StatusCode          int
	ProviderError       string
	ProviderDescription string
	Body                string
}

func (e *oidcTokenExchangeError) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("token exchange status=%d", e.StatusCode)}
	if strings.TrimSpace(e.ProviderError) != "" {
		parts = append(parts, "error="+strings.TrimSpace(e.ProviderError))
	}
	if strings.TrimSpace(e.ProviderDescription) != "" {
		parts = append(parts, "error_description="+strings.TrimSpace(e.ProviderDescription))
	}
	return strings.Join(parts, " ")
}

type oidcIDTokenClaims struct {
	Email             string `json:"email,omitempty"`
	EmailVerified     *bool  `json:"email_verified,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Name              string `json:"name,omitempty"`
	Nonce             string `json:"nonce,omitempty"`
	Azp               string `json:"azp,omitempty"`
	jwt.RegisteredClaims
}

type oidcUserInfoClaims struct {
	Email         string
	Username      string
	Subject       string
	EmailVerified *bool
}

type oidcJWKSet struct {
	Keys []oidcJWK `json:"keys"`
}

type oidcJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`

	N string `json:"n"`
	E string `json:"e"`

	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// OIDCOAuthStart 启动通用 OIDC OAuth 登录流程。
// GET /api/v1/auth/oauth/oidc/start?redirect=/dashboard
func (h *AuthHandler) OIDCOAuthStart(c *gin.Context) {
	cfg, err := h.getOIDCOAuthConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	state, err := oauth.GenerateState()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_STATE_GEN_FAILED", "failed to generate oauth state").WithCause(err))
		return
	}

	redirectTo := sanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirectTo == "" {
		redirectTo = oidcOAuthDefaultRedirectTo
	}

	secureCookie := isRequestHTTPS(c)
	oidcSetCookie(c, oidcOAuthStateCookieName, encodeCookieValue(state), oidcOAuthCookieMaxAgeSec, secureCookie)
	oidcSetCookie(c, oidcOAuthRedirectCookie, encodeCookieValue(redirectTo), oidcOAuthCookieMaxAgeSec, secureCookie)

	codeChallenge := ""
	if cfg.UsePKCE {
		verifier, genErr := oauth.GenerateCodeVerifier()
		if genErr != nil {
			response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_PKCE_GEN_FAILED", "failed to generate pkce verifier").WithCause(genErr))
			return
		}
		codeChallenge = oauth.GenerateCodeChallenge(verifier)
		oidcSetCookie(c, oidcOAuthVerifierCookie, encodeCookieValue(verifier), oidcOAuthCookieMaxAgeSec, secureCookie)
	}

	nonce := ""
	if cfg.ValidateIDToken {
		nonce, err = oauth.GenerateState()
		if err != nil {
			response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_NONCE_GEN_FAILED", "failed to generate oauth nonce").WithCause(err))
			return
		}
		oidcSetCookie(c, oidcOAuthNonceCookie, encodeCookieValue(nonce), oidcOAuthCookieMaxAgeSec, secureCookie)
	}

	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url not configured"))
		return
	}

	authURL, err := buildOIDCAuthorizeURL(cfg, state, nonce, codeChallenge, redirectURI)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
		return
	}

	c.Redirect(http.StatusFound, authURL)
}

// OIDCOAuthCallback 处理 OIDC 回调：校验 id_token、创建/登录用户并重定向到前端。
// GET /api/v1/auth/oauth/oidc/callback?code=...&state=...
func (h *AuthHandler) OIDCOAuthCallback(c *gin.Context) {
	cfg, cfgErr := h.getOIDCOAuthConfig(c.Request.Context())
	if cfgErr != nil {
		response.ErrorFrom(c, cfgErr)
		return
	}

	frontendCallback := strings.TrimSpace(cfg.FrontendRedirectURL)
	if frontendCallback == "" {
		frontendCallback = oidcOAuthDefaultFrontendCB
	}

	if providerErr := strings.TrimSpace(c.Query("error")); providerErr != "" {
		redirectOAuthError(c, frontendCallback, "provider_error", providerErr, c.Query("error_description"))
		return
	}

	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		redirectOAuthError(c, frontendCallback, "missing_params", "missing code/state", "")
		return
	}

	secureCookie := isRequestHTTPS(c)
	defer func() {
		oidcClearCookie(c, oidcOAuthStateCookieName, secureCookie)
		oidcClearCookie(c, oidcOAuthVerifierCookie, secureCookie)
		oidcClearCookie(c, oidcOAuthRedirectCookie, secureCookie)
		oidcClearCookie(c, oidcOAuthNonceCookie, secureCookie)
	}()

	expectedState, err := readCookieDecoded(c, oidcOAuthStateCookieName)
	if err != nil || expectedState == "" || state != expectedState {
		redirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth state", "")
		return
	}

	redirectTo, _ := readCookieDecoded(c, oidcOAuthRedirectCookie)
	redirectTo = sanitizeFrontendRedirectPath(redirectTo)
	if redirectTo == "" {
		redirectTo = oidcOAuthDefaultRedirectTo
	}

	codeVerifier := ""
	if cfg.UsePKCE {
		codeVerifier, _ = readCookieDecoded(c, oidcOAuthVerifierCookie)
		if codeVerifier == "" {
			redirectOAuthError(c, frontendCallback, "missing_verifier", "missing pkce verifier", "")
			return
		}
	}

	expectedNonce := ""
	if cfg.ValidateIDToken {
		expectedNonce, _ = readCookieDecoded(c, oidcOAuthNonceCookie)
		if expectedNonce == "" {
			redirectOAuthError(c, frontendCallback, "missing_nonce", "missing oauth nonce", "")
			return
		}
	}

	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		redirectOAuthError(c, frontendCallback, "config_error", "oauth redirect url not configured", "")
		return
	}

	tokenResp, err := oidcExchangeCode(c.Request.Context(), cfg, code, redirectURI, codeVerifier)
	if err != nil {
		description := ""
		var exchangeErr *oidcTokenExchangeError
		if errors.As(err, &exchangeErr) && exchangeErr != nil {
			log.Printf(
				"[OIDC OAuth] token exchange failed: status=%d provider_error=%q provider_description=%q body=%s",
				exchangeErr.StatusCode,
				exchangeErr.ProviderError,
				exchangeErr.ProviderDescription,
				truncateLogValue(exchangeErr.Body, 2048),
			)
			description = exchangeErr.Error()
		} else {
			log.Printf("[OIDC OAuth] token exchange failed: %v", err)
			description = err.Error()
		}
		redirectOAuthError(c, frontendCallback, "token_exchange_failed", "failed to exchange oauth code", singleLine(description))
		return
	}

	if cfg.ValidateIDToken && strings.TrimSpace(tokenResp.IDToken) == "" {
		redirectOAuthError(c, frontendCallback, "missing_id_token", "missing id_token", "")
		return
	}

	idClaims, err := oidcParseAndValidateIDToken(c.Request.Context(), cfg, tokenResp.IDToken, expectedNonce)
	if err != nil {
		log.Printf("[OIDC OAuth] id_token validation failed: %v", err)
		redirectOAuthError(c, frontendCallback, "invalid_id_token", "failed to validate id_token", "")
		return
	}

	userInfoClaims, err := oidcFetchUserInfo(c.Request.Context(), cfg, tokenResp)
	if err != nil {
		log.Printf("[OIDC OAuth] userinfo fetch failed: %v", err)
		redirectOAuthError(c, frontendCallback, "userinfo_failed", "failed to fetch user info", "")
		return
	}

	subject := strings.TrimSpace(idClaims.Subject)
	if subject == "" {
		subject = strings.TrimSpace(userInfoClaims.Subject)
	}
	if subject == "" {
		redirectOAuthError(c, frontendCallback, "missing_subject", "missing subject claim", "")
		return
	}
	issuer := strings.TrimSpace(idClaims.Issuer)
	if issuer == "" {
		issuer = strings.TrimSpace(cfg.IssuerURL)
	}
	if issuer == "" {
		redirectOAuthError(c, frontendCallback, "missing_issuer", "missing issuer claim", "")
		return
	}

	emailVerified := userInfoClaims.EmailVerified
	if emailVerified == nil {
		emailVerified = idClaims.EmailVerified
	}
	if cfg.RequireEmailVerified {
		if emailVerified == nil || !*emailVerified {
			redirectOAuthError(c, frontendCallback, "email_not_verified", "email is not verified", "")
			return
		}
	}

	identityKey := oidcIdentityKey(issuer, subject)
	email := oidcSelectLoginEmail(userInfoClaims.Email, idClaims.Email, identityKey)
	username := firstNonEmpty(
		userInfoClaims.Username,
		idClaims.PreferredUsername,
		idClaims.Name,
		oidcFallbackUsername(subject),
	)

	// 传入空邀请码；如果需要邀请码，服务层返回 ErrOAuthInvitationRequired
	tokenPair, _, err := h.authService.LoginOrRegisterOAuthWithTokenPair(c.Request.Context(), email, username, "")
	if err != nil {
		if errors.Is(err, service.ErrOAuthInvitationRequired) {
			pendingToken, tokenErr := h.authService.CreatePendingOAuthToken(email, username)
			if tokenErr != nil {
				redirectOAuthError(c, frontendCallback, "login_failed", "service_error", "")
				return
			}
			fragment := url.Values{}
			fragment.Set("error", "invitation_required")
			fragment.Set("pending_oauth_token", pendingToken)
			fragment.Set("redirect", redirectTo)
			redirectWithFragment(c, frontendCallback, fragment)
			return
		}
		redirectOAuthError(c, frontendCallback, "login_failed", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}

	fragment := url.Values{}
	fragment.Set("access_token", tokenPair.AccessToken)
	fragment.Set("refresh_token", tokenPair.RefreshToken)
	fragment.Set("expires_in", fmt.Sprintf("%d", tokenPair.ExpiresIn))
	fragment.Set("token_type", "Bearer")
	fragment.Set("redirect", redirectTo)
	redirectWithFragment(c, frontendCallback, fragment)
}

type completeOIDCOAuthRequest struct {
	PendingOAuthToken string `json:"pending_oauth_token" binding:"required"`
	InvitationCode    string `json:"invitation_code"     binding:"required"`
}

// CompleteOIDCOAuthRegistration completes a pending OAuth registration by validating
// the invitation code and creating the user account.
// POST /api/v1/auth/oauth/oidc/complete-registration
func (h *AuthHandler) CompleteOIDCOAuthRegistration(c *gin.Context) {
	var req completeOIDCOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST", "message": err.Error()})
		return
	}

	email, username, err := h.authService.VerifyPendingOAuthToken(req.PendingOAuthToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "INVALID_TOKEN", "message": "invalid or expired registration token"})
		return
	}

	tokenPair, _, err := h.authService.LoginOrRegisterOAuthWithTokenPair(c.Request.Context(), email, username, req.InvitationCode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_in":    tokenPair.ExpiresIn,
		"token_type":    "Bearer",
	})
}

func (h *AuthHandler) getOIDCOAuthConfig(ctx context.Context) (config.OIDCConnectConfig, error) {
	if h != nil && h.settingSvc != nil {
		return h.settingSvc.GetOIDCConnectOAuthConfig(ctx)
	}
	if h == nil || h.cfg == nil {
		return config.OIDCConnectConfig{}, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
	}
	if !h.cfg.OIDC.Enabled {
		return config.OIDCConnectConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
	}
	return h.cfg.OIDC, nil
}

func oidcExchangeCode(
	ctx context.Context,
	cfg config.OIDCConnectConfig,
	code string,
	redirectURI string,
	codeVerifier string,
) (*oidcTokenResponse, error) {
	client := req.C().SetTimeout(30 * time.Second)

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if cfg.UsePKCE {
		form.Set("code_verifier", codeVerifier)
	}

	r := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json")

	switch strings.ToLower(strings.TrimSpace(cfg.TokenAuthMethod)) {
	case "", "client_secret_post":
		form.Set("client_secret", cfg.ClientSecret)
	case "client_secret_basic":
		r.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	case "none":
	default:
		return nil, fmt.Errorf("unsupported token_auth_method: %s", cfg.TokenAuthMethod)
	}

	resp, err := r.SetFormDataFromValues(form).Post(cfg.TokenURL)
	if err != nil {
		return nil, fmt.Errorf("request token: %w", err)
	}
	body := strings.TrimSpace(resp.String())
	if !resp.IsSuccessState() {
		providerErr, providerDesc := parseOAuthProviderError(body)
		return nil, &oidcTokenExchangeError{
			StatusCode:          resp.StatusCode,
			ProviderError:       providerErr,
			ProviderDescription: providerDesc,
			Body:                body,
		}
	}

	tokenResp, ok := oidcParseTokenResponse(body)
	if !ok {
		return nil, &oidcTokenExchangeError{StatusCode: resp.StatusCode, Body: body}
	}
	if strings.TrimSpace(tokenResp.TokenType) == "" {
		tokenResp.TokenType = "Bearer"
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" && strings.TrimSpace(tokenResp.IDToken) == "" {
		return nil, &oidcTokenExchangeError{StatusCode: resp.StatusCode, Body: body}
	}
	return tokenResp, nil
}

func oidcParseTokenResponse(body string) (*oidcTokenResponse, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, false
	}

	accessToken := strings.TrimSpace(getGJSON(body, "access_token"))
	idToken := strings.TrimSpace(getGJSON(body, "id_token"))
	if accessToken != "" || idToken != "" {
		tokenType := strings.TrimSpace(getGJSON(body, "token_type"))
		refreshToken := strings.TrimSpace(getGJSON(body, "refresh_token"))
		scope := strings.TrimSpace(getGJSON(body, "scope"))
		expiresIn := gjson.Get(body, "expires_in").Int()
		return &oidcTokenResponse{
			AccessToken:  accessToken,
			TokenType:    tokenType,
			ExpiresIn:    expiresIn,
			RefreshToken: refreshToken,
			Scope:        scope,
			IDToken:      idToken,
		}, true
	}

	values, err := url.ParseQuery(body)
	if err != nil {
		return nil, false
	}
	accessToken = strings.TrimSpace(values.Get("access_token"))
	idToken = strings.TrimSpace(values.Get("id_token"))
	if accessToken == "" && idToken == "" {
		return nil, false
	}
	expiresIn := int64(0)
	if raw := strings.TrimSpace(values.Get("expires_in")); raw != "" {
		if v, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			expiresIn = v
		}
	}
	return &oidcTokenResponse{
		AccessToken:  accessToken,
		TokenType:    strings.TrimSpace(values.Get("token_type")),
		ExpiresIn:    expiresIn,
		RefreshToken: strings.TrimSpace(values.Get("refresh_token")),
		Scope:        strings.TrimSpace(values.Get("scope")),
		IDToken:      idToken,
	}, true
}

func oidcFetchUserInfo(
	ctx context.Context,
	cfg config.OIDCConnectConfig,
	token *oidcTokenResponse,
) (*oidcUserInfoClaims, error) {
	if strings.TrimSpace(cfg.UserInfoURL) == "" {
		return &oidcUserInfoClaims{}, nil
	}
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		return nil, errors.New("missing access_token for userinfo request")
	}

	client := req.C().SetTimeout(30 * time.Second)
	authorization, err := buildBearerAuthorization(token.TokenType, token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid token for userinfo request: %w", err)
	}

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", authorization).
		Get(cfg.UserInfoURL)
	if err != nil {
		return nil, fmt.Errorf("request userinfo: %w", err)
	}
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("userinfo status=%d", resp.StatusCode)
	}

	return oidcParseUserInfo(resp.String(), cfg), nil
}

func oidcParseUserInfo(body string, cfg config.OIDCConnectConfig) *oidcUserInfoClaims {
	claims := &oidcUserInfoClaims{}
	claims.Email = firstNonEmpty(
		getGJSON(body, cfg.UserInfoEmailPath),
		getGJSON(body, "email"),
		getGJSON(body, "user.email"),
		getGJSON(body, "data.email"),
		getGJSON(body, "attributes.email"),
	)
	claims.Username = firstNonEmpty(
		getGJSON(body, cfg.UserInfoUsernamePath),
		getGJSON(body, "preferred_username"),
		getGJSON(body, "username"),
		getGJSON(body, "name"),
		getGJSON(body, "user.username"),
		getGJSON(body, "user.name"),
	)
	claims.Subject = firstNonEmpty(
		getGJSON(body, cfg.UserInfoIDPath),
		getGJSON(body, "sub"),
		getGJSON(body, "id"),
		getGJSON(body, "user_id"),
		getGJSON(body, "uid"),
		getGJSON(body, "user.id"),
	)
	if verified, ok := getGJSONBool(body, "email_verified"); ok {
		claims.EmailVerified = &verified
	}
	claims.Email = strings.TrimSpace(claims.Email)
	claims.Username = strings.TrimSpace(claims.Username)
	claims.Subject = strings.TrimSpace(claims.Subject)
	return claims
}

func getGJSONBool(body string, path string) (bool, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, false
	}
	res := gjson.Get(body, path)
	if !res.Exists() {
		return false, false
	}
	return res.Bool(), true
}

func buildOIDCAuthorizeURL(cfg config.OIDCConnectConfig, state, nonce, codeChallenge, redirectURI string) (string, error) {
	u, err := url.Parse(cfg.AuthorizeURL)
	if err != nil {
		return "", fmt.Errorf("parse authorize_url: %w", err)
	}

	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	if strings.TrimSpace(cfg.Scopes) != "" {
		q.Set("scope", cfg.Scopes)
	}
	q.Set("state", state)
	if strings.TrimSpace(nonce) != "" {
		q.Set("nonce", nonce)
	}
	if cfg.UsePKCE {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func oidcParseAndValidateIDToken(ctx context.Context, cfg config.OIDCConnectConfig, idToken string, expectedNonce string) (*oidcIDTokenClaims, error) {
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return nil, errors.New("missing id_token")
	}
	allowed := oidcAllowedSigningAlgs(cfg.AllowedSigningAlgs)
	if len(allowed) == 0 {
		return nil, errors.New("empty allowed signing algorithms")
	}

	jwks, err := oidcFetchJWKSet(ctx, cfg.JWKSURL)
	if err != nil {
		return nil, err
	}
	leeway := time.Duration(cfg.ClockSkewSeconds) * time.Second
	claims := &oidcIDTokenClaims{}

	parsed, err := jwt.ParseWithClaims(
		idToken,
		claims,
		func(token *jwt.Token) (any, error) {
			alg := strings.TrimSpace(token.Method.Alg())
			if !containsString(allowed, alg) {
				return nil, fmt.Errorf("unexpected signing algorithm: %s", alg)
			}
			kid, _ := token.Header["kid"].(string)
			return oidcFindPublicKey(jwks, strings.TrimSpace(kid), alg)
		},
		jwt.WithValidMethods(allowed),
		jwt.WithAudience(cfg.ClientID),
		jwt.WithIssuer(cfg.IssuerURL),
		jwt.WithLeeway(leeway),
	)
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("id_token invalid")
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, errors.New("id_token missing sub")
	}
	if expectedNonce != "" && strings.TrimSpace(claims.Nonce) != strings.TrimSpace(expectedNonce) {
		return nil, errors.New("id_token nonce mismatch")
	}
	if len(claims.Audience) > 1 {
		if strings.TrimSpace(claims.Azp) == "" || strings.TrimSpace(claims.Azp) != strings.TrimSpace(cfg.ClientID) {
			return nil, errors.New("id_token azp mismatch")
		}
	}
	return claims, nil
}

func oidcAllowedSigningAlgs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"RS256", "ES256", "PS256"}
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		alg := strings.ToUpper(strings.TrimSpace(part))
		if alg == "" {
			continue
		}
		if _, ok := seen[alg]; ok {
			continue
		}
		seen[alg] = struct{}{}
		out = append(out, alg)
	}
	return out
}

func oidcFetchJWKSet(ctx context.Context, jwksURL string) (*oidcJWKSet, error) {
	jwksURL = strings.TrimSpace(jwksURL)
	if jwksURL == "" {
		return nil, errors.New("missing jwks_url")
	}
	resp, err := req.C().
		SetTimeout(30*time.Second).
		R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		Get(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("request jwks: %w", err)
	}
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("jwks status=%d", resp.StatusCode)
	}
	set := &oidcJWKSet{}
	if err := json.Unmarshal(resp.Bytes(), set); err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}
	if len(set.Keys) == 0 {
		return nil, errors.New("jwks empty keys")
	}
	return set, nil
}

func oidcFindPublicKey(set *oidcJWKSet, kid, alg string) (any, error) {
	if set == nil {
		return nil, errors.New("jwks not loaded")
	}
	alg = strings.ToUpper(strings.TrimSpace(alg))
	kid = strings.TrimSpace(kid)

	var lastErr error
	for i := range set.Keys {
		k := set.Keys[i]
		if strings.TrimSpace(k.Use) != "" && !strings.EqualFold(strings.TrimSpace(k.Use), "sig") {
			continue
		}
		if kid != "" && strings.TrimSpace(k.Kid) != kid {
			continue
		}
		if strings.TrimSpace(k.Alg) != "" && !strings.EqualFold(strings.TrimSpace(k.Alg), alg) {
			continue
		}
		pk, err := k.publicKey()
		if err != nil {
			lastErr = err
			continue
		}
		if pk != nil {
			return pk, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if kid != "" {
		return nil, fmt.Errorf("jwk not found for kid=%s", kid)
	}
	return nil, errors.New("jwk not found")
}

func (k oidcJWK) publicKey() (any, error) {
	switch strings.ToUpper(strings.TrimSpace(k.Kty)) {
	case "RSA":
		n, err := decodeBase64URLBigInt(k.N)
		if err != nil {
			return nil, fmt.Errorf("decode rsa n: %w", err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(k.E))
		if err != nil {
			return nil, fmt.Errorf("decode rsa e: %w", err)
		}
		if len(eBytes) == 0 {
			return nil, errors.New("empty rsa e")
		}
		e := 0
		for _, b := range eBytes {
			e = (e << 8) | int(b)
		}
		if e <= 0 {
			return nil, errors.New("invalid rsa exponent")
		}
		if n.Sign() <= 0 {
			return nil, errors.New("invalid rsa modulus")
		}
		return &rsa.PublicKey{N: n, E: e}, nil
	case "EC":
		var curve elliptic.Curve
		switch strings.TrimSpace(k.Crv) {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported ec curve: %s", k.Crv)
		}
		x, err := decodeBase64URLBigInt(k.X)
		if err != nil {
			return nil, fmt.Errorf("decode ec x: %w", err)
		}
		y, err := decodeBase64URLBigInt(k.Y)
		if err != nil {
			return nil, fmt.Errorf("decode ec y: %w", err)
		}
		if !curve.IsOnCurve(x, y) {
			return nil, errors.New("ec point is not on curve")
		}
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
	default:
		return nil, fmt.Errorf("unsupported jwk kty: %s", k.Kty)
	}
}

func decodeBase64URLBigInt(raw string) (*big.Int, error) {
	buf, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, errors.New("empty value")
	}
	return new(big.Int).SetBytes(buf), nil
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), target) {
			return true
		}
	}
	return false
}

func oidcIdentityKey(issuer, subject string) string {
	issuer = strings.TrimSpace(strings.ToLower(issuer))
	subject = strings.TrimSpace(subject)
	return issuer + "\x1f" + subject
}

func oidcSyntheticEmailFromIdentityKey(identityKey string) string {
	identityKey = strings.TrimSpace(identityKey)
	if identityKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(identityKey))
	return "oidc-" + hex.EncodeToString(sum[:16]) + service.OIDCConnectSyntheticEmailDomain
}

func oidcSelectLoginEmail(userInfoEmail, idTokenEmail, identityKey string) string {
	email := strings.TrimSpace(firstNonEmpty(userInfoEmail, idTokenEmail))
	if email != "" {
		return email
	}
	return oidcSyntheticEmailFromIdentityKey(identityKey)
}

func oidcFallbackUsername(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "oidc_user"
	}
	sum := sha256.Sum256([]byte(subject))
	return "oidc_" + hex.EncodeToString(sum[:])[:12]
}

func oidcSetCookie(c *gin.Context, name, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     oidcOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func oidcClearCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     oidcOAuthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
