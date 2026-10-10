//go:build unit

package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	cachedBillingInputPrice        = 0.00000125
	cachedBillingStoragePrice      = 0.0000045
	cachedBillingAliasStoragePrice = 0.000009
	cachedBillingTokens            = 1_000_000
)

type cachedBillingRepo struct {
	mu         sync.Mutex
	records    map[string]*service.GeminiCachedContent
	failUpdate bool
	nextID     int64
	deleted    []int64
}

func (r *cachedBillingRepo) Create(_ context.Context, record *service.GeminiCachedContent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	record.ID = r.nextID
	cp := *record
	r.records[record.PublicID] = &cp
	return nil
}

func (r *cachedBillingRepo) GetForOwner(_ context.Context, apiKeyID int64, publicID string) (*service.GeminiCachedContent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[publicID]
	if !ok || record.APIKeyID != apiKeyID {
		return nil, service.ErrGeminiCachedContentNotFound
	}
	cp := *record
	return &cp, nil
}

func (r *cachedBillingRepo) ListForOwner(context.Context, int64, int64, time.Time, int64, int) ([]*service.GeminiCachedContent, error) {
	return nil, nil
}

func (r *cachedBillingRepo) UpdateExpireTime(_ context.Context, id int64, expireTime time.Time) (time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failUpdate {
		return time.Time{}, errors.New("db unavailable")
	}
	for _, record := range r.records {
		if record.ID == id {
			previous := record.ExpireTime
			record.ExpireTime = expireTime
			return previous, nil
		}
	}
	return time.Time{}, service.ErrGeminiCachedContentNotFound
}

func (r *cachedBillingRepo) SoftDelete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = append(r.deleted, id)
	return nil
}

type cachedBillingAccountRepo struct {
	service.AccountRepository
	accounts map[int64]*service.Account
}

func (r *cachedBillingAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	account, ok := r.accounts[id]
	if !ok {
		return nil, service.ErrAccountNotFound
	}
	cp := *account
	return &cp, nil
}

type cachedBillingUpstream struct {
	service.HTTPUpstream
	mu         sync.Mutex
	createBody string
	calls      []string
	beforePost func(n int)
	// failures 按请求方法返回上游错误响应。
	failures map[string]cachedBillingUpstreamFailure
}

type cachedBillingUpstreamFailure struct {
	status int
	body   string
}

func (u *cachedBillingUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.calls = append(u.calls, req.Method+" "+req.URL.Path)
	n := len(u.calls)
	failure, failed := u.failures[req.Method]
	u.mu.Unlock()
	if failed {
		return &http.Response{StatusCode: failure.status, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(failure.body))}, nil
	}
	body := "{}"
	switch req.Method {
	case http.MethodPost:
		body = u.createBody
	case http.MethodPatch:
		if u.beforePost != nil {
			u.beforePost(n)
		}
		body = fmt.Sprintf(`{"name":"cachedContents/up1","expireTime":%q}`, time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339Nano))
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(body))}, nil
}

func (u *cachedBillingUpstream) methodCalls(method string) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	n := 0
	for _, call := range u.calls {
		if strings.HasPrefix(call, method+" ") {
			n++
		}
	}
	return n
}

type cachedBillingUserRepo struct {
	service.UserRepository
	balance float64
}

func (r *cachedBillingUserRepo) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id, Balance: r.balance}, nil
}

func (r *cachedBillingUserRepo) DeductBalance(context.Context, int64, float64) error { return nil }

type cachedBillingUsageRepo struct {
	service.UsageLogRepository
	mu   sync.Mutex
	logs []*service.UsageLog
}

func (r *cachedBillingUsageRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, log)
	return true, nil
}

func (r *cachedBillingUsageRepo) first() *service.UsageLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.logs[0]
}

func (r *cachedBillingUsageRepo) totals() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]float64, 0, len(r.logs))
	for _, log := range r.logs {
		out = append(out, log.TotalCost)
	}
	return out
}

type cachedBillingEnv struct {
	h        *GatewayHandler
	repo     *cachedBillingRepo
	upstream *cachedBillingUpstream
	usage    *cachedBillingUsageRepo
	apiKey   *service.APIKey
	group    *service.Group
	// accounts 是调度快照桩直接返回的账号对象，测试可改其凭据。
	accounts map[int64]*service.Account
}

