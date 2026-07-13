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

type antigravityMessagesExtraSchedulerCache struct {
	service.SchedulerCache
	account *service.Account
}

func (c *antigravityMessagesExtraSchedulerCache) GetSnapshot(_ context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	if c.account == nil || bucket.Platform != service.PlatformAntigravity {
		return nil, true, nil
	}
	return []*service.Account{c.account}, true, nil
}

func (c *antigravityMessagesExtraSchedulerCache) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if c.account != nil && c.account.ID == id {
		return c.account, nil
	}
	return nil, nil
}

type antigravityMessagesExtraGroupRepository struct {
	service.GroupRepository
	group *service.Group
}

func (r *antigravityMessagesExtraGroupRepository) GetByID(_ context.Context, id int64) (*service.Group, error) {
	if r.group != nil && r.group.ID == id {
		return r.group, nil
	}
	return nil, service.ErrNoAvailableAccounts
}

func (r *antigravityMessagesExtraGroupRepository) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.GetByID(ctx, id)
}

type antigravityMessagesExtraSettingRepository struct {
	service.SettingRepository
	waitTimeoutSeconds int
	reservePercent     float64
	minReservedSlots   int
	disabled           bool
}

func (r antigravityMessagesExtraSettingRepository) GetMultiple(context.Context, []string) (map[string]string, error) {
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

type antigravityMessagesExtraCapacity struct {
	accountID int64
}

func (c antigravityMessagesExtraCapacity) AdmissionCapacity(_ context.Context, platform string) (service.AdmissionCapacitySnapshot, error) {
	if platform != service.PlatformAntigravity {
		return service.AdmissionCapacitySnapshot{}, nil
	}
	return service.AdmissionCapacitySnapshot{
		TotalConcurrency:   1,
		AccountConcurrency: map[int64]int{c.accountID: 1},
	}, nil
}

type antigravityMessagesRejectLegacyConcurrencyCache struct {
	service.ConcurrencyCache
}

func (antigravityMessagesRejectLegacyConcurrencyCache) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	return false, errors.New("legacy concurrency path used for Antigravity Messages")
}

type antigravityMessagesExtraUpstream struct {
	arrivals chan int64
	release  <-chan struct{}
	calls    atomic.Int32
}

