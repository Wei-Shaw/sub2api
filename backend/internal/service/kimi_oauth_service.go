package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
)

// kimiDefaultAccessTokenTTL 是上游未返回 expires_in 时的兜底 access_token 有效期。
// Kimi access_token 是 JWT，实测有效期约 15 分钟。
const kimiDefaultAccessTokenTTL = 15 * time.Minute

// KimiOAuthService 处理 Kimi（kimi.com 订阅）OAuth 设备码登录流程（RFC 8628）。
// 对标 GrokOAuthService，但授权码流程替换为设备码流程：
// StartDeviceAuth 发起设备授权并保存会话，PollDeviceToken 由前端定时器驱动做单次轮询。
type KimiOAuthService struct {
	sessionStore *kimi.SessionStore
	proxyRepo    ProxyRepository
	oauthClient  KimiOAuthClient
}

func NewKimiOAuthService(proxyRepo ProxyRepository, oauthClient KimiOAuthClient) *KimiOAuthService {
	return &KimiOAuthService{
		sessionStore: kimi.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
	}
}

// KimiDeviceAuthResult 是设备授权发起结果（前端展示 user_code + 验证链接并驱动轮询）。
type KimiDeviceAuthResult struct {
	SessionID               string `json:"session_id"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int64  `json:"expires_in"`
}

// StartDeviceAuth 调用 auth.kimi.com 的 device_authorization 端点并保存设备码会话。
// device_id 在此处生成并随会话保存，后续建号时写入 credentials 持久化复用。
func (s *KimiOAuthService) StartDeviceAuth(ctx context.Context, proxyID *int64) (*KimiDeviceAuthResult, error) {
	sessionID, err := kimi.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "KIMI_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}
	deviceID, err := kimi.GenerateDeviceID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "KIMI_OAUTH_DEVICE_ID_FAILED", "failed to generate device id: %v", err)
	}

	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}

	deviceResp, err := s.oauthClient.DeviceAuthorization(ctx, proxyURL, deviceID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(deviceResp.DeviceCode) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "KIMI_OAUTH_DEVICE_AUTH_FAILED", "device authorization response missing device_code")
	}

	interval := deviceResp.Interval
	if interval <= 0 {
		interval = kimi.DefaultPollIntervalSeconds
	}
	now := time.Now()
	s.sessionStore.Set(sessionID, &kimi.DeviceSession{
		DeviceCode:              deviceResp.DeviceCode,
		UserCode:                deviceResp.UserCode,
		VerificationURI:         deviceResp.VerificationURI,
		VerificationURIComplete: deviceResp.VerificationURIComplete,
		IntervalSeconds:         interval,
		ClientID:                kimi.EffectiveClientID(),
		ProxyURL:                proxyURL,
		DeviceID:                deviceID,
		CreatedAt:               now,
		ExpiresAt:               now.Add(kimi.SessionTTLFromExpiresIn(deviceResp.ExpiresIn)),
	})

	return &KimiDeviceAuthResult{
		SessionID:               sessionID,
		UserCode:                deviceResp.UserCode,
		VerificationURI:         deviceResp.VerificationURI,
		VerificationURIComplete: deviceResp.VerificationURIComplete,
		Interval:                interval,
		ExpiresIn:               deviceResp.ExpiresIn,
	}, nil
}

// KimiTokenInfo 是 Kimi OAuth token 信息（轮询/刷新成功后返回给前端或写入 credentials）。
type KimiTokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
	ClientID     string `json:"client_id,omitempty"`
	Scope        string `json:"scope,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
}

// PollDeviceToken 执行一次设备码轮询（前端定时器按 interval 驱动）。
//
// 错误语义（通过 infraerrors.Reason + Metadata 传给前端）：
//   - authorization_pending → KIMI_OAUTH_AUTHORIZATION_PENDING，前端按 interval 继续轮询；
//   - slow_down → KIMI_OAUTH_SLOW_DOWN，Metadata.suggested_interval 为建议间隔（interval+5s）；
//   - expired_token → KIMI_OAUTH_DEVICE_CODE_EXPIRED，会话作废需重新发起登录；
//   - access_denied → KIMI_OAUTH_ACCESS_DENIED，用户拒绝授权；
//   - 成功后会话保留 token（直至过期），供 create-from-oauth 一步建号读取。
func (s *KimiOAuthService) PollDeviceToken(ctx context.Context, sessionID string, proxyID *int64) (*KimiTokenInfo, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_INVALID_INPUT", "session_id is required")
	}
	session, ok := s.sessionStore.Get(sessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}

	// 幂等：已成功轮询过的会话直接返回暂存的 token。
	if session.Completed && session.Token != nil {
		return s.tokenInfoFromResponse(session.Token, session.ClientID, session.DeviceID), nil
	}

	proxyURL := session.ProxyURL
	if proxyID != nil {
		var err error
		proxyURL, err = s.proxyURL(ctx, proxyID)
		if err != nil {
			return nil, err
		}
	}

	tokenResp, err := s.oauthClient.PollDeviceToken(ctx, session.DeviceCode, proxyURL, session.DeviceID)
	if err != nil {
		var oauthErr *kimi.OAuthError
		if errors.As(err, &oauthErr) {
			switch {
			case oauthErr.IsPending():
				return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_AUTHORIZATION_PENDING", "authorization is pending").
					WithMetadata(map[string]string{"interval": strconv.Itoa(session.IntervalSeconds)})
			case oauthErr.IsSlowDown():
				suggested := session.IntervalSeconds + kimi.SlowDownExtraSeconds
				return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SLOW_DOWN", "polling too fast, slow down").
					WithMetadata(map[string]string{"suggested_interval": strconv.Itoa(suggested)})
			case oauthErr.IsExpiredToken():
				s.sessionStore.Delete(sessionID)
				return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_DEVICE_CODE_EXPIRED", "device code expired, please start a new login")
			case oauthErr.IsAccessDenied():
				s.sessionStore.Delete(sessionID)
				return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_ACCESS_DENIED", "user denied the authorization request")
			case oauthErr.IsInvalidGrant():
				s.sessionStore.Delete(sessionID)
				return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_INVALID_GRANT", "invalid device code grant")
			}
		}
		return nil, err
	}

	// 成功：暂存 token 到会话（保留至过期），供 create-from-oauth 读取。
	session.Token = tokenResp
	session.Completed = true
	s.sessionStore.Set(sessionID, session)
	return s.tokenInfoFromResponse(tokenResp, session.ClientID, session.DeviceID), nil
}

