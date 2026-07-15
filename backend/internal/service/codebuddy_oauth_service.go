package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/codebuddy"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const codeBuddyDefaultAccessTokenTTL = 24 * time.Hour

type CodeBuddyOAuthService struct {
	sessionStore *codebuddy.SessionStore
	proxyRepo    ProxyRepository
	oauthClient  CodeBuddyOAuthClient
}

func NewCodeBuddyOAuthService(proxyRepo ProxyRepository, oauthClient CodeBuddyOAuthClient) *CodeBuddyOAuthService {
	return &CodeBuddyOAuthService{
		sessionStore: codebuddy.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
	}
}

// CodeBuddyAuthURLResult 是 GenerateAuthURL 的返回。
type CodeBuddyAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

func (s *CodeBuddyOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI string) (*CodeBuddyAuthURLResult, error) {
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}

	stateResult, err := s.oauthClient.FetchState(ctx, proxyURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(stateResult.State) == "" || strings.TrimSpace(stateResult.AuthURL) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "CODEBUDDY_INVALID_STATE", "invalid auth state response")
	}

	sessionID, err := codebuddy.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CODEBUDDY_SESSION_FAILED", "failed to generate session ID: %v", err)
	}
	s.sessionStore.Set(sessionID, &codebuddy.OAuthSession{
		State:     stateResult.State,
		ProxyURL:  proxyURL,
		CreatedAt: time.Now(),
	})

	return &CodeBuddyAuthURLResult{
		AuthURL:   stateResult.AuthURL,
		SessionID: sessionID,
		State:     stateResult.State,
	}, nil
}

// CodeBuddyExchangeStateInput 是用 state 换取 token 的输入。
type CodeBuddyExchangeStateInput struct {
	SessionID string
	State     string
	ProxyID   *int64
}

