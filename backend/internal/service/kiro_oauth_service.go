package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

const (
	// Kiro desktop social auth uses localhost loopback callbacks from a fixed
	// allowlist. Use one of the bundled ports from the official client.
	kiroSocialRedirectURI = "http://localhost:49153"
	// AWS IAM Identity Center native/public clients require an explicit loopback IP redirect URI.
	kiroIDCRedirectURI = "http://127.0.0.1:9876/oauth/callback"
)

type KiroOAuthService struct {
	sessionStore *kiropkg.SessionStore
	proxyRepo    ProxyRepository
}

func NewKiroOAuthService(proxyRepo ProxyRepository) *KiroOAuthService {
	return &KiroOAuthService{
		sessionStore: kiropkg.NewSessionStore(),
		proxyRepo:    proxyRepo,
	}
}

func (s *KiroOAuthService) Stop() {}

type KiroAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

type KiroIDCAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	ClientID  string `json:"client_id"`
	Region    string `json:"region"`
	StartURL  string `json:"start_url"`
}

type KiroTokenInfo struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ProfileArn   string `json:"profile_arn,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	AuthMethod   string `json:"auth_method,omitempty"`
	Provider     string `json:"provider,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	ClientIDHash string `json:"client_id_hash,omitempty"`
	Email        string `json:"email,omitempty"`
	StartURL     string `json:"start_url,omitempty"`
	Region       string `json:"region,omitempty"`
}

type KiroGenerateAuthURLInput struct {
	ProxyID  *int64
	Provider string
}

type KiroExchangeCodeInput struct {
	SessionID    string
	State        string
	Code         string
	CallbackPath string
	LoginOption  string
	ProxyID      *int64
}

type KiroGenerateIDCAuthURLInput struct {
	ProxyID  *int64
	StartURL string
	Region   string
}

type KiroRefreshTokenInput struct {
	RefreshToken string
	AuthMethod   string
	Provider     string
	ClientID     string
	ClientSecret string
	StartURL     string
	Region       string
	ProfileArn   string
	ProxyID      *int64
}

type KiroImportTokenInput struct {
	TokenJSON              string
	DeviceRegistrationJSON string
}

func (s *KiroOAuthService) GenerateAuthURL(ctx context.Context, input *KiroGenerateAuthURLInput) (*KiroAuthURLResult, error) {
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = string(kiropkg.SocialProviderGoogle)
	}
	if provider != string(kiropkg.SocialProviderGoogle) && provider != string(kiropkg.SocialProviderGitHub) {
		return nil, fmt.Errorf("unsupported kiro social provider: %s", provider)
	}
	state, err := kiropkg.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("generate state failed: %w", err)
	}
	codeVerifier, err := kiropkg.GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate code verifier failed: %w", err)
	}
	sessionID := kiropkg.GenerateSessionID()
	proxyURL, _ := s.resolveProxyURL(ctx, input.ProxyID)
	s.sessionStore.Set(sessionID, &kiropkg.AuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		ProxyURL:     proxyURL,
		CreatedAt:    time.Now(),
		AuthType:     "social",
		Provider:     provider,
		RedirectURI:  kiroSocialRedirectURI,
	})
	return &KiroAuthURLResult{
		AuthURL:   kiropkg.BuildSocialSignInURL(kiroSocialRedirectURI, kiropkg.GenerateCodeChallenge(codeVerifier), state),
		SessionID: sessionID,
		State:     state,
	}, nil
}

