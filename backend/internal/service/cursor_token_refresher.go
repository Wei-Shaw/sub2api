package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

const cursorTokenRefreshSkew = 10 * time.Minute

type CursorTokenRefresher struct {
	cursorOAuthService *CursorOAuthService
}

func NewCursorTokenRefresher(cursorOAuthService *CursorOAuthService) *CursorTokenRefresher {
	return &CursorTokenRefresher{cursorOAuthService: cursorOAuthService}
}

func CursorTokenCacheKey(account *Account) string {
	if account == nil {
		return "cursor:account:0"
	}
	return "cursor:account:" + strconv.FormatInt(account.ID, 10)
}

func (r *CursorTokenRefresher) CacheKey(account *Account) string {
	return CursorTokenCacheKey(account)
}

func (r *CursorTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.IsCursorOAuth() && strings.TrimSpace(account.GetCursorRefreshToken()) != ""
}

func (r *CursorTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	if strings.TrimSpace(account.GetCursorAccessToken()) == "" {
		return true
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}
	if refreshWindow < cursorTokenRefreshSkew {
		refreshWindow = cursorTokenRefreshSkew
	}
	return time.Until(*expiresAt) < refreshWindow
}

func (r *CursorTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.cursorOAuthService == nil {
		return nil, errors.New("cursor oauth service is not configured")
	}
	tokenInfo, err := r.cursorOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	return MergeCredentials(account.Credentials, r.cursorOAuthService.BuildAccountCredentials(tokenInfo)), nil
}
