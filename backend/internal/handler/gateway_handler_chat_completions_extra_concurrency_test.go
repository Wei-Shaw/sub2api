//go:build unit

package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type chatCompletionsAdmissionProbeUpstream struct {
	calls atomic.Int32
}

type chatCompletionsDisabledSettingRepository struct {
	service.SettingRepository
}

func (chatCompletionsDisabledSettingRepository) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{
		service.SettingKeyExtraConcurrencyEnabled:            "false",
		service.SettingKeyExtraConcurrencyWaitTimeoutSeconds: "1",
		service.SettingKeyExtraConcurrencyReservePercent:     "0",
		service.SettingKeyExtraConcurrencyMinReservedSlots:   "0",
		service.SettingKeyExtraConcurrencyPlatformReserves:   "{}",
	}, nil
}

type chatCompletionsTargetLeaseStore struct {
	mu              sync.Mutex
	users           map[string]service.AdmissionClass
	targetRequestID string
}

func (s *chatCompletionsTargetLeaseStore) TryAcquireUserLease(_ context.Context, request service.UserLeaseRequest) (service.UserLeaseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if class, ok := s.users[request.RequestID]; ok {
		return service.UserLeaseResult{Acquired: true, Class: class}, nil
	}
	if len(s.users) >= 2 {
		return service.UserLeaseResult{}, nil
	}
	class := service.AdmissionClassStandard
	if len(s.users) == 1 {
		class = service.AdmissionClassExtra
	}
	s.users[request.RequestID] = class
	return service.UserLeaseResult{Acquired: true, Class: class}, nil
}

func (s *chatCompletionsTargetLeaseStore) RenewUserLease(_ context.Context, _ int64, requestID string, class service.AdmissionClass) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.users[requestID] == class, nil
}

func (s *chatCompletionsTargetLeaseStore) ReleaseUserLease(_ context.Context, _ int64, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, requestID)
	return nil
}

func (s *chatCompletionsTargetLeaseStore) TryAcquireTargetLease(_ context.Context, request service.TargetLeaseRequest) (service.TargetLeaseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.targetRequestID == "" {
		s.targetRequestID = request.RequestID
	}
	return service.TargetLeaseResult{Acquired: s.targetRequestID == request.RequestID}, nil
}

func (s *chatCompletionsTargetLeaseStore) BeginTargetDispatch(_ context.Context, request service.TargetDispatchRequest) (service.TargetDispatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return service.TargetDispatchResult{Started: s.targetRequestID == request.RequestID}, nil
}

func (s *chatCompletionsTargetLeaseStore) RenewTargetLease(_ context.Context, _ string, _ int64, requestID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.targetRequestID == requestID, nil
}

func (s *chatCompletionsTargetLeaseStore) ReleaseTargetLease(_ context.Context, _ string, _ int64, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.targetRequestID == requestID {
		s.targetRequestID = ""
	}
	return nil
}

type chatCompletionsRetryUpstream struct {
	calls    atomic.Int32
	failedA  atomic.Bool
	arrivals chan string
	release  <-chan struct{}
}

