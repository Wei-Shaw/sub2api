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

func (e openAIExpiryExecutor) NeedsRefresh(a *Account, window time.Duration) bool {
	return (&OpenAITokenRefresher{}).NeedsRefresh(a, window)
}

func TestOpenAIOAuthExpiryWithoutMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		want        bool
	}{
		{"expired_jwt", syntheticOpenAIExpiryToken(time.Now().Add(-time.Hour)), true},
		{"fresh_jwt", syntheticOpenAIExpiryToken(time.Now().Add(time.Hour)), false},
		{"unknown_expiry", "opaque-synthetic-token", true},
		{"malformed_jwt", "a.invalid.c", true},
		{"jwt_without_exp", "eyJhbGciOiJub25lIn0.e30.c3ludGhldGlj", true},
		{"jwt_invalid_exp", "eyJhbGciOiJub25lIn0.eyJleHAiOiJiYWQifQ.c3ludGhldGlj", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newOpenAI401TestAccount()
			delete(a.Credentials, "expires_at")
			a.Credentials["access_token"] = tc.token
			require.Equal(t, tc.want, (&OpenAITokenRefresher{}).NeedsRefresh(a, 5*time.Minute))
			cache := newOpenAITokenCacheStub()
			require.NoError(t, cache.SetAccessToken(context.Background(), OpenAITokenCacheKey(a), tc.token, time.Hour))
			repo := &refreshAPIAccountRepo{account: a}
			executor := openAIExpiryExecutor{&refreshAPIExecutorStub{credentials: map[string]any{"access_token": "fresh-token", "refresh_token": "rotated-rt", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)}}}
			provider := NewOpenAITokenProvider(repo, cache, nil)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
			token, err := provider.GetAccessToken(context.Background(), a)
			require.NoError(t, err)
			if tc.want {
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
	a := newOpenAI401TestAccount()
	a.Credentials["access_token"] = syntheticOpenAIExpiryToken(time.Now().Add(-time.Hour))
	require.False(t, (&OpenAITokenRefresher{}).NeedsRefresh(a, 5*time.Minute))
	delete(a.Credentials, "expires_at")
	delete(a.Credentials, "refresh_token")
	require.False(t, (&OpenAITokenRefresher{}).NeedsRefresh(a, 5*time.Minute))
	a.Credentials["refresh_token"] = "stale-rt"
	a.Credentials["auth_mode"] = OpenAIAuthModePersonalAccessToken
	require.False(t, (&OpenAITokenRefresher{}).NeedsRefresh(a, 5*time.Minute))
}
