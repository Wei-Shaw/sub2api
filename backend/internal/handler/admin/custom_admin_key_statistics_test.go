//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedStatisticsAccounts struct {
	service.AdminService
	accounts map[int64]*service.Account
}

func (s *scopedStatisticsAccounts) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	return s.accounts[id], nil
}
func statisticsPolicy(permission string) adminaccess.Policy {
	return adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{permission}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}, Platforms: []string{"openai"}}}}
}
func statisticsContext(c *gin.Context, permission string) {
	c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: statisticsPolicy(permission)}))
}

func TestScopedAdminStatisticsRejectBeforeCacheOrService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &scopedStatisticsAccounts{accounts: map[int64]*service.Account{1: {ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}}, 2: {ID: 2, Platform: "openai", Type: "oauth", GroupIDs: []int64{2}}}}
	h := &AccountHandler{adminService: svc}
	for _, path := range []string{"/api/v1/admin/accounts/1/today-stats", "/api/v1/admin/accounts/1/stats", "/api/v1/admin/accounts/today-stats/batch", "/api/v1/admin/accounts/usage/batch"} {
		t.Run(path, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) { statisticsContext(c, "accounts.read") })
			r.Use(h.ScopedAdminKeyAccounts)
			method := "GET"
			if strings.Contains(path, "batch") {
				method = "POST"
			}
			r.Handle(method, strings.Replace(path, "/1/", "/:id/", 1), func(c *gin.Context) { t.Fatal("不得调用原处理器") })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(`{"account_ids":[1,2]}`)))
			require.Equal(t, 404, w.Code, w.Body.String())
		})
	}
}

func TestScopedAdminOpenAIRejectsBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &scopedStatisticsAccounts{accounts: map[int64]*service.Account{1: {ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}}}}
	stub := &referralHandlerStub{}
	h := &OpenAIOAuthHandler{adminService: svc, referralService: stub}
	r := gin.New()
	r.Use(func(c *gin.Context) { statisticsContext(c, "accounts.read") })
	r.POST("/:id/referrals/invite", h.SendReferralInvite)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/1/referrals/invite", strings.NewReader(`{"email":"friend@example.com","program_id":"codex_referral_consumer","confirmed":true}`)))
	require.Equal(t, 404, w.Code, w.Body.String())
	require.Zero(t, stub.sends)
}

func TestScopedAdminUsageProjectionKeepsPlatformQuotaAndHidesCosts(t *testing.T) {
	a := &service.Account{ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}}
	u := &service.UsageInfo{FiveHour: &service.UsageProgress{Utilization: 25, WindowStats: &service.WindowStats{Cost: 123}}, SevenDaySonnet: &service.UsageProgress{Utilization: 30}, GeminiProDaily: &service.UsageProgress{Utilization: 40}, AntigravityQuota: map[string]*service.AntigravityModelQuota{"model": {Utilization: 50}}, Error: "TOKEN_SECRET", ValidationURL: "URL_SECRET", ForbiddenReason: "IDENTITY_SECRET"}
	out := scopedUsageDTO(u, statisticsPolicy("accounts.usage.read"), a)
	require.Nil(t, out["five_hour"].(*service.UsageProgress).WindowStats)
	require.NotNil(t, out["seven_day_sonnet"])
	require.NotNil(t, out["gemini_pro_daily"])
	require.NotNil(t, out["antigravity_quota"])
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "SECRET")
	require.NotContains(t, string(raw), "123")
	require.NotNil(t, u.FiveHour.WindowStats, "裁剪不能更改共享缓存")
}