func (u *chatCompletionsRetryUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	requestName := "B"
	if strings.Contains(string(body), "request-a") {
		requestName = "A"
	}
	u.arrivals <- requestName
	if requestName == "A" && u.failedA.CompareAndSwap(false, true) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"message":"rate limited"}}`)),
		}, nil
	}
	if u.release != nil {
		<-u.release
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_chat_retry","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5","stop_reason":"","usage":{"input_tokens":1}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"ok"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n"))),
	}, nil
}

func (u *chatCompletionsRetryUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *chatCompletionsAdmissionProbeUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.calls.Add(1)
	return nil, errors.New("chat completions upstream should not be reached")
}

func (u *chatCompletionsAdmissionProbeUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestGatewayHandlerChatCompletionsUsesExtraAdmissionWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2110)
	accountID := int64(1110)
	userID := int64(4110)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "anthropic-chat-extra",
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
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	upstream := &chatCompletionsAdmissionProbeUpstream{}
	h, _, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, &config.Config{})
	h.gatewayAdmission = service.NewGatewayAdmission(
		expiringTargetAdmissionStore{},
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	apiKey := &service.APIKey{
		ID:      3110,
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
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           userID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.ChatCompletions(c)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "EXTRA_CONCURRENCY_UNAVAILABLE", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Zero(t, upstream.calls.Load())
}

func TestGatewayHandlerChatCompletionsSameAccountRetryKeepsExtraTargetLease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2111)
	accountID := int64(1111)
	userID := int64(4111)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "anthropic-chat-retry",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":               "upstream-key",
			"base_url":              "https://api.anthropic.com",
			"pool_mode":             true,
			"pool_mode_retry_count": 1,
		},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	releaseUpstream := make(chan struct{})
	t.Cleanup(func() { close(releaseUpstream) })
	upstream := &chatCompletionsRetryUpstream{
		arrivals: make(chan string, 3),
		release:  releaseUpstream,
	}
	h, _, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{waitTimeoutSeconds: "2"}, &config.Config{})
	h.gatewayAdmission = service.NewGatewayAdmission(
		&chatCompletionsTargetLeaseStore{users: make(map[string]service.AdmissionClass)},
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)

	request := func(name string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"request-` + name + `"}]}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
		c.Request = req
		apiKey := &service.APIKey{
			ID:      3111,
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
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:           userID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})
		h.ChatCompletions(c)
		return recorder
	}

	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() { responses <- request("a") }()
	require.Equal(t, "A", <-upstream.arrivals)
	go func() { responses <- request("b") }()

	select {
	case arrival := <-upstream.arrivals:
		require.Equal(t, "A", arrival, "same-account retry must run before the waiting request")
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("timed out waiting for the same-account retry")
	}
	releaseUpstream <- struct{}{}
	select {
	case arrival := <-upstream.arrivals:
		require.Equal(t, "B", arrival)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("timed out waiting for the queued request")
	}
	releaseUpstream <- struct{}{}

	firstResponse := <-responses
	secondResponse := <-responses
	require.Equal(t, http.StatusOK, firstResponse.Code, firstResponse.Body.String())
	require.Equal(t, http.StatusOK, secondResponse.Code, secondResponse.Body.String())
	require.Equal(t, int32(3), upstream.calls.Load())
}

func TestGatewayHandlerChatCompletionsWaitedExtraRequestRechecksBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2112)
	accountID := int64(1112)
	userID := int64(4112)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:            accountID,
		Name:          "anthropic-chat-billing-recheck",
		Platform:      service.PlatformAnthropic,
		Type:          service.AccountTypeAPIKey,
		Concurrency:   1,
		Priority:      1,
		Status:        service.StatusActive,
		Schedulable:   true,
		Credentials:   map[string]any{"api_key": "upstream-key", "base_url": "https://api.anthropic.com"},
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	upstream := &chatCompletionsAdmissionProbeUpstream{}
	h, _, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, &config.Config{})
	h.gatewayAdmission = service.NewGatewayAdmission(
		&waitThenAcquireAdmissionStore{},
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)
	balanceCache := &changingBalanceCache{}
	billingCacheService := service.NewBillingCacheService(balanceCache, nil, nil, nil, nil, nil, &config.Config{}, nil)
	defer billingCacheService.Stop()
	h.billingCacheService = billingCacheService

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	apiKey := &service.APIKey{
		ID:      3112,
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
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           userID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.ChatCompletions(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, "billing_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Equal(t, int32(2), balanceCache.reads.Load())
	require.Zero(t, upstream.calls.Load())
}

func TestGatewayHandlerChatCompletionsFeatureDisabledUsesLegacyAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2113)
	accountID := int64(1113)
	userID := int64(4113)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "anthropic-chat-feature-disabled",
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
		AccountGroups: []service.AccountGroup{{AccountID: accountID, GroupID: groupID}},
	}
	upstream := &chatCompletionsRetryUpstream{arrivals: make(chan string, 1)}
	h, legacyCache, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	legacyCache.userSeq = []bool{true}
	legacyCache.accountSeq = []bool{true}
	h.settingService = service.NewSettingService(chatCompletionsDisabledSettingRepository{}, &config.Config{})
	h.gatewayAdmission = service.NewGatewayAdmission(
		expiringTargetAdmissionStore{},
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"feature-disabled"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	apiKey := &service.APIKey{
		ID:      3113,
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
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:           userID,
		Concurrency:      1,
		ExtraConcurrency: 1,
	})

	h.ChatCompletions(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, int32(1), upstream.calls.Load())
	require.Equal(t, 1, legacyCache.userAcquireCalls)
}

func TestGatewayHandlerChatCompletionsGeminiMisconfigurationKeepsCCErrorAndReleasesTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(2114)
	accountID := int64(1114)
	userID := int64(4114)
	group := &service.Group{
		ID:       groupID,
		Hydrated: true,
		Platform: service.PlatformGemini,
		Status:   service.StatusActive,
	}
	account := &service.Account{
		ID:          accountID,
		Name:        "gemini-chat-misconfigured",
		Platform:    service.PlatformGemini,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Priority:    1,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"api_key": "gemini-key"},
		AccountGroups: []service.AccountGroup{{
			AccountID: accountID,
			GroupID:   groupID,
		}},
	}
	upstream := &chatCompletionsAdmissionProbeUpstream{}
	h, _, cleanup := newSuccessfulExtraConcurrencyHandler(t, group, account, upstream)
	defer cleanup()
	h.settingService = service.NewSettingService(extraConcurrencySettingRepository{}, &config.Config{})
	store := &reusableAdmissionStore{}
	h.gatewayAdmission = service.NewGatewayAdmission(
		store,
		h.gatewayService,
		fixedAdmissionCapacity{accountID: accountID},
	)

	for range 2 {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hello"}]}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
		c.Request = req
		apiKey := &service.APIKey{
			ID:      3114,
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
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
			UserID:           userID,
			Concurrency:      1,
			ExtraConcurrency: 1,
		})

		h.ChatCompletions(c)

		require.Equal(t, http.StatusBadGateway, recorder.Code)
		require.Equal(t, "upstream_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
		require.Equal(t, "Gemini compatibility service is not configured", gjson.GetBytes(recorder.Body.Bytes(), "error.message").String())
	}
	require.Zero(t, upstream.calls.Load())
}
