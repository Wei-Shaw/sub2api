//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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
	mu      sync.Mutex
	records map[string]*service.GeminiCachedContent
	// mutations 按记录 id 保存修改意图。
	mutations  map[int64]cachedBillingMutation
	failCommit bool
	nextID     int64
	deleted    []int64
}

type cachedBillingMutation struct {
	token      string
	leaseUntil time.Time
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
	if !ok || record.APIKeyID != apiKeyID || r.liveLocked(record.ID) == nil {
		return nil, service.ErrGeminiCachedContentNotFound
	}
	cp := *record
	return &cp, nil
}

func (r *cachedBillingRepo) ListForOwner(context.Context, int64, int64, time.Time, int64, int) ([]*service.GeminiCachedContent, error) {
	return nil, nil
}

// liveLocked 返回未删除的记录；调用方须持有 mu。
func (r *cachedBillingRepo) liveLocked(id int64) *service.GeminiCachedContent {
	for _, deleted := range r.deleted {
		if deleted == id {
			return nil
		}
	}
	for _, record := range r.records {
		if record.ID == id {
			return record
		}
	}
	return nil
}

func (r *cachedBillingRepo) BeginMutation(_ context.Context, id int64, baseline time.Time, token string, lease time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.liveLocked(id)
	if record == nil {
		return false, service.ErrGeminiCachedContentNotFound
	}
	if _, pending := r.mutations[id]; pending || !record.ExpireTime.Equal(baseline) {
		return false, nil
	}
	r.mutations[id] = cachedBillingMutation{token: token, leaseUntil: time.Now().Add(lease)}
	return true, nil
}

func (r *cachedBillingRepo) TakeOverStaleMutation(_ context.Context, id int64, token string, lease time.Duration) (*service.GeminiCachedContent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.liveLocked(id)
	mutation, pending := r.mutations[id]
	if record == nil || !pending || mutation.leaseUntil.After(time.Now()) {
		return nil, nil
	}
	r.mutations[id] = cachedBillingMutation{token: token, leaseUntil: time.Now().Add(lease)}
	cp := *record
	return &cp, nil
}

func (r *cachedBillingRepo) CommitMutation(_ context.Context, id int64, token string, expireTime time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failCommit {
		return errors.New("db unavailable")
	}
	record := r.liveLocked(id)
	mutation, pending := r.mutations[id]
	if record == nil || !pending || mutation.token != token {
		return service.ErrGeminiCachedContentMutationConflict
	}
	record.ExpireTime = expireTime
	delete(r.mutations, id)
	return nil
}

func (r *cachedBillingRepo) ReleaseMutation(_ context.Context, id int64, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if mutation, pending := r.mutations[id]; pending && mutation.token == token {
		delete(r.mutations, id)
	}
	return nil
}

func (r *cachedBillingRepo) ListStaleMutations(_ context.Context, limit int) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []int64
	for id, mutation := range r.mutations {
		if len(ids) < limit && mutation.leaseUntil.Before(time.Now()) && r.liveLocked(id) != nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (r *cachedBillingRepo) SoftDelete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = append(r.deleted, id)
	return nil
}

// pendingMutation 返回记录是否有未清除的修改意图。
func (r *cachedBillingRepo) pendingMutation(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, pending := r.mutations[id]
	return pending
}

// expireLease 让记录上的修改意图租约立即过期。
func (r *cachedBillingRepo) expireLease(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mutation := r.mutations[id]
	mutation.leaseUntil = time.Now().Add(-time.Second)
	r.mutations[id] = mutation
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
	// patchTTLs 按顺序记录 PATCH 请求体中的 ttl。
	patchTTLs  []string
	beforePost func(n int)
	// failures 按请求方法返回上游错误响应；transportErrors 按请求方法返回传输层错误（上游结果未知）。
	failures        map[string]cachedBillingUpstreamFailure
	transportErrors map[string]error
}

type cachedBillingUpstreamFailure struct {
	status int
	body   string
}