func newCachedBillingEnv(t *testing.T, balance float64, accountIDs ...int64) *cachedBillingEnv {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model_pricing.json"), []byte(fmt.Sprintf(`{"gemini-2.5-pro":{
		"litellm_provider":"gemini","mode":"chat","input_cost_per_token":%g,"output_cost_per_token":0.00001,
		"cache_storage_cost_per_token_per_hour":%g},"gemini-alias":{
		"litellm_provider":"gemini","mode":"chat","input_cost_per_token":%g,"output_cost_per_token":0.00001,
		"cache_storage_cost_per_token_per_hour":%g}}`,
		cachedBillingInputPrice, cachedBillingStoragePrice, cachedBillingInputPrice, cachedBillingAliasStoragePrice)), 0o644))
	cfg := &config.Config{}
	cfg.Pricing.DataDir = dir
	cfg.Pricing.UpdateIntervalHours = 100000
	cfg.Default.RateMultiplier = 1
	pricing := service.NewPricingService(cfg, nil)
	require.NoError(t, pricing.Initialize())
	billing := service.NewBillingService(cfg, pricing)

	groupID := int64(42)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive, RateMultiplier: 1}
	accounts := map[int64]*service.Account{}
	var scheduled []*service.Account
	for _, id := range accountIDs {
		account := &service.Account{ID: id, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "k"}, Status: service.StatusActive, Schedulable: true,
			Concurrency: 5, Priority: 1, AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}}}
		accounts[id] = account
		scheduled = append(scheduled, account)
	}
	userRepo := &cachedBillingUserRepo{balance: balance}
	usage := &cachedBillingUsageRepo{}
	bcs := service.NewBillingCacheService(nil, userRepo, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(bcs.Stop)
	snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: scheduled}, nil, nil, nil, nil)
	gw := service.NewGatewayService(nil, &fakeGroupRepo{group: group}, usage, nil, userRepo, nil, nil, nil, cfg, snapshot, nil,
		billing, nil, &service.BillingCacheService{}, nil, nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	upstream := &cachedBillingUpstream{}
	compat := service.NewGeminiMessagesCompatService(&cachedBillingAccountRepo{accounts: accounts}, nil, nil, nil, nil, nil, upstream, nil, cfg)
	repo := &cachedBillingRepo{records: map[string]*service.GeminiCachedContent{}}
	h := &GatewayHandler{
		gatewayService:             gw,
		billingCacheService:        bcs,
		concurrencyHelper:          NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatNone, 0),
		geminiCachedContentService: service.NewGeminiCachedContentService(repo, compat, cfg),
	}
	apiKey := &service.APIKey{ID: 7, UserID: 3, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 3, Concurrency: 10, Balance: balance}}
	return &cachedBillingEnv{h: h, repo: repo, upstream: upstream, usage: usage, apiKey: apiKey, group: group, accounts: accounts}
}

// TestGeminiExplicitCacheExclusions_FiltersUnsupportedAccountsFromSnapshot 候选过滤直接按快照里的凭据判定：
// 第三方地址且未声明上游显式缓存的 Key 账号在选号前就被排除，官方地址与声明了上游显式缓存的账号保留。
func TestGeminiExplicitCacheExclusions_FiltersUnsupportedAccountsFromSnapshot(t *testing.T) {
	env := newCachedBillingEnv(t, 500, 900, 901, 902)
	env.accounts[901].Credentials["base_url"] = "https://third.example.com"
	env.accounts[902].Credentials["base_url"] = "https://gw.example.com"
	env.accounts[902].Credentials[service.GeminiExplicitCacheUpstreamCredentialKey] = true

	groupID := env.group.ID
	excluded, err := env.h.gatewayService.GeminiExplicitCacheExclusions(context.Background(), &groupID, 0)
	require.NoError(t, err)
	require.Equal(t, map[int64]struct{}{901: {}}, excluded)

	excluded, err = env.h.gatewayService.GeminiExplicitCacheExclusions(context.Background(), &groupID, 902)
	require.NoError(t, err)
	require.Equal(t, map[int64]struct{}{900: {}, 901: {}}, excluded, "绑定缓存的请求只保留持有账号")
}

func (e *cachedBillingEnv) serve(method, target, body string, params gin.Params, call func(*gin.Context)) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, e.group))
	c.Params = params
	c.Set(string(middleware.ContextKeyAPIKey), e.apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: e.apiKey.UserID, Concurrency: 10})
	call(c)
	return rec
}

// waitForUsage 等待异步计费任务落库（无 worker 池时同步执行，直接返回）。
func (e *cachedBillingEnv) waitForUsage(t *testing.T, n int) []float64 {
	t.Helper()
	require.Eventually(t, func() bool { return len(e.usage.totals()) >= n }, time.Second, 10*time.Millisecond)
	return e.usage.totals()
}

func (e *cachedBillingEnv) create() *httptest.ResponseRecorder {
	return e.serve(http.MethodPost, "/v1beta/cachedContents",
		`{"model":"models/gemini-2.5-pro","ttl":"86400s","contents":[{"role":"user","parts":[{"text":"doc"}]}]}`,
		nil, e.h.GeminiCachedContentsCreate)
}

func (e *cachedBillingEnv) seed(expire time.Time) {
	e.repo.records["abc"] = &service.GeminiCachedContent{
		ID: 100, PublicID: "abc", UserID: 3, APIKeyID: 7, GroupID: 42, AccountID: 900,
		UpstreamName: "cachedContents/up1", Model: "gemini-2.5-pro", UpstreamModel: "models/gemini-2.5-pro",
		TotalTokenCount: cachedBillingTokens, ExpireTime: expire,
	}
}

func (e *cachedBillingEnv) patch() *httptest.ResponseRecorder {
	return e.serve(http.MethodPatch, "/v1beta/cachedContents/abc?updateMask=ttl", `{"ttl":"86400s"}`,
		gin.Params{{Key: "cacheID", Value: "abc"}}, e.h.GeminiCachedContentsPatch)
}

// TestGeminiCachedContentsPatch_UsesChannelUsageRecordedAtCreate 延长有效期沿用创建时的渠道计费口径：
// 计费模型来源为请求模型时按别名定价，用量记录带渠道 ID。
func TestGeminiCachedContentsPatch_UsesChannelUsageRecordedAtCreate(t *testing.T) {
	env := newCachedBillingEnv(t, 500, 900)
	env.seed(time.Now().Add(time.Hour))
	env.repo.records["abc"].ChannelUsage = service.ChannelUsageFields{
		ChannelID: 5, OriginalModel: "gemini-alias", ChannelMappedModel: "gemini-2.5-pro",
		BillingModelSource: service.BillingModelSourceRequested,
	}

	rec := env.patch()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	totals := env.waitForUsage(t, 1)
	require.Len(t, totals, 1)
	require.InDelta(t, cachedBillingTokens*cachedBillingAliasStoragePrice*23, totals[0], 0.01, "按创建时的计费模型（请求模型别名）计存储费")
	usageLog := env.usage.first()
	require.NotNil(t, usageLog.ChannelID)
	require.Equal(t, int64(5), *usageLog.ChannelID)
	require.True(t, strings.HasPrefix(usageLog.RequestID, "gcache:patch:abc:"), usageLog.RequestID)
}

func cachedBillingCreateBody(expire time.Time) string {
	return fmt.Sprintf(`{"name":"cachedContents/up1","usageMetadata":{"totalTokenCount":%d},"expireTime":%q}`,
		cachedBillingTokens, expire.UTC().Format(time.RFC3339Nano))
}

func TestGeminiCachedContentsCreate_ChargesInputAndStorage(t *testing.T) {
	env := newCachedBillingEnv(t, 500, 900)
	env.upstream.createBody = cachedBillingCreateBody(time.Now().Add(100 * time.Hour))

	rec := env.create()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	totals := env.waitForUsage(t, 1)
	require.Len(t, totals, 1)
	require.InDelta(t, cachedBillingTokens*cachedBillingInputPrice+cachedBillingTokens*cachedBillingStoragePrice*24, totals[0], 1e-9)
	require.Len(t, env.repo.records, 1)
	for _, record := range env.repo.records {
		require.False(t, record.ExpireTime.After(time.Now().Add(24*time.Hour)), "本地到期时间不超过请求的有效期")
		require.Equal(t, "gcache:create:"+record.PublicID, env.usage.first().RequestID, "费用行按缓存 ID 可检索")
	}
}

func TestGeminiCachedContentsCreate_UnaffordableStorageDeletesUpstreamAndBillsInputOnly(t *testing.T) {
	env := newCachedBillingEnv(t, 1, 900)
	env.upstream.createBody = cachedBillingCreateBody(time.Now().Add(24 * time.Hour))

	rec := env.create()
	require.NotEqual(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "error")
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodDelete), "删除已创建的上游缓存，避免产生存储费")
	require.Empty(t, env.repo.records, "不登记缓存")
	totals := env.waitForUsage(t, 1)
	require.Len(t, totals, 1)
	require.InDelta(t, cachedBillingTokens*cachedBillingInputPrice, totals[0], 1e-9, "只收已发生的缓存 token 输入费")
}

func TestGeminiCachedContentsCreate_UnusableUpstreamResponseCleansUpWithoutRetry(t *testing.T) {
	env := newCachedBillingEnv(t, 500, 900, 901)
	env.upstream.createBody = `{"name":"cachedContents/up1","expireTime":"2099-01-01T00:00:00Z"}`

	rec := env.create()
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPost), "不换号重建")
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodDelete), "删除已创建的上游缓存")
	require.Empty(t, env.repo.records)
	require.Empty(t, env.usage.totals())
}

func TestGeminiCachedContentsPatch_ConcurrentExtensionsChargeOnlyActualProgress(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	original := time.Now().Add(time.Hour)
	env.seed(original)
	env.upstream.beforePost = func(n int) {
		if n == 1 {
			// 第一个 PATCH 的上游调用返回前，第二个 PATCH 完整执行（读到的仍是旧到期时间）。
			require.Equal(t, http.StatusOK, env.patch().Code)
		}
	}
	require.Equal(t, http.StatusOK, env.patch().Code)

	totals := env.waitForUsage(t, 1)
	final := env.repo.records["abc"].ExpireTime
	fair := cachedBillingTokens * cachedBillingStoragePrice * final.Sub(original).Hours()
	sum := 0.0
	for _, v := range totals {
		sum += v
	}
	require.InDelta(t, fair, sum, cachedBillingTokens*cachedBillingStoragePrice*(time.Minute.Hours()), "两次延长合计只收实际推进的时长")
}

func TestGeminiCachedContentsPatch_PersistFailureDoesNotCharge(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	env.seed(time.Now().Add(time.Hour))
	env.repo.failUpdate = true

	rec := env.patch()
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch))
	require.Empty(t, env.usage.totals(), "落库失败不收费")
}

func TestGeminiCachedContentsPatch_UnaffordableExtensionRejectedBeforeUpstream(t *testing.T) {
	env := newCachedBillingEnv(t, 1, 900)
	env.seed(time.Now().Add(time.Hour))

	rec := env.patch()
	require.NotEqual(t, http.StatusOK, rec.Code)
	require.Zero(t, env.upstream.methodCalls(http.MethodPatch), "余额不足以支付延长部分时不调用上游")
	require.Empty(t, env.usage.totals())
}

// 上游对缓存不存在的响应：AI Studio 403；Vertex 缓存资源接口 404，文案含上游缓存数字 ID。
var cachedBillingUpstreamNotFoundCases = []struct {
	name    string
	failure cachedBillingUpstreamFailure
}{
	{name: "ai studio 403", failure: cachedBillingUpstreamFailure{status: http.StatusForbidden,
		body: `{"error":{"code":403,"message":"CachedContent not found (or permission denied)","status":"PERMISSION_DENIED"}}`}},
	{name: "vertex 404", failure: cachedBillingUpstreamFailure{status: http.StatusNotFound,
		body: `{"error":{"code":404,"message":"Cached content 8079767076522164224 is not found.","status":"NOT_FOUND"}}`}},
}

func TestGeminiCachedContentsPatch_UpstreamNotFoundForgetsAndReturnsStandardForbidden(t *testing.T) {
	for _, tc := range cachedBillingUpstreamNotFoundCases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCachedBillingEnv(t, 1000, 900)
			env.seed(time.Now().Add(time.Hour))
			env.upstream.failures = map[string]cachedBillingUpstreamFailure{http.MethodPatch: tc.failure}

			rec := env.patch()
			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Equal(t, service.GeminiCachedContentNotFoundResponse, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "8079767076522164224", "不应暴露上游缓存 ID")
			require.Equal(t, []int64{100}, env.repo.deleted, "上游已不存在的缓存应从本地移除")
			require.Empty(t, env.usage.totals(), "上游未延长不收费")
		})
	}
}

func TestGeminiCachedContentsDelete_UpstreamNotFoundStillForgets(t *testing.T) {
	for _, tc := range cachedBillingUpstreamNotFoundCases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCachedBillingEnv(t, 1000, 900)
			env.seed(time.Now().Add(time.Hour))
			env.upstream.failures = map[string]cachedBillingUpstreamFailure{http.MethodDelete: tc.failure}

			rec := env.serve(http.MethodDelete, "/v1beta/cachedContents/abc", "",
				gin.Params{{Key: "cacheID", Value: "abc"}}, env.h.GeminiCachedContentsDelete)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.JSONEq(t, `{}`, rec.Body.String())
			require.Equal(t, []int64{100}, env.repo.deleted)
		})
	}
}