// GetSessionToken 返回已成功轮询会话的 token（供 create-from-oauth 一步建号）。
func (s *KimiOAuthService) GetSessionToken(sessionID string) (*KimiTokenInfo, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_INVALID_INPUT", "session_id is required")
	}
	session, ok := s.sessionStore.Get(sessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}
	if !session.Completed || session.Token == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_AUTHORIZATION_PENDING", "device authorization has not completed yet").
			WithMetadata(map[string]string{"interval": strconv.Itoa(session.IntervalSeconds)})
	}
	return s.tokenInfoFromResponse(session.Token, session.ClientID, session.DeviceID), nil
}

// DeleteSession 删除设备码会话（建号完成或前端取消登录时调用）。
func (s *KimiOAuthService) DeleteSession(sessionID string) {
	s.sessionStore.Delete(strings.TrimSpace(sessionID))
}

// RefreshToken 用 refresh_token 换新 token。
// refresh_token 每次刷新轮换：若响应未携带新值则保留旧值（对齐 opencode-kimi-auth 的做法）。
// 401/403/invalid_grant 表示凭据作废（KIMI_OAUTH_TOKEN_INVALID，不可重试），需重新登录。
func (s *KimiOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL, deviceID string) (*KimiTokenInfo, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_NO_REFRESH_TOKEN", "refresh_token is required")
	}
	tokenResp, err := s.oauthClient.RefreshToken(ctx, refreshToken, proxyURL, strings.TrimSpace(deviceID))
	if err != nil {
		var oauthErr *kimi.OAuthError
		if errors.As(err, &oauthErr) && oauthErr.IsUnauthorized() {
			return nil, infraerrors.Newf(http.StatusUnauthorized, "KIMI_OAUTH_TOKEN_INVALID", "kimi refresh token rejected, re-authentication required: %s", oauthErr.Code)
		}
		return nil, err
	}
	tokenInfo := s.tokenInfoFromResponse(tokenResp, "", deviceID)
	if tokenInfo.RefreshToken == "" {
		tokenInfo.RefreshToken = refreshToken
	}
	return tokenInfo, nil
}

// RefreshAccountToken 刷新账号凭据中的 token（保留 credentials 里的 device_id / base_url）。
func (s *KimiOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*KimiTokenInfo, error) {
	if account == nil || account.Platform != PlatformKimi {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_INVALID_ACCOUNT", "account is not a Kimi account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
	}

	proxyURL, err := s.proxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	refreshToken := account.GetCredential("refresh_token")
	if strings.TrimSpace(refreshToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
	}

	return s.RefreshToken(ctx, refreshToken, proxyURL, account.GetKimiDeviceID())
}

// BuildAccountCredentials 把 token 信息转换为账号 credentials。
// refresh_token 轮换值必须随本次结果原子写回；device_id 持久化供指纹头复用。
func (s *KimiOAuthService) BuildAccountCredentials(tokenInfo *KimiTokenInfo) map[string]any {
	if tokenInfo == nil {
		return nil
	}
	expiresAt := time.Unix(tokenInfo.ExpiresAt, 0).UTC().Format(time.RFC3339)
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
		"expires_at":   expiresAt,
		"base_url":     kimi.DefaultBaseURL,
	}
	if tokenInfo.RefreshToken != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}
	if tokenInfo.TokenType != "" {
		creds["token_type"] = tokenInfo.TokenType
	}
	if tokenInfo.ClientID != "" {
		creds["client_id"] = tokenInfo.ClientID
	}
	if tokenInfo.Scope != "" {
		creds["scope"] = tokenInfo.Scope
	}
	if tokenInfo.DeviceID != "" {
		creds["device_id"] = tokenInfo.DeviceID
	}
	return creds
}

// Stop 停止会话存储的清理协程。
func (s *KimiOAuthService) Stop() {
	s.sessionStore.Stop()
}

func (s *KimiOAuthService) tokenInfoFromResponse(tokenResp *kimi.TokenResponse, clientID, deviceID string) *KimiTokenInfo {
	now := time.Now()
	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64(kimiDefaultAccessTokenTTL.Seconds())
	}
	info := &KimiTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    expiresIn,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second).Unix(),
		ClientID:     strings.TrimSpace(clientID),
		Scope:        tokenResp.Scope,
		DeviceID:     strings.TrimSpace(deviceID),
	}
	if info.ClientID == "" {
		info.ClientID = kimi.EffectiveClientID()
	}
	if info.TokenType == "" {
		info.TokenType = "Bearer"
	}
	return info
}

func (s *KimiOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "KIMI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}
