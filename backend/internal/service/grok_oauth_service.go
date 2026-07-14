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

const grokDefaultAccessTokenTTL = 6 * time.Hour

type GrokOAuthService struct {
	sessionStore *xai.SessionStore
	proxyRepo    ProxyRepository
	oauthClient  GrokOAuthClient
}

func NewGrokOAuthService(proxyRepo ProxyRepository, oauthClient GrokOAuthClient) *GrokOAuthService {
	return &GrokOAuthService{
		sessionStore: xai.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
	}
}

type GrokAuthURLResult struct {
	AuthURL    string `json:"auth_url"`
	SessionID  string `json:"session_id"`
	State      string `json:"state"`
	Flow       string `json:"flow,omitempty"`
	UserCode   string `json:"user_code,omitempty"`
	Interval   int    `json:"interval,omitempty"`
	ExpiresIn  int    `json:"expires_in,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
}

// GenerateAuthURL starts a Grok CLI device-code OAuth session (matches official Grok CLI / clandes).
// Authorization-code PKCE against the public xAI client can issue refresh tokens that fail the
// first refresh with invalid_grant; device-code tokens refresh correctly.
func (s *GrokOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI string) (*GrokAuthURLResult, error) {
	_ = redirectURI // device flow does not use redirect_uri; kept for API compatibility

	sessionID, err := xai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}
	state, err := xai.GenerateState()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_STATE_FAILED", "failed to generate state: %v", err)
	}

	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}

	clientID := xai.EffectiveClientID()
	scope := xai.EffectiveScope()
	device, err := s.oauthClient.RequestDeviceCode(ctx, proxyURL, clientID, scope)
	if err != nil {
		return nil, err
	}

	authURL := xai.DeviceAuthURL(device)
	if authURL == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_DEVICE_URL_MISSING", "device flow response missing verification URL")
	}

	now := time.Now()
	s.sessionStore.Set(sessionID, &xai.OAuthSession{
		State:      state,
		ClientID:   clientID,
		Scope:      scope,
		ProxyURL:   proxyURL,
		Flow:       xai.OAuthFlowDevice,
		DeviceCode: device.DeviceCode,
		UserCode:   device.UserCode,
		Interval:   device.Interval,
		ExpiresIn:  device.ExpiresIn,
		CreatedAt:  now,
	})

	return &GrokAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
		State:     state,
		Flow:      xai.OAuthFlowDevice,
		UserCode:  device.UserCode,
		Interval:  device.Interval,
		ExpiresIn: device.ExpiresIn,
		ExpiresAt: now.Add(time.Duration(device.ExpiresIn) * time.Second).Unix(),
	}, nil
}

type GrokExchangeCodeInput struct {
	SessionID   string
	Code        string
	State       string
	RedirectURI string
	ProxyID     *int64
}

type GrokDevicePollInput struct {
	SessionID string
	ProxyID   *int64
}

type GrokDevicePollInfo struct {
	Status    string         `json:"status"`
	Interval  int            `json:"interval,omitempty"`
	ExpiresAt int64          `json:"expires_at,omitempty"`
	UserCode  string         `json:"user_code,omitempty"`
	Token     *GrokTokenInfo `json:"token,omitempty"`
	Error     string         `json:"error,omitempty"`
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

// PollDeviceLogin performs a single device-token poll for a pending Grok OAuth session.
func (s *GrokOAuthService) PollDeviceLogin(ctx context.Context, input *GrokDevicePollInput) (*GrokDevicePollInfo, error) {
	if input == nil || strings.TrimSpace(input.SessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}
	if session.Flow != "" && session.Flow != xai.OAuthFlowDevice {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_NOT_DEVICE_FLOW", "session is not a device OAuth flow")
	}
	if strings.TrimSpace(session.DeviceCode) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_DEVICE_CODE_REQUIRED", "session is missing device_code")
	}

	expiresAt := session.CreatedAt.Add(time.Duration(session.ExpiresIn) * time.Second)
	if session.ExpiresIn > 0 && time.Now().After(expiresAt) {
		s.sessionStore.Delete(input.SessionID)
		return &GrokDevicePollInfo{
			Status: string(xai.DevicePollExpired),
			Error:  "device code expired",
		}, nil
	}

	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		var err error
		proxyURL, err = s.proxyURL(ctx, input.ProxyID)
		if err != nil {
			return nil, err
		}
	}

	result, err := s.oauthClient.PollDeviceToken(ctx, session.DeviceCode, proxyURL, session.ClientID)
	if err != nil {
		return nil, err
	}

	info := &GrokDevicePollInfo{
		Status:    string(result.Status),
		Interval:  session.Interval,
		ExpiresAt: expiresAt.Unix(),
		UserCode:  session.UserCode,
		Error:     result.Error,
	}
	if session.Interval <= 0 {
		info.Interval = 5
	}

	switch result.Status {
	case xai.DevicePollAuthorized:
		if result.Token == nil {
			return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_DEVICE_TOKEN_EMPTY", "device authorization succeeded without tokens")
		}
		s.sessionStore.Delete(input.SessionID)
		info.Token = s.tokenInfoFromResponse(result.Token, session.ClientID, nil)
		return info, nil
	case xai.DevicePollSlowDown:
		info.Interval += 5
		return info, nil
	case xai.DevicePollPending:
		return info, nil
	case xai.DevicePollDenied, xai.DevicePollExpired:
		s.sessionStore.Delete(input.SessionID)
		return info, nil
	default:
		// Keep session for transient upstream errors so the client can retry.
		if info.Error == "" {
			info.Error = "device token poll failed"
		}
		return info, nil
	}
}

func (s *GrokOAuthService) ExchangeCode(ctx context.Context, input *GrokExchangeCodeInput) (*GrokTokenInfo, error) {
	if input == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_INPUT", "input is required")
	}
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}

	// Device-code sessions complete via PollDeviceLogin. ExchangeCode still accepts
	// a single-shot completion (no code required) so older clients can finish login
	// after the user authorizes in the browser.
	if session.Flow == xai.OAuthFlowDevice || strings.TrimSpace(session.DeviceCode) != "" {
		poll, err := s.PollDeviceLogin(ctx, &GrokDevicePollInput{
			SessionID: input.SessionID,
			ProxyID:   input.ProxyID,
		})
		if err != nil {
			return nil, err
		}
		switch poll.Status {
		case string(xai.DevicePollAuthorized):
			if poll.Token == nil {
				return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_DEVICE_TOKEN_EMPTY", "device authorization succeeded without tokens")
			}
			return poll.Token, nil
		case string(xai.DevicePollPending), string(xai.DevicePollSlowDown):
			return nil, infraerrors.New(http.StatusAccepted, "GROK_OAUTH_AUTHORIZATION_PENDING", "user has not completed device authorization yet")
		case string(xai.DevicePollDenied):
			return nil, infraerrors.New(http.StatusForbidden, "GROK_OAUTH_AUTHORIZATION_DENIED", "device authorization was denied")
		case string(xai.DevicePollExpired):
			return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_DEVICE_CODE_EXPIRED", "device code expired")
		default:
			msg := poll.Error
			if msg == "" {
				msg = "device authorization failed"
			}
			return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_DEVICE_POLL_FAILED", msg)
		}
	}

	defer s.sessionStore.Delete(input.SessionID)

	parsed := xai.ParseAuthorizationInput(input.Code)
	code := strings.TrimSpace(parsed.Code)
	if code == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_CODE_REQUIRED", "authorization code is required")
	}
	state := strings.TrimSpace(input.State)
	if state == "" {
		state = strings.TrimSpace(parsed.State)
	}
	if parsed.RequiresState && state == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_STATE_REQUIRED", "oauth state is required for callback URLs")
	}
	if state != "" && subtle.ConstantTimeCompare([]byte(state), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_STATE", "invalid oauth state")
	}

	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		var err error
		proxyURL, err = s.proxyURL(ctx, input.ProxyID)
		if err != nil {
			return nil, err
		}
	}
	redirectURI := session.RedirectURI
	if strings.TrimSpace(input.RedirectURI) != "" {
		redirectURI = input.RedirectURI
	}

	tokenResp, err := s.oauthClient.ExchangeCode(ctx, code, session.CodeVerifier, redirectURI, proxyURL, session.ClientID)
	if err != nil {
		return nil, err
	}
	return s.tokenInfoFromResponse(tokenResp, session.ClientID, nil), nil
}

func (s *GrokOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL, clientID string) (*GrokTokenInfo, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_NO_REFRESH_TOKEN", "refresh_token is required")
	}
	tokenResp, err := s.oauthClient.RefreshToken(ctx, refreshToken, proxyURL, clientID)
	if err != nil {
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

func (s *GrokOAuthService) ConvertFromSSO(ctx context.Context, ssoToken string, proxyID *int64) (*GrokTokenInfo, error) {
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	tokenResp, err := s.oauthClient.ConvertSSOToBuild(ctx, ssoToken, proxyURL)
	if err != nil {
		return nil, err
	}
	return s.tokenInfoFromResponse(tokenResp, xai.DefaultClientID, nil), nil
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
		return "", infraerrors.Newf(http.StatusBadRequest, "GROK_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
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
