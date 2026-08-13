package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const grokDefaultAccessTokenTTL = 6 * time.Hour

type GrokOAuthService struct {
	sessionStore *xai.SessionStore
	proxyRepo    ProxyRepository
	oauthClient  GrokOAuthClient
	config       *config.Config
}

func NewGrokOAuthService(proxyRepo ProxyRepository, oauthClient GrokOAuthClient, configs ...*config.Config) *GrokOAuthService {
	service := &GrokOAuthService{
		sessionStore: xai.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
	}
	if len(configs) > 0 {
		service.config = configs[0]
	}
	return service
}

// WithSessionStore replaces the in-memory OAuth session store (e.g. Redis-backed
// for cross-instance single-use callbacks). Redis wiring stays in Wire providers
// so this service package does not import go-redis (depguard).
func (s *GrokOAuthService) WithSessionStore(store *xai.SessionStore) *GrokOAuthService {
	if s != nil && store != nil {
		if s.sessionStore != nil {
			s.sessionStore.Stop()
		}
		s.sessionStore = store
	}
	return s
}

type GrokOAuthCapabilities struct {
	PasswordAuthEnabled bool `json:"password_auth_enabled"`
}

func (s *GrokOAuthService) GetCapabilities() GrokOAuthCapabilities {
	return GrokOAuthCapabilities{PasswordAuthEnabled: s.passwordAuthEnabled()}
}

func (s *GrokOAuthService) passwordAuthEnabled() bool {
	return s.config != nil && s.config.Gateway.Grok.PasswordAuthEnabled
}

type GrokAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

func (s *GrokOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI string) (*GrokAuthURLResult, error) {
	state, err := xai.GenerateState()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_STATE_FAILED", "failed to generate state: %v", err)
	}
	nonce, err := xai.GenerateNonce()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_NONCE_FAILED", "failed to generate nonce: %v", err)
	}
	codeVerifier, err := xai.GenerateCodeVerifier()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_VERIFIER_FAILED", "failed to generate code verifier: %v", err)
	}
	sessionID, err := xai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}

	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	redirectURI = xai.EffectiveRedirectURI(redirectURI)
	codeChallenge := xai.GenerateCodeChallenge(codeVerifier)

	authURL, err := xai.BuildAuthorizationURL(state, codeChallenge, redirectURI, nonce)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "GROK_OAUTH_INVALID_AUTHORIZE_URL", "%v", err)
	}

	s.sessionStore.Set(sessionID, &xai.OAuthSession{
		State:         state,
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
		ClientID:      xai.EffectiveClientID(),
		Scope:         xai.EffectiveScope(),
		ProxyURL:      proxyURL,
		RedirectURI:   redirectURI,
		CreatedAt:     time.Now(),
	})

	return &GrokAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
		State:     state,
	}, nil
}

type GrokExchangeCodeInput struct {
	SessionID   string
	Code        string
	State       string
	RedirectURI string
	ProxyID     *int64
}

type GrokTokenInfo struct {
	AccessToken       string `json:"access_token"`
	RefreshToken      string `json:"refresh_token,omitempty"`
	IDToken           string `json:"id_token,omitempty"`
	TokenType         string `json:"token_type,omitempty"`
	ExpiresIn         int64  `json:"expires_in"`
	ExpiresAt         int64  `json:"expires_at"`
	ClientID          string `json:"client_id,omitempty"`
	Scope             string `json:"scope,omitempty"`
	Email             string `json:"email,omitempty"`
	Subject           string `json:"sub,omitempty"`
	TeamID            string `json:"team_id,omitempty"`
	SubscriptionTier  string `json:"subscription_tier,omitempty"`
	EntitlementStatus string `json:"entitlement_status,omitempty"`
}

// GrokPasswordLoginResult is an ephemeral password-login outcome.
// SSOToken is never persisted and must only feed ConvertSSOToBuild.
type GrokPasswordLoginResult struct {
	Email    string `json:"email,omitempty"`
	SSOToken string `json:"sso_token"`
}

