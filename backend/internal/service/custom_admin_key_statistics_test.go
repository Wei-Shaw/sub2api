//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func scopedStatisticsServiceContext(action string) context.Context {
	return adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{action}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}}}})
}
func TestScopedAdminUsageServiceRechecksScopeBeforeSnapshot(t *testing.T) {
	a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{2}, Extra: map[string]any{"codex_5h_used_percent": 25.0}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{1: a}}
	svc := &AccountUsageService{accountRepo: repo}
	u, err := svc.getUsageForAccount(scopedStatisticsServiceContext("accounts.usage.read"), a, false)
	require.Nil(t, u)
	require.Equal(t, 404, infraerrors.Code(err))
}
func TestScopedAdminReferralServiceChecksScopeBeforeResolvingParent(t *testing.T) {
	parentID := int64(2)
	shadow := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}, ParentAccountID: &parentID}
	parent := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{2}}
	svc := &OpenAIQuotaService{accountRepo: &stubQuotaAccountRepo{accounts: map[int64]*Account{1: shadow, 2: parent}}}
	a, err := svc.referralAccount(scopedStatisticsServiceContext("openai.referrals.read"), 1, false)
	require.Nil(t, a)
	require.Equal(t, 404, infraerrors.Code(err))
}

func TestScopedAdminQuotaCacheWriteRechecksScope(t *testing.T) {
	a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{2}}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{1: a}}
	svc := &OpenAIQuotaService{accountRepo: repo}
	err := svc.CacheCreditsSnapshot(scopedStatisticsServiceContext("openai.quota.read"), 1, &OpenAIQuotaUsage{})
	require.Equal(t, 404, infraerrors.Code(err))
	require.Zero(t, repo.extraUpdateCalls)
}

type scopedUsageRecoveryRepo struct {
	stubQuotaAccountRepo
	clears int
}

func (r *scopedUsageRecoveryRepo) ClearError(context.Context, int64) error { r.clears++; return nil }

func TestScopedAdminUsageReadDoesNotRecoverAccount(t *testing.T) {
	for _, platform := range []string{PlatformGemini, PlatformAnthropic} {
		t.Run(platform, func(t *testing.T) {
			a := &Account{ID: 1, Platform: platform, Type: AccountTypeOAuth, GroupIDs: []int64{1}, Status: StatusError, ErrorMessage: "unauthenticated", Credentials: map[string]any{"access_token": "token", "scope": "user:profile"}}
			repo := &scopedUsageRecoveryRepo{stubQuotaAccountRepo: stubQuotaAccountRepo{accounts: map[int64]*Account{1: a}}}
			cache := NewUsageCache()
			cache.apiCache.Store(int64(1), &apiUsageCache{response: &ClaudeUsageResponse{}, timestamp: time.Now()})
			svc := &AccountUsageService{accountRepo: repo, usageLogRepo: &usageBatchLogRepoStub{}, cache: cache}
			ctx := scopedStatisticsServiceContext("accounts.usage.read")
			adminaccess.FromContext(ctx).Policy.Rules[0].Platforms = []string{platform}
			_, err := svc.getUsageForAccount(ctx, a, false)
			require.NoError(t, err)
			require.Zero(t, repo.clears)
			require.Equal(t, StatusError, a.Status)
		})
	}
}

type scopedStatisticsBatchRepo struct{ stubQuotaAccountRepo }

func (r *scopedStatisticsBatchRepo) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	out := []*Account{}
	for _, id := range ids {
		a, err := r.GetByID(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

type scopedStatisticsLogRepo struct {
	usageBatchLogRepoStub
	reads int
}

func (r *scopedStatisticsLogRepo) GetAccountTodayStats(context.Context, int64) (*usagestats.AccountStats, error) {
	r.reads++
	return &usagestats.AccountStats{Cost: 23}, nil
}
func (r *scopedStatisticsLogRepo) GetAccountWindowStats(context.Context, int64, time.Time) (*usagestats.AccountStats, error) {
	r.reads++
	return &usagestats.AccountStats{Cost: 23}, nil
}

func TestScopedAdminBatchRejectsWholeRequestBeforeData(t *testing.T) {
	for _, action := range []string{"accounts.usage.read", "accounts.stats.read"} {
		for _, id := range []int64{2, 3} {
			t.Run(fmt.Sprintf("%s/id=%d", action, id), func(t *testing.T) {
				repo := &scopedStatisticsBatchRepo{stubQuotaAccountRepo: stubQuotaAccountRepo{accounts: map[int64]*Account{1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}}, 2: {ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{2}}}}}
				logs := &scopedStatisticsLogRepo{}
				svc := &AccountUsageService{accountRepo: repo, usageLogRepo: logs, cache: NewUsageCache()}
				ctx := scopedStatisticsServiceContext(action)
				var err error
				if action == "accounts.usage.read" {
					_, _, err = svc.GetUsageBatch(ctx, []int64{1, id}, true)
				} else {
					_, err = svc.GetTodayStatsBatch(ctx, []int64{1, id})
				}
				require.Equal(t, 404, infraerrors.Code(err))
				require.Zero(t, logs.reads)
				require.Zero(t, repo.extraUpdateCalls)
			})
		}
	}
}

