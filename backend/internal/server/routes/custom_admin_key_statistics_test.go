//go:build unit

package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type scopedRouteStatisticsAccounts struct{ scopedRouteAccounts }

func (s *scopedRouteStatisticsAccounts) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	a, err := s.scopedRouteAccounts.GetAccount(ctx, id)
	a.Credentials["access_token"] = "PRIVATE_TOKEN"
	a.Credentials["chatgpt_account_id"] = "PRIVATE_ACCOUNT"
	a.Credentials["plan_type"] = "plus"
	return a, err
}
func (*scopedRouteStatisticsAccounts) GetGroup(_ context.Context, id int64) (*service.Group, error) {
	return &service.Group{ID: id, Name: "组", Platform: "openai", Status: service.StatusActive}, nil
}
func (*scopedRouteStatisticsAccounts) ScopedAdminListGroupModels(context.Context, pagination.PaginationParams, bool, string, string, string) ([]service.Group, int64, error) {
	return []service.Group{{ID: 1, Name: "组", Platform: "openai", Status: service.StatusActive}}, 1, nil
}

type scopedRouteStatisticsRepo struct {
	service.AccountRepository
	admin *scopedRouteStatisticsAccounts
}

func (s *scopedRouteStatisticsRepo) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	return s.admin.GetAccount(ctx, id)
}
func (s *scopedRouteStatisticsRepo) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	out := []*service.Account{}
	for _, id := range ids {
		a, err := s.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
func (*scopedRouteStatisticsRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}
func (*scopedRouteStatisticsRepo) ListSchedulableCapacityByGroupIDs(context.Context, []int64) ([]service.GroupAccountCapacityRow, error) {
	return nil, nil
}

type scopedRouteStatisticsLogs struct {
	service.UsageLogRepository
	reads atomic.Int64
}

func (s *scopedRouteStatisticsLogs) GetAccountTodayStats(context.Context, int64) (*usagestats.AccountStats, error) {
	s.reads.Add(1)
	return &usagestats.AccountStats{Requests: 7, Cost: 12}, nil
}
func (s *scopedRouteStatisticsLogs) GetAccountWindowStats(context.Context, int64, time.Time) (*usagestats.AccountStats, error) {
	s.reads.Add(1)
	return &usagestats.AccountStats{Requests: 7, Cost: 12}, nil
}
func (s *scopedRouteStatisticsLogs) GetAccountUsageStats(context.Context, int64, time.Time, time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	s.reads.Add(1)
	return &usagestats.AccountUsageStatsResponse{}, nil
}

type scopedRouteCapacityGroups struct{ service.GroupRepository }

func (*scopedRouteCapacityGroups) ListScopedAdminCapacityGroupIDs(context.Context) ([]int64, error) {
	return []int64{1}, nil
}

type scopedRouteReferrals struct{ queries, sends atomic.Int64 }

func (s *scopedRouteReferrals) QueryEligibility(context.Context, service.OpenAIReferralCall) (*service.OpenAIReferralEligibility, error) {
	s.queries.Add(1)
	count := 2
	confirm := true
	return &service.OpenAIReferralEligibility{ShouldShow: true, RemainingSendCapacity: &count, RequiresExplicitConfirmation: &confirm}, nil
}
func (s *scopedRouteReferrals) SendInvite(context.Context, service.OpenAIReferralCall, string) error {
	s.sends.Add(1)
	return nil
}

func TestScopedAdminRegisteredStatisticsAndOpenAIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		permission, method, path, body string
		want                           int
	}{
		{"accounts.stats.read", "GET", "/accounts/1/today-stats", "", 200},
		{"accounts.stats.read", "GET", "/accounts/1/stats", "", 200},
		{"accounts.stats.read", "POST", "/accounts/today-stats/batch", `{"account_ids":[1,1]}`, 200},
		{"accounts.usage.read", "GET", "/accounts/1/usage", "", 200},
		{"accounts.usage.read", "POST", "/accounts/usage/batch", `{"account_ids":[1,1]}`, 200},
		{"openai.quota.read", "GET", "/openai/accounts/1/quota", "", 200},
		{"openai.quota.read", "POST", "/openai/accounts/1/quota/refresh", "", 200},
		{"openai.quota.reset", "POST", "/openai/accounts/1/reset-quota", "", 200},
		{"openai.referrals.read", "POST", "/openai/accounts/1/referrals/refresh", "", 200},
		{"openai.referrals.invite", "POST", "/openai/accounts/1/referrals/invite", `{"email":"friend@example.com","program_id":"codex_referral_consumer","confirmed":true}`, 200},
		{"groups.list.read", "GET", "/groups", "", 200},
		{"groups.capacity.read", "GET", "/groups/capacity-summary", "", 200},
		{"openai.tokens.refresh", "POST", "/openai/refresh-token", `{}`, 400},
	}
	for _, tc := range cases {
		for _, allowed := range []bool{false, true} {
			t.Run(tc.permission+tc.path+map[bool]string{false: "/默认拒绝", true: "/显式授权"}[allowed], func(t *testing.T) {
				policy := adminaccess.Policy{Rules: []adminaccess.Rule{{GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}}}
				if allowed {
					if tc.permission == "openai.tokens.refresh" {
						policy.GlobalPermissions = []string{tc.permission}
					} else {
						policy.Rules[0].Permissions = []string{tc.permission}
					}
				}
				settings := service.NewSettingService(nil, nil)
				settings.SetScopedAdminKeyRepository(&scopedRouteKeys{})
				key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{Name: "统计路由", Enabled: true, Policy: policy}, 9)
				require.NoError(t, err)
				users := service.NewUserService(&scopedRouteUsers{}, nil, nil, nil)
				auth := middleware.NewAdminAuthMiddleware(nil, users, settings, nil)
				admin := &scopedRouteStatisticsAccounts{scopedRouteAccounts: scopedRouteAccounts{t: t}}
				repo := &scopedRouteStatisticsRepo{admin: admin}
				logs := &scopedRouteStatisticsLogs{}
				referrals := &scopedRouteReferrals{}
				var calls atomic.Int64
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					if strings.HasSuffix(r.URL.Path, "/consume") {
						_, _ = w.Write([]byte(`{"code":"success","windows_reset":1}`))
					} else if strings.HasSuffix(r.URL.Path, "/credits") {
						_, _ = w.Write([]byte(`{"available_count":0,"credits":[]}`))
					} else {
						_, _ = w.Write([]byte(`{"email":"PRIVATE_EMAIL","account_id":"PRIVATE_ACCOUNT","rate_limit":{"allowed":true,"primary_window":{"used_percent":25}},"rate_limit_reset_credits":{"available_count":0},"credits":{"has_credits":true,"balance":"50"}}`))
					}
				}))
				defer srv.Close()
				target, _ := url.Parse(srv.URL)
				factory := func(string) (*req.Client, error) {
					return req.C().WrapRoundTripFunc(func(rt req.RoundTripper) req.RoundTripFunc {
						return func(r *req.Request) (*req.Response, error) {
							r.URL.Scheme = target.Scheme
							r.URL.Host = target.Host
							return rt.RoundTrip(r)
						}
					}), nil
				}
				quota := service.NewOpenAIQuotaService(repo, nil, service.NewOpenAITokenProvider(repo, nil, nil), factory, referrals)
				usage := service.NewAccountUsageService(repo, logs, nil, nil, nil, nil, nil, quota, service.NewUsageCache(), nil, nil)
				accounts := adminhandler.NewAccountHandler(admin, nil, nil, nil, nil, nil, nil, usage, nil, nil, nil, nil, nil, nil)
				openai := adminhandler.NewOpenAIOAuthHandler(nil, admin, quota, nil)
				groups := adminhandler.NewGroupHandler(admin, nil, service.NewGroupCapacityService(repo, &scopedRouteCapacityGroups{}, nil, nil, nil))
				router := gin.New()
				RegisterAdminRoutes(router.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{Account: accounts, OpenAIOAuth: openai, Group: groups}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
				request := httptest.NewRequest(tc.method, "/api/v1/admin"+tc.path, strings.NewReader(tc.body))
				request.Header.Set("x-api-key", key)
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				want := 403
				if allowed {
					want = tc.want
				}
				require.Equal(t, want, recorder.Code, recorder.Body.String())
				require.NotContains(t, recorder.Body.String(), "PRIVATE_")
				if !allowed {
					require.Zero(t, calls.Load())
					require.Zero(t, logs.reads.Load())
					require.Zero(t, referrals.queries.Load())
					require.Zero(t, referrals.sends.Load())
				}
				if tc.permission != "openai.referrals.invite" {
					require.Zero(t, referrals.sends.Load())
				}
			})
		}
	}
}
