package service

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

type XAIOAuthService struct {
	sessionStore *xai.SessionStore
	proxyRepo    ProxyRepository
}

func NewXAIOAuthService(proxyRepo ProxyRepository) *XAIOAuthService {
	return &XAIOAuthService{
		sessionStore: xai.NewSessionStore(),
		proxyRepo:    proxyRepo,
	}
}

type XAIAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

func (s *XAIOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI string) (*XAIAuthURLResult, error) {
	state, err := xai.GenerateState()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "XAI_OAUTH_STATE_FAILED", "failed to generate state: %v", err)
	}
	nonce, err := xai.GenerateNonce()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "XAI_OAUTH_NONCE_FAILED", "failed to generate nonce: %v", err)
	}
	codeVerifier, err := xai.GenerateCodeVerifier()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "XAI_OAUTH_VERIFIER_FAILED", "failed to generate code verifier: %v", err)
	}
	sessionID, err := xai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "XAI_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}

	proxyURL, err := s.resolveProxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(redirectURI) == "" {
		redirectURI = xai.DefaultRedirectURI
	}

	client, err := xai.NewClient(proxyURL)
	if err != nil {
		return nil, err
	}
	doc, err := client.Discover(ctx)
	if err != nil {
		return nil, err
	}

	session := &xai.OAuthSession{
		State:         state,
		Nonce:         nonce,
		CodeVerifier:  codeVerifier,
		RedirectURI:   redirectURI,
		TokenEndpoint: doc.TokenEndpoint,
		ProxyURL:      proxyURL,
		CreatedAt:     time.Now(),
	}
	s.sessionStore.Set(sessionID, session)

	authURL, err := xai.BuildAuthorizeURL(xai.AuthorizeParams{
		AuthorizationEndpoint: doc.AuthorizationEndpoint,
		State:                 state,
		Nonce:                 nonce,
		CodeChallenge:         xai.GenerateCodeChallenge(codeVerifier),
		RedirectURI:           redirectURI,
	})
	if err != nil {
		return nil, err
	}

	return &XAIAuthURLResult{AuthURL: authURL, SessionID: sessionID, State: state}, nil
}

type XAIExchangeCodeInput struct {
	SessionID   string
	State       string
	Code        string
	RedirectURI string
	ProxyID     *int64
}

type XAITokenInfo struct {
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	IDToken       string `json:"id_token,omitempty"`
	TokenType     string `json:"token_type,omitempty"`
	ExpiresIn     int64  `json:"expires_in,omitempty"`
	ExpiresAt     int64  `json:"expires_at"`
	Email         string `json:"email,omitempty"`
	Subject       string `json:"sub,omitempty"`
	BaseURL       string `json:"base_url,omitempty"`
	RedirectURI   string `json:"redirect_uri,omitempty"`
	TokenEndpoint string `json:"token_endpoint,omitempty"`
}

func (s *XAIOAuthService) ExchangeCode(ctx context.Context, input *XAIExchangeCodeInput) (*XAITokenInfo, error) {
	if input == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "XAI_OAUTH_INVALID_REQUEST", "request is empty")
	}
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "XAI_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}
	if strings.TrimSpace(input.State) == "" || subtle.ConstantTimeCompare([]byte(input.State), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "XAI_OAUTH_INVALID_STATE", "invalid oauth state")
	}

	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		var err error
		proxyURL, err = s.resolveProxyURL(ctx, input.ProxyID)
		if err != nil {
			return nil, err
		}
	}
	redirectURI := session.RedirectURI
	if strings.TrimSpace(input.RedirectURI) != "" {
		redirectURI = strings.TrimSpace(input.RedirectURI)
	}

	client, err := xai.NewClient(proxyURL)
	if err != nil {
		return nil, err
	}
	tokenResp, err := client.ExchangeCodeForTokens(ctx, input.Code, redirectURI, session.CodeVerifier, session.TokenEndpoint)
	if err != nil {
		return nil, err
	}
	s.sessionStore.Delete(input.SessionID)
	return s.buildTokenInfo(tokenResp, redirectURI, session.TokenEndpoint, nil), nil
}

func (s *XAIOAuthService) RefreshToken(ctx context.Context, refreshToken, tokenEndpoint, proxyURL string) (*XAITokenInfo, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "XAI_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
	}
	client, err := xai.NewClient(proxyURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(tokenEndpoint) == "" {
		doc, err := client.Discover(ctx)
		if err != nil {
			return nil, err
		}
		tokenEndpoint = doc.TokenEndpoint
	}
	tokenResp, err := client.RefreshTokens(ctx, refreshToken, tokenEndpoint)
	if err != nil {
		return nil, err
	}
	return s.buildTokenInfo(tokenResp, "", tokenEndpoint, nil), nil
}

func (s *XAIOAuthService) ValidateRefreshToken(ctx context.Context, refreshToken string, proxyID *int64) (*XAITokenInfo, error) {
	proxyURL, err := s.resolveProxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	return s.RefreshToken(ctx, refreshToken, "", proxyURL)
}

