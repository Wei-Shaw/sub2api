package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

type CursorOAuthService struct {
	sessionStore *cursor.SessionStore
	proxyRepo    ProxyRepository
}

func NewCursorOAuthService(proxyRepo ProxyRepository) *CursorOAuthService {
	return &CursorOAuthService{
		sessionStore: cursor.NewSessionStore(),
		proxyRepo:    proxyRepo,
	}
}

type CursorAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
}

func (s *CursorOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64) (*CursorAuthURLResult, error) {
	verifier, challenge, err := cursor.GeneratePKCE()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CURSOR_OAUTH_PKCE_FAILED", "failed to generate PKCE: %v", err)
	}
	sessionID, err := cursor.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CURSOR_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}
	loginUUID := uuid.NewString()
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	s.sessionStore.Set(sessionID, &cursor.OAuthSession{
		UUID:      loginUUID,
		Verifier:  verifier,
		Challenge: challenge,
		ProxyURL:  proxyURL,
		CreatedAt: time.Now(),
	})
	return &CursorAuthURLResult{
		AuthURL:   cursor.BuildLoginURL(challenge, loginUUID),
		SessionID: sessionID,
	}, nil
}

type CursorTokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    int64  `json:"expires_at"`
	UserID       string `json:"user_id,omitempty"`
	Email        string `json:"email,omitempty"`
	Pending      bool   `json:"pending,omitempty"`
}

func (s *CursorOAuthService) Poll(ctx context.Context, sessionID string, proxyID *int64) (*CursorTokenInfo, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	session, ok := s.sessionStore.Get(sessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}
	proxyURL := session.ProxyURL
	if proxyID != nil {
		resolved, err := s.proxyURL(ctx, proxyID)
		if err != nil {
			return nil, err
		}
		proxyURL = resolved
	}
	client, err := cursor.NewClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "CURSOR_OAUTH_PROXY_INVALID", "invalid proxy: %v", err)
	}
	tokens, pending, err := client.PollOnce(ctx, session.UUID, session.Verifier)
	if err != nil {
		return nil, err
	}
	if pending {
		return &CursorTokenInfo{Pending: true}, nil
	}
	s.sessionStore.Delete(sessionID)
	info := tokenInfoFromCursor(tokens)
	if email := fetchCursorEmail(ctx, client, info.AccessToken); email != "" {
		info.Email = email
	}
	return info, nil
}

func (s *CursorOAuthService) RefreshToken(ctx context.Context, refreshToken string, proxyID *int64) (*CursorTokenInfo, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_REFRESH_REQUIRED", "refresh_token is required")
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	client, err := cursor.NewClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "CURSOR_OAUTH_PROXY_INVALID", "invalid proxy: %v", err)
	}
	tokens, err := client.Refresh(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	return tokenInfoFromCursor(tokens), nil
}

func (s *CursorOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*CursorTokenInfo, error) {
	if account == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_ACCOUNT_REQUIRED", "account is required")
	}
	return s.RefreshToken(ctx, account.GetCursorRefreshToken(), account.ProxyID)
}

func (s *CursorOAuthService) BuildAccountCredentials(info *CursorTokenInfo) map[string]any {
	if info == nil {
		return map[string]any{}
	}
	credentials := map[string]any{
		"access_token": info.AccessToken,
		"expires_at":   info.ExpiresAt,
	}
	if info.RefreshToken != "" {
		credentials["refresh_token"] = info.RefreshToken
	}
	if info.UserID != "" {
		credentials["user_id"] = info.UserID
	}
	if info.Email != "" {
		credentials["email"] = info.Email
	}
	return credentials
}

func (s *CursorOAuthService) QueryUsage(ctx context.Context, account *Account) (json.RawMessage, error) {
	if account == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_ACCOUNT_REQUIRED", "account is required")
	}
	proxyURL, err := s.proxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	client, err := cursor.NewClient(proxyURL)
	if err != nil {
		return nil, err
	}
	return client.FetchUsage(ctx, account.GetCursorAccessToken())
}

func (s *CursorOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil || s.proxyRepo == nil {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "CURSOR_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}

func tokenInfoFromCursor(tokens *cursor.TokenResponse) *CursorTokenInfo {
	info := &CursorTokenInfo{
		AccessToken:  strings.TrimSpace(tokens.AccessToken),
		RefreshToken: strings.TrimSpace(tokens.RefreshToken),
		ExpiresAt:    cursor.TokenExpiry(tokens.AccessToken).Unix(),
		UserID:       cursor.ExtractUserID(tokens.AccessToken),
	}
	return info
}

func fetchCursorEmail(ctx context.Context, client *cursor.Client, accessToken string) string {
	raw, err := client.FetchAuthMe(ctx, accessToken)
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	email, _ := payload["email"].(string)
	return strings.TrimSpace(email)
}