// CodeBuddyTokenInfo 是 CodeBuddy token 交换/刷新的结果（含账号与模型信息）。
type CodeBuddyTokenInfo struct {
	AccessToken   string   `json:"access_token"`
	RefreshToken  string   `json:"refresh_token,omitempty"`
	TokenType     string   `json:"token_type,omitempty"`
	ExpiresIn     int64    `json:"expires_in"`
	ExpiresAt     int64    `json:"expires_at"`
	UID           string   `json:"uid,omitempty"`
	Nickname      string   `json:"nickname,omitempty"`
	EnabledModels []string                `json:"enabled_models,omitempty"`
	ModelsMeta    []codebuddy.ModelInfo    `json:"model_meta,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	Scope         string   `json:"scope,omitempty"`
}

// ExchangeState 用 state 换取 token，并依次拉取账号信息与可用模型配置。
func (s *CodeBuddyOAuthService) ExchangeState(ctx context.Context, input *CodeBuddyExchangeStateInput) (*CodeBuddyTokenInfo, error) {
	if input == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "CODEBUDDY_INVALID_INPUT", "input is required")
	}
	state := strings.TrimSpace(input.State)
	if state == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CODEBUDDY_STATE_REQUIRED", "state is required")
	}

	var proxyURL string
	if input.SessionID != "" {
		if session, ok := s.sessionStore.Get(input.SessionID); ok {
			proxyURL = session.ProxyURL
		}
	}
	if input.ProxyID != nil {
		p, err := s.proxyURL(ctx, input.ProxyID)
		if err != nil {
			return nil, err
		}
		proxyURL = p
	}
	defer func() {
		if input.SessionID != "" {
			s.sessionStore.Delete(input.SessionID)
		}
	}()

	tokenResp, err := s.oauthClient.FetchToken(ctx, state, proxyURL)
	if err != nil {
		return nil, err
	}
	info := s.tokenInfoFromResponse(tokenResp)

	// 拉取账号信息（用于昵称/uid，uid 用于 /v3/config 的 X-User-Id）。
	account, accErr := s.oauthClient.GetAccountInfo(ctx, info.AccessToken, state, proxyURL)
	if accErr == nil && account != nil {
		info.UID = account.UID
		info.Nickname = account.Nickname
	}

	// 拉取用户配置，解析可用模型。
	configBody, cfgErr := s.oauthClient.GetConfig(ctx, info.AccessToken, info.UID, proxyURL)
	if cfgErr == nil && len(configBody) > 0 {
		if models, parseErr := codebuddy.ParseEnabledModels(configBody); parseErr == nil {
			info.EnabledModels = models
		}
		if meta, parseErr := codebuddy.ParseModels(configBody); parseErr == nil {
			info.ModelsMeta = meta
		}
	}

	return info, nil
}

// RefreshToken 用 refreshToken 刷新 token 对。
func (s *CodeBuddyOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*CodeBuddyTokenInfo, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CODEBUDDY_NO_REFRESH_TOKEN", "refresh_token is required")
	}
	tokenResp, err := s.oauthClient.RefreshToken(ctx, refreshToken, proxyURL)
	if err != nil {
		return nil, err
	}
	info := s.tokenInfoFromResponse(tokenResp)
	if info.RefreshToken == "" {
		info.RefreshToken = refreshToken
	}
	return info, nil
}

// RefreshAccountToken 刷新 CodeBuddy 账号的 token。
func (s *CodeBuddyOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*CodeBuddyTokenInfo, error) {
	if account == nil || account.Platform != PlatformCodeBuddy {
		return nil, infraerrors.New(http.StatusBadRequest, "CODEBUDDY_INVALID_ACCOUNT", "account is not a CodeBuddy account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "CODEBUDDY_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
	}
	proxyURL, err := s.proxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	refreshToken := account.GetCodeBuddyRefreshToken()
	if strings.TrimSpace(refreshToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CODEBUDDY_NO_REFRESH_TOKEN", "no refresh token available")
	}
	return s.RefreshToken(ctx, refreshToken, proxyURL)
}

// BuildAccountCredentials 将 token 信息转为账号 credentials（含可用模型与模型映射）。
func (s *CodeBuddyOAuthService) BuildAccountCredentials(tokenInfo *CodeBuddyTokenInfo) map[string]any {
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
	if tokenInfo.UID != "" {
		creds["uid"] = tokenInfo.UID
	}
	if tokenInfo.Nickname != "" {
		creds["nickname"] = tokenInfo.Nickname
	}
	if tokenInfo.Domain != "" {
		creds["domain"] = tokenInfo.Domain
	}
	if tokenInfo.Scope != "" {
		creds["scope"] = tokenInfo.Scope
	}
	creds["base_url"] = codebuddy.DefaultBaseURL

	// 仅当本次交换拿到了可用模型（首次 OAuth 登录）时才写入 models / model_mapping，
	// 避免刷新 token（无模型列表）时覆盖已同步的模型配置。
	enabledModels := tokenInfo.EnabledModels
	if len(enabledModels) > 0 {
		creds["models"] = enabledModels
		modelMapping := make(map[string]string, len(enabledModels))
		for _, m := range enabledModels {
			modelMapping[m] = m
		}
		creds["model_mapping"] = modelMapping
	}
	if len(tokenInfo.ModelsMeta) > 0 {
		if metaJSON, mErr := json.Marshal(tokenInfo.ModelsMeta); mErr == nil {
			creds["codebuddy_models_meta"] = string(metaJSON)
		}
	}

	return creds
}

// Stop 停止会话清理协程。
func (s *CodeBuddyOAuthService) Stop() {
	s.sessionStore.Stop()
}

func (s *CodeBuddyOAuthService) tokenInfoFromResponse(tokenResp *codebuddy.TokenResponse) *CodeBuddyTokenInfo {
	now := time.Now()
	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64(codeBuddyDefaultAccessTokenTTL.Seconds())
	}
	info := &CodeBuddyTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    expiresIn,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second).Unix(),
		Domain:       tokenResp.Domain,
		Scope:        tokenResp.Scope,
	}
	if info.TokenType == "" {
		info.TokenType = "Bearer"
	}
	return info
}

func (s *CodeBuddyOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "CODEBUDDY_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "CODEBUDDY_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}