func (u *cachedBillingUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	var patch struct {
		TTL string `json:"ttl"`
	}
	if req.Method == http.MethodPatch {
		_ = json.NewDecoder(req.Body).Decode(&patch)
	}
	u.mu.Lock()
	u.calls = append(u.calls, req.Method+" "+req.URL.Path)
	n := len(u.calls)
	if req.Method == http.MethodPatch {
		u.patchTTLs = append(u.patchTTLs, patch.TTL)
	}
	failure, failed := u.failures[req.Method]
	transportErr := u.transportErrors[req.Method]
	u.mu.Unlock()
	if transportErr != nil {
		return nil, transportErr
	}
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
		seconds, _ := strconv.ParseInt(strings.TrimSuffix(patch.TTL, "s"), 10, 64)
		body = fmt.Sprintf(`{"name":"cachedContents/up1","expireTime":%q}`, time.Now().Add(time.Duration(seconds)*time.Second).UTC().Format(time.RFC3339Nano))
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
	repo := &cachedBillingRepo{records: map[string]*service.GeminiCachedContent{}, mutations: map[int64]cachedBillingMutation{}}
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

func (e *cachedBillingEnv) patchTTL(id, ttl string) *httptest.ResponseRecorder {
	return e.serve(http.MethodPatch, "/v1beta/cachedContents/"+id+"?updateMask=ttl", fmt.Sprintf(`{"ttl":%q}`, ttl),
		gin.Params{{Key: "cacheID", Value: id}}, e.h.GeminiCachedContentsPatch)
}

// requireTTLNear 断言发往上游的 ttl（"Ns"）与 want 相差不超过 5 秒。
func requireTTLNear(t *testing.T, ttl string, want time.Duration) {
	t.Helper()
	seconds, err := strconv.ParseInt(strings.TrimSuffix(ttl, "s"), 10, 64)
	require.NoError(t, err, ttl)
	require.InDelta(t, want.Seconds(), float64(seconds), 5, ttl)
}

// 同一缓存的修改串行执行：第一个 PATCH 的上游调用返回前，第二个 PATCH 直接得到 409 且不调用上游，
// 本地到期时间与上游最后生效的修改一致，只按第一个 PATCH 实际推进的时长收费。
func TestGeminiCachedContentsPatch_ConcurrentUpdateConflictsAndKeepsUpstreamInSync(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	env.seed(time.Now().Add(time.Hour))
	env.upstream.beforePost = func(n int) {
		if n == 1 {
			rec := env.patchTTL("abc", "86400s")
			require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), `"ABORTED"`)
		}
	}

	rec := env.patchTTL("abc", "7200s")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch), "冲突的修改不调用上游")
	require.WithinDuration(t, time.Now().Add(2*time.Hour), env.repo.records["abc"].ExpireTime, 5*time.Second, "本地到期时间与上游一致")
	require.False(t, env.repo.pendingMutation(100))
	totals := env.waitForUsage(t, 1)
	require.Len(t, totals, 1)
	require.InDelta(t, cachedBillingTokens*cachedBillingStoragePrice, totals[0], cachedBillingTokens*cachedBillingStoragePrice*time.Minute.Hours(), "只收推进的 1 小时")
}

// 上游已修改但本地写回失败：不计费并保留修改意图；租约内的修改返回 409，
// 租约过期后后台恢复把上游有效期回滚到修改前的到期时间并清除意图。
func TestGeminiCachedContentsPatch_CommitFailureRollsBackUpstreamWithoutCharge(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	baseline := time.Now().Add(time.Hour)
	env.seed(baseline)
	env.repo.failCommit = true

	require.Equal(t, http.StatusInternalServerError, env.patch().Code)
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch))
	require.True(t, env.repo.pendingMutation(100), "上游可能已修改：保留修改意图")
	require.Empty(t, env.usage.totals(), "未提交的修改不收费")

	env.repo.failCommit = false
	require.Equal(t, http.StatusConflict, env.patch().Code, "租约内的修改被拒绝")
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch))

	env.repo.expireLease(100)
	recovered, err := env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Equal(t, 2, env.upstream.methodCalls(http.MethodPatch))
	requireTTLNear(t, env.upstream.patchTTLs[1], time.Until(baseline))
	require.False(t, env.repo.pendingMutation(100))
	require.True(t, env.repo.records["abc"].ExpireTime.Equal(baseline), "本地到期时间保持修改前的值")
	require.Empty(t, env.usage.totals())
}