func TestScopedAdminSnapshotWriteDoesNotNotifyAutomaticReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}}
		repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{1: a}}
		svc := &AccountUsageService{accountRepo: repo}
		notifier := NewOpenAIQuotaAutoResetService(nil, nil, nil, nil, nil, nil, nil)
		openAIAutoResetNotifierRegistry.Lock()
		previous := openAIAutoResetNotifierRegistry.service
		openAIAutoResetNotifierRegistry.service = notifier
		openAIAutoResetNotifierRegistry.Unlock()
		defer func() {
			openAIAutoResetNotifierRegistry.Lock()
			openAIAutoResetNotifierRegistry.service = previous
			openAIAutoResetNotifierRegistry.Unlock()
			notifier.cancel()
		}()
		svc.persistOpenAICodexProbeSnapshotContext(scopedStatisticsServiceContext("accounts.usage.read"), 1, map[string]any{"codex_5h_used_percent": 25.0})
		synctest.Wait()
		require.Equal(t, 1, repo.extraUpdateCalls)
		require.Empty(t, notifier.queue)
	})
}

func TestScopedAdminQuotaHeaderReloadRechecksScope(t *testing.T) {
	a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{2}}
	svc := &OpenAIQuotaService{accountRepo: &stubQuotaAccountRepo{accounts: map[int64]*Account{1: a}}}
	headers, _, err := svc.buildCodexQuotaHeaders(scopedStatisticsServiceContext("openai.quota.read"), 1, "token", "upstream-id", false)
	require.Nil(t, headers)
	require.Equal(t, 404, infraerrors.Code(err))
}

func TestScopedAdminGrokQuotaReloadRechecksScope(t *testing.T) {
	a := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth, GroupIDs: []int64{2}}
	svc := &GrokQuotaService{accountRepo: &stubQuotaAccountRepo{accounts: map[int64]*Account{1: a}}}
	ctx := scopedStatisticsServiceContext("accounts.usage.read")
	adminaccess.FromContext(ctx).Policy.Rules[0].Platforms = []string{PlatformGrok}
	account, err := svc.loadGrokOAuthAccount(ctx, 1)
	require.Nil(t, account)
	require.Equal(t, 404, infraerrors.Code(err))
}

func TestScopedAdminGrokBillingChecksScopeBeforeJoiningFlight(t *testing.T) {
	a := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth, GroupIDs: []int64{2}}
	svc := &GrokQuotaService{accountRepo: &stubQuotaAccountRepo{accounts: map[int64]*Account{1: a}}}
	ctx := scopedStatisticsServiceContext("accounts.usage.read")
	adminaccess.FromContext(ctx).Policy.Rules[0].Platforms = []string{PlatformGrok}
	result, err := svc.ProbeBilling(ctx, 1)
	require.Nil(t, result)
	require.Equal(t, 404, infraerrors.Code(err))
}
func TestScopedAdminGrokSharedFlightPreservesPrincipal(t *testing.T) {
	svc := &GrokQuotaService{}
	ctx := scopedStatisticsServiceContext("accounts.usage.read")
	_, err := svc.runProbeFlight(ctx, "context", func(shared context.Context) (*GrokQuotaProbeResult, error) {
		if adminaccess.FromContext(ctx) != adminaccess.FromContext(shared) {
			return nil, fmt.Errorf("共享任务丢失授权身份")
		}
		return &GrokQuotaProbeResult{}, nil
	})
	require.NoError(t, err)
}

type scopedQuotaParentDisappears struct {
	AccountRepository
	shadow, parent *Account
	parentReads    int
}

