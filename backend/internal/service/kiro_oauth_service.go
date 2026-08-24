package service

import (
	"context"
	"fmt"
	"net/url"
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
	// External IdP(Microsoft Entra ID)第二阶段直连 Azure authorize/token 端点,
	// redirect_uri 必须与 Kiro 官方 Azure 企业应用注册的白名单逐字符匹配。
	// 官方登录流(kiro-login-helper)使用 3128 端口,不能复用社交登录的 49153——
	// Azure 白名单与 app.kiro.dev 门户白名单相互独立,端口不符会被 Entra 拒绝(AADSTS50011)。
	kiroExternalIdpRedirectURI = "http://localhost:3128/oauth/callback"
)

var kiroDiscoverExternalIdp = func(ctx context.Context, proxyURL, issuerURL string) (string, string, error) {
	discovery, err := kiropkg.DiscoverExternalIdp(ctx, proxyURL, issuerURL)
	if err != nil {
		return "", "", err
	}
	return discovery.AuthorizationEndpoint, discovery.TokenEndpoint, nil
}

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
	AuthURL       string `json:"auth_url,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	State         string `json:"state,omitempty"`
	AccessToken   string `json:"access_token,omitempty"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	ProfileArn    string `json:"profile_arn,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	AuthMethod    string `json:"auth_method,omitempty"`
	Provider      string `json:"provider,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	ClientSecret  string `json:"client_secret,omitempty"`
	ClientIDHash  string `json:"client_id_hash,omitempty"`
	Email         string `json:"email,omitempty"`
	StartURL      string `json:"start_url,omitempty"`
	Region        string `json:"region,omitempty"`
	TokenEndpoint string `json:"token_endpoint,omitempty"`
	IssuerURL     string `json:"issuer_url,omitempty"`
	Scopes        string `json:"scopes,omitempty"`
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
	RefreshToken  string
	AuthMethod    string
	Provider      string
	ClientID      string
	ClientSecret  string
	StartURL      string
	Region        string
	ProfileArn    string
	TokenEndpoint string
	IssuerURL     string
	Scopes        string
	ProxyID       *int64
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
	if provider != string(kiropkg.SocialProviderGoogle) && provider != string(kiropkg.SocialProviderGitHub) && provider != kiropkg.ProviderExternalIdp {
		return nil, fmt.Errorf("unsupported kiro oauth provider: %s", provider)
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
	authType := "social"
	if provider == kiropkg.ProviderExternalIdp {
		authType = "external_idp"
	}
	s.sessionStore.Set(sessionID, &kiropkg.AuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		ProxyURL:     proxyURL,
		CreatedAt:    time.Now(),
		AuthType:     authType,
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
	case "external_idp":
		if external, ok := parseKiroExternalIdpDescriptor(input.Code); ok {
			return s.prepareExternalIdpAuthorization(ctx, proxyURL, input.SessionID, session, external)
		}
		if strings.TrimSpace(session.TokenEndpoint) == "" || strings.TrimSpace(session.ClientID) == "" {
			return nil, fmt.Errorf("kiro external_idp callback descriptor is required")
		}
		token, err := kiropkg.ExchangeExternalIdpAuthCode(ctx, proxyURL, session.TokenEndpoint, session.ClientID, input.Code, session.CodeVerifier, session.RedirectURI, session.Scopes, session.IssuerURL)
		if err != nil {
			return nil, err
		}
		if token.ProfileArn == "" {
			account := &Account{
				Platform: PlatformKiro,
				Type:     AccountTypeOAuth,
				ProxyID:  input.ProxyID,
				Credentials: map[string]any{
					"auth_method": "external_idp",
					"provider":    kiropkg.ProviderExternalIdp,
					"client_id":   session.ClientID,
				},
			}
			if session.Region != "" {
				account.Credentials["api_region"] = session.Region
			}
			if arn := kiroResolveAndPersistProfileArn(ctx, nil, account, token.AccessToken); arn != "" {
				token.ProfileArn = arn
			}
		}
		token.Provider = kiropkg.ProviderExternalIdp
		s.sessionStore.Delete(input.SessionID)
		return toKiroTokenInfo(token), nil
	default:
		return nil, fmt.Errorf("unsupported auth session type: %s", session.AuthType)
	}
}

