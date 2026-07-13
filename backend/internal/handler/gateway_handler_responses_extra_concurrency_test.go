//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type responsesRetryAdmissionStore struct {
	userClaims     atomic.Int32
	targetClaims   atomic.Int32
	userReleases   atomic.Int32
	targetReleases atomic.Int32
}

func (s *responsesRetryAdmissionStore) TryAcquireUserLease(context.Context, service.UserLeaseRequest) (service.UserLeaseResult, error) {
	s.userClaims.Add(1)
	return service.UserLeaseResult{Acquired: true, Class: service.AdmissionClassExtra}, nil
}

func (s *responsesRetryAdmissionStore) RenewUserLease(context.Context, int64, string, service.AdmissionClass) (bool, error) {
	return true, nil
}

func (s *responsesRetryAdmissionStore) ReleaseUserLease(context.Context, int64, string) error {
	s.userReleases.Add(1)
	return nil
}

func (s *responsesRetryAdmissionStore) TryAcquireTargetLease(context.Context, service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	s.targetClaims.Add(1)
	return service.TargetLeaseResult{Acquired: true}, nil
}

func (s *responsesRetryAdmissionStore) BeginTargetDispatch(context.Context, service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	return service.TargetDispatchResult{Started: true}, nil
}

func (s *responsesRetryAdmissionStore) RenewTargetLease(context.Context, string, int64, string) (bool, error) {
	return true, nil
}

func (s *responsesRetryAdmissionStore) ReleaseTargetLease(context.Context, string, int64, string) error {
	s.targetReleases.Add(1)
	return nil
}

type responsesRetryAdmissionCapacity struct {
	accountID int64
}

func (c responsesRetryAdmissionCapacity) AdmissionCapacity(context.Context, string) (service.AdmissionCapacitySnapshot, error) {
	return service.AdmissionCapacitySnapshot{
		TotalConcurrency:   1,
		AccountConcurrency: map[int64]int{c.accountID: 1},
	}, nil
}

type responsesExtraConcurrencySettingRepository struct {
	extraConcurrencySettingRepository
	enabled string
}

func (r responsesExtraConcurrencySettingRepository) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	values, err := r.extraConcurrencySettingRepository.GetMultiple(ctx, keys)
	if err != nil {
		return nil, err
	}
	values[service.SettingKeyExtraConcurrencyEnabled] = r.enabled
	return values, nil
}

type responsesSameAccountRetryUpstream struct {
	store     *responsesRetryAdmissionStore
	failFirst bool
	cancel    context.CancelFunc
	calls     atomic.Int32

	mu                    sync.Mutex
	accountIDs            []int64
	targetReleasesAtCalls []int32
}

func (u *responsesSameAccountRetryUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	call := u.calls.Add(1)
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	u.targetReleasesAtCalls = append(u.targetReleasesAtCalls, u.store.targetReleases.Load())
	u.mu.Unlock()

	if u.failFirst && call == 1 {
		if u.cancel != nil {
			u.cancel()
		}
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"rate_limit_error","message":"retry on the same account"}}`)),
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"x-request-id": []string{"responses-retry-success"},
		},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_responses_retry","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","stop_reason":"","usage":{"input_tokens":1}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"ok"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
			``,
		}, "\n"))),
	}, nil
}

func (u *responsesSameAccountRetryUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *responsesSameAccountRetryUpstream) snapshot() ([]int64, []int32) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...), append([]int32(nil), u.targetReleasesAtCalls...)
}

func TestGatewayHandlerResponsesUsesExtraAdmissionAndKeepsTargetAcrossSameAccountRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(2104)
	accountID := int64(1104)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "responses-extra-retry",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":   "upstream-key",
			"base_url":  "https://api.anthropic.com",
			"pool_mode": true,
		},
		GroupIDs:      []int64{groupID},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	store := &responsesRetryAdmissionStore{}
	upstream := &responsesSameAccountRetryUpstream{store: store, failFirst: true}
	h, legacyCache, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	legacyCache.userSeq = []bool{true}
	legacyCache.accountSeq = []bool{true}
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, h.cfg)
	h.gatewayAdmission = service.NewGatewayAdmission(
		store,
		h.gatewayService,
		responsesRetryAdmissionCapacity{accountID: accountID},
	)

	apiKey := &service.APIKey{
		ID:      3104,
		UserID:  4104,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               4104,
			Status:           service.StatusActive,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/responses",
		bytes.NewBufferString(`{"model":"claude-sonnet-4-5","stream":false,"input":"hello"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           apiKey.UserID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.Responses(c)

	accountIDs, targetReleasesAtCalls := upstream.snapshot()
	require.Equal(t, int32(1), store.userClaims.Load())
	require.Equal(t, int32(1), store.targetClaims.Load())
	require.Equal(t, int32(1), store.targetReleases.Load())
	require.Equal(t, int32(1), store.userReleases.Load())
	require.Zero(t, legacyCache.userAcquireCalls)
	require.Zero(t, legacyCache.accountAcquireCalls)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, int32(2), upstream.calls.Load())
	require.Equal(t, []int64{accountID, accountID}, accountIDs)
	require.Equal(t, []int32{0, 0}, targetReleasesAtCalls)
}

