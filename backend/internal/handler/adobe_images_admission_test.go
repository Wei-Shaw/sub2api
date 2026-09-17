//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// adobeAdmissionSlotsCache 记录账号/用户槽的占用与释放，并可按账号拒绝占槽。
type adobeAdmissionSlotsCache struct {
	fakeConcurrencyCache
	mu               sync.Mutex
	busyAccounts     map[int64]bool
	userQueueFull    bool
	userDenied       bool
	accountAttempts  []int64
	accountsAcquired int
	accountsReleased int
	usersAcquired    int
	usersReleased    int
}

func (s *adobeAdmissionSlotsCache) AcquireAccountSlot(_ context.Context, id int64, _ int, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountAttempts = append(s.accountAttempts, id)
	if s.busyAccounts[id] {
		return false, nil
	}
	s.accountsAcquired++
	return true, nil
}

func (s *adobeAdmissionSlotsCache) ReleaseAccountSlot(context.Context, int64, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountsReleased++
	return nil
}

func (s *adobeAdmissionSlotsCache) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userDenied {
		return false, nil
	}
	s.usersAcquired++
	return true, nil
}

func (s *adobeAdmissionSlotsCache) ReleaseUserSlot(context.Context, int64, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usersReleased++
	return nil
}

func (s *adobeAdmissionSlotsCache) IncrementWaitCount(context.Context, int64, int) (bool, error) {
	return !s.userQueueFull, nil
}

type adobeAdmissionFixture struct {
	handler *GatewayHandler
	slots   *adobeAdmissionSlotsCache
	group   *service.Group
	apiKey  *service.APIKey
	cfg     *config.Config
}

func newAdobeAdmissionFixture(t *testing.T, accountIDs ...int64) *adobeAdmissionFixture {
	t.Helper()
	if len(accountIDs) == 0 {
		accountIDs = []int64{9301, 9302}
	}
	accounts := make([]*service.Account, 0, len(accountIDs))
	for _, id := range accountIDs {
		// 没有 access_token：占到槽的账号会在取 token 时失败并换号，测试不会真的打 Firefly。
		accounts = append(accounts, &service.Account{
			ID: id, Platform: service.PlatformAdobe, Type: service.AccountTypeOAuth,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1,
			Credentials: map[string]any{},
		})
	}
	return newAdobeAdmissionFixtureWithAccounts(t, nil, accounts...)
}

// newAdobeAdmissionFixtureWithAccounts 用给定账号构造 fixture；upstream 非空时作为中转号的 HTTP 上游。
func newAdobeAdmissionFixtureWithAccounts(t *testing.T, upstream service.HTTPUpstream, accounts ...*service.Account) *adobeAdmissionFixture {
	t.Helper()
	return newAdobeAdmissionFixtureWithUsage(t, upstream, nil, accounts...)
}

// newAdobeAdmissionFixtureWithUsage 额外注入 usage 仓库，用来断言记账是否真的发生。
func newAdobeAdmissionFixtureWithUsage(t *testing.T, upstream service.HTTPUpstream, usageRepo service.UsageLogRepository, accounts ...*service.Account) *adobeAdmissionFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	groupID := int64(9300)
	group := &service.Group{
		ID: groupID, Hydrated: true, Platform: service.PlatformAdobe,
		Status: service.StatusActive, AllowImageGeneration: true,
	}
	for _, account := range accounts {
		account.AccountGroups = []service.AccountGroup{{AccountID: account.ID, GroupID: groupID}}
	}
	schedulerSnapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: accounts}, nil, nil, nil, nil)
	gatewayService := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, nil, nil, nil, nil, nil, nil, nil,
		schedulerSnapshot, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	openAI := service.NewOpenAIGatewayService(nil, usageRepo, nil, nil, nil, nil, nil, cfg,
		nil, nil, service.NewBillingService(cfg, nil), nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil)

	slots := &adobeAdmissionSlotsCache{busyAccounts: map[int64]bool{}}
	h := &GatewayHandler{
		gatewayService:       gatewayService,
		openAIGatewayService: openAI,
		billingCacheService:  billingCacheService,
		concurrencyHelper:    NewConcurrencyHelper(service.NewConcurrencyService(slots), SSEPingFormatClaude, 0),
		adobeImageService:    service.NewAdobeImageService(nil),
		imageLimiter:         &imageConcurrencyLimiter{},
		maxAccountSwitches:   5,
		cfg:                  cfg,
	}
	apiKey := &service.APIKey{
		ID: 9303, UserID: 9304, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 9304, Concurrency: 10, Balance: 100},
	}
	return &adobeAdmissionFixture{handler: h, slots: slots, group: group, apiKey: apiKey, cfg: cfg}
}

