package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryUsageSnapshotOnlyReadsUsage(t *testing.T) {
	account := &Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "workspace"}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: account}}
	tokens := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "fake-token"}}
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"member","account_id":"workspace","rate_limit":{"secondary_window":{"used_percent":0,"limit_window_seconds":604800,"reset_at":1800000000}}}`))
	}))
	defer srv.Close()
	svc := NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, tokens, nil), newQuotaRedirectingFactory(srv))
	usage, err := svc.QueryUsageSnapshot(context.Background(), 100)
	require.NoError(t, err)
	require.Equal(t, []string{"GET /backend-api/wham/usage"}, paths)
	require.True(t, usage.RateLimit.SecondaryWindow.observationFieldsPresent)
	require.Zero(t, usage.RateLimit.SecondaryWindow.UsedPercent)
}

func TestQuotaWindowPresenceDoesNotInventZero(t *testing.T) {
	for _, body := range []string{
		`{"limit_window_seconds":604800,"reset_at":1800000000}`,
		`{"used_percent":null,"limit_window_seconds":604800,"reset_at":1800000000}`,
		`{"used_percent":0,"reset_at":1800000000}`,
		`{"used_percent":0,"limit_window_seconds":604800,"reset_at":null}`,
	} {
		var window OpenAIRateLimitWindow
		require.NoError(t, json.Unmarshal([]byte(body), &window))
		require.False(t, window.observationFieldsPresent, body)
	}
}