func TestGatewayHandlerResponsesFeatureDisabledUsesLegacyAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(2106)
	accountID := int64(1106)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "responses-legacy-disabled",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":  "upstream-key",
			"base_url": "https://api.anthropic.com",
		},
		GroupIDs:      []int64{groupID},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	store := &responsesRetryAdmissionStore{}
	upstream := &responsesSameAccountRetryUpstream{store: store}
	h, legacyCache, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	legacyCache.userSeq = []bool{true}
	legacyCache.accountSeq = []bool{true}
	h.settingService = service.NewSettingService(responsesExtraConcurrencySettingRepository{
		extraConcurrencySettingRepository: extraConcurrencySettingRepository{},
		enabled:                           "false",
	}, h.cfg)
	h.gatewayAdmission = service.NewGatewayAdmission(
		store,
		h.gatewayService,
		responsesRetryAdmissionCapacity{accountID: accountID},
	)

	apiKey := &service.APIKey{
		ID:      3106,
		UserID:  4106,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:          4106,
			Status:      service.StatusActive,
			Concurrency: 1,
			Balance:     100,
		},
		Group: group,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/responses",
		bytes.NewBufferString(`{"model":"claude-sonnet-4-5","stream":false,"input":"hello"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      apiKey.UserID,
		Concurrency: 1,
	})

	h.Responses(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Zero(t, store.userClaims.Load())
	require.Zero(t, store.targetClaims.Load())
	require.Equal(t, 1, legacyCache.userAcquireCalls)
	require.Equal(t, 1, legacyCache.userReleaseCalls)
	require.Equal(t, 1, legacyCache.apiKeyTrackCalls)
	require.Equal(t, 1, legacyCache.apiKeyReleaseCalls)
}

func TestGatewayHandlerResponsesCanceledSameAccountRetryReleasesAdmissionWithoutFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(2107)
	accountID := int64(1107)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "responses-extra-cancel",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":   "upstream-key",
			"base_url":  "https://api.anthropic.com",
			"pool_mode": true,
		},
		GroupIDs:      []int64{groupID},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	store := &responsesRetryAdmissionStore{}
	upstream := &responsesSameAccountRetryUpstream{store: store, failFirst: true, cancel: cancel}
	h, legacyCache, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, h.cfg)
	h.gatewayAdmission = service.NewGatewayAdmission(
		store,
		h.gatewayService,
		responsesRetryAdmissionCapacity{accountID: accountID},
	)

	apiKey := &service.APIKey{
		ID:      3107,
		UserID:  4107,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:               4107,
			Status:           service.StatusActive,
			Concurrency:      1,
			ExtraConcurrency: 1,
			Balance:          100,
		},
		Group: group,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/responses",
		bytes.NewBufferString(`{"model":"claude-sonnet-4-5","stream":false,"input":"hello"}`),
	)
	c.Request = req.WithContext(requestCtx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           apiKey.UserID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.Responses(c)

	require.ErrorIs(t, requestCtx.Err(), context.Canceled)
	require.Equal(t, int32(1), upstream.calls.Load())
	require.Empty(t, recorder.Body.String())
	require.Equal(t, int32(1), store.userClaims.Load())
	require.Equal(t, int32(1), store.targetClaims.Load())
	require.Eventually(t, func() bool {
		return store.userReleases.Load() == 1 && store.targetReleases.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Zero(t, legacyCache.userAcquireCalls)
	require.Zero(t, legacyCache.accountAcquireCalls)
}