// 租约过期的遗留修改意图在下一次修改前先回滚，随后的修改按修改前的到期时间计费。
func TestGeminiCachedContentsPatch_StaleMutationRolledBackBeforeNextUpdate(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	baseline := time.Now().Add(time.Hour)
	env.seed(baseline)
	env.repo.failCommit = true
	require.Equal(t, http.StatusInternalServerError, env.patch().Code)
	env.repo.failCommit = false
	env.repo.expireLease(100)

	rec := env.patch()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 3, env.upstream.methodCalls(http.MethodPatch), "失败的修改、回滚、本次修改")
	requireTTLNear(t, env.upstream.patchTTLs[1], time.Until(baseline))
	require.Equal(t, "86400s", env.upstream.patchTTLs[2])
	totals := env.waitForUsage(t, 1)
	require.Len(t, totals, 1)
	require.InDelta(t, cachedBillingTokens*cachedBillingStoragePrice*23, totals[0], 0.01)
}

// 上游结果未知（传输层错误）时保留修改意图等待回滚；上游明确拒绝时清除修改意图。两种情况都不收费。
func TestGeminiCachedContentsPatch_MutationKeptOnlyWhenUpstreamOutcomeUnknown(t *testing.T) {
	t.Run("transport error", func(t *testing.T) {
		env := newCachedBillingEnv(t, 1000, 900)
		env.seed(time.Now().Add(time.Hour))
		env.upstream.transportErrors = map[string]error{http.MethodPatch: errors.New("connection reset")}

		require.Equal(t, http.StatusBadGateway, env.patch().Code)
		require.True(t, env.repo.pendingMutation(100))
		require.Empty(t, env.usage.totals())
	})
	t.Run("upstream rejected", func(t *testing.T) {
		env := newCachedBillingEnv(t, 1000, 900)
		env.seed(time.Now().Add(time.Hour))
		env.upstream.failures = map[string]cachedBillingUpstreamFailure{http.MethodPatch: {status: http.StatusBadRequest,
			body: `{"error":{"code":400,"message":"invalid ttl","status":"INVALID_ARGUMENT"}}`}}

		require.Equal(t, http.StatusBadRequest, env.patch().Code)
		require.False(t, env.repo.pendingMutation(100))
		require.Empty(t, env.usage.totals())
	})
}

// 修改前的到期时间已过：恢复时删除上游缓存与本地记录。
func TestGeminiCachedContentMutationRecovery_DeletesUpstreamWhenBaselineExpired(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	env.seed(time.Now().Add(-time.Minute))
	env.repo.mutations[100] = cachedBillingMutation{token: "stale", leaseUntil: time.Now().Add(-time.Second)}

	recovered, err := env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodDelete))
	require.Zero(t, env.upstream.methodCalls(http.MethodPatch))
	require.Equal(t, []int64{100}, env.repo.deleted)
}

// cachedBillingReservationCache 是启用在途预留时的计费缓存桩：余额与订阅用量取固定值，各维度预留按 key 记账。
type cachedBillingReservationCache struct {
	service.BillingCache
	mu       sync.Mutex
	balance  float64
	sub      *service.SubscriptionCacheData
	reserved map[string]map[string]float64
}

func (c *cachedBillingReservationCache) GetUserBalance(context.Context, int64) (float64, error) {
	return c.balance, nil
}

func (c *cachedBillingReservationCache) GetSubscriptionCache(context.Context, int64, int64) (*service.SubscriptionCacheData, error) {
	return c.sub, nil
}

func (c *cachedBillingReservationCache) reserveLocked(key, requestID string, amount float64) float64 {
	if c.reserved == nil {
		c.reserved = map[string]map[string]float64{}
	}
	if c.reserved[key] == nil {
		c.reserved[key] = map[string]float64{}
	}
	others := 0.0
	for _, v := range c.reserved[key] {
		others += v
	}
	c.reserved[key][requestID] = amount
	return others
}