type kiroExternalIdpDescriptor struct {
	ClientID  string
	IssuerURL string
	Scopes    string
	LoginHint string
}

func parseKiroExternalIdpDescriptor(raw string) (*kiroExternalIdpDescriptor, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.RawQuery == "" {
		parsed, err = url.Parse("http://localhost/callback?" + strings.TrimPrefix(trimmed, "?"))
		if err != nil {
			return nil, false
		}
	}
	if strings.EqualFold(parsed.Path, "/oauth/callback") {
		return nil, false
	}
	q := parsed.Query()
	isExternal := strings.EqualFold(strings.TrimSpace(q.Get("login_option")), "external_idp") || strings.TrimSpace(q.Get("issuer_url")) != ""
	if !isExternal {
		return nil, false
	}
	return &kiroExternalIdpDescriptor{
		ClientID:  strings.TrimSpace(q.Get("client_id")),
		IssuerURL: strings.TrimSpace(q.Get("issuer_url")),
		Scopes:    strings.TrimSpace(q.Get("scopes")),
		LoginHint: strings.TrimSpace(q.Get("login_hint")),
	}, true
}

func (s *KiroOAuthService) prepareExternalIdpAuthorization(ctx context.Context, proxyURL, sessionID string, session *kiropkg.AuthSession, descriptor *kiroExternalIdpDescriptor) (*KiroTokenInfo, error) {
	clientID := strings.TrimSpace(descriptor.ClientID)
	issuerURL := strings.TrimSpace(descriptor.IssuerURL)
	if clientID == "" || issuerURL == "" {
		return nil, fmt.Errorf("kiro external_idp callback descriptor requires client_id and issuer_url")
	}
	authEndpoint, tokenEndpoint, err := kiroDiscoverExternalIdp(ctx, proxyURL, issuerURL)
	if err != nil {
		return nil, err
	}
	state, err := kiropkg.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("generate state failed: %w", err)
	}
	codeVerifier, err := kiropkg.GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate code verifier failed: %w", err)
	}
	// External IdP 第二阶段直连 Azure,redirect_uri 必须用官方 Azure 应用注册的端口(3128),
	// 不能复用社交登录门户的 49153 端口(session.RedirectURI),否则 Entra 拒绝授权请求。
	redirectURI := kiroExternalIdpRedirectURI
	session.State = state
	session.CodeVerifier = codeVerifier
	session.ProxyURL = proxyURL
	session.CreatedAt = time.Now()
	session.AuthType = "external_idp"
	session.Provider = kiropkg.ProviderExternalIdp
	session.RedirectURI = redirectURI
	session.ClientID = clientID
	session.TokenEndpoint = strings.TrimSpace(tokenEndpoint)
	session.IssuerURL = issuerURL
	session.Scopes = strings.TrimSpace(descriptor.Scopes)
	session.LoginHint = strings.TrimSpace(descriptor.LoginHint)
	s.sessionStore.Set(sessionID, session)
	return &KiroTokenInfo{
		AuthURL: kiropkg.BuildExternalIdpAuthURL(
			strings.TrimSpace(authEndpoint),
			clientID,
			redirectURI,
			session.Scopes,
			kiropkg.GenerateCodeChallenge(codeVerifier),
			state,
			session.LoginHint,
		),
		SessionID: sessionID,
		State:     state,
	}, nil
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
	refreshToken := strings.TrimSpace(input.RefreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("kiro refresh token is required")
	}
	authMethod := resolveKiroRefreshAuthMethod(input.AuthMethod, input.ClientID, input.ClientSecret)

	var token *kiropkg.TokenData
	var err error
	switch authMethod {
	case "external_idp":
		clientID := strings.TrimSpace(input.ClientID)
		tokenEndpoint := strings.TrimSpace(input.TokenEndpoint)
		if clientID == "" || tokenEndpoint == "" {
			return nil, fmt.Errorf("kiro external_idp refresh requires client_id and token_endpoint")
		}
		token, err = kiropkg.RefreshExternalIdpToken(ctx, proxyURL, clientID, refreshToken, tokenEndpoint, input.IssuerURL, input.Scopes)
	case "idc":
		clientID := strings.TrimSpace(input.ClientID)
		clientSecret := strings.TrimSpace(input.ClientSecret)
		if clientID == "" || clientSecret == "" {
			return nil, fmt.Errorf("kiro idc refresh requires client_id and client_secret")
		}
		token, err = kiropkg.RefreshIDCToken(ctx, proxyURL, clientID, clientSecret, refreshToken, input.Region, input.StartURL, input.Provider)
	default:
		token, err = kiropkg.RefreshSocialToken(ctx, proxyURL, refreshToken, input.Provider)
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
	if token.TokenEndpoint == "" {
		token.TokenEndpoint = input.TokenEndpoint
	}
	if token.IssuerURL == "" {
		token.IssuerURL = input.IssuerURL
	}
	if token.Scopes == "" {
		token.Scopes = input.Scopes
	}
	return toKiroTokenInfo(token), nil
}