func (f *adobeAdmissionFixture) serve(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return f.serveBody(t, `{"model":"gpt-image-2","prompt":"a cat"}`)
}

func (f *adobeAdmissionFixture) serveBody(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), ctxkey.Group, f.group)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations",
		bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set(string(middleware.ContextKeyAPIKey), f.apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: f.apiKey.UserID, Concurrency: 10})
	f.handler.AdobeImages(c)
	return recorder
}

func (f *adobeAdmissionFixture) assertSlotsReleased(t *testing.T) {
	t.Helper()
	f.slots.mu.Lock()
	defer f.slots.mu.Unlock()
	require.Equal(t, f.slots.accountsAcquired, f.slots.accountsReleased, "account slots leaked")
	require.Equal(t, f.slots.usersAcquired, f.slots.usersReleased, "user slots leaked")
}

func TestAdobeImagesRejectsWhenImageConcurrencyFull(t *testing.T) {
	f := newAdobeAdmissionFixture(t)
	f.cfg.Gateway.ImageConcurrency.Enabled = true
	f.cfg.Gateway.ImageConcurrency.MaxConcurrentRequests = 1
	hold, ok := f.handler.imageLimiter.TryAcquire(true, 1)
	require.True(t, ok)
	defer hold()

	w := f.serve(t)

	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Image generation concurrency limit exceeded")
	require.Empty(t, f.slots.accountAttempts, "must not reach account selection")
	f.assertSlotsReleased(t)
}

func TestAdobeImagesRejectsWhenUserQueueFull(t *testing.T) {
	f := newAdobeAdmissionFixture(t)
	f.slots.userDenied = true
	f.slots.userQueueFull = true

	w := f.serve(t)

	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	require.Empty(t, f.slots.accountAttempts, "must not reach account selection")
	f.assertSlotsReleased(t)
}

func TestAdobeImagesSkipsBusyAccount(t *testing.T) {
	f := newAdobeAdmissionFixture(t)
	f.slots.busyAccounts[9301] = true

	f.serve(t)

	require.Contains(t, f.slots.accountAttempts, int64(9301))
	require.Contains(t, f.slots.accountAttempts, int64(9302), "busy account should be skipped for the next candidate")
	require.Equal(t, 1, f.slots.accountsAcquired)
	f.assertSlotsReleased(t)
}

func TestAdobeImagesAllAccountsBusyReturns429(t *testing.T) {
	f := newAdobeAdmissionFixture(t)
	f.slots.busyAccounts[9301] = true
	f.slots.busyAccounts[9302] = true

	w := f.serve(t)

	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Too many concurrent requests for available accounts")
	require.Zero(t, f.slots.accountsAcquired)
	f.assertSlotsReleased(t)
}

// 并发槽满的账号只是被跳过，不算一次换号：预算为 0 时仍能越过多个 busy 账号选到空闲账号。
func TestAdobeImagesBusyAccountsDoNotConsumeSwitchBudget(t *testing.T) {
	f := newAdobeAdmissionFixture(t, 9311, 9312, 9313)
	f.handler.maxAccountSwitches = 0
	f.slots.busyAccounts[9311] = true
	f.slots.busyAccounts[9312] = true

	f.serve(t)

	require.ElementsMatch(t, []int64{9311, 9312, 9313}, f.slots.accountAttempts)
	require.Equal(t, 1, f.slots.accountsAcquired, "the free account must still be tried")
	f.assertSlotsReleased(t)
}