func (u *antigravityMessagesExtraUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	if u.arrivals != nil {
		u.arrivals <- accountID
	}
	if u.release != nil {
		<-u.release
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"msg_antigravity_extra","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"hello from antigravity"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":3}}`,
		)),
	}, nil
}

func (u *antigravityMessagesExtraUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type antigravityMessagesExtraRoutesHarness struct {
	router    *gin.Engine
	upstream  *antigravityMessagesExtraUpstream
	store     service.GatewayAdmissionStore
	userID    int64
	accountID int64
}

func newAntigravityMessagesExtraRoutesHarness(
	t *testing.T,
	settings antigravityMessagesExtraSettingRepository,
	upstream *antigravityMessagesExtraUpstream,
	accountConcurrencyOverride ...int,
) *antigravityMessagesExtraRoutesHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rdb := startAuthRouteRedis(t, context.Background())
	groupID := int64(2501)
	accountID := int64(1501)
	userID := int64(4501)
	accountConcurrency := 1
	if len(accountConcurrencyOverride) > 0 {
		accountConcurrency = accountConcurrencyOverride[0]
	}
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAntigravity,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "antigravity-extra",
		Platform:    service.PlatformAntigravity,
		Type:        service.AccountTypeUpstream,
		Concurrency: accountConcurrency,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":  "antigravity-upstream-key",
			"base_url": "https://antigravity.example.com",
			"model_mapping": map[string]any{
				"claude-sonnet-4-5": "claude-sonnet-4-5",
			},
		},
		GroupIDs:      []int64{groupID},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	groupRepo := &antigravityMessagesExtraGroupRepository{group: group}
	schedulerSnapshot := service.NewSchedulerSnapshotService(
		&antigravityMessagesExtraSchedulerCache{account: account},
		nil,
		nil,
		nil,
		nil,
	)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)
	rateLimitService := &service.RateLimitService{}
	if upstream == nil {
		upstream = &antigravityMessagesExtraUpstream{}
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
	settingService := service.NewSettingService(settings, cfg)
	antigravityGatewayService := service.NewAntigravityGatewayService(
		nil,
		nil,
		schedulerSnapshot,
		nil,
		rateLimitService,
		upstream,
		settingService,
		nil,
	)
	admissionStore := repository.NewGatewayAdmissionStore(rdb, 5*time.Second)
	admission := service.NewGatewayAdmission(
		admissionStore,
		gatewayService,
		antigravityMessagesExtraCapacity{accountID: accountID},
	)
	legacyConcurrency := service.NewConcurrencyService(antigravityMessagesRejectLegacyConcurrencyCache{})
	if settings.disabled {
		legacyConcurrency = service.NewConcurrencyService(repository.NewConcurrencyCache(rdb, 1, 30))
	}
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
		ID:      3501,
		UserID:  userID,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               userID,
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
	return &antigravityMessagesExtraRoutesHarness{
		router:    router,
		upstream:  upstream,
		store:     admissionStore,
		userID:    userID,
		accountID: accountID,
	}
}

func (h *antigravityMessagesExtraRoutesHarness) request() *httptest.ResponseRecorder {
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func TestGatewayAntigravityMessagesExtraConcurrencyReachesUpstreamWithRealRedis(t *testing.T) {
	harness := newAntigravityMessagesExtraRoutesHarness(t, antigravityMessagesExtraSettingRepository{}, nil)
	blocker, err := harness.store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "antigravity-standard-blocker",
		UserID:        harness.userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	require.Equal(t, service.AdmissionClassStandard, blocker.Class)
	t.Cleanup(func() {
		_ = harness.store.ReleaseUserLease(context.Background(), harness.userID, "antigravity-standard-blocker")
	})

	recorder := harness.request()

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "hello from antigravity")
	require.Equal(t, int32(1), harness.upstream.calls.Load())
}

func TestGatewayAntigravityMessagesExtraConcurrencyReserveBlocksUpstreamWithRealRedis(t *testing.T) {
	harness := newAntigravityMessagesExtraRoutesHarness(t, antigravityMessagesExtraSettingRepository{
		waitTimeoutSeconds: 1,
		reservePercent:     100,
		minReservedSlots:   1,
	}, nil)
	blocker, err := harness.store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "antigravity-reserve-standard-blocker",
		UserID:        harness.userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	require.Equal(t, service.AdmissionClassStandard, blocker.Class)
	t.Cleanup(func() {
		_ = harness.store.ReleaseUserLease(context.Background(), harness.userID, "antigravity-reserve-standard-blocker")
	})

	recorder := harness.request()

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "EXTRA_CONCURRENCY_UNAVAILABLE", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Zero(t, harness.upstream.calls.Load())
}

func TestGatewayAntigravityMessagesStandardAccountTimeoutReturnsConcurrencyError(t *testing.T) {
	harness := newAntigravityMessagesExtraRoutesHarness(t, antigravityMessagesExtraSettingRepository{
		waitTimeoutSeconds: 1,
	}, nil)
	blocker, err := harness.store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "antigravity-account-blocker",
		Platform:         service.PlatformAntigravity,
		AccountID:        harness.accountID,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	t.Cleanup(func() {
		_ = harness.store.ReleaseTargetLease(context.Background(), service.PlatformAntigravity, harness.accountID, "antigravity-account-blocker")
	})

	recorder := harness.request()

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "rate_limit_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Zero(t, harness.upstream.calls.Load())
}

func TestGatewayAntigravityMessagesUnlimitedAccountPreservesLegacySemantics(t *testing.T) {
	harness := newAntigravityMessagesExtraRoutesHarness(t, antigravityMessagesExtraSettingRepository{}, nil, 0)
	blocker, err := harness.store.TryAcquireUserLease(t.Context(), service.UserLeaseRequest{
		RequestID:     "antigravity-unlimited-standard-blocker",
		UserID:        harness.userID,
		StandardLimit: 1,
		ExtraLimit:    1,
		MaxWaiting:    20,
		WaitTimeout:   2 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, blocker.Acquired)
	t.Cleanup(func() {
		_ = harness.store.ReleaseUserLease(context.Background(), harness.userID, "antigravity-unlimited-standard-blocker")
	})

	recorder := harness.request()

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "hello from antigravity")
	require.Equal(t, int32(1), harness.upstream.calls.Load())
}

func TestGatewayAdmissionUnlimitedTargetDoesNotConsumeFinitePlatformCapacity(t *testing.T) {
	rdb := startAuthRouteRedis(t, context.Background())
	store := repository.NewGatewayAdmissionStore(rdb, 5*time.Second)

	unlimited, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID: "unlimited-platform-target",
		Platform:  service.PlatformAntigravity,
		AccountID: 1601,
		Class:     service.AdmissionClassExtra,
		Unlimited: true,
	})
	require.NoError(t, err)
	require.True(t, unlimited.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseTargetLease(context.Background(), service.PlatformAntigravity, 1601, "unlimited-platform-target")
	})

	finite, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "finite-platform-target",
		Platform:         service.PlatformAntigravity,
		AccountID:        1602,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
	})
	require.NoError(t, err)
	require.True(t, finite.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseTargetLease(context.Background(), service.PlatformAntigravity, 1602, "finite-platform-target")
	})
}

func TestGatewayAdmissionUnlimitedExtraStillYieldsToStandardWaiter(t *testing.T) {
	rdb := startAuthRouteRedis(t, context.Background())
	store := repository.NewGatewayAdmissionStore(rdb, 5*time.Second)

	busy, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "busy-standard-target",
		Platform:         service.PlatformAntigravity,
		AccountID:        1701,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
	})
	require.NoError(t, err)
	require.True(t, busy.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseTargetLease(context.Background(), service.PlatformAntigravity, 1701, "busy-standard-target")
	})

	waiter, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "queued-standard-target",
		Platform:         service.PlatformAntigravity,
		AccountID:        1701,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      2 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, waiter.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseTargetLease(context.Background(), service.PlatformAntigravity, 1701, "queued-standard-target")
	})

	unlimited, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID: "unlimited-extra-behind-standard",
		Platform:  service.PlatformAntigravity,
		AccountID: 1702,
		Class:     service.AdmissionClassExtra,
		Unlimited: true,
	})
	require.NoError(t, err)
	require.False(t, unlimited.Acquired)
}

func TestGatewayAdmissionUnlimitedTargetClearsPriorFinitePending(t *testing.T) {
	rdb := startAuthRouteRedis(t, context.Background())
	store := repository.NewGatewayAdmissionStore(rdb, 5*time.Second)

	busy, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "pending-cleanup-busy",
		Platform:         service.PlatformAntigravity,
		AccountID:        1801,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
	})
	require.NoError(t, err)
	require.True(t, busy.Acquired)

	pending, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "pending-then-unlimited",
		Platform:         service.PlatformAntigravity,
		AccountID:        1801,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      2 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, pending.Acquired)

	unlimited, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID: "pending-then-unlimited",
		Platform:  service.PlatformAntigravity,
		AccountID: 1802,
		Class:     service.AdmissionClassStandard,
		Unlimited: true,
	})
	require.NoError(t, err)
	require.True(t, unlimited.Acquired)
	require.NoError(t, store.ReleaseTargetLease(t.Context(), service.PlatformAntigravity, 1801, "pending-cleanup-busy"))

	finiteExtra, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "finite-extra-after-unlimited",
		Platform:         service.PlatformAntigravity,
		AccountID:        1803,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassExtra,
	})
	require.NoError(t, err)
	require.True(t, finiteExtra.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseTargetLease(context.Background(), service.PlatformAntigravity, 1803, "finite-extra-after-unlimited")
	})
}

func TestGatewayAdmissionStandardWaiterOnlyBlocksItsOwnPlatform(t *testing.T) {
	rdb := startAuthRouteRedis(t, context.Background())
	store := repository.NewGatewayAdmissionStore(rdb, 5*time.Second)

	busy, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "antigravity-platform-busy",
		Platform:         service.PlatformAntigravity,
		AccountID:        1901,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
	})
	require.NoError(t, err)
	require.True(t, busy.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseTargetLease(context.Background(), service.PlatformAntigravity, 1901, "antigravity-platform-busy")
	})

	waiter, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "antigravity-platform-waiter",
		Platform:         service.PlatformAntigravity,
		AccountID:        1901,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassStandard,
		WaitTimeout:      2 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, waiter.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseTargetLease(context.Background(), service.PlatformAntigravity, 1901, "antigravity-platform-waiter")
	})

	otherPlatformExtra, err := store.TryAcquireTargetLease(t.Context(), service.TargetLeaseRequest{
		RequestID:        "gemini-platform-extra",
		Platform:         service.PlatformGemini,
		AccountID:        1902,
		AccountLimit:     1,
		PlatformCapacity: 1,
		Class:            service.AdmissionClassExtra,
	})
	require.NoError(t, err)
	require.True(t, otherPlatformExtra.Acquired)
	t.Cleanup(func() {
		_ = store.ReleaseTargetLease(context.Background(), service.PlatformGemini, 1902, "gemini-platform-extra")
	})
}

func TestGatewayAntigravityMessagesFeatureDisabledKeepsLegacyStandardLimit(t *testing.T) {
	releaseUpstream := make(chan struct{})
	upstream := &antigravityMessagesExtraUpstream{
		arrivals: make(chan int64, 2),
		release:  releaseUpstream,
	}
	harness := newAntigravityMessagesExtraRoutesHarness(t, antigravityMessagesExtraSettingRepository{disabled: true}, upstream)
	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() { responses <- harness.request() }()
	select {
	case <-upstream.arrivals:
	case <-time.After(2 * time.Second):
		t.Fatal("first Antigravity request did not reach upstream")
	}

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
