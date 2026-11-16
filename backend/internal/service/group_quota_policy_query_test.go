package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupQuotaPolicyQueryUsageWindowsReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		wantWindow bool
	}{
		{"explicit zero", `{"rate_limit":{"primary_window":{"used_percent":0,"limit_window_seconds":18000},"secondary_window":null}}`, true},
		{"missing percentage", `{"rate_limit":{"primary_window":{"limit_window_seconds":18000},"secondary_window":null}}`, false},
		{"null percentage", `{"rate_limit":{"primary_window":{"used_percent":null,"limit_window_seconds":18000},"secondary_window":null}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
				Credentials: map[string]any{"chatgpt_account_id": "test-workspace"}}
			repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{100: account}}
			tokens := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "test-token"}}
			var paths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if r.Method != http.MethodGet || r.URL.Path != "/backend-api/wham/usage" {
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			svc := NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, tokens, nil), newQuotaRedirectingFactory(srv), nil)
			usage, err := svc.QueryUsageWindows(context.Background(), 100)
			require.NoError(t, err)
			require.Positive(t, usage.FetchedAt)
			require.NotNil(t, usage.RateLimit)
			if tc.wantWindow {
				require.NotNil(t, usage.RateLimit.PrimaryWindow)
				require.Zero(t, usage.RateLimit.PrimaryWindow.UsedPercent)
			} else {
				require.Nil(t, usage.RateLimit.PrimaryWindow, "missing data must not be interpreted as unused quota")
			}
			require.Nil(t, usage.RateLimit.SecondaryWindow)
			require.Equal(t, []string{"GET /backend-api/wham/usage"}, paths,
				"background monitoring must not request credit details or consume reset credits")
			require.Zero(t, repo.extraUpdateCalls, "monitoring must not mutate account settings")
		})
	}
}
