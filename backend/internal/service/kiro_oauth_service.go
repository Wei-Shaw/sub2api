package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// KiroAPI is the seam between KiroOAuthService and the pkg/kiro package.
// Production code uses kiroRealAPI{}; tests pass a stub.
type KiroAPI interface {
	RefreshSocial(refreshToken, proxyURL string) (*kiro.TokenInfo, error)
	FetchProfile(accessToken, proxyURL string) (*kiro.Profile, error)
	StartIdCLogin(store *kiro.SessionStore, startURL, region, proxyURL string) (*kiro.IdCLoginStarted, *kiro.IdCSession, error)
	CompleteIdCLogin(store *kiro.SessionStore, sessionID, callbackURL string) (*kiro.TokenInfo, error)
	StartBuilderIDLogin(store *kiro.SessionStore, region, proxyURL string) (*kiro.BuilderIDLoginStarted, error)
	PollBuilderIDLogin(store *kiro.SessionStore, sessionID string) (*kiro.BuilderIDPollResult, error)
}

// kiroRealAPI bridges to pkg/kiro top-level functions in production.
type kiroRealAPI struct{}

func (kiroRealAPI) RefreshSocial(rt, proxy string) (*kiro.TokenInfo, error) {
	return kiro.RefreshSocial(rt, proxy)
}

func (kiroRealAPI) FetchProfile(at, proxy string) (*kiro.Profile, error) {
	return kiro.FetchProfile(at, proxy)
}

func (kiroRealAPI) StartIdCLogin(store *kiro.SessionStore, startURL, region, proxyURL string) (*kiro.IdCLoginStarted, *kiro.IdCSession, error) {
	return kiro.StartIdCLogin(store, startURL, region, proxyURL)
}

func (kiroRealAPI) CompleteIdCLogin(store *kiro.SessionStore, sessionID, callbackURL string) (*kiro.TokenInfo, error) {
	return kiro.CompleteIdCLogin(store, sessionID, callbackURL)
}

func (kiroRealAPI) StartBuilderIDLogin(store *kiro.SessionStore, region, proxyURL string) (*kiro.BuilderIDLoginStarted, error) {
	return kiro.StartBuilderIDLogin(store, region, proxyURL)
}

func (kiroRealAPI) PollBuilderIDLogin(store *kiro.SessionStore, sessionID string) (*kiro.BuilderIDPollResult, error) {
	return kiro.PollBuilderIDLogin(store, sessionID)
}

// KiroOAuthService is the admin-facing wrapper around Kiro auth flows.
// All three auth methods (Social refresh-token paste, IdC SSO, Builder ID
// device-code) flow through this service.
type KiroOAuthService struct {
	api       KiroAPI
	proxyRepo ProxyRepository
	sessions  *kiro.SessionStore
}

// NewKiroOAuthService constructs the service. proxyRepo may be nil in
// tests that don't exercise account-level proxy resolution.
func NewKiroOAuthService(proxyRepo ProxyRepository) *KiroOAuthService {
	return &KiroOAuthService{
		api:       kiroRealAPI{},
		proxyRepo: proxyRepo,
		sessions:  kiro.NewSessionStore(),
	}
}

// newKiroOAuthServiceWithAPI is a test-only constructor that injects a
// stub KiroAPI. Kept unexported to avoid leaking the seam to callers.
func newKiroOAuthServiceWithAPI(api KiroAPI, proxyRepo ProxyRepository) *KiroOAuthService {
	if api == nil {
		api = kiroRealAPI{}
	}
	return &KiroOAuthService{
		api:       api,
		proxyRepo: proxyRepo,
		sessions:  kiro.NewSessionStore(),
	}
}

// Stop terminates background goroutines (session cleanup). Safe to call
// during graceful shutdown.
func (s *KiroOAuthService) Stop() {
	if s.sessions != nil {
		s.sessions.Stop()
	}
}

// KiroTokenInfo is the API-shaped payload returned to admins after a
// successful Social validate or IdC/Builder ID exchange.
type KiroTokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	ProfileARN   string `json:"profile_arn,omitempty"`
	AuthMethod   string `json:"auth_method"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	Region       string `json:"region,omitempty"`
	StartURL     string `json:"start_url,omitempty"`
	Email        string `json:"email,omitempty"`
	UserID       string `json:"user_id,omitempty"`
}

// KiroIdCAuthURLResult is the response of StartIdCLogin: the URL the admin
// opens and the session id the UI sends back with the callback URL.
type KiroIdCAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	ExpiresIn int    `json:"expires_in"`
}

// KiroBuilderIDLoginResult is the response of StartBuilderIDLogin: enough
// information for the UI to show the device flow and start polling.
type KiroBuilderIDLoginResult struct {
	SessionID       string `json:"session_id"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
	ExpiresAt       int64  `json:"expires_at"`
}

