//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeSetupTokenRefreshAPI mock claudeSetupTokenRefreshAPI，避免依赖真实 OAuthService
type fakeSetupTokenRefreshAPI struct {
	tokenInfo  *TokenInfo
	err        error
	gotAccount *Account
}

func (f *fakeSetupTokenRefreshAPI) RefreshAccountToken(_ context.Context, account *Account) (*TokenInfo, error) {
	f.gotAccount = account
	if f.err != nil {
		return nil, f.err
	}
	return f.tokenInfo, nil
}

func TestClaudeSetupTokenRefresher_CanRefresh(t *testing.T) {
	refresher := &ClaudeSetupTokenRefresher{}

	tests := []struct {
		name     string
		platform string
		accType  string
		want     bool
	}{
		{
			name:     "anthropic setup-token - can refresh",
			platform: PlatformAnthropic,
			accType:  AccountTypeSetupToken,
			want:     true,
		},
		{
			name:     "anthropic oauth - cannot refresh (handled by ClaudeTokenRefresher)",
			platform: PlatformAnthropic,
			accType:  AccountTypeOAuth,
			want:     false,
		},
		{
			name:     "anthropic api-key - cannot refresh",
			platform: PlatformAnthropic,
			accType:  AccountTypeAPIKey,
			want:     false,
		},
		{
			name:     "openai setup-token - cannot refresh",
			platform: PlatformOpenAI,
			accType:  AccountTypeSetupToken,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: tt.platform,
				Type:     tt.accType,
			}

			require.Equal(t, tt.want, refresher.CanRefresh(account))
		})
	}
}

func TestClaudeSetupTokenRefresher_NeedsRefresh(t *testing.T) {
	refresher := &ClaudeSetupTokenRefresher{}
	refreshWindow := 30 * time.Minute

	withinWindow := time.Now().Add(15 * time.Minute).Unix()
	outsideWindow := time.Now().Add(1 * time.Hour).Unix()

	tests := []struct {
		name        string
		credentials map[string]any
		wantRefresh bool
	}{
		{
			name: "expired - needs refresh",
			credentials: map[string]any{
				"expires_at": "1000", // 1970-01-01 00:16:40 UTC, 已过期
			},
			wantRefresh: true,
		},
		{
			name: "within refresh window - needs refresh",
			credentials: map[string]any{
				"expires_at": strconv.FormatInt(withinWindow, 10),
			},
			wantRefresh: true,
		},
		{
			name: "outside refresh window - no refresh",
			credentials: map[string]any{
				"expires_at": strconv.FormatInt(outsideWindow, 10),
			},
			wantRefresh: false,
		},
		{
			name:        "expires_at missing - no refresh",
			credentials: map[string]any{},
			wantRefresh: false,
		},
		{
			name: "expires_at is invalid string - no refresh",
			credentials: map[string]any{
				"expires_at": "invalid",
			},
			wantRefresh: false,
		},
		{
			name:        "credentials is nil - no refresh",
			credentials: nil,
			wantRefresh: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformAnthropic,
				Type:        AccountTypeSetupToken,
				Credentials: tt.credentials,
			}

			require.Equal(t, tt.wantRefresh, refresher.NeedsRefresh(account, refreshWindow))
		})
	}
}

func TestClaudeSetupTokenRefresher_Refresh(t *testing.T) {
	newExpiresAt := time.Now().Add(24 * time.Hour).Unix()
	api := &fakeSetupTokenRefreshAPI{
		tokenInfo: &TokenInfo{
			AccessToken:  "new-access-token",
			TokenType:    "Bearer",
			ExpiresIn:    86400,
			ExpiresAt:    newExpiresAt,
			RefreshToken: "new-refresh-token",
			Scope:        "user:inference",
		},
	}
	refresher := &ClaudeSetupTokenRefresher{oauthService: api}

	account := &Account{
		ID:       41,
		Platform: PlatformAnthropic,
		Type:     AccountTypeSetupToken,
		Credentials: map[string]any{
			"access_token":      "old-access-token",
			"refresh_token":     "old-refresh-token",
			"expires_at":        "1000",
			"scope":             "user:inference",
			"subscription_type": "max", // 非 token 字段，刷新后必须保留
		},
	}

	newCredentials, err := refresher.Refresh(context.Background(), account)
	require.NoError(t, err)
	require.Same(t, account, api.gotAccount, "should refresh with the given account")

	// token 相关字段被更新
	require.Equal(t, "new-access-token", newCredentials["access_token"])
	require.Equal(t, "new-refresh-token", newCredentials["refresh_token"])
	require.Equal(t, strconv.FormatInt(newExpiresAt, 10), newCredentials["expires_at"])
	require.Equal(t, "86400", newCredentials["expires_in"])
	require.Equal(t, "Bearer", newCredentials["token_type"])
	require.Equal(t, "user:inference", newCredentials["scope"])

	// 原有非 token 字段被保留
	require.Equal(t, "max", newCredentials["subscription_type"])
}

func TestClaudeSetupTokenRefresher_Refresh_Error(t *testing.T) {
	api := &fakeSetupTokenRefreshAPI{err: errors.New("invalid_grant")}
	refresher := &ClaudeSetupTokenRefresher{oauthService: api}

	account := &Account{
		Platform:    PlatformAnthropic,
		Type:        AccountTypeSetupToken,
		Credentials: map[string]any{"refresh_token": "old-refresh-token"},
	}

	newCredentials, err := refresher.Refresh(context.Background(), account)
	require.Error(t, err)
	require.Nil(t, newCredentials)
}

func TestClaudeSetupTokenRefresher_CacheKey(t *testing.T) {
	refresher := &ClaudeSetupTokenRefresher{}
	account := &Account{ID: 41}

	require.Equal(t, ClaudeTokenCacheKey(account), refresher.CacheKey(account))
}
