//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func syntheticOpenAIExpiryToken(exp time.Time) string {
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d,"aud":"synthetic"}`, exp.Unix()))) + ".c3ludGhldGlj"
}

type openAIExpiryExecutor struct{ *refreshAPIExecutorStub }

func (e openAIExpiryExecutor) NeedsRefresh(account *Account, window time.Duration) bool {
	return (&OpenAITokenRefresher{}).NeedsRefresh(account, window)
}

func TestOpenAIOAuthExpiryWithoutMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		wantRefresh bool
	}{
		{"expired_jwt", syntheticOpenAIExpiryToken(time.Now().Add(-time.Hour)), true},
		{"fresh_jwt", syntheticOpenAIExpiryToken(time.Now().Add(time.Hour)), false},
		{"unknown_expiry", "opaque-synthetic-token", true},
		{"malformed_jwt", "a.invalid.c", true},
		{"jwt_without_exp", "eyJhbGciOiJub25lIn0.e30.c3ludGhldGlj", true},
		{"jwt_invalid_exp", "eyJhbGciOiJub25lIn0.eyJleHAiOiJiYWQifQ.c3ludGhldGlj", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := newOpenAIOAuthRecoveryAccount()
			delete(account.Credentials, "expires_at")
			account.Credentials["access_token"] = tc.token
			require.Equal(t, tc.wantRefresh, (&OpenAITokenRefresher{}).NeedsRefresh(account, 5*time.Minute))
			cache := newOpenAITokenCacheStub()
			require.NoError(t, cache.SetAccessToken(context.Background(), OpenAITokenCacheKey(account), tc.token, time.Hour))
			repo := &refreshAPIAccountRepo{account: account}
			executor := openAIExpiryExecutor{&refreshAPIExecutorStub{credentials: map[string]any{
				"access_token":  "fresh-token",
				"refresh_token": "rotated-rt",
				"expires_at":    time.Now().Add(time.Hour).Format(time.RFC3339),
			}}}
			provider := NewOpenAITokenProvider(repo, cache, nil)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
			token, err := provider.GetAccessToken(context.Background(), account)
			require.NoError(t, err)
			if tc.wantRefresh {
				require.Equal(t, "fresh-token", token)
				require.Equal(t, 1, executor.refreshCalls)
				require.NotEmpty(t, repo.account.GetCredential("expires_at"))
			} else {
				require.Equal(t, tc.token, token)
				require.Zero(t, executor.refreshCalls)
			}
		})
	}
}

func TestOpenAIOAuthExpiryMetadataPrecedenceAndExclusions(t *testing.T) {
	account := newOpenAIOAuthRecoveryAccount()
	account.Credentials["access_token"] = syntheticOpenAIExpiryToken(time.Now().Add(-time.Hour))
	require.False(t, (&OpenAITokenRefresher{}).NeedsRefresh(account, 5*time.Minute))
	delete(account.Credentials, "expires_at")
	delete(account.Credentials, "refresh_token")
	require.False(t, (&OpenAITokenRefresher{}).NeedsRefresh(account, 5*time.Minute))
	account.Credentials["refresh_token"] = "stale-rt"
	account.Credentials["auth_mode"] = OpenAIAuthModePersonalAccessToken
	require.False(t, (&OpenAITokenRefresher{}).NeedsRefresh(account, 5*time.Minute))
}