func resolveKiroRefreshAuthMethod(authMethod, clientID, clientSecret string) string {
	method := strings.ToLower(strings.TrimSpace(authMethod))
	if method != "" {
		return method
	}
	if strings.TrimSpace(clientID) != "" && strings.TrimSpace(clientSecret) != "" {
		return "idc"
	}
	return "social"
}

func (s *KiroOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*KiroTokenInfo, error) {
	if account.Platform != PlatformKiro || account.Type != AccountTypeOAuth {
		return nil, fmt.Errorf("not a kiro oauth account")
	}
	return s.RefreshToken(ctx, &KiroRefreshTokenInput{
		RefreshToken:  account.GetCredential("refresh_token"),
		AuthMethod:    account.GetCredential("auth_method"),
		Provider:      account.GetCredential("provider"),
		ClientID:      account.GetCredential("client_id"),
		ClientSecret:  account.GetCredential("client_secret"),
		StartURL:      account.GetCredential("start_url"),
		Region:        account.GetCredential("region"),
		ProfileArn:    account.GetCredential("profile_arn"),
		TokenEndpoint: account.GetCredential("token_endpoint"),
		IssuerURL:     account.GetCredential("issuer_url"),
		Scopes:        account.GetCredential("scopes"),
		ProxyID:       account.ProxyID,
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
	if tokenInfo.TokenEndpoint != "" {
		creds["token_endpoint"] = tokenInfo.TokenEndpoint
	}
	if tokenInfo.IssuerURL != "" {
		creds["issuer_url"] = tokenInfo.IssuerURL
	}
	if tokenInfo.Scopes != "" {
		creds["scopes"] = tokenInfo.Scopes
	}

	return creds
}

func toKiroTokenInfo(token *kiropkg.TokenData) *KiroTokenInfo {
	if token == nil {
		return nil
	}
	return &KiroTokenInfo{
		AccessToken:   token.AccessToken,
		RefreshToken:  token.RefreshToken,
		ProfileArn:    token.ProfileArn,
		ExpiresAt:     token.ExpiresAt,
		AuthMethod:    token.AuthMethod,
		Provider:      token.Provider,
		ClientID:      token.ClientID,
		ClientSecret:  token.ClientSecret,
		ClientIDHash:  token.ClientIDHash,
		Email:         token.Email,
		StartURL:      token.StartURL,
		Region:        token.Region,
		TokenEndpoint: token.TokenEndpoint,
		IssuerURL:     token.IssuerURL,
		Scopes:        token.Scopes,
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