func TestScopedAdminOpenAIQuotaActionsAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		action, path, method string
		allow                bool
	}{
		{"openai.quota.read", "/1/quota", "GET", true}, {"openai.quota.read", "/1/quota/refresh", "POST", true},
		{"accounts.usage.read", "/1/quota", "GET", false}, {"openai.quota.reset", "/1/quota", "GET", false},
	} {
		t.Run(tc.action+tc.path, func(t *testing.T) {
			svc := &scopedStatisticsAccounts{accounts: map[int64]*service.Account{1: {ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}}}}
			stub := &openAIQuotaWorkflowStub{queryResult: &service.OpenAIQuotaUsage{UserID: "IDENTITY_SECRET", Email: "IDENTITY_SECRET", AccountID: "IDENTITY_SECRET", PlanType: "IDENTITY_SECRET", RateLimit: &service.OpenAIRateLimit{PrimaryWindow: &service.OpenAIRateLimitWindow{UsedPercent: 23}}, RateLimitResetCredits: &service.OpenAIRateLimitResetCredits{AvailableCount: 3}}}
			h := &OpenAIOAuthHandler{adminService: svc, quotaService: stub}
			r := gin.New()
			r.Use(func(c *gin.Context) { statisticsContext(c, tc.action) })
			r.GET("/:id/quota", h.QueryQuota)
			r.POST("/:id/quota/refresh", h.RefreshQuota)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if tc.allow {
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, 1, stub.queryCalls)
				require.Contains(t, w.Body.String(), `"available_count":3`)
				require.Contains(t, w.Body.String(), `"used_percent":23`)
			} else {
				require.Equal(t, 404, w.Code)
				require.Zero(t, stub.queryCalls)
				require.Zero(t, stub.cacheCalls)
			}
			require.NotContains(t, w.Body.String(), "SECRET")
		})
	}
}

func TestScopedAdminAccountScopeAndShadowSource(t *testing.T) {
	parent := int64(2)
	for _, tc := range []struct {
		name      string
		a, source *service.Account
		rules     []adminaccess.Rule
		allowed   bool
	}{
		{"同规则", &service.Account{ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}}, nil, nil, true},
		{"其他组", &service.Account{ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{2}}, nil, nil, false},
		{"其他平台", &service.Account{ID: 1, Platform: "anthropic", Type: "oauth", GroupIDs: []int64{1}}, nil, nil, false},
		{"其他类型", &service.Account{ID: 1, Platform: "openai", Type: "apikey", GroupIDs: []int64{1}}, nil, nil, false},
		{"母账户越权", &service.Account{ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}, ParentAccountID: &parent}, &service.Account{ID: 2, Platform: "openai", Type: "oauth", GroupIDs: []int64{2}}, nil, false},
		{"不能拼接规则", &service.Account{ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}}, nil, []adminaccess.Rule{{Permissions: []string{"openai.quota.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"apikey"}}, {Permissions: []string{"openai.quota.read"}, GroupIDs: []int64{2}, AccountTypes: []string{"oauth"}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &scopedStatisticsAccounts{accounts: map[int64]*service.Account{1: tc.a, 2: tc.source}}
			r := gin.New()
			r.Use(func(c *gin.Context) {
				statisticsContext(c, "openai.quota.read")
				if tc.rules != nil {
					p := adminaccess.FromContext(c.Request.Context())
					p.Policy.Rules = tc.rules
				}
			})
			r.GET("/:id/quota", func(c *gin.Context) {
				_, ok := scopedAuthorizedAccount(c, svc, 1, "openai.quota.read", true)
				if ok {
					c.Status(204)
				}
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/1/quota", nil))
			want := 404
			if tc.allowed {
				want = 204
			}
			require.Equal(t, want, w.Code, w.Body.String())
		})
	}
}

func TestScopedAdminMutationResponseRequiresReadPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, read := range []bool{false, true} {
		for _, kind := range []string{"reset", "invite"} {
			t.Run(fmt.Sprintf("%s/read=%v", kind, read), func(t *testing.T) {
				count := 2
				svc := &scopedStatisticsAccounts{accounts: map[int64]*service.Account{1: {ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}}}}
				quota := &openAIQuotaWorkflowStub{resetResult: &service.OpenAIQuotaResetResult{Code: "success"}, queryResult: &service.OpenAIQuotaUsage{RateLimitResetCredits: &service.OpenAIRateLimitResetCredits{AvailableCount: 3}, Email: "IDENTITY_SECRET"}}
				referral := &referralHandlerStub{eligibility: &service.OpenAIReferralEligibility{ShouldShow: true, AvailableInvites: &count}}
				h := &OpenAIOAuthHandler{adminService: svc, quotaService: quota, referralService: referral, rateLimitService: &openAIAccountStateRecovererStub{}}
				action, readAction, path := "openai.quota.reset", "openai.quota.read", "/1/reset"
				if kind == "invite" {
					action, readAction, path = "openai.referrals.invite", "openai.referrals.read", "/1/invite"
				}
				r := gin.New()
				r.Use(func(c *gin.Context) {
					statisticsContext(c, action)
					if read {
						p := adminaccess.FromContext(c.Request.Context())
						p.Policy.Rules[0].Permissions = append(p.Policy.Rules[0].Permissions, readAction)
					}
				})
				r.POST("/:id/reset", h.ResetQuota)
				r.POST("/:id/invite", h.SendReferralInvite)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(`{"email":"friend@example.com","program_id":"codex_referral_consumer","confirmed":true}`)))
				require.Equal(t, 200, w.Code, w.Body.String())
				require.NotContains(t, w.Body.String(), "IDENTITY_SECRET")
				if kind == "reset" {
					require.Equal(t, 1, quota.resetCalls)
					if read {
						require.Contains(t, w.Body.String(), "available_count")
					} else {
						require.NotContains(t, w.Body.String(), "available_count")
					}
				} else {
					require.Equal(t, 1, referral.sends)
					if read {
						require.Contains(t, w.Body.String(), "available_invites")
					} else {
						require.NotContains(t, w.Body.String(), "available_invites")
					}
				}
			})
		}
	}
}

