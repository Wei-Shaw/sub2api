package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
)

const cursorTokenRefreshSkew = 5 * time.Minute

var errCursorRefreshTokenMissing = errors.New("cursor: missing refresh_token")

// CursorTokenRefresher refreshes Cursor OAuth access tokens.
type CursorTokenRefresher struct {
	httpClient *http.Client
	refresh    func(ctx context.Context, refreshToken string) (*cursor.TokenRefreshResult, error)
}

// NewCursorTokenRefresher creates a Cursor token refresher.
func NewCursorTokenRefresher() *CursorTokenRefresher {
	return &CursorTokenRefresher{}
}

// CacheKey returns the distributed-lock cache key.
func (r *CursorTokenRefresher) CacheKey(account *Account) string {
	return CursorTokenCacheKey(account)
}

// CanRefresh reports whether this refresher can handle the account.
func (r *CursorTokenRefresher) CanRefresh(account *Account) bool {
	if account == nil || account.IsCredentialShadow() {
		return false
	}
	return account.Platform == PlatformCursor &&
		account.Type == AccountTypeOAuth &&
		strings.TrimSpace(account.GetCredential("refresh_token")) != ""
}

// NeedsRefresh reports whether the access token is missing or inside the refresh window.
func (r *CursorTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	if strings.TrimSpace(normalizeCursorAccessToken(account.GetCredential("access_token"))) == "" {
		return true
	}
	expiresAt := cursorAccessTokenExpiresAt(account)
	if expiresAt == nil {
		return true
	}
	if refreshWindow < 0 {
		refreshWindow = 0
	}
	return time.Until(*expiresAt) < refreshWindow
}

// Refresh exchanges the stored refresh token and returns merged credentials.
func (r *CursorTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if account == nil {
		return nil, errors.New("cursor: account is nil")
	}
	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	if refreshToken == "" {
		return nil, errCursorRefreshTokenMissing
	}

	result, err := r.doRefresh(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	return applyCursorTokenCredentials(account, result), nil
}

func (r *CursorTokenRefresher) doRefresh(ctx context.Context, refreshToken string) (*cursor.TokenRefreshResult, error) {
	if r != nil && r.refresh != nil {
		return r.refresh(ctx, refreshToken)
	}
	var client *http.Client
	if r != nil {
		client = r.httpClient
	}
	return cursor.RefreshSession(ctx, client, refreshToken)
}

func CursorTokenCacheKey(account *Account) string {
	if account == nil {
		return "cursor:account:0"
	}
	return "cursor:account:" + strconv.FormatInt(account.ID, 10)
}

func cursorAccessTokenExpiresAt(account *Account) *time.Time {
	if account == nil {
		return nil
	}
	if expiresAt := account.GetCredentialAsTime("expires_at"); expiresAt != nil {
		return expiresAt
	}
	return cursor.AccessTokenExpiry(account.GetCredential("access_token"))
}

func applyCursorTokenCredentials(account *Account, result *cursor.TokenRefreshResult) map[string]any {
	if result == nil {
		return MergeCredentials(account.Credentials, nil)
	}
	now := time.Now()
	creds := map[string]any{
		"access_token": normalizeCursorAccessToken(result.AccessToken),
		"expires_at":   result.ExpiresAt(now).UTC().Format(time.RFC3339),
	}
	if strings.TrimSpace(result.RefreshToken) != "" {
		creds["refresh_token"] = strings.TrimSpace(result.RefreshToken)
	}
	if account == nil {
		return creds
	}
	return MergeCredentials(account.Credentials, creds)
}

func isCursorAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "status 401") {
		return true
	}
	return strings.Contains(s, "unauthenticated") ||
		strings.Contains(s, "unauthorized") ||
		strings.Contains(s, "invalid_grant") ||
		strings.Contains(s, "not authenticated") ||
		strings.Contains(s, "error_unauthorized")
}

func copyCursorAccountCredentials(dst, src *Account) {
	if dst == nil || src == nil {
		return
	}
	dst.Credentials = shallowCopyMap(src.Credentials)
}

func (s *CursorGatewayService) ensureCursorAccessToken(ctx context.Context, account *Account) error {
	return s.refreshCursorAccount(ctx, account, false)
}

func (s *CursorGatewayService) refreshCursorAccount(ctx context.Context, account *Account, force bool) error {
	if s == nil || account == nil {
		return nil
	}
	refresher := s.refresher
	if refresher == nil {
		refresher = NewCursorTokenRefresher()
		s.refresher = refresher
	}
	if !refresher.CanRefresh(account) {
		return nil
	}
	if !force && !refresher.NeedsRefresh(account, cursorTokenRefreshSkew) {
		return nil
	}

	if s.refreshAPI != nil && !force {
		result, err := s.refreshAPI.RefreshIfNeeded(withOAuthRefreshRequestPath(ctx), account, refresher, cursorTokenRefreshSkew)
		if err != nil {
			return err
		}
		if result != nil && result.Account != nil {
			copyCursorAccountCredentials(account, result.Account)
		}
		return nil
	}

	creds, err := refresher.Refresh(ctx, account)
	if err != nil {
		return err
	}
	account.Credentials = shallowCopyMap(creds)
	if err := persistAccountCredentials(ctx, s.accountRepo, account, creds); err != nil {
		return fmt.Errorf("cursor: persist refreshed credentials: %w", err)
	}
	return nil
}