func (c *cachedBillingReservationCache) ReserveInflightBalance(_ context.Context, userID int64, requestID string, amount, balance float64, _ time.Duration) (bool, float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := fmt.Sprintf("user:%d", userID)
	others := c.reserveLocked(key, requestID, amount)
	if len(c.reserved[key]) > 1 && balance-others < amount {
		delete(c.reserved[key], requestID)
		return false, others, nil
	}
	return true, others, nil
}

func (c *cachedBillingReservationCache) ReleaseInflightBalance(_ context.Context, userID int64, requestID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.reserved[fmt.Sprintf("user:%d", userID)], requestID)
	return nil
}

func (c *cachedBillingReservationCache) ReserveInflightScope(_ context.Context, scope, requestID string, amount float64, _ time.Duration) (float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reserveLocked(scope, requestID, amount), nil
}

func (c *cachedBillingReservationCache) ReleaseInflightScope(_ context.Context, scope, requestID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.reserved[scope], requestID)
	return nil
}

func enableCachedBillingReservations(t *testing.T, env *cachedBillingEnv, cache *cachedBillingReservationCache) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Billing.InflightReservation.Enabled = true
	bcs := service.NewBillingCacheService(cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(bcs.Stop)
	env.h.billingCacheService = bcs
}

// 确定金额的续期费按 API Key 与余额维度预留：第一笔续期（约 $103.5）尚未入账时，
// 第二个缓存的续期超出 Key 剩余额度或余额，在调用上游前被拒绝。
func TestGeminiCachedContentsPatch_UnsettledChargeHoldsKeyQuotaAndBalance(t *testing.T) {
	for _, tc := range []struct {
		name           string
		balance, quota float64
	}{
		{name: "api key quota", balance: 1000, quota: 110},
		{name: "balance", balance: 110},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newCachedBillingEnv(t, tc.balance, 900)
			env.seed(time.Now().Add(time.Hour))
			second := *env.repo.records["abc"]
			second.ID, second.PublicID, second.UpstreamName = 101, "def", "cachedContents/up2"
			env.repo.records["def"] = &second
			env.apiKey.Quota = tc.quota
			enableCachedBillingReservations(t, env, &cachedBillingReservationCache{balance: tc.balance})
			env.upstream.beforePost = func(n int) {
				if n == 1 {
					rec := env.patchTTL("def", "86400s")
					require.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
				}
			}

			require.Equal(t, http.StatusOK, env.patch().Code)
			require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch), "第二笔续期没有到达上游")
		})
	}
}

// 订阅模式的确定金额费用按订阅维度预留：第一笔未入账时第二笔超出日剩余额度被拒绝，释放后放行。
func TestReserveGeminiCachedContentCharge_UnsettledChargeHoldsSubscriptionHeadroom(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	limit := 110.0
	env.group.SubscriptionType = service.SubscriptionTypeSubscription
	env.group.DailyLimitUSD = &limit
	enableCachedBillingReservations(t, env, &cachedBillingReservationCache{balance: 1000,
		sub: &service.SubscriptionCacheData{Status: service.SubscriptionStatusActive, ExpiresAt: time.Now().Add(time.Hour)}})
	sub := &service.UserSubscription{ID: 1, UserID: env.apiKey.UserID, GroupID: env.group.ID, Status: service.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(time.Hour)}
	reserve := func() (func(), error) {
		var done func()
		var err error
		env.serve(http.MethodPatch, "/", `{}`, nil, func(c *gin.Context) {
			done, err = env.h.reserveGeminiCachedContentCharge(c, env.apiKey, sub, 103.5)
		})
		return done, err
	}

	firstDone, err := reserve()
	require.NoError(t, err)
	_, err = reserve()
	require.ErrorIs(t, err, service.ErrDailyLimitExceeded)
	firstDone()
	secondDone, err := reserve()
	require.NoError(t, err)
	secondDone()
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

// 持有账号暂时不支持显式缓存（能力被关闭）时，恢复流程不能把修改意图当作已回滚而清除；
// 能力恢复后再回滚上游并清除意图。
func TestGeminiCachedContentMutationRecovery_KeepsIntentWhileAccountCannotCache(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	env.accounts[900].Credentials["base_url"] = "https://gw.example.com"
	env.accounts[900].Credentials[service.GeminiExplicitCacheUpstreamCredentialKey] = true
	baseline := time.Now().Add(time.Hour)
	env.seed(baseline)
	env.repo.failCommit = true
	require.Equal(t, http.StatusInternalServerError, env.patch().Code)
	require.True(t, env.repo.pendingMutation(100))
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch))
	env.repo.failCommit = false
	env.repo.expireLease(100)

	env.accounts[900].Credentials[service.GeminiExplicitCacheUpstreamCredentialKey] = false
	recovered, err := env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.Error(t, err, "账号暂不支持缓存时恢复失败")
	require.Zero(t, recovered)
	require.True(t, env.repo.pendingMutation(100), "未回滚上游前保留修改意图")
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch), "不改动上游")
	require.Empty(t, env.repo.deleted)

	env.accounts[900].Credentials[service.GeminiExplicitCacheUpstreamCredentialKey] = true
	env.repo.expireLease(100)
	recovered, err = env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Equal(t, 2, env.upstream.methodCalls(http.MethodPatch))
	requireTTLNear(t, env.upstream.patchTTLs[1], time.Until(baseline))
	require.False(t, env.repo.pendingMutation(100))
	require.True(t, env.repo.records["abc"].ExpireTime.Equal(baseline))
	require.Empty(t, env.usage.totals())
}

