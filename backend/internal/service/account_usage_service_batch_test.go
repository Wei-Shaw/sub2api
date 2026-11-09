package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// Minimal UsageLogRepository stub for batch usage tests (HEAD lacks geminiUsageLogRepoStub).
type usageBatchLogRepoStub struct{}

var _ UsageLogRepository = (*usageBatchLogRepoStub)(nil)

func (r *usageBatchLogRepoStub) Create(context.Context, *UsageLog) (bool, error) {
	return false, nil
}
func (r *usageBatchLogRepoStub) GetByID(context.Context, int64) (*UsageLog, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) Delete(context.Context, int64) error { return nil }
func (r *usageBatchLogRepoStub) ListByUser(context.Context, int64, pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByAPIKey(context.Context, int64, pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByAccount(context.Context, int64, pagination.PaginationParams) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByUserAndTimeRange(context.Context, int64, time.Time, time.Time) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByAPIKeyAndTimeRange(context.Context, int64, time.Time, time.Time) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByAccountAndTimeRange(context.Context, int64, time.Time, time.Time) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) ListByModelAndTimeRange(context.Context, string, time.Time, time.Time) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) GetAccountWindowStats(context.Context, int64, time.Time) (*usagestats.AccountStats, error) {
	return &usagestats.AccountStats{}, nil
}
func (r *usageBatchLogRepoStub) GetAccountTodayStats(context.Context, int64) (*usagestats.AccountStats, error) {
	return &usagestats.AccountStats{}, nil
}
func (r *usageBatchLogRepoStub) GetDashboardStats(context.Context) (*usagestats.DashboardStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUsageTrendWithFilters(context.Context, time.Time, time.Time, string, int64, int64, int64, int64, string, *int16, *bool, *int8) ([]usagestats.TrendDataPoint, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetModelStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8) ([]usagestats.ModelStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetEndpointStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, string, *int16, *bool, *int8) ([]usagestats.EndpointStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUpstreamEndpointStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, string, *int16, *bool, *int8) ([]usagestats.EndpointStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetGroupStatsWithFilters(context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8) ([]usagestats.GroupStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserBreakdownStats(context.Context, time.Time, time.Time, usagestats.UserBreakdownDimension, int) ([]usagestats.UserBreakdownItem, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAllGroupUsageSummary(context.Context, time.Time) ([]usagestats.GroupUsageSummary, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAPIKeyUsageTrend(context.Context, time.Time, time.Time, string, int) ([]usagestats.APIKeyUsageTrendPoint, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserUsageTrend(context.Context, time.Time, time.Time, string, int, string) ([]usagestats.UserUsageTrendPoint, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserSpendingRanking(context.Context, time.Time, time.Time, int) (*usagestats.UserSpendingRankingResponse, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetBatchUserUsageStats(context.Context, []int64, time.Time, time.Time) (map[int64]*usagestats.BatchUserUsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetBatchAPIKeyUsageStats(context.Context, []int64, time.Time, time.Time) (map[int64]*usagestats.BatchAPIKeyUsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserDashboardStats(context.Context, int64) (*usagestats.UserDashboardStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAPIKeyDashboardStats(context.Context, int64) (*usagestats.UserDashboardStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserUsageTrendByUserID(context.Context, int64, time.Time, time.Time, string) ([]usagestats.TrendDataPoint, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserModelStats(context.Context, int64, time.Time, time.Time) ([]usagestats.ModelStat, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, usagestats.UsageLogFilters) ([]UsageLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *usageBatchLogRepoStub) GetGlobalStats(context.Context, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetStatsWithFilters(context.Context, usagestats.UsageLogFilters) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAccountUsageStats(context.Context, int64, time.Time, time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetUserStatsAggregated(context.Context, int64, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAPIKeyStatsAggregated(context.Context, int64, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetAccountStatsAggregated(context.Context, int64, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetModelStatsAggregated(context.Context, string, time.Time, time.Time) (*usagestats.UsageStats, error) {
	return nil, nil
}
func (r *usageBatchLogRepoStub) GetDailyStatsAggregated(context.Context, int64, time.Time, time.Time) ([]map[string]any, error) {
	return nil, nil
}

func TestAccountUsageService_GetUsageBatch_BestEffortByAccount(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)

	repo := &stubOpenAIAccountRepo{
		accounts: []Account{
			{
				ID:       7001,
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"passive_usage_7d_utilization": 0.62,
				},
			},
			{
				ID:       7002,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"codex_usage_updated_at":  time.Now().UTC().Format(time.RFC3339),
					"codex_5h_used_percent":   18.0,
					"codex_5h_reset_at":       resetAt.Format(time.RFC3339),
					"codex_7d_used_percent":   34.0,
					"codex_7d_reset_at":       resetAt.Add(24 * time.Hour).Format(time.RFC3339),
					"workspace_id":            "org-test",
					"chatgpt_account_id":      "acct-test",
					"openai_snapshot_version": "test",
				},
			},
			{
				ID:       7003,
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
			},
		},
	}

	svc := &AccountUsageService{
		accountRepo:  repo,
		usageLogRepo: &usageBatchLogRepoStub{},
		cache:        NewUsageCache(),
	}

	usageByAccount, errorsByAccount, err := svc.GetUsageBatch(context.Background(), []int64{7001, 7002, 7003, 7002}, false, "")
	if err != nil {
		t.Fatalf("GetUsageBatch() error = %v", err)
	}

	if usageByAccount[7001] == nil || usageByAccount[7001].Source != "passive" {
		t.Fatalf("expected anthropic passive usage, got %#v", usageByAccount[7001])
	}

	if usageByAccount[7002] == nil || usageByAccount[7002].FiveHour == nil || usageByAccount[7002].FiveHour.Utilization != 18.0 {
		t.Fatalf("expected openai snapshot usage, got %#v", usageByAccount[7002])
	}

	if !strings.Contains(strings.ToLower(errorsByAccount[7003]), "does not support usage query") {
		t.Fatalf("expected API key account error to be preserved, got %q", errorsByAccount[7003])
	}
}

// Model ClearError's persisted effect so this regression catches both the write
// and mutations of the account returned by the repository.
type usageErrorAccountRepo struct {
	stubOpenAIAccountRepo
	clearCalls int
}

func (r *usageErrorAccountRepo) ClearError(ctx context.Context, id int64) error {
	r.clearCalls++
	account, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	account.Status = StatusActive
	account.ErrorMessage = ""
	return nil
}

func TestAccountUsageService_OpenAIQueriesPreserveRefreshError(t *testing.T) {
	for _, scenario := range []string{"cached_expired_access", "cached_valid_access_invalid_refresh", "failed_probe_missing_access", "probe_throttled"} {
		for _, query := range []string{"usage", "usage_batch", "forced_usage_batch", "today", "today_batch"} {
			t.Run(scenario+"/"+query, func(t *testing.T) {
				const message = "Token refresh failed (non-retryable): refresh_token_invalidated"
				account := Account{
					ID: 7358, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Status: StatusError, ErrorMessage: message,
					Credentials: map[string]any{"refresh_token": "invalid-refresh-token"},
				}
				cache := NewUsageCache()
				if strings.HasPrefix(scenario, "cached_") {
					account.Credentials["access_token"] = "cached-access-token"
					expiresAt := time.Now().Add(-time.Hour)
					if scenario == "cached_valid_access_invalid_refresh" {
						expiresAt = time.Now().Add(time.Hour)
					}
					account.Credentials["expires_at"] = expiresAt.Format(time.RFC3339)
					account.Extra = map[string]any{"codex_5h_used_percent": 18.0, "codex_7d_used_percent": 34.0}
				}
				if scenario == "probe_throttled" {
					cache.openAIProbeCache.Store(account.ID, time.Now())
				}
				// Forced requests take the probe path, but fail locally without
				// credentials; no external network is needed for this regression.
				if query == "forced_usage_batch" {
					delete(account.Credentials, "access_token")
				}
				repo := &usageErrorAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
				svc := &AccountUsageService{accountRepo: repo, usageLogRepo: &usageBatchLogRepoStub{}, cache: cache}
				ctx := context.Background()
				switch query {
				case "usage":
					usage, err := svc.GetUsage(ctx, account.ID)
					if err != nil || usage == nil {
						t.Fatalf("GetUsage: usage=%v err=%v", usage, err)
					}
				case "usage_batch", "forced_usage_batch":
					usage, failures, err := svc.GetUsageBatch(ctx, []int64{account.ID}, query == "forced_usage_batch", "")
					if err != nil || len(failures) != 0 || usage[account.ID] == nil {
						t.Fatalf("GetUsageBatch: usage=%v failures=%v err=%v", usage, failures, err)
					}
				case "today":
					if _, err := svc.GetTodayStats(ctx, account.ID); err != nil {
						t.Fatal(err)
					}
				case "today_batch":
					if _, err := svc.GetTodayStatsBatch(ctx, []int64{account.ID}); err != nil {
						t.Fatal(err)
					}
				}
				stored, err := repo.GetByID(ctx, account.ID)
				if err != nil {
					t.Fatal(err)
				}
				if repo.clearCalls != 0 || stored.Status != StatusError || stored.ErrorMessage != message {
					t.Fatalf("query erased refresh error: clearCalls=%d status=%q error=%q", repo.clearCalls, stored.Status, stored.ErrorMessage)
				}
			})
		}
	}
}

type autoRefreshUsageFetcher struct {
	ClaudeUsageFetcher
	calls    atomic.Int32
	response *ClaudeUsageResponse
	err      error
}

func (f *autoRefreshUsageFetcher) FetchUsageWithOptions(context.Context, *ClaudeUsageFetchOptions) (*ClaudeUsageResponse, error) {
	f.calls.Add(1)
	return f.response, f.err
}

// Active batches use the same upstream cache as the Query button, rather than
// returning the persisted header sample forever. Automatic polling must also
// retain negative caching so an upstream outage cannot create a retry storm.
func TestAccountUsageService_GetUsageBatch_ActiveSource(t *testing.T) {
	t.Parallel()
	for _, cachedError := range []bool{false, true} {
		t.Run(fmt.Sprintf("cached_error_%v", cachedError), func(t *testing.T) {
			account := Account{ID: 8701, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
				Extra:       map[string]any{"passive_usage_7d_utilization": 0.12},
				Credentials: map[string]any{"access_token": "test-token"}}
			repo := &accountUsageCodexProbeRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
			svc := &AccountUsageService{accountRepo: repo, usageLogRepo: &usageBatchLogRepoStub{}, cache: NewUsageCache()}
			upstream := &ClaudeUsageResponse{}
			upstream.SevenDay.Utilization = 43
			upstream.SevenDay.ResetsAt = time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
			fetcher := &autoRefreshUsageFetcher{response: upstream}
			if cachedError {
				fetcher.response = nil
				fetcher.err = errors.New("upstream usage unavailable")
			}
			svc.usageFetcher = fetcher
			for _, source := range []string{"", "passive", "active", "active"} {
				usage, failures, err := svc.GetUsageBatch(t.Context(), []int64{account.ID}, false, source)
				if err != nil {
					t.Fatal(err)
				}
				if source == "active" && cachedError {
					if !strings.Contains(failures[account.ID], "upstream usage unavailable") {
						t.Fatalf("missing negative-cache error: %v", failures)
					}
					continue
				}
				if len(failures) != 0 {
					t.Fatalf("unexpected failures: %v", failures)
				}
				want := 12.0
				if source == "active" {
					want = 43
				}
				if usage[account.ID] == nil || usage[account.ID].SevenDay == nil || usage[account.ID].SevenDay.Utilization != want {
					t.Fatalf("source %q: expected %.0f, got %#v", source, want, usage[account.ID])
				}
			}
			if fetcher.calls.Load() != 1 {
				t.Fatalf("expected one cached upstream read, got %d", fetcher.calls.Load())
			}
		})
	}
}

func TestAccountUsageService_ManualRefreshBypassesSuccessfulClaudeCache(t *testing.T) {
	account := Account{ID: 8702, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Extra: map[string]any{}}
	repo := &accountUsageCodexProbeRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	cache := NewUsageCache()
	old := &ClaudeUsageResponse{}
	old.FiveHour.Utilization = 21
	fresh := &ClaudeUsageResponse{}
	fresh.FiveHour.Utilization = 44
	fetcher := &autoRefreshUsageFetcher{response: fresh}
	svc := &AccountUsageService{accountRepo: repo, usageLogRepo: &usageBatchLogRepoStub{}, cache: cache, usageFetcher: fetcher}
	cache.apiCache.Store(account.ID, &apiUsageCache{response: old, timestamp: time.Now()})
	for _, step := range []struct {
		force bool
		want  float64
		calls int32
	}{{false, 21, 0}, {true, 44, 1}, {false, 44, 1}} {
		usage, failures, err := svc.GetUsageBatch(t.Context(), []int64{account.ID}, step.force, "active")
		if err != nil || len(failures) != 0 {
			t.Fatalf("refresh failed: %v %v", err, failures)
		}
		if usage[account.ID].FiveHour.Utilization != step.want || fetcher.calls.Load() != step.calls {
			t.Fatalf("force=%v got %.0f with %d calls; want %.0f with %d", step.force, usage[account.ID].FiveHour.Utilization, fetcher.calls.Load(), step.want, step.calls)
		}
	}
}

func TestAccountUsageService_ManualRefreshPreservesClaudeErrorBackoff(t *testing.T) {
	account := Account{ID: 8703, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test-token"}}
	repo := &accountUsageCodexProbeRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	cache := NewUsageCache()
	cache.apiCache.Store(account.ID, &apiUsageCache{err: errors.New("upstream rate limited"), timestamp: time.Now()})
	fetcher := &autoRefreshUsageFetcher{}
	svc := &AccountUsageService{accountRepo: repo, usageLogRepo: &usageBatchLogRepoStub{}, cache: cache, usageFetcher: fetcher}
	_, failures, err := svc.GetUsageBatch(t.Context(), []int64{account.ID}, true, "active")
	if err != nil || !strings.Contains(failures[account.ID], "rate limited") || fetcher.calls.Load() != 0 {
		t.Fatalf("error backoff bypassed: %v %v", err, failures)
	}
}