// Firefly 原生出图没有流式协议：stream=true 落到原生号时明确 400，而不是返回 JSON 冒充 SSE。
func TestAdobeImagesRejectsStreamOnNativeAccount(t *testing.T) {
	f := newAdobeAdmissionFixture(t)

	w := f.serveBody(t, `{"model":"gpt-image-2","prompt":"a cat","stream":true}`)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "stream is not supported")
	f.assertSlotsReleased(t)
}

// adobeBlockingRelayUpstream 在中转请求发出后阻塞，直到测试放行，用来观察「上游仍在跑」时的槽位状态。
type adobeBlockingRelayUpstream struct {
	service.HTTPUpstream
	started chan struct{}
	release chan struct{}
}

func (u *adobeBlockingRelayUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	close(u.started)
	<-u.release
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"message":"relay failed"}}`)),
	}, nil
}

// 上游请求与客户端连接脱钩：客户端断开后上游仍在消耗账号额度，用户槽与账号槽都必须占到尝试结束，
// 否则用户可以「提交后立刻断开」绕过并发上限。
func TestAdobeImagesHoldsSlotsUntilUpstreamFinishesAfterClientDisconnect(t *testing.T) {
	upstream := &adobeBlockingRelayUpstream{started: make(chan struct{}), release: make(chan struct{})}
	relay := &service.Account{
		ID: 9321, Platform: service.PlatformAdobe, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-relay", "base_url": "https://relay.example.com"},
	}
	f := newAdobeAdmissionFixtureWithAccounts(t, upstream, relay)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxkey.Group, f.group))
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations",
		bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"a cat"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set(string(middleware.ContextKeyAPIKey), f.apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: f.apiKey.UserID, Concurrency: 10})

	done := make(chan struct{})
	go func() {
		defer close(done)
		f.handler.AdobeImages(c)
	}()

	select {
	case <-upstream.started:
	case <-time.After(5 * time.Second):
		t.Fatal("relay upstream was never called")
	}
	cancel()
	// context.AfterFunc 式的提前释放会在取消后很快触发；给它足够时间暴露出来。
	time.Sleep(100 * time.Millisecond)

	f.slots.mu.Lock()
	usersReleased, accountsReleased := f.slots.usersReleased, f.slots.accountsReleased
	f.slots.mu.Unlock()
	require.Zero(t, usersReleased, "user slot must be held while the upstream request is still running")
	require.Zero(t, accountsReleased, "account slot must be held while the upstream request is still running")

	close(upstream.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after upstream finished")
	}
	f.assertSlotsReleased(t)
	require.Equal(t, 1, f.slots.usersAcquired)
	require.Equal(t, 1, f.slots.accountsAcquired)
}

func (f *adobeAdmissionFixture) serveGemini(t *testing.T, model, action, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), ctxkey.Group, f.group)
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/"+model+":"+action,
		bytes.NewBufferString(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	subject := middleware.AuthSubject{UserID: f.apiKey.UserID, Concurrency: 10}
	c.Set(string(middleware.ContextKeyAPIKey), f.apiKey)
	c.Set(string(middleware.ContextKeyUser), subject)
	f.handler.AdobeGeminiImages(c, f.apiKey, subject, model, action)
	return recorder
}

// adobeGeminiTestPNG 是 1x1 PNG 的 base64，用来构造中转号返回的 b64_json。
const adobeGeminiTestPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// adobeStaticRelayUpstream 让中转号对出图请求返回固定的 2xx body，其余请求一律 404。
type adobeStaticRelayUpstream struct {
	service.HTTPUpstream
	body     string
	mu       sync.Mutex
	requests []string
}

func (u *adobeStaticRelayUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.requests = append(u.requests, req.URL.String())
	u.mu.Unlock()
	if req.URL.Host != "relay.example.com" {
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(u.body)),
	}, nil
}

func newAdobeGeminiRelayFixture(t *testing.T, relayBody string) (*adobeAdmissionFixture, *adobeStaticRelayUpstream, *openAIWSUsageHandlerUsageLogRepoStub) {
	t.Helper()
	upstream := &adobeStaticRelayUpstream{body: relayBody}
	relay := &service.Account{
		ID: 9331, Platform: service.PlatformAdobe, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-relay", "base_url": "https://relay.example.com"},
	}
	usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
	return newAdobeAdmissionFixtureWithUsage(t, upstream, usageRepo, relay), upstream, usageRepo
}

// 中转只给了网关拿不到的 http url：降级为 fileData 交给客户端，照常按交付张数记账。
func TestAdobeGeminiImagesRelayDegradesURLToFileData(t *testing.T) {
	f, _, usageRepo := newAdobeGeminiRelayFixture(t, `{"created":1,"data":[{"url":"http://cdn.example.com/out.png"}]}`)

	w := f.serveGemini(t, "nano-banana-pro", "generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"a cat"}]}],"generationConfig":{"imageConfig":{"aspectRatio":"16:9"}}}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "http://cdn.example.com/out.png", gjson.Get(w.Body.String(), "candidates.0.content.parts.0.fileData.fileUri").String())
	require.Equal(t, int64(0), gjson.Get(w.Body.String(), "candidates.0.index").Int())

	select {
	case log := <-usageRepo.created:
		require.Equal(t, int64(9331), log.AccountID)
		require.Equal(t, 1, log.ImageCount)
	case <-time.After(5 * time.Second):
		t.Fatal("relay image usage was not recorded")
	}
	f.assertSlotsReleased(t)
}

// 中转 2xx 但一张可用的图都没有：返回 Google 形 502，且不记账。
func TestAdobeGeminiImagesRelayDoesNotRecordUsageWhenNoImageUsable(t *testing.T) {
	f, _, usageRepo := newAdobeGeminiRelayFixture(t, `{"created":1,"data":[{"b64_json":"!!!"}]}`)

	w := f.serveGemini(t, "nano-banana-pro", "generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"a cat"}]}]}`)

	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"status"`, "Gemini 入口必须返回 Google 形错误")
	require.NotContains(t, w.Body.String(), `"type"`)

	select {
	case log := <-usageRepo.created:
		t.Fatalf("usage must not be recorded when no image is delivered, got %+v", log)
	case <-time.After(300 * time.Millisecond):
	}
	f.assertSlotsReleased(t)
}

// 中转部分项无效：只把可用的图交付给客户端，并按交付张数记账。
func TestAdobeGeminiImagesRelayBillsOnlyDeliveredImages(t *testing.T) {
	f, _, usageRepo := newAdobeGeminiRelayFixture(t,
		`{"created":1,"data":[{"b64_json":"`+adobeGeminiTestPNG+`"},{"b64_json":"!!!"}]}`)

	w := f.serveGemini(t, "nano-banana-pro", "generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"a cat"}]}],"generationConfig":{"candidateCount":2}}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, gjson.Get(w.Body.String(), "candidates").Array(), 1)
	require.Equal(t, adobeGeminiTestPNG, gjson.Get(w.Body.String(), "candidates.0.content.parts.0.inlineData.data").String())

	select {
	case log := <-usageRepo.created:
		require.Equal(t, 1, log.ImageCount)
	case <-time.After(5 * time.Second):
		t.Fatal("relay image usage was not recorded")
	}
	f.assertSlotsReleased(t)
}

// 运维配置的别名不在 Adobe 目录里，也不该在 Gemini 入口被当成「非出图模型」拒掉。
func TestAdobeGeminiImagesAllowsUnknownAliasButRejectsOtherPlatformModels(t *testing.T) {
	f := newAdobeAdmissionFixture(t)

	w := f.serveGemini(t, "gemini-2.5-flash", "generateContent", `{"contents":[{"parts":[{"text":"hi"}]}]}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "image model")

	w = f.serveGemini(t, "my-banana", "generateContent", `{"contents":[{"parts":[{"text":"hi"}]}]}`)
	require.NotContains(t, w.Body.String(), "image model", w.Body.String())
	f.assertSlotsReleased(t)
}
