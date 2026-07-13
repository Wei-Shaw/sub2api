//go:build integration

package routes

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
)

type crossPlatformExtraConcurrencySettings struct {
	extraConcurrencySettingRepository
	platformReserves string
}

func (r crossPlatformExtraConcurrencySettings) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	values, err := r.extraConcurrencySettingRepository.GetMultiple(ctx, keys)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(r.platformReserves) != "" {
		values[service.SettingKeyExtraConcurrencyPlatformReserves] = r.platformReserves
	}
	return values, nil
}

type crossPlatformAdmissionCapacity struct {
	byPlatform map[string]service.AdmissionCapacitySnapshot
}

func (c crossPlatformAdmissionCapacity) AdmissionCapacity(_ context.Context, platform string) (service.AdmissionCapacitySnapshot, error) {
	return c.byPlatform[platform], nil
}

type crossPlatformFailoverUpstream struct {
	anthropicAccountID int64
	arrivals           chan int64
	releaseAnthropic   <-chan struct{}
	calls              atomic.Int32
}

func (u *crossPlatformFailoverUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	u.arrivals <- accountID
	if accountID == u.anthropicAccountID {
		<-u.releaseAnthropic
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"api_error","message":"switch platform"}}`)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"msg_cross_platform","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
		)),
	}, nil
}

func (u *crossPlatformFailoverUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type crossPlatformExtraConcurrencyHarness struct {
	router               *gin.Engine
	store                service.GatewayAdmissionStore
	observer             *observingOpenAIGatewayAdmissionStore
	upstream             *crossPlatformFailoverUpstream
	releaseAnthropic     chan<- struct{}
	userID               int64
	anthropicAccountID   int64
	antigravityAccountID int64
}

func newCrossPlatformExtraConcurrencyHarness(
	t *testing.T,
	settings crossPlatformExtraConcurrencySettings,
) *crossPlatformExtraConcurrencyHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	rdb := startAuthRouteRedis(t, ctx)
	groupID := int64(2601)
	userID := int64(4601)
	anthropicAccountID := int64(1601)
	antigravityAccountID := int64(1602)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	accounts := []service.Account{
		{
			ID:          anthropicAccountID,
			Name:        "cross-platform-anthropic",
			Platform:    service.PlatformAnthropic,
			Type:        service.AccountTypeAPIKey,
			Concurrency: 1,
			Priority:    0,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"api_key":  "anthropic-key",
				"base_url": "https://api.anthropic.com",
			},
			Extra:         map[string]any{"anthropic_passthrough": true},
			GroupIDs:      []int64{groupID},
			AccountGroups: []service.AccountGroup{{AccountID: anthropicAccountID, GroupID: groupID}},
		},
		{
			ID:          antigravityAccountID,
			Name:        "cross-platform-antigravity",
			Platform:    service.PlatformAntigravity,
			Type:        service.AccountTypeUpstream,
			Concurrency: 1,
			Priority:    1,
			Status:      service.StatusActive,
			Schedulable: true,
			Credentials: map[string]any{
				"api_key":  "antigravity-upstream-key",
				"base_url": "https://antigravity.example.com",
			},
			Extra:         map[string]any{"mixed_scheduling": true},
			GroupIDs:      []int64{groupID},
			AccountGroups: []service.AccountGroup{{AccountID: antigravityAccountID, GroupID: groupID}},
		},
	}
	accountRepo := openAIExtraConcurrencyAccountRepository{accounts: accounts}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.MaxAccountSwitches = 2
	legacyConcurrency := service.NewConcurrencyService(repository.NewConcurrencyCache(rdb, 1, 30))
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	settingService := service.NewSettingService(settings, cfg)
	releaseAnthropic := make(chan struct{}, 1)
	upstream := &crossPlatformFailoverUpstream{
		anthropicAccountID: anthropicAccountID,
		arrivals:           make(chan int64, 4),
		releaseAnthropic:   releaseAnthropic,
	}
	t.Cleanup(func() {
		select {
		case releaseAnthropic <- struct{}{}:
		default:
		}
	})
	schedulerSnapshot := service.NewSchedulerSnapshotService(
		&extraConcurrencySchedulerCache{accounts: []*service.Account{&accounts[0], &accounts[1]}},
		nil,
		nil,
		nil,
		nil,
	)
	rateLimitService := &service.RateLimitService{}
	gatewayService := service.NewGatewayService(
		accountRepo,
		&extraConcurrencyGroupRepository{group: group},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		schedulerSnapshot,
		legacyConcurrency,
		service.NewBillingService(cfg, nil),
		rateLimitService,
		billingCacheService,
		nil,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		settingService,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	antigravityGatewayService := service.NewAntigravityGatewayService(
		accountRepo,
		nil,
		schedulerSnapshot,
		nil,
		rateLimitService,
		upstream,
		settingService,
		nil,
	)
	admissionStore := repository.NewGatewayAdmissionStore(rdb, 30*time.Second)
	observer := newObservingOpenAIGatewayAdmissionStore(admissionStore)
	admission := service.NewGatewayAdmission(
		observer,
		gatewayService,
		crossPlatformAdmissionCapacity{byPlatform: map[string]service.AdmissionCapacitySnapshot{
			service.PlatformAnthropic: {
				TotalConcurrency:   1,
				AccountConcurrency: map[int64]int{anthropicAccountID: 1},
			},
			service.PlatformAntigravity: {
				TotalConcurrency:   1,
				AccountConcurrency: map[int64]int{antigravityAccountID: 1},
			},
		}},
	)
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		AutoScaleEnabled: false,
	})
	pool.Stop()
	gatewayHandler := handler.NewGatewayHandler(
		gatewayService,
		nil,
		antigravityGatewayService,
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
		ID:      3601,
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
	router.POST("/v1/messages", func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Group, group))
		c.Set(string(servermiddleware.ContextKeyAPIKey), apiKey)
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{
			UserID:           userID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		gatewayHandler.Messages(c)
	})

	return &crossPlatformExtraConcurrencyHarness{
		router:               router,
		store:                admissionStore,
		observer:             observer,
		upstream:             upstream,
		releaseAnthropic:     releaseAnthropic,
		userID:               userID,
		anthropicAccountID:   anthropicAccountID,
		antigravityAccountID: antigravityAccountID,
	}
}

func (h *crossPlatformExtraConcurrencyHarness) request(requestName string) *httptest.ResponseRecorder {
	body := `{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), openAIExtraConcurrencyRequestNameKey{}, requestName))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func (h *crossPlatformExtraConcurrencyHarness) allowAnthropicFailure() {
	select {
	case h.releaseAnthropic <- struct{}{}:
	default:
	}
}