func (s *GrokOAuthService) ExchangeCode(ctx context.Context, input *GrokExchangeCodeInput) (*GrokTokenInfo, error) {
	if input == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_INPUT", "input is required")
	}
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}

	parsed := xai.ParseAuthorizationInput(input.Code)
	code := strings.TrimSpace(parsed.Code)
	if code == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_CODE_REQUIRED", "authorization code is required")
	}
	state := strings.TrimSpace(input.State)
	if state == "" {
		state = strings.TrimSpace(parsed.State)
	}
	if state == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_STATE_REQUIRED", "oauth state is required")
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_STATE", "invalid oauth state")
	}
	if redirectURI := strings.TrimSpace(input.RedirectURI); redirectURI != "" &&
		redirectURI != strings.TrimSpace(session.RedirectURI) {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_REDIRECT_URI_MISMATCH", "redirect_uri does not match the OAuth session")
	}

	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		var err error
		proxyURL, err = s.proxyURL(ctx, input.ProxyID)
		if err != nil {
			return nil, err
		}
	}
	if err := s.requireOAuthClient(); err != nil {
		return nil, err
	}
	if !s.sessionStore.TryConsumeSession(input.SessionID) {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_SESSION_ALREADY_USED", "oauth session has already been used")
	}
	defer s.sessionStore.Delete(input.SessionID)
	tokenResp, err := s.oauthClient.ExchangeCode(ctx, code, session.CodeVerifier, session.RedirectURI, proxyURL, session.ClientID)
	if err != nil {
		return nil, err
	}
	if err := validateGrokTokenResponse(tokenResp); err != nil {
		return nil, err
	}
	return s.tokenInfoFromResponse(tokenResp, session.ClientID, nil), nil
}

func (s *GrokOAuthService) requireOAuthClient() error {
	if s == nil || s.oauthClient == nil {
		return infraerrors.New(http.StatusInternalServerError, "GROK_OAUTH_CLIENT_NOT_CONFIGURED", "oauth client is not configured")
	}
	return nil
}

func (s *GrokOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL, clientID string) (*GrokTokenInfo, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_NO_REFRESH_TOKEN", "refresh_token is required")
	}
	if err := s.requireOAuthClient(); err != nil {
		return nil, err
	}
	tokenResp, err := s.oauthClient.RefreshToken(ctx, refreshToken, proxyURL, clientID)
	if err != nil {
		return nil, err
	}
	if err := validateGrokTokenResponse(tokenResp); err != nil {
		return nil, err
	}
	tokenInfo := s.tokenInfoFromResponse(tokenResp, clientID, nil)
	if tokenInfo.RefreshToken == "" {
		tokenInfo.RefreshToken = refreshToken
	}
	return tokenInfo, nil
}

func (s *GrokOAuthService) ValidateRefreshToken(ctx context.Context, refreshToken string, proxyID *int64) (*GrokTokenInfo, error) {
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	return s.RefreshToken(ctx, refreshToken, proxyURL, xai.EffectiveClientID())
}

// ValidateSSOToken converts a Web SSO cookie into Build OAuth tokens.
// The raw sso_token is never stored on GrokTokenInfo or account credentials.
func (s *GrokOAuthService) ValidateSSOToken(ctx context.Context, ssoToken string, proxyID *int64) (*GrokTokenInfo, error) {
	ssoToken = strings.TrimSpace(ssoToken)
	if ssoToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_NO_SSO_TOKEN", "sso_token is required")
	}
	if err := s.requireOAuthClient(); err != nil {
		return nil, err
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	tokenResp, err := s.oauthClient.ConvertSSOToBuild(ctx, ssoToken, proxyURL)
	if err != nil {
		return nil, err
	}
	if err := validateGrokTokenResponse(tokenResp); err != nil {
		return nil, err
	}
	return s.tokenInfoFromResponse(tokenResp, xai.DefaultClientID, nil), nil
}

// ConvertFromSSO is the batch-import entry point; same semantics as ValidateSSOToken.
func (s *GrokOAuthService) ConvertFromSSO(ctx context.Context, ssoToken string, proxyID *int64) (*GrokTokenInfo, error) {
	return s.ValidateSSOToken(ctx, ssoToken, proxyID)
}

