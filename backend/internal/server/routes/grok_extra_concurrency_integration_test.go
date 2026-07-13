//go:build integration

package routes

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokExtraConcurrencyCapacity struct {
	accountID int64
}

func (c grokExtraConcurrencyCapacity) AdmissionCapacity(_ context.Context, platform string) (service.AdmissionCapacitySnapshot, error) {
	if platform != service.PlatformGrok {
		return service.AdmissionCapacitySnapshot{}, nil
	}
	return service.AdmissionCapacitySnapshot{
		TotalConcurrency:   1,
		AccountConcurrency: map[int64]int{c.accountID: 1},
	}, nil
}

type grokExtraConcurrencyUpstream struct {
	calls    atomic.Int32
	arrivals chan int64
	release  <-chan struct{}

	mu            sync.Mutex
	lastPath      string
	authorization string
}

func (u *grokExtraConcurrencyUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	u.mu.Lock()
	if req != nil && req.URL != nil {
		u.lastPath = req.URL.Path
	}
	if req != nil {
		u.authorization = req.Header.Get("Authorization")
	}
	u.mu.Unlock()
	if u.arrivals != nil {
		u.arrivals <- accountID
	}
	if u.release != nil {
		<-u.release
	}
	body := `{"id":"resp_grok_extra","object":"response","status":"completed","model":"grok-4.3","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	if req != nil && req.URL != nil && strings.Contains(req.URL.Path, "chat/completions") {
		body = `{"id":"chatcmpl_grok_extra","object":"chat.completion","created":1,"model":"grok-4.3","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{"application/json"},
			"Xai-Request-Id": []string{"grok-extra-request"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (u *grokExtraConcurrencyUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *grokExtraConcurrencyUpstream) requestSnapshot() (string, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastPath, u.authorization
}

type grokExtraConcurrencyRoutesHarness struct {
	router   *gin.Engine
	store    service.GatewayAdmissionStore
	upstream *grokExtraConcurrencyUpstream
	userID   int64
}

func newGrokExtraConcurrencyRoutesHarness(
	t *testing.T,
	settings extraConcurrencySettingRepository,
	upstreamOverride ...*grokExtraConcurrencyUpstream,
) *grokExtraConcurrencyRoutesHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rdb := startAuthRouteRedis(t, context.Background())
	groupID := int64(2401)
	accountID := int64(1401)
	userID := int64(4401)
	account := service.Account{
		ID:          accountID,
		Name:        "grok-extra",
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Concurrency: 1,
		Priority:    0,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token": "grok-access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"base_url":     "https://api.x.ai/v1",
		},
		GroupIDs: []int64{groupID},
	}
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformGrok,
		Status:   service.StatusActive,
	}
	accountRepo := openAIExtraConcurrencyAccountRepository{accounts: []service.Account{account}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	legacyConcurrency := service.NewConcurrencyService(repository.NewConcurrencyCache(rdb, 1, 30))
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	upstream := &grokExtraConcurrencyUpstream{}
	if len(upstreamOverride) > 0 && upstreamOverride[0] != nil {
		upstream = upstreamOverride[0]
	}
	openAIService := service.NewOpenAIGatewayService(
		accountRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		legacyConcurrency,
		service.NewBillingService(cfg, nil),
		nil,
		billingCacheService,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	settingService := service.NewSettingService(settings, cfg)
	admissionStore := repository.NewGatewayAdmissionStore(rdb, 5*time.Second)
	admission := service.NewGatewayAdmission(
		admissionStore,
		nil,
		grokExtraConcurrencyCapacity{accountID: accountID},
	)
	openAIHandler := handler.NewOpenAIGatewayHandler(
		openAIService,
		legacyConcurrency,
		admission,
		billingCacheService,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil,
		nil,
		nil,
		nil,
		cfg,
		settingService,
	)
	apiKey := &service.APIKey{
		ID:      3401,
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
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), apiKey)
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{
			UserID:           userID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		openAIHandler.Responses(c)
	})
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), apiKey)
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{
			UserID:           userID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		openAIHandler.ChatCompletions(c)
	})
	return &grokExtraConcurrencyRoutesHarness{
		router:   router,
		store:    admissionStore,
		upstream: upstream,
		userID:   userID,
	}
}

func (h *grokExtraConcurrencyRoutesHarness) occupyStandard(t *testing.T, requestID string) {
	t.Helper()
	result, err := h.store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     requestID,
		UserID:        h.userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, service.AdmissionClassStandard, result.Class)
	t.Cleanup(func() {
		_ = h.store.ReleaseUserLease(context.Background(), h.userID, requestID)
	})
}