func occupyCrossPlatformUserStandardLease(t *testing.T, h *crossPlatformExtraConcurrencyHarness, requestID string) {
	t.Helper()
	result, err := h.store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     requestID,
		UserID:        h.userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    1,
		WaitTimeout:   10 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, service.AdmissionClassStandard, result.Class)
	t.Cleanup(func() {
		_ = h.store.ReleaseUserLease(context.Background(), h.userID, requestID)
	})
}

func TestGatewayMessagesExtraCrossPlatformDoesNotBypassNewPlatformStandardWaiter(t *testing.T) {
	harness := newCrossPlatformExtraConcurrencyHarness(t, crossPlatformExtraConcurrencySettings{
		extraConcurrencySettingRepository: extraConcurrencySettingRepository{waitTimeoutSeconds: 1},
	})
	occupyCrossPlatformUserStandardLease(t, harness, "cross-platform-user-standard-blocker")

	blockerRequest := service.TargetLeaseRequest{
		RequestID:        "antigravity-standard-blocker",
		Platform:         service.PlatformAntigravity,
		AccountID:        harness.antigravityAccountID,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      10 * time.Second,
	}
	blocker, err := harness.store.TryAcquireTargetLease(t.Context(), blockerRequest)
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	t.Cleanup(func() {
		_ = harness.store.ReleaseTargetLease(context.Background(), blockerRequest.Platform, blockerRequest.AccountID, blockerRequest.RequestID)
	})

	waiterRequest := blockerRequest
	waiterRequest.RequestID = "antigravity-standard-waiter"
	waiter, err := harness.store.TryAcquireTargetLease(t.Context(), waiterRequest)
	require.NoError(t, err)
	require.False(t, waiter.Acquired)
	t.Cleanup(func() {
		_ = harness.store.ReleaseTargetLease(context.Background(), waiterRequest.Platform, waiterRequest.AccountID, waiterRequest.RequestID)
	})

	responses := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responses <- harness.request("cross-platform-waiter")
	}()
	userAttempt := requireObservedOpenAIUserLeaseAttempt(t, harness.observer.userAttempts, "CROSS-PLATFORM-WAITER", true)
	require.Equal(t, service.AdmissionClassExtra, userAttempt.result.Class)

	var targetAttempts []observedOpenAITargetLeaseAttempt
	dispatchTimer := time.NewTimer(3 * time.Second)
	defer dispatchTimer.Stop()
