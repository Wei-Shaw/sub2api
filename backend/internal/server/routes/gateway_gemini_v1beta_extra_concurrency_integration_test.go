//go:build integration

package routes

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type geminiV1BetaExtraSchedulerCache struct {
	service.SchedulerCache
	account *service.Account
}

func (c *geminiV1BetaExtraSchedulerCache) GetSnapshot(_ context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	if c.account == nil || bucket.Platform != service.PlatformGemini {
		return nil, true, nil
	}
	return []*service.Account{c.account}, true, nil
}

func (c *geminiV1BetaExtraSchedulerCache) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if c.account != nil && c.account.ID == id {
		return c.account, nil
	}
	return nil, nil
}

type geminiV1BetaExtraGroupRepository struct {
	service.GroupRepository
	group *service.Group
}

func (r *geminiV1BetaExtraGroupRepository) GetByID(_ context.Context, id int64) (*service.Group, error) {
	if r.group != nil && r.group.ID == id {
		return r.group, nil
	}
	return nil, service.ErrNoAvailableAccounts
}

func (r *geminiV1BetaExtraGroupRepository) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.GetByID(ctx, id)
}

type geminiV1BetaExtraSettingRepository struct {
	service.SettingRepository
	waitTimeoutSeconds int
	reservePercent     float64
	minReservedSlots   int
	disabled           bool
}

func (r geminiV1BetaExtraSettingRepository) GetMultiple(context.Context, []string) (map[string]string, error) {
	waitTimeoutSeconds := r.waitTimeoutSeconds
	if waitTimeoutSeconds <= 0 {
		waitTimeoutSeconds = 1
	}
	enabled := "true"
	if r.disabled {
		enabled = "false"
	}
	return map[string]string{
		service.SettingKeyExtraConcurrencyEnabled:            enabled,
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: strconv.Itoa(waitTimeoutSeconds),
		service.SettingKeyExtraConcurrencyReservePercent:     strconv.FormatFloat(r.reservePercent, 'f', -1, 64),
		service.SettingKeyExtraConcurrencyMinReservedSlots:   strconv.Itoa(r.minReservedSlots),
		service.SettingKeyExtraConcurrencyPlatformReserves:   "{}",
	}, nil
}

type geminiV1BetaExtraCapacity struct {
	accountID int64
}

func (c geminiV1BetaExtraCapacity) AdmissionCapacity(_ context.Context, platform string) (service.AdmissionCapacitySnapshot, error) {
	if platform != service.PlatformGemini {
		return service.AdmissionCapacitySnapshot{}, nil
	}
	return service.AdmissionCapacitySnapshot{
		TotalConcurrency:   1,
		AccountConcurrency: map[int64]int{c.accountID: 1},
	}, nil
}

type geminiV1BetaRejectLegacyConcurrencyCache struct {
	service.ConcurrencyCache
}

func (geminiV1BetaRejectLegacyConcurrencyCache) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	return false, errors.New("legacy concurrency path used for Gemini v1beta")
}

type geminiV1BetaExtraUpstream struct {
	calls         atomic.Int32
	arrivals      chan int64
	namedArrivals chan string
	release       <-chan struct{}
	failFirst     bool
}

func (u *geminiV1BetaExtraUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	call := u.calls.Add(1)
	if u.arrivals != nil {
		u.arrivals <- accountID
	}
	if u.namedArrivals != nil {
		requestName, _ := req.Context().Value(openAIExtraConcurrencyRequestNameKey{}).(string)
		u.namedArrivals <- strings.ToUpper(requestName)
	}
	if u.failFirst && call == 1 {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":400,"message":"Invalid project resource name","status":"INVALID_ARGUMENT"}}`)),
		}, nil
	}
	if u.release != nil {
		<-u.release
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"x-request-id": []string{"gemini-v1beta-extra-1"},
		},
		Body: io.NopCloser(strings.NewReader(
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"hello from gemini native"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":3,"totalTokenCount":7},"modelVersion":"gemini-2.5-flash"}`,
		)),
	}, nil
}

