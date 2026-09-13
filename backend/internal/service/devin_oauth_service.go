package service

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type DevinOAuthService struct {
	sessionStore *devin.SessionStore
	proxyRepo    ProxyRepository
}

func NewDevinOAuthService(proxyRepo ProxyRepository) *DevinOAuthService {
	return &DevinOAuthService{
		sessionStore: devin.NewSessionStore(),
		proxyRepo:    proxyRepo,
	}
}

type DevinAuthURLResult struct {
	AuthURL     string `json:"auth_url"`
	SessionID   string `json:"session_id"`
	State       string `json:"state"`
	RedirectURI string `json:"redirect_uri"`
}

func (s *DevinOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64) (*DevinAuthURLResult, error) {
	state, err := devin.GenerateState()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "DEVIN_OAUTH_STATE_FAILED", "failed to generate state: %v", err)
	}
	verifier, challenge, err := devin.GeneratePKCE()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "DEVIN_OAUTH_PKCE_FAILED", "failed to generate PKCE: %v", err)
	}
	sessionID, err := devin.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "DEVIN_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	s.sessionStore.Set(sessionID, &devin.OAuthSession{
		State:        state,
		CodeVerifier: verifier,
		Challenge:    challenge,
		ProxyURL:     proxyURL,
		RedirectURI:  devin.RedirectURI,
		CreatedAt:    time.Now(),
	})
	return &DevinAuthURLResult{
		AuthURL:     devin.BuildAuthorizationURL(state, challenge, devin.RedirectURI),
		SessionID:   sessionID,
		State:       state,
		RedirectURI: devin.RedirectURI,
	}, nil
}

type DevinExchangeCodeInput struct {
	SessionID string
	Code      string
	State     string
	ProxyID   *int64
}

type DevinTokenInfo struct {
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	ExpiresAt     int64  `json:"expires_at"`
	APIEndpoint   string `json:"api_endpoint,omitempty"`
	EnterpriseURL string `json:"enterprise_url,omitempty"`
}

func (s *DevinOAuthService) ExchangeCode(ctx context.Context, input *DevinExchangeCodeInput) (*DevinTokenInfo, error) {
	if input == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "DEVIN_OAUTH_INVALID_INPUT", "input is required")
	}
	session, ok := s.sessionStore.Get(strings.TrimSpace(input.SessionID))
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "DEVIN_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}
	code, state := parseDevinCallback(input.Code, input.State)
	if code == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "DEVIN_OAUTH_CODE_REQUIRED", "authorization code is required")
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "DEVIN_OAUTH_INVALID_STATE", "invalid oauth state")
	}
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		resolved, err := s.proxyURL(ctx, input.ProxyID)
		if err != nil {
			return nil, err
		}
		proxyURL = resolved
	}
	client, err := devin.NewClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "DEVIN_OAUTH_PROXY_INVALID", "invalid proxy: %v", err)
	}
	tokens, err := client.ExchangeCode(ctx, code, session.CodeVerifier)
	if err != nil {
		return nil, err
	}
	s.sessionStore.Delete(input.SessionID)
	info := tokenInfoFromDevin(tokens.Token)
	if stream, err := devin.NewStreamClient(proxyURL); err == nil {
		if _, err := devin.ValidateSession(ctx, stream, info.AccessToken); err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "DEVIN_OAUTH_SESSION_INVALID", "devin session validation failed: %v", err)
		}
	}
	return info, nil
}

func (s *DevinOAuthService) ImportSessionToken(token string) (*DevinTokenInfo, error) {
	normalized := devin.NormalizeSessionToken(token)
	if normalized == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "DEVIN_OAUTH_TOKEN_REQUIRED", "session token is required")
	}
	return tokenInfoFromDevin(normalized), nil
}

func (s *DevinOAuthService) BuildAccountCredentials(info *DevinTokenInfo) map[string]any {
	if info == nil {
		return map[string]any{}
	}
	return map[string]any{
		"access_token":   info.AccessToken,
		"refresh_token":  info.RefreshToken,
		"expires_at":     info.ExpiresAt,
		"api_endpoint":   info.APIEndpoint,
		"enterprise_url": info.EnterpriseURL,
	}
}

func (s *DevinOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil || s.proxyRepo == nil {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "DEVIN_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}

func tokenInfoFromDevin(token string) *DevinTokenInfo {
	normalized := devin.NormalizeSessionToken(token)
	return &DevinTokenInfo{
		AccessToken:   normalized,
		RefreshToken:  normalized,
		ExpiresAt:     devin.TokenExpiry(normalized).Unix(),
		APIEndpoint:   devin.DefaultAPIHost,
		EnterpriseURL: "https://app.devin.ai",
	}
}

func parseDevinCallback(code, state string) (string, string) {
	code = strings.TrimSpace(code)
	state = strings.TrimSpace(state)
	if strings.Contains(code, "://") || strings.HasPrefix(code, "?") || strings.Contains(code, "code=") {
		raw := code
		if !strings.Contains(raw, "://") {
			raw = "http://127.0.0.1/callback" + strings.TrimPrefix(raw, "?")
			if !strings.Contains(raw, "?") {
				raw = "http://127.0.0.1/callback?" + strings.TrimPrefix(code, "?")
			}
		}
		if parsed, err := url.Parse(raw); err == nil {
			query := parsed.Query()
			if parsedCode := strings.TrimSpace(query.Get("code")); parsedCode != "" {
				code = parsedCode
			}
			if parsedState := strings.TrimSpace(query.Get("state")); parsedState != "" {
				state = parsedState
			}
		}
	}
	if i := strings.Index(code, "#"); i >= 0 {
		if state == "" {
			state = strings.TrimSpace(code[i+1:])
		}
		code = strings.TrimSpace(code[:i])
	}
	return code, state
}
