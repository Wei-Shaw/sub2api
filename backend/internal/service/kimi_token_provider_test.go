//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/stretchr/testify/require"
)

func TestKimiTokenProviderRefreshesExpiredTokenOnRequestPath(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       54,
		Platform: PlatformKimi,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiredAt,
			"base_url":      "https://api.kimi.com/coding/v1",
			"device_id":     "device-xyz",
			"client_id":     "client-id",
		},
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{54: account}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	oauthSvc := NewKimiOAuthService(nil, &kimiOAuthClientStub{
		refreshResponse: &kimi.TokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "rotated-refresh-token",
			TokenType:    "Bearer",
			ExpiresIn:    900,
		},
	})
	defer oauthSvc.Stop()

	provider := NewKimiTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewKimiTokenRefresher(oauthSvc))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "new-access-token", token)
	require.Equal(t, 1, repo.updateCredentialsCalls)

	updated := repo.accountsByID[54]
	require.Equal(t, "new-access-token", updated.GetKimiAccessToken())
	require.Equal(t, "rotated-refresh-token", updated.GetKimiRefreshToken(), "refresh_token 轮换值必须写回")
	require.Equal(t, "https://api.kimi.com/coding/v1", updated.GetKimiBaseURL(), "base_url 应保留旧凭据")
	require.Equal(t, "device-xyz", updated.GetKimiDeviceID(), "device_id 应保留旧凭据")

	require.Equal(t, "kimi:account:54", cache.setKey)
	require.Equal(t, "new-access-token", cache.setToken)
	require.Greater(t, cache.setTTL, time.Duration(0))
	require.Equal(t, 1, cache.releaseCalls)
}

func TestKimiTokenProviderKeepsRefreshTokenWhenUpstreamDoesNotRotate(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       55,
		Platform: PlatformKimi,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
			"refresh_token": "original-refresh-token",
			"expires_at":    expiredAt,
			"device_id":     "device-xyz",
		},
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{55: account}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	oauthSvc := NewKimiOAuthService(nil, &kimiOAuthClientStub{
		refreshResponse: &kimi.TokenResponse{
			AccessToken: "new-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   900,
		},
	})
	defer oauthSvc.Stop()

	provider := NewKimiTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewKimiTokenRefresher(oauthSvc))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "new-access-token", token)
	require.Equal(t, "original-refresh-token", repo.accountsByID[55].GetKimiRefreshToken(), "未轮换时保留旧 refresh_token")
}

func TestKimiTokenProviderRefreshFailureUnschedulesWithRedactedReason(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       56,
		Platform: PlatformKimi,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiredAt,
		},
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{56: account}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	tempCache := &tempUnschedCacheStub{}
	provider := NewKimiTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: errors.New("temporary refresh failure access_token=leaked-access refresh_token=leaked-refresh"),
	})
	provider.SetTempUnschedCache(tempCache)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	require.Equal(t, 1, repo.setTempUnschedCalls)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Contains(t, repo.lastTempUnschedReason, "access_token=***")
	require.Contains(t, repo.lastTempUnschedReason, "refresh_token=***")
	require.NotContains(t, repo.lastTempUnschedReason, "leaked-access")
	require.NotContains(t, repo.lastTempUnschedReason, "leaked-refresh")
	require.Equal(t, 1, tempCache.setCalls)
	require.NotNil(t, tempCache.lastState)
	require.NotContains(t, tempCache.lastState.ErrorMessage, "leaked-access")
	require.NotContains(t, tempCache.lastState.ErrorMessage, "leaked-refresh")
}

func TestKimiTokenProviderNonRetryableRefreshFailureSetsErrorStatus(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       57,
		Platform: PlatformKimi,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiredAt,
		},
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{57: account}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	tempCache := &tempUnschedCacheStub{}
	provider := NewKimiTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: errors.New("kimi_oauth_token_invalid: refresh token rejected"),
	})
	provider.SetTempUnschedCache(tempCache)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	// 不可重试错误（refresh_token 被上游拒绝）直接置 error 状态，不做临时停调度
	require.Equal(t, 1, repo.setErrorCalls)
	require.Equal(t, 0, repo.setTempUnschedCalls)
	require.Equal(t, 0, tempCache.setCalls)
	require.Contains(t, repo.lastErrorMessage, "kimi token refresh failed (non-retryable)")
}

func TestKimiTokenProviderReturnsCachedTokenWithoutRefresh(t *testing.T) {
	account := &Account{
		ID:       58,
		Platform: PlatformKimi,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &tokenRefreshAccountRepo{}
	repo.accountsByID = map[int64]*Account{58: account}
	cache := &grokTokenCacheForProviderTest{token: "cached-access-token"}
	provider := NewKimiTokenProvider(repo, cache)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "cached-access-token", token)
	require.Equal(t, 0, repo.updateCredentialsCalls)
}

func TestKimiTokenCacheKeyUsesAccountID(t *testing.T) {
	require.Equal(t, "kimi:account:42", KimiTokenCacheKey(&Account{ID: 42}))
	require.Equal(t, "kimi:account:0", KimiTokenCacheKey(nil))
}

func TestKimiTokenRefresherRefreshMergesCredentials(t *testing.T) {
	account := &Account{
		ID:       59,
		Platform: PlatformKimi,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "old-access-token",
			"refresh_token": "old-refresh-token",
			"expires_at":    time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
			"base_url":      kimi.DefaultBaseURL,
			"device_id":     "device-xyz",
		},
	}
	oauthSvc := NewKimiOAuthService(nil, &kimiOAuthClientStub{
		refreshResponse: &kimi.TokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "rotated-refresh-token",
			ExpiresIn:    900,
		},
	})
	defer oauthSvc.Stop()

	refresher := NewKimiTokenRefresher(oauthSvc)
	require.True(t, refresher.CanRefresh(account))
	require.True(t, refresher.NeedsRefresh(account, time.Minute))
	require.Equal(t, "kimi:account:59", refresher.CacheKey(account))

	newCredentials, err := refresher.Refresh(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "new-access-token", newCredentials["access_token"])
	require.Equal(t, "rotated-refresh-token", newCredentials["refresh_token"])
	require.Equal(t, kimi.DefaultBaseURL, newCredentials["base_url"])
	require.Equal(t, "device-xyz", newCredentials["device_id"])
}
