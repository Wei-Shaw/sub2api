package dataset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// persistingTokenSource wraps OAuth refreshes and atomically persists the latest
// token so a restart never falls back to an expired access token on disk.
type persistingTokenSource struct {
	mu        sync.Mutex
	refreshMu sync.Mutex
	ctx       context.Context
	config    *oauth2.Config
	source    oauth2.TokenSource
	token     *oauth2.Token
	tokenPath string
}

func newOAuthHTTPClient(ctx context.Context, source *persistingTokenSource) *http.Client {
	client := oauth2.NewClient(ctx, nil)
	client.Transport = &oauth2.Transport{Source: source, Base: client.Transport}
	return client
}

func newPersistingTokenSource(ctx context.Context, config *oauth2.Config, token *oauth2.Token, tokenPath string) (*persistingTokenSource, error) {
	if config == nil {
		return nil, fmt.Errorf("OAuth config is required")
	}
	if token == nil {
		return nil, fmt.Errorf("OAuth token is required")
	}
	if tokenPath == "" {
		return nil, fmt.Errorf("OAuth token persistence path is required")
	}
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("OAuth refresh token is required")
	}

	// Older token files may contain expires_in without an absolute expiry. It is
	// relative to an unknown base, so treat it as stale and refresh immediately.
	if token.Expiry.IsZero() {
		token.Expiry = time.Now().Add(-time.Minute)
	}
	source := oauth2.ReuseTokenSource(token, config.TokenSource(ctx, token))
	return &persistingTokenSource{
		ctx:       ctx,
		config:    config,
		source:    source,
		token:     token,
		tokenPath: tokenPath,
	}, nil
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	source := s.source
	s.mu.Unlock()

	token, err := source.Token()
	if err != nil {
		return nil, safeOAuthRefreshError("OAuth token refresh failed", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.source != source && s.token != nil && s.token.Valid() {
		return s.token, nil
	}
	if token.RefreshToken == "" && s.token != nil {
		token.RefreshToken = s.token.RefreshToken
	}
	if token.Expiry.IsZero() {
		return nil, fmt.Errorf("OAuth token source returned token without expiry")
	}
	if err := writeOAuthTokenAtomically(s.tokenPath, token); err != nil {
		return nil, fmt.Errorf("persist refreshed OAuth token: %w", err)
	}
	s.token = token
	return token, nil
}

func (s *persistingTokenSource) ForceRefresh() (*oauth2.Token, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	s.mu.Lock()
	if s.token == nil || s.token.RefreshToken == "" {
		s.mu.Unlock()
		return nil, fmt.Errorf("OAuth refresh token is unavailable")
	}
	refreshToken := *s.token
	s.mu.Unlock()

	// Do not hold mu during token acquisition: callers can use the same
	// TokenSource for authenticated API calls while the refresh request runs.
	refreshToken.Expiry = time.Now().Add(-time.Minute)
	refreshed, err := s.config.TokenSource(s.ctx, &refreshToken).Token()
	if err != nil {
		return nil, safeOAuthRefreshError("refresh OAuth access token failed", err)
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = refreshToken.RefreshToken
	}
	if refreshed.Expiry.IsZero() {
		return nil, fmt.Errorf("OAuth refresh returned token without expiry")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeOAuthTokenAtomically(s.tokenPath, refreshed); err != nil {
		return nil, fmt.Errorf("persist refreshed OAuth token: %w", err)
	}
	s.token = refreshed
	s.source = oauth2.ReuseTokenSource(refreshed, s.config.TokenSource(s.ctx, refreshed))
	return refreshed, nil
}

func safeOAuthRefreshError(message string, err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: canceled", message)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: deadline exceeded", message)
	}
	return errors.New(message)
}

func writeOAuthTokenAtomically(path string, token *oauth2.Token) error {
	return writeOAuthTokenAtomicallyWithRename(path, token, os.Rename)
}

func writeOAuthTokenAtomicallyWithRename(path string, token *oauth2.Token, renameFile func(string, string) error) error {
	if token == nil {
		return fmt.Errorf("OAuth token is required")
	}
	if renameFile == nil {
		return fmt.Errorf("OAuth token rename operation is required")
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.Expiry.IsZero() {
		return fmt.Errorf("OAuth token must include access token, refresh token, and expiry")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}

	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("encode OAuth token: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".oauth-token-*")
	if err != nil {
		return fmt.Errorf("create temporary token file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set token permissions: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary token: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary token: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary token: %w", err)
	}
	if err := renameFile(tmpPath, path); err != nil {
		return fmt.Errorf("replace OAuth token: %w", err)
	}
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		err = dir.Sync()
		closeErr := dir.Close()
		if err != nil {
			return fmt.Errorf("sync OAuth token directory: %w", err)
		}
		if closeErr != nil {
			return fmt.Errorf("close OAuth token directory: %w", closeErr)
		}
	}
	return nil
}

var _ oauth2.TokenSource = (*persistingTokenSource)(nil)