func (s *XAIOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*XAITokenInfo, error) {
	if account == nil || account.Platform != PlatformXAI {
		return nil, infraerrors.New(http.StatusBadRequest, "XAI_OAUTH_INVALID_ACCOUNT", "account is not an xAI account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "XAI_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
	}

	refreshToken := account.GetXAIRefreshToken()
	if strings.TrimSpace(refreshToken) == "" {
		accessToken := account.GetXAIAccessToken()
		if strings.TrimSpace(accessToken) == "" {
			return nil, infraerrors.New(http.StatusBadRequest, "XAI_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
		}
		tokenInfo := &XAITokenInfo{
			AccessToken:   accessToken,
			RefreshToken:  "",
			IDToken:       account.GetCredential("id_token"),
			TokenType:     account.GetCredential("token_type"),
			Email:         account.GetCredential("email"),
			Subject:       account.GetCredential("sub"),
			BaseURL:       account.GetXAIBaseURL(),
			RedirectURI:   account.GetCredential("redirect_uri"),
			TokenEndpoint: account.GetCredential("token_endpoint"),
		}
		if expiresAt := account.GetCredentialAsTime("expires_at"); expiresAt != nil {
			tokenInfo.ExpiresAt = expiresAt.Unix()
			tokenInfo.ExpiresIn = int64(time.Until(*expiresAt).Seconds())
		}
		return tokenInfo, nil
	}

	proxyURL, err := s.resolveAccountProxyURL(ctx, account)
	if err != nil {
		return nil, err
	}
	tokenInfo, err := s.RefreshToken(ctx, refreshToken, account.GetCredential("token_endpoint"), proxyURL)
	if err != nil {
		return nil, err
	}
	tokenInfo.BaseURL = account.GetXAIBaseURL()
	tokenInfo.RedirectURI = account.GetCredential("redirect_uri")
	if tokenInfo.Email == "" {
		tokenInfo.Email = account.GetCredential("email")
	}
	if tokenInfo.Subject == "" {
		tokenInfo.Subject = account.GetCredential("sub")
	}
	return tokenInfo, nil
}

func (s *XAIOAuthService) BuildAccountCredentials(tokenInfo *XAITokenInfo) map[string]any {
	if tokenInfo == nil {
		return map[string]any{}
	}
	expiresAt := time.Unix(tokenInfo.ExpiresAt, 0).Format(time.RFC3339)
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
		"expires_at":   expiresAt,
		"auth_kind":    "oauth",
		"type":         "xai",
	}
	if strings.TrimSpace(tokenInfo.RefreshToken) != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}
	if tokenInfo.IDToken != "" {
		creds["id_token"] = tokenInfo.IDToken
	}
	if tokenInfo.TokenType != "" {
		creds["token_type"] = tokenInfo.TokenType
	}
	if tokenInfo.ExpiresIn > 0 {
		creds["expires_in"] = tokenInfo.ExpiresIn
	}
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
	}
	if tokenInfo.Subject != "" {
		creds["sub"] = tokenInfo.Subject
	}
	if tokenInfo.BaseURL != "" {
		creds["base_url"] = tokenInfo.BaseURL
	}
	if tokenInfo.RedirectURI != "" {
		creds["redirect_uri"] = tokenInfo.RedirectURI
	}
	if tokenInfo.TokenEndpoint != "" {
		creds["token_endpoint"] = tokenInfo.TokenEndpoint
	}
	return creds
}

func (s *XAIOAuthService) buildTokenInfo(tokenResp *xai.TokenResponse, redirectURI string, tokenEndpoint string, account *Account) *XAITokenInfo {
	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	expiresAt := time.Now().Unix() + expiresIn
	identity := xai.ParseJWTIdentity(tokenResp.IDToken)
	if identity.Email == "" || identity.Sub == "" {
		accessIdentity := xai.ParseJWTIdentity(tokenResp.AccessToken)
		if identity.Email == "" {
			identity.Email = accessIdentity.Email
		}
		if identity.Sub == "" {
			identity.Sub = accessIdentity.Sub
		}
	}
	info := &XAITokenInfo{
		AccessToken:   tokenResp.AccessToken,
		RefreshToken:  tokenResp.RefreshToken,
		IDToken:       tokenResp.IDToken,
		TokenType:     tokenResp.TokenType,
		ExpiresIn:     expiresIn,
		ExpiresAt:     expiresAt,
		Email:         identity.Email,
		Subject:       identity.Sub,
		BaseURL:       xai.DefaultAPIBaseURL,
		RedirectURI:   redirectURI,
		TokenEndpoint: tokenEndpoint,
	}
	if account != nil {
		info.BaseURL = account.GetXAIBaseURL()
		if info.RedirectURI == "" {
			info.RedirectURI = account.GetCredential("redirect_uri")
		}
	}
	return info
}

func (s *XAIOAuthService) resolveProxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "XAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}

func (s *XAIOAuthService) resolveAccountProxyURL(ctx context.Context, account *Account) (string, error) {
	if account == nil || account.ProxyID == nil {
		return "", nil
	}
	if account.Proxy != nil {
		return account.Proxy.URL(), nil
	}
	return s.resolveProxyURL(ctx, account.ProxyID)
}

func (s *XAIOAuthService) Stop() {
	if s != nil && s.sessionStore != nil {
		s.sessionStore.Stop()
	}
}