// 持有账号长期不支持显式缓存，且修改前到期时间加上单次最长有效期都已过去：上游缓存必已自行过期，
// 恢复流程不再保留意图，直接移除本地记录。
func TestGeminiCachedContentMutationRecovery_AbandonsIntentOnceUpstreamMustHaveExpired(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	env.accounts[900].Credentials["base_url"] = "https://gw.example.com"
	env.accounts[900].Credentials[service.GeminiExplicitCacheUpstreamCredentialKey] = true
	env.seed(time.Now().Add(time.Hour))
	env.repo.failCommit = true
	require.Equal(t, http.StatusInternalServerError, env.patch().Code)
	env.repo.failCommit = false
	env.repo.expireLease(100)
	env.accounts[900].Credentials[service.GeminiExplicitCacheUpstreamCredentialKey] = false

	// 修改前到期时间 + 24h 上限 + 1h 容忍仍未过去：保留意图。
	env.repo.records["abc"].ExpireTime = time.Now().Add(-20 * time.Hour)
	recovered, err := env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.Error(t, err)
	require.Zero(t, recovered)
	require.True(t, env.repo.pendingMutation(100))
	require.Empty(t, env.repo.deleted)

	env.repo.expireLease(100)
	env.repo.records["abc"].ExpireTime = time.Now().Add(-26 * time.Hour)
	recovered, err = env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch), "不改动上游")
	require.Zero(t, env.upstream.methodCalls(http.MethodDelete))
	require.Equal(t, []int64{100}, env.repo.deleted)
	require.Empty(t, env.usage.totals())

	recovered, err = env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.NoError(t, err)
	require.Zero(t, recovered, "作废的记录不再被选中")
}

