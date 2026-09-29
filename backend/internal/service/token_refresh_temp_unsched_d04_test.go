//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func d04TempUnschedStateJSON(t *testing.T, statusCode int) string {
	t.Helper()
	raw, err := json.Marshal(TempUnschedState{StatusCode: statusCode, MatchedKeyword: "quota"})
	require.NoError(t, err)
	return string(raw)
}

// A successful token refresh only resolves auth-caused temp-unschedulable
// states; penalties for unrelated upstream errors must keep running.
func TestTokenRefreshService_RefreshWithRetry_TempUnschedClearOnlyForAuthReasons(t *testing.T) {
	cases := []struct {
		name      string
		reason    string
		wantClear bool
	}{
		{name: "no reason", reason: "", wantClear: true},
		{name: "oauth 401", reason: "OAuth 401: unauthorized", wantClear: true},
		{name: "auth failed 401", reason: "Authentication failed (401): invalid or expired credentials", wantClear: true},
		{name: "refresh retry exhausted", reason: "token refresh retry exhausted", wantClear: true},
		{name: "request path refresh failure", reason: "token refresh failed on request path: boom", wantClear: true},
		{name: "401 rule state", reason: d04TempUnschedStateJSON(t, http.StatusUnauthorized), wantClear: true},
		{name: "internal 500 penalty", reason: "INTERNAL 500 x2 (temp unsched 2h0m0s)", wantClear: false},
		{name: "503 rule state", reason: d04TempUnschedStateJSON(t, http.StatusServiceUnavailable), wantClear: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &tokenRefreshAccountRepo{}
			tempCache := &tempUnschedCacheStub{}
			cfg := &config.Config{TokenRefresh: config.TokenRefreshConfig{MaxRetries: 1, RetryBackoffSeconds: 0}}
			svc := NewTokenRefreshService(repo, nil, nil, nil, nil, &tokenCacheInvalidatorStub{}, nil, cfg, tempCache)
			until := time.Now().Add(2 * time.Hour)
			account := &Account{
				ID:                      16,
				Platform:                PlatformGemini,
				Type:                    AccountTypeOAuth,
				TempUnschedulableUntil:  &until,
				TempUnschedulableReason: tc.reason,
			}
			refresher := &tokenRefresherStub{credentials: map[string]any{"access_token": "new-token"}}

			require.NoError(t, svc.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour))

			want := 0
			if tc.wantClear {
				want = 1
			}
			require.Equal(t, want, repo.clearTempCalls)
			require.Equal(t, want, tempCache.deleteCalls)
		})
	}
}
