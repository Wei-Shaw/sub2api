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
}

// kiroRealAPI bridges to pkg/kiro top-level functions in production.
type kiroRealAPI struct{}

func (kiroRealAPI) RefreshSocial(rt, proxy string) (*kiro.TokenInfo, error) {
	return kiro.RefreshSocial(rt, proxy)
}

func (kiroRealAPI) FetchProfile(at, proxy string) (*kiro.Profile, error) {
	return kiro.FetchProfile(at, proxy)
}

// KiroOAuthService is the admin-facing wrapper around Kiro auth flows.
// Phase 2 supports the Social refresh-token paste flow; Phase 3 adds
// IdC SSO and Builder ID device-code flows.
type KiroOAuthService struct {
	api       KiroAPI
	proxyRepo ProxyRepository
}

// NewKiroOAuthService constructs the service. proxyRepo may be nil in
// tests that don't exercise account-level proxy resolution.
func NewKiroOAuthService(proxyRepo ProxyRepository) *KiroOAuthService {
	return &KiroOAuthService{api: kiroRealAPI{}, proxyRepo: proxyRepo}
}

// newKiroOAuthServiceWithAPI is a test-only constructor that injects a
// stub KiroAPI. Kept unexported to avoid leaking the seam to callers.
func newKiroOAuthServiceWithAPI(api KiroAPI, proxyRepo ProxyRepository) *KiroOAuthService {
	if api == nil {
		api = kiroRealAPI{}
	}
	return &KiroOAuthService{api: api, proxyRepo: proxyRepo}
}

// KiroTokenInfo is the API-shaped payload returned to admins after a
// successful Social validate (and, later, IdC/Builder ID exchange).
type KiroTokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	ProfileARN   string `json:"profile_arn,omitempty"`
	AuthMethod   string `json:"auth_method"`
	Email        string `json:"email,omitempty"`
	UserID       string `json:"user_id,omitempty"`
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

// BuildAccountCredentials translates a KiroTokenInfo into the JSON-shaped
// credentials map persisted on the account row. It mirrors
// BuildClaudeAccountCredentials / antigravity's BuildAccountCredentials.
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
	return &KiroTokenInfo{
		AccessToken:  info.AccessToken,
		RefreshToken: info.RefreshToken,
		ExpiresAt:    info.ExpiresAt,
		ProfileARN:   info.ProfileARN,
		AuthMethod:   string(info.AuthMethod),
	}, nil
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