// 作废判定的边界：默认上限 24h 加 1h 容忍；配置上限低于 24h 时仍按 24h 兜底。
func TestGeminiCachedContentMutationRecovery_AbandonBoundary(t *testing.T) {
	disableCache := func(env *cachedBillingEnv) {
		env.accounts[900].Credentials["base_url"] = "https://gw.example.com"
		env.accounts[900].Credentials[service.GeminiExplicitCacheUpstreamCredentialKey] = false
	}
	t.Run("within grace keeps intent", func(t *testing.T) {
		env := newCachedBillingEnv(t, 1000, 900)
		env.seed(time.Now().Add(-25*time.Hour + time.Minute))
		env.repo.mutations[100] = cachedBillingMutation{token: "stale", leaseUntil: time.Now().Add(-time.Second)}
		disableCache(env)

		recovered, err := env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
		require.Error(t, err)
		require.Zero(t, recovered)
		require.True(t, env.repo.pendingMutation(100))
		require.Empty(t, env.repo.deleted)
	})
	t.Run("configured max ttl below default still waits a day", func(t *testing.T) {
		env := newCachedBillingEnv(t, 1000, 900)
		env.seed(time.Now().Add(-12 * time.Hour))
		env.repo.mutations[100] = cachedBillingMutation{token: "stale", leaseUntil: time.Now().Add(-time.Second)}
		disableCache(env)
		cfg := &config.Config{}
		cfg.Gateway.GeminiCachedContentMaxTTLSeconds = 3600
		compat := service.NewGeminiMessagesCompatService(&cachedBillingAccountRepo{accounts: env.accounts}, nil, nil, nil, nil, nil, env.upstream, nil, cfg)
		svc := service.NewGeminiCachedContentService(env.repo, compat, cfg)

		recovered, err := svc.RecoverStaleMutations(context.Background())
		require.Error(t, err)
		require.Zero(t, recovered)
		require.True(t, env.repo.pendingMutation(100))

		env.repo.expireLease(100)
		env.repo.records["abc"].ExpireTime = time.Now().Add(-26 * time.Hour)
		recovered, err = svc.RecoverStaleMutations(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, recovered)
		require.Equal(t, []int64{100}, env.repo.deleted)
	})
}

// 持有账号可用但上游回滚持续失败（如 Key 已失效）：上游缓存必已自行过期后不再重试，移除本地记录。
func TestGeminiCachedContentMutationRecovery_AbandonsIntentWhenRollbackKeepsFailing(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	env.seed(time.Now().Add(-time.Minute))
	env.repo.mutations[100] = cachedBillingMutation{token: "stale", leaseUntil: time.Now().Add(-time.Second)}
	env.upstream.failures = map[string]cachedBillingUpstreamFailure{http.MethodDelete: {status: http.StatusUnauthorized,
		body: `{"error":{"code":401,"message":"API key not valid","status":"UNAUTHENTICATED"}}`}}

	recovered, err := env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.Error(t, err, "上游缓存可能仍存活：保留意图等待重试")
	require.Zero(t, recovered)
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodDelete))
	require.True(t, env.repo.pendingMutation(100))
	require.Empty(t, env.repo.deleted)

	env.repo.expireLease(100)
	env.repo.records["abc"].ExpireTime = time.Now().Add(-26 * time.Hour)
	recovered, err = env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Equal(t, 2, env.upstream.methodCalls(http.MethodDelete), "账号可用时仍先尝试回滚")
	require.Equal(t, []int64{100}, env.repo.deleted)
	require.Empty(t, env.usage.totals())
}

// 持有账号已删除时没有凭据可回滚上游：恢复流程移除本地记录并清除意图，不再重试。
func TestGeminiCachedContentMutationRecovery_ForgetsRecordWhenAccountDeleted(t *testing.T) {
	env := newCachedBillingEnv(t, 1000, 900)
	env.seed(time.Now().Add(time.Hour))
	env.repo.failCommit = true
	require.Equal(t, http.StatusInternalServerError, env.patch().Code)
	require.True(t, env.repo.pendingMutation(100))
	env.repo.failCommit = false
	env.repo.expireLease(100)

	delete(env.accounts, 900)
	recovered, err := env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	require.Equal(t, 1, env.upstream.methodCalls(http.MethodPatch), "无凭据，不改动上游")
	require.Zero(t, env.upstream.methodCalls(http.MethodDelete))
	require.Equal(t, []int64{100}, env.repo.deleted, "本地记录作废")
	require.Empty(t, env.usage.totals())
	require.Equal(t, http.StatusForbidden, env.serve(http.MethodGet, "/v1beta/cachedContents/abc", "", gin.Params{{Key: "cacheID", Value: "abc"}}, env.h.GeminiCachedContentsGet).Code, "作废的记录对客户端不可见")

	recovered, err = env.h.geminiCachedContentService.RecoverStaleMutations(context.Background())
	require.NoError(t, err)
	require.Zero(t, recovered, "不再重复处理")
}