func (u *geminiV1BetaExtraUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type geminiV1BetaExtraRoutesHarness struct {
	router   *gin.Engine
	upstream *geminiV1BetaExtraUpstream
	store    service.GatewayAdmissionStore
	userID   int64
}

func newGeminiV1BetaExtraRoutesHarness(
	t *testing.T,
	settings geminiV1BetaExtraSettingRepository,
	upstreamOverride ...*geminiV1BetaExtraUpstream,
) *geminiV1BetaExtraRoutesHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rdb := startAuthRouteRedis(t, context.Background())
	groupID := int64(2501)
	accountID := int64(1501)
	userID := int64(4501)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformGemini,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "gemini-v1beta-extra",
		Platform:    service.PlatformGemini,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":  "gemini-key",
			"base_url": "https://generativelanguage.googleapis.com",
		},
		GroupIDs:      []int64{groupID},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	groupRepo := &geminiV1BetaExtraGroupRepository{group: group}
	schedulerSnapshot := service.NewSchedulerSnapshotService(
		&geminiV1BetaExtraSchedulerCache{account: account},
		nil,
		nil,
		nil,
		nil,
	)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	rateLimitService := &service.RateLimitService{}
	upstream := &geminiV1BetaExtraUpstream{}
	if len(upstreamOverride) > 0 && upstreamOverride[0] != nil {
		upstream = upstreamOverride[0]
	}
	gatewayService := service.NewGatewayService(
		nil,
		groupRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		schedulerSnapshot,
		nil,
		nil,
		rateLimitService,
		billingCacheService,
		nil,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	geminiCompatService := service.NewGeminiMessagesCompatService(
		nil,
		groupRepo,
		nil,
		schedulerSnapshot,
		nil,
		rateLimitService,
		upstream,
		nil,
		cfg,
	)
	settingService := service.NewSettingService(settings, cfg)
	admissionStore := repository.NewGatewayAdmissionStore(rdb, 5*time.Second)
	admission := service.NewGatewayAdmission(
		admissionStore,
		gatewayService,
		geminiV1BetaExtraCapacity{accountID: accountID},
	)
	legacyCache := service.ConcurrencyCache(geminiV1BetaRejectLegacyConcurrencyCache{})
	if settings.disabled {
		legacyCache = repository.NewConcurrencyCache(rdb, 1, 30)
	}
	legacyConcurrency := service.NewConcurrencyService(legacyCache)
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		AutoScaleEnabled: false,
	})
	pool.Stop()
	gatewayHandler := handler.NewGatewayHandler(
		gatewayService,
		geminiCompatService,
		nil,
		nil,
		legacyConcurrency,
		admission,
		billingCacheService,
		nil,
		nil,
		pool,
		nil,
		nil,
		nil,
		cfg,
		settingService,
	)
	apiKey := &service.APIKey{
		ID:      3501,
		UserID:  userID,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               userID,
			Status:           service.StatusActive,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}
	router := gin.New()
	router.POST("/v1beta/models/*modelAction", func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Group, group))
		c.Set(string(servermiddleware.ContextKeyAPIKey), apiKey)
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{
			UserID:           userID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		gatewayHandler.GeminiV1BetaModels(c)
	})
	return &geminiV1BetaExtraRoutesHarness{
		router:   router,
		upstream: upstream,
		store:    admissionStore,
		userID:   userID,
	}
}

func (h *geminiV1BetaExtraRoutesHarness) occupyStandardUserLease(t *testing.T) {
	t.Helper()
	requestID := "gemini-v1beta-standard-blocker"
	lease, err := h.store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     requestID,
		UserID:        h.userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, lease.Acquired)
	require.Equal(t, service.AdmissionClassStandard, lease.Class)
	t.Cleanup(func() {
		_ = h.store.ReleaseUserLease(context.Background(), h.userID, requestID)
	})
}

func (h *geminiV1BetaExtraRoutesHarness) request() *httptest.ResponseRecorder {
	return h.requestWithText("hello")
}