func (s *KiroOAuthService) ExchangeCode(ctx context.Context, input *KiroExchangeCodeInput) (*KiroTokenInfo, error) {
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, fmt.Errorf("session not found or expired")
	}
	if strings.TrimSpace(input.State) == "" || input.State != session.State {
		return nil, fmt.Errorf("state invalid")
	}
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxyURL, _ = s.resolveProxyURL(ctx, input.ProxyID)
	}

	switch session.AuthType {
	case "social":
		token, err := kiropkg.CreateSocialToken(
			ctx,
			proxyURL,
			input.Code,
			session.CodeVerifier,
			buildKiroSocialExchangeRedirectURI(session.RedirectURI, session.Provider, input.CallbackPath, input.LoginOption),
		)
		if err != nil {
			return nil, err
		}
		token.Provider = session.Provider
		s.sessionStore.Delete(input.SessionID)
		return toKiroTokenInfo(token), nil
	case "idc":
		token, err := kiropkg.ExchangeIDCAuthCode(ctx, proxyURL, session.ClientID, session.ClientSecret, input.Code, session.CodeVerifier, session.RedirectURI, session.Region, session.StartURL)
		if err != nil {
			return nil, err
		}
		s.sessionStore.Delete(input.SessionID)
		return toKiroTokenInfo(token), nil
	default:
		return nil, fmt.Errorf("unsupported auth session type: %s", session.AuthType)
	}
}

func buildKiroSocialExchangeRedirectURI(baseRedirectURI, provider, callbackPath, loginOption string) string {
	option := strings.ToLower(strings.TrimSpace(loginOption))
	if option == "" {
		switch provider {
		case string(kiropkg.SocialProviderGitHub):
			option = "github"
		case string(kiropkg.SocialProviderGoogle):
			option = "google"
		}
	}
	return kiropkg.BuildSocialTokenRedirectURI(baseRedirectURI, callbackPath, option)
}

func (s *KiroOAuthService) GenerateIDCAuthURL(ctx context.Context, input *KiroGenerateIDCAuthURLInput) (*KiroIDCAuthURLResult, error) {
	startURL := strings.TrimSpace(input.StartURL)
	if startURL == "" {
		startURL = kiropkg.BuilderIDStartURL
	}
	region := strings.TrimSpace(input.Region)
	if region == "" {
		region = "us-east-1"
	}
	state, err := kiropkg.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("generate state failed: %w", err)
	}
	codeVerifier, err := kiropkg.GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate code verifier failed: %w", err)
	}
	proxyURL, _ := s.resolveProxyURL(ctx, input.ProxyID)
	reg, err := kiropkg.RegisterIDCClient(ctx, proxyURL, kiroIDCRedirectURI, startURL, region)
	if err != nil {
		return nil, err
	}
	sessionID := kiropkg.GenerateSessionID()
	s.sessionStore.Set(sessionID, &kiropkg.AuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		ProxyURL:     proxyURL,
		CreatedAt:    time.Now(),
		AuthType:     "idc",
		RedirectURI:  kiroIDCRedirectURI,
		ClientID:     reg.ClientID,
		ClientSecret: reg.ClientSecret,
		Region:       region,
		StartURL:     startURL,
	})
	return &KiroIDCAuthURLResult{
		AuthURL:   kiropkg.BuildIDCAuthURL(reg.ClientID, kiroIDCRedirectURI, state, kiropkg.GenerateCodeChallenge(codeVerifier), region),
		SessionID: sessionID,
		State:     state,
		ClientID:  reg.ClientID,
		Region:    region,
		StartURL:  startURL,
	}, nil
}

func (s *KiroOAuthService) RefreshToken(ctx context.Context, input *KiroRefreshTokenInput) (*KiroTokenInfo, error) {
	proxyURL, _ := s.resolveProxyURL(ctx, input.ProxyID)
	authMethod := strings.ToLower(strings.TrimSpace(input.AuthMethod))
	if authMethod == "" {
		authMethod = "social"
	}

	var token *kiropkg.TokenData
	var err error
	switch authMethod {
	case "idc":
		token, err = kiropkg.RefreshIDCToken(ctx, proxyURL, input.ClientID, input.ClientSecret, input.RefreshToken, input.Region, input.StartURL)
	default:
		token, err = kiropkg.RefreshSocialToken(ctx, proxyURL, input.RefreshToken, input.Provider)
	}
	if err != nil {
		return nil, err
	}
	if token.ProfileArn == "" {
		token.ProfileArn = input.ProfileArn
	}
	if token.ClientID == "" {
		token.ClientID = input.ClientID
	}
	if token.ClientSecret == "" {
		token.ClientSecret = input.ClientSecret
	}
	if token.StartURL == "" {
		token.StartURL = input.StartURL
	}
	if token.Region == "" {
		token.Region = input.Region
	}
	return toKiroTokenInfo(token), nil
}