// AuthorizePassword logs in with email/password, converts the resulting SSO cookie
// to Build OAuth, and returns OAuth tokens only. Password and raw SSO are never persisted.
func (s *GrokOAuthService) AuthorizePassword(ctx context.Context, email, password string, proxyID *int64) (*GrokTokenInfo, error) {
	if !s.passwordAuthEnabled() {
		return nil, infraerrors.New(http.StatusForbidden, "GROK_OAUTH_PASSWORD_AUTH_DISABLED", "Grok password authorization is disabled")
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_EMAIL_REQUIRED", "email is required")
	}
	if strings.TrimSpace(password) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_PASSWORD_REQUIRED", "password is required")
	}
	if err := s.requireOAuthClient(); err != nil {
		return nil, err
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	loginResult, err := s.oauthClient.LoginWithPassword(ctx, email, password, proxyURL)
	if err != nil {
		return nil, err
	}
	if loginResult == nil || strings.TrimSpace(loginResult.SSOToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_PASSWORD_LOGIN_FAILED", "grok password login did not return sso_token")
	}
	info, err := s.ValidateSSOToken(ctx, loginResult.SSOToken, proxyID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(info.Email) == "" {
		info.Email = loginResult.Email
	}
	return info, nil
}

func validateGrokTokenResponse(tokenResp *xai.TokenResponse) error {
	if tokenResp == nil || strings.TrimSpace(tokenResp.AccessToken) == "" {
		return infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_INVALID_TOKEN_RESPONSE", "grok oauth token response missing access_token")
	}
	return nil
}

func (s *GrokOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*GrokTokenInfo, error) {
	if account == nil || account.Platform != PlatformGrok {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_ACCOUNT", "account is not a Grok account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
	}

	proxyURL, err := s.proxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	refreshToken := account.GetCredential("refresh_token")
	if strings.TrimSpace(refreshToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
	}

	clientID := account.GetCredential("client_id")
	tokenInfo, err := s.RefreshToken(ctx, refreshToken, proxyURL, clientID)
	if err != nil {
		return nil, err
	}
	tokenInfo.SubscriptionTier = account.GetCredential("subscription_tier")
	tokenInfo.EntitlementStatus = account.GetCredential("entitlement_status")
	return tokenInfo, nil
}

func (s *GrokOAuthService) BuildAccountCredentials(tokenInfo *GrokTokenInfo) map[string]any {
	if tokenInfo == nil {
		return nil
	}
	expiresAt := time.Unix(tokenInfo.ExpiresAt, 0).UTC().Format(time.RFC3339)
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
		"expires_at":   expiresAt,
	}
	if tokenInfo.RefreshToken != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}
	if tokenInfo.TokenType != "" {
		creds["token_type"] = tokenInfo.TokenType
	}
	if tokenInfo.IDToken != "" {
		creds["id_token"] = tokenInfo.IDToken
	}
	if tokenInfo.ClientID != "" {
		creds["client_id"] = tokenInfo.ClientID
	}
	if tokenInfo.Scope != "" {
		creds["scope"] = tokenInfo.Scope
	}
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
	}
	if tokenInfo.Subject != "" {
		creds["sub"] = tokenInfo.Subject
	}
	if tokenInfo.TeamID != "" {
		creds["team_id"] = tokenInfo.TeamID
	}
	if tokenInfo.SubscriptionTier != "" {
		creds["subscription_tier"] = tokenInfo.SubscriptionTier
	}
	if tokenInfo.EntitlementStatus != "" {
		creds["entitlement_status"] = tokenInfo.EntitlementStatus
	}
	creds["base_url"] = xai.DefaultCLIBaseURL
	return creds
}

func (s *GrokOAuthService) Stop() {
	s.sessionStore.Stop()
}

func (s *GrokOAuthService) tokenInfoFromResponse(tokenResp *xai.TokenResponse, clientID string, existing map[string]any) *GrokTokenInfo {
	now := time.Now()
	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64(grokDefaultAccessTokenTTL.Seconds())
	}
	info := &GrokTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    expiresIn,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second).Unix(),
		ClientID:     strings.TrimSpace(clientID),
		Scope:        tokenResp.Scope,
	}
	if info.ClientID == "" {
		info.ClientID = xai.EffectiveClientID()
	}
	if info.TokenType == "" {
		info.TokenType = "Bearer"
	}
	applyGrokTokenClaims(info, tokenResp.IDToken)
	applyGrokTokenClaims(info, tokenResp.AccessToken)
	if existing != nil {
		if info.Email == "" {
			if email, _ := existing["email"].(string); email != "" {
				info.Email = email
			}
		}
		if info.Subject == "" {
			if subject, _ := existing["sub"].(string); subject != "" {
				info.Subject = subject
			}
		}
		if info.TeamID == "" {
			if teamID, _ := existing["team_id"].(string); teamID != "" {
				info.TeamID = teamID
			}
		}
	}
	return info
}

func (s *GrokOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		if errors.Is(err, ErrProxyNotFound) {
			return "", infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_PROXY_NOT_FOUND", "configured proxy was not found")
		}
		return "", infraerrors.New(http.StatusServiceUnavailable, "GROK_OAUTH_PROXY_LOOKUP_FAILED", "proxy lookup is temporarily unavailable")
	}
	if proxy == nil {
		return "", infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_PROXY_NOT_FOUND", "configured proxy was not found")
	}
	return proxy.URL(), nil
}

func applyGrokTokenClaims(info *GrokTokenInfo, token string) {
	if info == nil || strings.TrimSpace(token) == "" {
		return
	}
	claims := xai.DecodeJWTClaims(token)
	if claims == nil {
		return
	}
	if info.Email == "" {
		info.Email = xai.JWTClaimString(claims, "email")
	}
	if info.Subject == "" {
		info.Subject = xai.JWTClaimString(claims, "sub")
	}
	if info.TeamID == "" {
		info.TeamID = xai.JWTClaimString(claims, "team_id")
	}
}