// KiroBuilderIDPollResult is the response of PollBuilderIDLogin. When
// Status == "completed" the TokenInfo is populated.
type KiroBuilderIDPollResult struct {
	Status    string         `json:"status"`
	TokenInfo *KiroTokenInfo `json:"token_info,omitempty"`
}

// ValidateSocialRefreshToken validates a Kiro Social refresh token (captured
// from the Kiro desktop app), returns the fresh tokens, and best-effort
// populates Email/UserID for naming the account. The admin then POSTs to
// /accounts to persist.
func (s *KiroOAuthService) ValidateSocialRefreshToken(
	ctx context.Context,
	refreshToken string,
	proxyID *int64,
) (*KiroTokenInfo, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("kiro: refresh_token required")
	}
	proxyURL := s.resolveProxyURL(ctx, proxyID)

	info, err := s.api.RefreshSocial(refreshToken, proxyURL)
	if err != nil {
		return nil, fmt.Errorf("kiro: validate refresh token: %w", err)
	}

	out := &KiroTokenInfo{
		AccessToken:  info.AccessToken,
		RefreshToken: info.RefreshToken,
		ExpiresAt:    info.ExpiresAt,
		ProfileARN:   info.ProfileARN,
		AuthMethod:   string(kiro.AuthMethodSocial),
	}
	// Best-effort profile fetch: tokens are usable without it.
	if prof, err := s.api.FetchProfile(info.AccessToken, proxyURL); err == nil && prof != nil {
		out.Email = prof.Email
		out.UserID = prof.UserID
	}
	return out, nil
}

// StartIdCLogin kicks off a PKCE auth-code flow against the user-supplied
// AWS Identity Center startUrl. The admin opens AuthURL in their browser,
// completes the IdC login, then sends the redirected callback URL to
// CompleteIdCLogin together with the SessionID.
func (s *KiroOAuthService) StartIdCLogin(
	ctx context.Context,
	startURL, region string,
	proxyID *int64,
) (*KiroIdCAuthURLResult, error) {
	if strings.TrimSpace(startURL) == "" {
		return nil, fmt.Errorf("kiro: start_url required")
	}
	proxyURL := s.resolveProxyURL(ctx, proxyID)
	started, _, err := s.api.StartIdCLogin(s.sessions, startURL, region, proxyURL)
	if err != nil {
		return nil, err
	}
	return &KiroIdCAuthURLResult{
		AuthURL:   started.AuthorizeURL,
		SessionID: started.SessionID,
		ExpiresIn: started.ExpiresInSecs,
	}, nil
}

// CompleteIdCLogin finishes the PKCE flow. The admin pastes the full
// redirected URL (which includes ?code=...&state=...) back into the UI.
// Returns a populated KiroTokenInfo with auth_method=idc plus the
// persistent client_id / client_secret / region / start_url that subsequent
// refreshes need.
func (s *KiroOAuthService) CompleteIdCLogin(
	ctx context.Context,
	sessionID, callbackURL string,
) (*KiroTokenInfo, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("kiro: session_id required")
	}
	if strings.TrimSpace(callbackURL) == "" {
		return nil, fmt.Errorf("kiro: callback_url required")
	}
	info, err := s.api.CompleteIdCLogin(s.sessions, sessionID, callbackURL)
	if err != nil {
		return nil, err
	}
	out := s.tokenInfoFromKiro(info)
	// Best-effort profile fetch.
	if prof, err := s.api.FetchProfile(info.AccessToken, s.resolveProxyURL(ctx, nil)); err == nil && prof != nil {
		out.Email = prof.Email
		out.UserID = prof.UserID
	}
	return out, nil
}

// StartBuilderIDLogin kicks off an AWS Builder ID device-code flow.
// The admin opens VerificationURI in a browser, enters UserCode, then
// the UI polls PollBuilderIDLogin until Status=completed.
func (s *KiroOAuthService) StartBuilderIDLogin(
	ctx context.Context,
	region string,
	proxyID *int64,
) (*KiroBuilderIDLoginResult, error) {
	proxyURL := s.resolveProxyURL(ctx, proxyID)
	started, err := s.api.StartBuilderIDLogin(s.sessions, region, proxyURL)
	if err != nil {
		return nil, err
	}
	return &KiroBuilderIDLoginResult{
		SessionID:       started.SessionID,
		UserCode:        started.UserCode,
		VerificationURI: started.VerificationURI,
		Interval:        started.Interval,
		ExpiresAt:       started.ExpiresAtUnix,
	}, nil
}