func TestScopedAdminUsageBillingPreservesQuotaWithoutCost(t *testing.T) {
	limit, used, percent := 100.0, 123.0, 25.0
	u := &service.UsageInfo{GrokBilling: &xai.BillingSummary{MonthlyLimit: &limit, MonthlyUsed: &used, UsedCents: &used, UsedPercent: &percent, Plan: "IDENTITY_SECRET"}}
	a := &service.Account{ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}}
	out := scopedUsageDTO(u, statisticsPolicy("accounts.usage.read"), a)
	billing, ok := out["grok_billing"].(*xai.BillingSummary)
	require.True(t, ok, "须保留真实上游额度窗口")
	require.Equal(t, &limit, billing.MonthlyLimit)
	require.Equal(t, &percent, billing.UsedPercent)
	require.Nil(t, billing.MonthlyUsed)
	require.Nil(t, billing.UsedCents)
	require.Empty(t, billing.Plan)
	require.NotNil(t, u.GrokBilling.MonthlyUsed, "不能裁剪共享缓存")
}

func TestScopedAdminBatchAuthorizationPrecedesETag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &scopedStatisticsAccounts{accounts: map[int64]*service.Account{1: {ID: 1, Platform: "openai", Type: "oauth", GroupIDs: []int64{1}}, 2: {ID: 2, Platform: "openai", Type: "oauth", GroupIDs: []int64{2}}}}
	h := &AccountHandler{adminService: svc}
	cached := accountTodayStatsBatchCache.Set(buildAccountTodayStatsBatchCacheKey([]int64{1, 2}), gin.H{"stats": "CACHE_SECRET"})
	r := gin.New()
	r.Use(func(c *gin.Context) { statisticsContext(c, "accounts.stats.read") })
	r.Use(h.ScopedAdminKeyAccounts)
	r.POST("/api/v1/admin/accounts/today-stats/batch", h.GetBatchTodayStats)
	request := httptest.NewRequest("POST", "/api/v1/admin/accounts/today-stats/batch", strings.NewReader(`{"account_ids":[1,2]}`))
	request.Header.Set("If-None-Match", cached.ETag)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request)
	require.Equal(t, 404, w.Code, w.Body.String())
	require.Empty(t, w.Header().Get("ETag"))
	require.NotContains(t, w.Body.String(), "CACHE_SECRET")
}
