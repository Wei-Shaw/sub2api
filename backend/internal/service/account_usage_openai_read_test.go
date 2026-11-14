package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIActiveUsageOnlySendsQuotaGETs(t *testing.T) {
	account := &Account{ID: 8706, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "test-account"}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	tokens := &stubQuotaTokenCache{tokens: map[string]string{OpenAITokenCacheKey(account): "fixture-token"}}
	var reads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.URL.Path, "/wham/")
		require.NotContains(t, r.URL.Path, "/consume")
		require.NotContains(t, r.URL.Path, "/responses")
		if r.URL.Path == "/backend-api/wham/usage" {
			reads.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":44,"limit_window_seconds":18000,"reset_after_seconds":600}}}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	quota := NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, tokens, nil), newQuotaRedirectingFactory(srv), nil)
	svc := &AccountUsageService{accountRepo: repo, cache: NewUsageCache(), openAIQuotaService: quota}
	usage, err := svc.GetActiveUsage(context.Background(), account.ID, false)
	require.NoError(t, err)
	require.Equal(t, 44.0, usage.FiveHour.Utilization)
	require.EqualValues(t, 1, reads.Load())
}

type openAIReadFixture struct {
	calls atomic.Int32
	err   error
}

func (f *openAIReadFixture) QueryUsage(context.Context, int64) (*OpenAIQuotaUsage, error) {
	f.calls.Add(1)
	return &OpenAIQuotaUsage{RateLimit: &OpenAIRateLimit{
		PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 44, LimitWindowSeconds: 18000, ResetAfterSeconds: 600},
		SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 57, LimitWindowSeconds: 604800, ResetAfterSeconds: 3600},
	}}, f.err
}

func TestOpenAIActiveUsageReadsFreshQuotaAndRetainsBackoff(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "upstream_error"}[failed], func(t *testing.T) {
			account := Account{ID: 8705, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Extra: map[string]any{"codex_5h_used_percent": 8.0, "codex_7d_used_percent": 9.0}}
			repo := &accountUsageCodexProbeRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
			reader := &openAIReadFixture{}
			if failed {
				reader.err = errors.New("quota rate limited")
			}
			svc := &AccountUsageService{accountRepo: repo, cache: NewUsageCache(), openAIQuotaService: reader}
			for _, force := range []bool{false, false, true} {
				usage, errs, err := svc.GetUsageBatch(context.Background(), []int64{account.ID}, force, "active")
				require.NoError(t, err)
				if failed {
					require.Contains(t, errs[account.ID], "quota rate limited")
					continue
				}
				require.Empty(t, errs)
				require.Equal(t, 44.0, usage[account.ID].FiveHour.Utilization)
				require.Equal(t, 57.0, usage[account.ID].SevenDay.Utilization)
				require.Equal(t, "active", usage[account.ID].Source)
			}
			if failed {
				require.EqualValues(t, 1, reader.calls.Load())
			} else {
				require.EqualValues(t, 2, reader.calls.Load())
			}
			// An expired entry must actually reread upstream on the next timer poll.
			svc.cache.openAIReadCache.Store(account.ID, &openAIReadUsageCache{at: time.Now().Add(-2 * time.Minute)})
			_, _, err := svc.GetUsageBatch(context.Background(), []int64{account.ID}, false, "active")
			require.NoError(t, err)
			// Caller-owned projections must not modify the repository's old snapshot.
			require.Equal(t, 8.0, account.Extra["codex_5h_used_percent"])
		})
	}
}