func (s *KiroOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*KiroTokenInfo, error) {
	if account.Platform != PlatformKiro || account.Type != AccountTypeOAuth {
		return nil, fmt.Errorf("not a kiro oauth account")
	}
	return s.RefreshToken(ctx, &KiroRefreshTokenInput{
		RefreshToken: account.GetCredential("refresh_token"),
		AuthMethod:   account.GetCredential("auth_method"),
		Provider:     account.GetCredential("provider"),
		ClientID:     account.GetCredential("client_id"),
		ClientSecret: account.GetCredential("client_secret"),
		StartURL:     account.GetCredential("start_url"),
		Region:       account.GetCredential("region"),
		ProfileArn:   account.GetCredential("profile_arn"),
		ProxyID:      account.ProxyID,
	})
}

func (s *KiroOAuthService) ImportToken(input *KiroImportTokenInput) (*KiroTokenInfo, error) {
	token, err := kiropkg.ParseImportedToken(input.TokenJSON, input.DeviceRegistrationJSON)
	if err != nil {
		return nil, err
	}
	return toKiroTokenInfo(token), nil
}

func (s *KiroOAuthService) BuildAccountCredentials(tokenInfo *KiroTokenInfo) map[string]any {
	if tokenInfo == nil {
		return map[string]any{}
	}

	creds := map[string]any{}
	if tokenInfo.AccessToken != "" {
		creds["access_token"] = tokenInfo.AccessToken
	}
	if tokenInfo.RefreshToken != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}
	if tokenInfo.ProfileArn != "" {
		creds["profile_arn"] = tokenInfo.ProfileArn
	}
	if tokenInfo.ExpiresAt != "" {
		creds["expires_at"] = tokenInfo.ExpiresAt
	}
	if tokenInfo.AuthMethod != "" {
		creds["auth_method"] = tokenInfo.AuthMethod
	}
	if tokenInfo.Provider != "" {
		creds["provider"] = tokenInfo.Provider
	}
	if tokenInfo.ClientID != "" {
		creds["client_id"] = tokenInfo.ClientID
	}
	if tokenInfo.ClientSecret != "" {
		creds["client_secret"] = tokenInfo.ClientSecret
	}
	if tokenInfo.ClientIDHash != "" {
		creds["client_id_hash"] = tokenInfo.ClientIDHash
	}
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
	}
	if tokenInfo.StartURL != "" {
		creds["start_url"] = tokenInfo.StartURL
	}
	if tokenInfo.Region != "" {
		creds["region"] = tokenInfo.Region
	}

	return creds
}

func toKiroTokenInfo(token *kiropkg.TokenData) *KiroTokenInfo {
	if token == nil {
		return nil
	}
	return &KiroTokenInfo{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ProfileArn:   token.ProfileArn,
		ExpiresAt:    token.ExpiresAt,
		AuthMethod:   token.AuthMethod,
		Provider:     token.Provider,
		ClientID:     token.ClientID,
		ClientSecret: token.ClientSecret,
		ClientIDHash: token.ClientIDHash,
		Email:        token.Email,
		StartURL:     token.StartURL,
		Region:       token.Region,
	}
}

func (s *KiroOAuthService) resolveProxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil || s.proxyRepo == nil {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil || proxy == nil {
		return "", err
	}
	return proxy.URL(), nil
}
