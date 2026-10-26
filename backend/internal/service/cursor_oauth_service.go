package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
)

// CursorOAuthService drives Cursor's deep-control browser login: the admin
// opens the generated URL, authenticates on cursor.com, and sub2api polls
// until Cursor releases an access/refresh token pair for the CLI client.
type CursorOAuthService struct {
	proxyRepo ProxyRepository
	// httpClientFor overrides the transport factory in tests.
	httpClientFor func(proxyURL string) (*http.Client, error)
}

// NewCursorOAuthService creates the Cursor OAuth service. proxyRepo is
// optional; when present, poll requests dial through the given proxy.
func NewCursorOAuthService(proxyRepo ProxyRepository) *CursorOAuthService {
	return &CursorOAuthService{proxyRepo: proxyRepo}
}

// CursorOAuthStartResult is one login attempt: the browser URL plus the
// PKCE/uuid pair the poll endpoint needs.
type CursorOAuthStartResult struct {
	URL      string `json:"url"`
	UUID     string `json:"uuid"`
	Verifier string `json:"verifier"`
}

func (s *CursorOAuthService) StartAuth(ctx context.Context) (*CursorOAuthStartResult, error) {
	_ = ctx // login params are generated locally; only polling hits the network
	params, err := cursor.GenerateAuthParams()
	if err != nil {
		return nil, err
	}
	return &CursorOAuthStartResult{
		URL:      params.LoginURL,
		UUID:     params.UUID,
		Verifier: params.Verifier,
	}, nil
}

// ErrCursorAuthPending reports the browser login has not completed yet.
var ErrCursorAuthPending = errors.New("cursor auth pending")

// CursorOAuthPollResult reports the login state. When Done, Credentials holds
// the ready-to-store credential map (access/refresh tokens, token_kind,
// expires_at).
type CursorOAuthPollResult struct {
	Done        bool           `json:"done"`
	Credentials map[string]any `json:"credentials,omitempty"`
}

func (s *CursorOAuthService) PollAuth(ctx context.Context, uuidStr, verifier string, proxyID *int64) (*CursorOAuthPollResult, error) {
	if uuidStr == "" || verifier == "" {
		return nil, fmt.Errorf("cursor: uuid and verifier are required")
	}
	proxyURL, err := s.proxyURLFor(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	httpClientFor := s.httpClientFor
	if httpClientFor == nil {
		httpClientFor = cursor.UnaryHTTPClient
	}
	client, err := httpClientFor(proxyURL)
	if err != nil {
		return nil, err
	}
	session, err := cursor.PollAuthSession(ctx, client, uuidStr, verifier)
	if err != nil {
		if errors.Is(err, cursor.ErrAuthPending) {
			return &CursorOAuthPollResult{Done: false}, nil
		}
		return nil, err
	}

	creds := map[string]any{
		"access_token": normalizeCursorAccessToken(session.AccessToken),
		"token_kind":   cursor.TokenKindDeepControl,
	}
	if session.RefreshToken != "" {
		creds["refresh_token"] = session.RefreshToken
	}
	if expiry := cursor.AccessTokenExpiry(session.AccessToken); expiry != nil {
		creds["expires_at"] = expiry.UTC().Format(time.RFC3339)
	}
	return &CursorOAuthPollResult{Done: true, Credentials: creds}, nil
}

func (s *CursorOAuthService) proxyURLFor(ctx context.Context, proxyID *int64) (string, error) {
	if s == nil || s.proxyRepo == nil || proxyID == nil || *proxyID <= 0 {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", fmt.Errorf("cursor: load proxy: %w", err)
	}
	if proxy == nil {
		return "", fmt.Errorf("cursor: proxy %d not found", *proxyID)
	}
	return proxy.URL(), nil
}

// SetHTTPClientFactoryForTest swaps the transport factory. Test seam only.
func (s *CursorOAuthService) SetHTTPClientFactoryForTest(f func(proxyURL string) (*http.Client, error)) {
	s.httpClientFor = f
}