waitForAnthropicDispatch:
	for {
		select {
		case accountID := <-harness.upstream.arrivals:
			require.Equal(t, harness.anthropicAccountID, accountID)
			break waitForAnthropicDispatch
		case attempt := <-harness.observer.targetAttempts:
			targetAttempts = append(targetAttempts, attempt)
		case response := <-responses:
			t.Fatalf("request returned before the initial Anthropic dispatch: status=%d body=%s target_attempts=%+v", response.Code, response.Body.String(), targetAttempts)
		case <-dispatchTimer.C:
			t.Fatalf("timed out waiting for the initial Anthropic dispatch; target_attempts=%+v", targetAttempts)
		}
	}

	require.NoError(t, harness.store.ReleaseTargetLease(
		t.Context(),
		blockerRequest.Platform,
		blockerRequest.AccountID,
		blockerRequest.RequestID,
	))
	harness.allowAnthropicFailure()

	response := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "cross-platform-waiter")
	require.Equal(t, http.StatusTooManyRequests, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "EXTRA_CONCURRENCY_UNAVAILABLE")
	require.Equal(t, int32(1), harness.upstream.calls.Load(), "extra request must not reach Antigravity ahead of its standard waiter")
}

func TestGatewayMessagesExtraCrossPlatformHonorsAntigravityFullStandardReservation(t *testing.T) {
	harness := newCrossPlatformExtraConcurrencyHarness(t, crossPlatformExtraConcurrencySettings{
		extraConcurrencySettingRepository: extraConcurrencySettingRepository{waitTimeoutSeconds: 1},
		platformReserves:                  `{"antigravity":{"reserve_percent":100,"min_reserved_slots":0}}`,
	})
	occupyCrossPlatformUserStandardLease(t, harness, "cross-platform-reserve-user-standard-blocker")

	responses := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responses <- harness.request("cross-platform-reserve")
	}()
	userAttempt := requireObservedOpenAIUserLeaseAttempt(t, harness.observer.userAttempts, "CROSS-PLATFORM-RESERVE", true)
	require.Equal(t, service.AdmissionClassExtra, userAttempt.result.Class)

	select {
	case accountID := <-harness.upstream.arrivals:
		require.Equal(t, harness.anthropicAccountID, accountID)
	case response := <-responses:
		t.Fatalf("request returned before the initial Anthropic dispatch: status=%d body=%s", response.Code, response.Body.String())
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the initial Anthropic dispatch")
	}
	harness.allowAnthropicFailure()

	antigravityAttempt := requireObservedOpenAITargetLeaseAttempt(
		t,
		harness.observer.targetAttempts,
		"CROSS-PLATFORM-RESERVE",
		harness.antigravityAccountID,
		false,
	)
	require.Equal(t, service.PlatformAntigravity, antigravityAttempt.request.Platform)
	require.Equal(t, service.AdmissionClassExtra, antigravityAttempt.request.Class)
	require.Equal(t, 1, antigravityAttempt.request.PlatformCapacity)
	require.Equal(t, 1, antigravityAttempt.request.ReservedSlots)

	response := requireOpenAIExtraConcurrencyHTTPResponse(t, responses, "cross-platform-reserve")
	require.Equal(t, http.StatusTooManyRequests, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "EXTRA_CONCURRENCY_UNAVAILABLE")
	require.Equal(t, int32(1), harness.upstream.calls.Load(), "100% Antigravity standard reservation must reject the extra failover")
}