func (h *geminiV1BetaExtraRoutesHarness) requestWithText(text string) *httptest.ResponseRecorder {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"` + text + `"}]}]}`)
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1beta/models/gemini-2.5-flash:generateContent",
		bytes.NewReader(body),
	)
	req = req.WithContext(context.WithValue(req.Context(), openAIExtraConcurrencyRequestNameKey{}, strings.TrimPrefix(text, "request-")))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func TestGatewayGeminiV1BetaExtraConcurrencyUsesRealRedisAndReleasesAdmission(t *testing.T) {
	harness := newGeminiV1BetaExtraRoutesHarness(t, geminiV1BetaExtraSettingRepository{})
	harness.occupyStandardUserLease(t)

	for range 2 {
		recorder := harness.request()

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "hello from gemini native", gjson.GetBytes(recorder.Body.Bytes(), "candidates.0.content.parts.0.text").String())
	}
	require.Equal(t, int32(2), harness.upstream.calls.Load())
}

func TestGatewayGeminiV1BetaSameAccountRetryKeepsTargetLease(t *testing.T) {
	releaseUpstream := make(chan struct{})
	t.Cleanup(func() { close(releaseUpstream) })
	upstream := &geminiV1BetaExtraUpstream{
		namedArrivals: make(chan string, 3),
		release:       releaseUpstream,
		failFirst:     true,
	}
	harness := newGeminiV1BetaExtraRoutesHarness(t, geminiV1BetaExtraSettingRepository{}, upstream)

	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() { responses <- harness.requestWithText("request-a") }()
	require.Equal(t, "A", <-upstream.namedArrivals)
	go func() { responses <- harness.requestWithText("request-b") }()

	select {
	case second := <-upstream.namedArrivals:
		require.Equal(t, "A", second, "same-account retry must run before the waiting request")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the same-account retry")
	}
	releaseUpstream <- struct{}{}
	select {
	case third := <-upstream.namedArrivals:
		require.Equal(t, "B", third)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the queued request")
	}
	releaseUpstream <- struct{}{}

	firstResponse := <-responses
	secondResponse := <-responses
	require.Equal(t, http.StatusOK, firstResponse.Code)
	require.Equal(t, http.StatusOK, secondResponse.Code)
	require.Equal(t, int32(3), upstream.calls.Load())
}

func TestGatewayGeminiV1BetaExtraConcurrencyTimeoutUsesGoogleErrorWithoutUpstream(t *testing.T) {
	harness := newGeminiV1BetaExtraRoutesHarness(t, geminiV1BetaExtraSettingRepository{
		waitTimeoutSeconds: 1,
		reservePercent:     100,
		minReservedSlots:   1,
	})
	harness.occupyStandardUserLease(t)

	recorder := harness.request()

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "RESOURCE_EXHAUSTED", gjson.GetBytes(recorder.Body.Bytes(), "error.status").String())
	require.Equal(t, "EXTRA_CONCURRENCY_UNAVAILABLE", gjson.GetBytes(recorder.Body.Bytes(), "error.details.0.reason").String())
	require.Zero(t, harness.upstream.calls.Load())
}

func TestGatewayGeminiV1BetaFeatureDisabledKeepsLegacyStandardLimit(t *testing.T) {
	releaseUpstream := make(chan struct{})
	upstream := &geminiV1BetaExtraUpstream{
		arrivals: make(chan int64, 2),
		release:  releaseUpstream,
	}
	harness := newGeminiV1BetaExtraRoutesHarness(t, geminiV1BetaExtraSettingRepository{disabled: true}, upstream)
	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() { responses <- harness.request() }()
	<-upstream.arrivals
	go func() { responses <- harness.request() }()

	secondReachedBeforeRelease := false
	select {
	case <-upstream.arrivals:
		secondReachedBeforeRelease = true
	case <-time.After(750 * time.Millisecond):
	}
	close(releaseUpstream)
	firstResponse := <-responses
	secondResponse := <-responses

	require.False(t, secondReachedBeforeRelease)
	require.Equal(t, http.StatusOK, firstResponse.Code)
	require.Equal(t, http.StatusOK, secondResponse.Code)
	require.Equal(t, int32(2), upstream.calls.Load())
}