func (h *grokExtraConcurrencyRoutesHarness) responsesRequest() *httptest.ResponseRecorder {
	body := `{"model":"grok-4.3","stream":false,"prompt_cache_key":"grok-extra","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func (h *grokExtraConcurrencyRoutesHarness) chatCompletionsRequest() *httptest.ResponseRecorder {
	body := `{"model":"grok-4.3","stream":false,"prompt_cache_key":"grok-extra-chat","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func TestGrokResponsesExtraConcurrencyReachesUpstreamWithStandardOccupied(t *testing.T) {
	harness := newGrokExtraConcurrencyRoutesHarness(t, extraConcurrencySettingRepository{
		waitTimeoutSeconds: 1,
	})
	harness.occupyStandard(t, "grok-standard-blocker")

	recorder := harness.responsesRequest()

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"id":"resp_grok_extra"`)
	require.Equal(t, int32(1), harness.upstream.calls.Load())
	path, authorization := harness.upstream.requestSnapshot()
	require.Equal(t, "/v1/responses", path)
	require.Equal(t, "Bearer grok-access-token", authorization)
}

func TestGrokResponsesExtraConcurrencyReserveRejectsWithoutUpstream(t *testing.T) {
	harness := newGrokExtraConcurrencyRoutesHarness(t, extraConcurrencySettingRepository{
		waitTimeoutSeconds: 1,
		reservePercent:     100,
		minReservedSlots:   1,
	})
	harness.occupyStandard(t, "grok-reserve-standard-blocker")

	recorder := harness.responsesRequest()

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Contains(t, recorder.Body.String(), "EXTRA_CONCURRENCY_UNAVAILABLE")
	require.Zero(t, harness.upstream.calls.Load())
}

func TestGrokChatCompletionsExtraConcurrencyReachesUpstreamWithStandardOccupied(t *testing.T) {
	harness := newGrokExtraConcurrencyRoutesHarness(t, extraConcurrencySettingRepository{
		waitTimeoutSeconds: 1,
	})
	harness.occupyStandard(t, "grok-chat-standard-blocker")

	recorder := harness.chatCompletionsRequest()

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"id":"chatcmpl_grok_extra"`)
	require.Equal(t, int32(1), harness.upstream.calls.Load())
	path, authorization := harness.upstream.requestSnapshot()
	require.Equal(t, "/v1/chat/completions", path)
	require.Equal(t, "Bearer grok-access-token", authorization)
}

func TestGrokChatCompletionsExtraConcurrencyReserveRejectsWithoutUpstream(t *testing.T) {
	harness := newGrokExtraConcurrencyRoutesHarness(t, extraConcurrencySettingRepository{
		waitTimeoutSeconds: 1,
		reservePercent:     100,
		minReservedSlots:   1,
	})
	harness.occupyStandard(t, "grok-chat-reserve-standard-blocker")

	recorder := harness.chatCompletionsRequest()

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Contains(t, recorder.Body.String(), "EXTRA_CONCURRENCY_UNAVAILABLE")
	require.Zero(t, harness.upstream.calls.Load())
}

func TestGrokResponsesFeatureOffKeepsSecondRequestBehindStandardLimit(t *testing.T) {
	releaseUpstream := make(chan struct{})
	upstream := &grokExtraConcurrencyUpstream{
		arrivals: make(chan int64, 2),
		release:  releaseUpstream,
	}
	harness := newGrokExtraConcurrencyRoutesHarness(t, extraConcurrencySettingRepository{
		disabled: true,
	}, upstream)
	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() { responses <- harness.responsesRequest() }()
	<-upstream.arrivals
	go func() { responses <- harness.responsesRequest() }()

	select {
	case <-upstream.arrivals:
		t.Fatal("feature-off request used extra concurrency before the standard slot was released")
	case <-time.After(300 * time.Millisecond):
	}

	close(releaseUpstream)
	select {
	case <-upstream.arrivals:
	case <-time.After(3 * time.Second):
		t.Fatal("second feature-off request did not proceed after the standard slot was released")
	}
	firstResponse := <-responses
	secondResponse := <-responses
	require.Equal(t, http.StatusOK, firstResponse.Code)
	require.Equal(t, http.StatusOK, secondResponse.Code)
	require.Equal(t, int32(2), upstream.calls.Load())
}