// PollBuilderIDLogin polls AWS once for a device-code session. Returns
// status ∈ {pending, slow_down, completed}. On completed the TokenInfo
// is populated and the session is removed; on pending/slow_down the UI
// continues polling.
func (s *KiroOAuthService) PollBuilderIDLogin(
	ctx context.Context,
	sessionID string,
) (*KiroBuilderIDPollResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("kiro: session_id required")
	}
	result, err := s.api.PollBuilderIDLogin(s.sessions, sessionID)
	if err != nil {
		return nil, err
	}
	out := &KiroBuilderIDPollResult{Status: string(result.Status)}
	if result.Status == kiro.BuilderIDPollCompleted && result.Token != nil {
		info := s.tokenInfoFromKiro(result.Token)
		if prof, err := s.api.FetchProfile(result.Token.AccessToken, s.resolveProxyURL(ctx, nil)); err == nil && prof != nil {
			info.Email = prof.Email
			info.UserID = prof.UserID
		}
		out.TokenInfo = info
	}
	return out, nil
}

// tokenInfoFromKiro converts a pkg/kiro.TokenInfo into the service-layer
// KiroTokenInfo struct used in HTTP responses + credential building.
func (s *KiroOAuthService) tokenInfoFromKiro(info *kiro.TokenInfo) *KiroTokenInfo {
	if info == nil {
		return nil
	}
	return &KiroTokenInfo{
		AccessToken:  info.AccessToken,
		RefreshToken: info.RefreshToken,
		ExpiresAt:    info.ExpiresAt,
		ProfileARN:   info.ProfileARN,
		AuthMethod:   string(info.AuthMethod),
		ClientID:     info.ClientID,
		ClientSecret: info.ClientSecret,
		Region:       info.Region,
		StartURL:     info.StartURL,
	}
}

// BuildAccountCredentials translates a KiroTokenInfo into the JSON-shaped
// credentials map persisted on the account row. For IdC and Builder ID
// the long-lived client_id / client_secret / region (and start_url for
// IdC) are persisted alongside the rotating tokens so the refresher has
// what it needs.
func (s *KiroOAuthService) BuildAccountCredentials(tokenInfo *KiroTokenInfo) map[string]any {
	if tokenInfo == nil {
		return map[string]any{}
	}
	creds := map[string]any{
		"access_token":  tokenInfo.AccessToken,
		"refresh_token": tokenInfo.RefreshToken,
		"expires_at":    tokenInfo.ExpiresAt,
		"auth_method":   tokenInfo.AuthMethod,
	}
	if tokenInfo.ClientID != "" {
		creds["client_id"] = tokenInfo.ClientID
	}
	if tokenInfo.ClientSecret != "" {
		creds["client_secret"] = tokenInfo.ClientSecret
	}
	if tokenInfo.Region != "" {
		creds["region"] = tokenInfo.Region
	}
	if tokenInfo.StartURL != "" {
		creds["start_url"] = tokenInfo.StartURL
	}
	return creds
}

// RefreshAccountToken is called by KiroTokenRefresher when an account's
// access token is in the refresh window.
func (s *KiroOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*KiroTokenInfo, error) {
	if account == nil {
		return nil, fmt.Errorf("kiro: nil account")
	}
	method := kiro.AuthMethod(account.GetCredential("auth_method"))
	if method == "" {
		// Backward compat: oldest social accounts may have no auth_method field.
		method = kiro.AuthMethodSocial
	}

	proxyURL := ""
	if s.proxyRepo != nil && account.ProxyID != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
		}
	}

	info, err := kiro.RefreshToken(&kiro.RefreshableAccount{
		AuthMethod:   method,
		RefreshToken: account.GetCredential("refresh_token"),
		ClientID:     account.GetCredential("client_id"),
		ClientSecret: account.GetCredential("client_secret"),
		Region:       account.GetCredential("region"),
		ProxyURL:     proxyURL,
	})
	if err != nil {
		return nil, err
	}
	return s.tokenInfoFromKiro(info), nil
}

func (s *KiroOAuthService) resolveProxyURL(ctx context.Context, proxyID *int64) string {
	if proxyID == nil || s.proxyRepo == nil {
		return ""
	}
	p, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil || p == nil {
		return ""
	}
	return p.URL()
}