func (r *scopedQuotaParentDisappears) GetByID(_ context.Context, id int64) (*Account, error) {
	if id == r.shadow.ID {
		return r.shadow, nil
	}
	r.parentReads++
	if r.parentReads == 1 {
		return r.parent, nil
	}
	return nil, fmt.Errorf("母账户读取失败")
}
func TestScopedAdminQuotaHeaderParentResolveFailureFailsClosed(t *testing.T) {
	parentID := int64(2)
	repo := &scopedQuotaParentDisappears{shadow: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}, ParentAccountID: &parentID}, parent: &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}}}
	svc := &OpenAIQuotaService{accountRepo: repo}
	headers, _, err := svc.buildCodexQuotaHeaders(scopedStatisticsServiceContext("openai.quota.read"), 1, "old-token", "old-account-id", false)
	require.Nil(t, headers)
	require.Equal(t, 404, infraerrors.Code(err))
}

type scopedUsageRoundTripper func(*http.Request) (*http.Response, error)

func (f scopedUsageRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestScopedAdminOpenAIUsageRefreshNeverGeneratesModelRequests(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		restricted, shadow, full bool
		wantModel                int
		wantUsage                float64
	}{
		{"受限普通账户", true, false, false, 0, 37}, {"受限影子账户", true, true, false, 0, 67},
		{"JWT普通账户", false, false, false, 1, 98}, {"全权密钥普通账户", false, false, true, 1, 98},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parentID := int64(2)
			a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}, Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}}
			parent := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{1}, Credentials: a.Credentials}
			if tc.shadow {
				a.ParentAccountID = &parentID
				a.QuotaDimension = QuotaDimensionSpark
			}
			updates := make(chan map[string]any, 1)
			repo := &sparkShadowUsageTestRepo{accounts: map[int64]*Account{1: a, 2: parent}, updateExtraCh: updates}
			var quotaCalls, nonReadCalls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					nonReadCalls++
				}
				if strings.HasSuffix(r.URL.Path, "/usage") {
					quotaCalls++
				}
				w.Header().Set("Content-Type", "application/json")
				if !strings.HasSuffix(r.URL.Path, "/usage") {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":37,"limit_window_seconds":18000,"reset_after_seconds":3600}},"additional_rate_limits":[{"metered_feature":"codex_bengalfox","rate_limit":{"primary_window":{"used_percent":67,"limit_window_seconds":18000,"reset_after_seconds":3600}}}],"rate_limit_reset_credits":{"available_count":0}}`))
			}))
			defer upstream.Close()
			quota := NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, nil, nil), newQuotaRedirectingFactory(upstream), nil)
			client, err := httppool.GetClient(httppool.Options{Timeout: 15 * time.Second, ResponseHeaderTimeout: 10 * time.Second})
			require.NoError(t, err)
			previous := client.Transport
			defer func() { client.Transport = previous }()
			modelCalls := 0
			client.Transport = scopedUsageRoundTripper(func(r *http.Request) (*http.Response, error) {
				modelCalls++
				headers := make(http.Header)
				headers.Set("x-codex-primary-used-percent", "98")
				headers.Set("x-codex-primary-window-minutes", "300")
				headers.Set("x-codex-primary-reset-after-seconds", "3600")
				return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			svc := &AccountUsageService{accountRepo: repo, openAIQuotaService: quota, cache: NewUsageCache()}
			ctx := context.Background()
			if tc.restricted {
				ctx = scopedStatisticsServiceContext("accounts.usage.read")
			}
			if tc.full {
				ctx = adminaccess.WithPrincipal(ctx, &adminaccess.Principal{Policy: adminaccess.Policy{FullAccess: true}})
			}
			usage, err := svc.GetUsageForAccount(ctx, a, true)
			require.NoError(t, err)
			require.Equal(t, tc.wantModel, modelCalls, "读取额度不能合成模型消费请求")
			if tc.restricted {
				require.Equal(t, 1, quotaCalls)
				require.Zero(t, nonReadCalls)
			} else {
				require.Zero(t, quotaCalls)
			}
			require.NotNil(t, usage.FiveHour)
			require.Equal(t, tc.wantUsage, usage.FiveHour.Utilization)
			select {
			case <-updates:
			case <-time.After(time.Second):
				t.Fatal("展示快照未写入")
			}
		})
	}
}
