//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subscriptionExpiryHandlerStub struct {
	mu        sync.Mutex
	results   map[int64]*service.OpenAISubscriptionExpiryResult
	errors    map[int64]error
	calls     []int64
	active    int
	maxActive int
	started   chan struct{}
	release   <-chan struct{}
}

func (s *subscriptionExpiryHandlerStub) QuerySubscriptionExpiry(_ context.Context, accountID int64) (*service.OpenAISubscriptionExpiryResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, accountID)
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.active--
		s.mu.Unlock()
	}()

	if s.started != nil {
		s.started <- struct{}{}
	}
	if s.release != nil {
		<-s.release
	}
	if err := s.errors[accountID]; err != nil {
		return nil, err
	}
	return s.results[accountID], nil
}

func newSubscriptionExpiryHandlerResult(accountID int64) *service.OpenAISubscriptionExpiryResult {
	return &service.OpenAISubscriptionExpiryResult{
		AccountID: accountID,
		Snapshot: service.OpenAISubscriptionExpirySnapshot{
			Status:    service.OpenAISubscriptionExpiryStatusAvailable,
			ExpiresAt: "2026-08-08T07:23:45Z",
			CheckedAt: "2026-08-07T06:17:00Z",
			Source:    service.OpenAISubscriptionExpirySourceSubscriptions,
			PlanType:  "plus",
			WillRenew: false,
		},
		EffectiveExpiresAt: "2026-08-08T07:23:45Z",
		EffectiveSource:    service.OpenAISubscriptionExpiryEffectiveSourceUpstream,
	}
}

func TestQuerySubscriptionExpiryHandler_ReturnsSingleResultContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionExpiryHandlerStub{
		results: map[int64]*service.OpenAISubscriptionExpiryResult{7: newSubscriptionExpiryHandlerResult(7)},
		errors:  map[int64]error{},
	}
	h := &OpenAIOAuthHandler{subscriptionExpiryService: stub}
	router := gin.New()
	router.POST("/api/v1/admin/openai/accounts/:id/subscription-expiry/query", h.QuerySubscriptionExpiry)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/accounts/7/subscription-expiry/query", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Data service.OpenAISubscriptionExpiryResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, int64(7), envelope.Data.AccountID)
	require.Equal(t, service.OpenAISubscriptionExpiryStatusAvailable, envelope.Data.Snapshot.Status)
	require.Equal(t, "2026-08-08T07:23:45Z", envelope.Data.EffectiveExpiresAt)
	require.Equal(t, service.OpenAISubscriptionExpiryEffectiveSourceUpstream, envelope.Data.EffectiveSource)
}

func TestQuerySubscriptionExpiryBatchHandler_DeduplicatesAndKeepsPerAccountErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionExpiryHandlerStub{
		results: map[int64]*service.OpenAISubscriptionExpiryResult{1: newSubscriptionExpiryHandlerResult(1)},
		errors:  map[int64]error{2: errors.New("account query failed")},
	}
	h := &OpenAIOAuthHandler{subscriptionExpiryService: stub}
	router := gin.New()
	router.POST("/api/v1/admin/openai/accounts/subscription-expiry/query", h.QuerySubscriptionExpiryBatch)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/accounts/subscription-expiry/query", strings.NewReader(`{"account_ids":[1,1,2]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Data struct {
			Results []struct {
				AccountID int64 `json:"account_id"`
				Snapshot  *struct {
					Status string `json:"status"`
				} `json:"snapshot"`
				Error string `json:"error"`
			} `json:"results"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Results, 2)
	require.Equal(t, int64(1), envelope.Data.Results[0].AccountID)
	require.Equal(t, service.OpenAISubscriptionExpiryStatusAvailable, envelope.Data.Results[0].Snapshot.Status)
	require.Equal(t, int64(2), envelope.Data.Results[1].AccountID)
	require.Nil(t, envelope.Data.Results[1].Snapshot)
	require.NotEmpty(t, envelope.Data.Results[1].Error)

	stub.mu.Lock()
	defer stub.mu.Unlock()
	require.ElementsMatch(t, []int64{1, 2}, stub.calls)
}

func TestQuerySubscriptionExpiryBatchHandlerCapsConcurrencyAtFour(t *testing.T) {
	gin.SetMode(gin.TestMode)
	release := make(chan struct{})
	stub := &subscriptionExpiryHandlerStub{
		results: make(map[int64]*service.OpenAISubscriptionExpiryResult),
		errors:  make(map[int64]error),
		started: make(chan struct{}, 5),
		release: release,
	}
	for id := int64(1); id <= 5; id++ {
		stub.results[id] = newSubscriptionExpiryHandlerResult(id)
	}
	h := &OpenAIOAuthHandler{subscriptionExpiryService: stub}
	router := gin.New()
	router.POST("/api/v1/admin/openai/accounts/subscription-expiry/query", h.QuerySubscriptionExpiryBatch)

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/accounts/subscription-expiry/query", strings.NewReader(`{"account_ids":[1,2,3,4,5]}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
	}()

	for i := 0; i < 4; i++ {
		select {
		case <-stub.started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for a batch worker")
		}
	}
	select {
	case <-stub.started:
		t.Fatal("batch started more than four account queries")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for batch response")
	}
	require.Equal(t, http.StatusOK, recorder.Code)
	stub.mu.Lock()
	maxActive := stub.maxActive
	stub.mu.Unlock()
	require.LessOrEqual(t, maxActive, 4)
}

func TestQuerySubscriptionExpiryHandler_NilCapabilityGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewOpenAIOAuthHandler(nil, newStubAdminService(), nil, nil)
	require.Nil(t, h.subscriptionExpiryService)

	router := gin.New()
	router.POST("/api/v1/admin/openai/accounts/:id/subscription-expiry/query", h.QuerySubscriptionExpiry)
	router.POST("/api/v1/admin/openai/accounts/subscription-expiry/query", h.QuerySubscriptionExpiryBatch)

	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/v1/admin/openai/accounts/7/subscription-expiry/query"},
		{path: "/api/v1/admin/openai/accounts/subscription-expiry/query", body: `{"account_ids":[7]}`},
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusBadRequest, recorder.Code, tc.path)
	}
}
