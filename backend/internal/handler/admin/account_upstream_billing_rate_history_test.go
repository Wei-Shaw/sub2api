package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type upstreamBillingRateHistoryHandlerRepo struct {
	service.AccountRepository
	account *service.Account
	events  []service.UpstreamBillingRateHistoryEvent
	limit   int
}

func (r *upstreamBillingRateHistoryHandlerRepo) GetByID(context.Context, int64) (*service.Account, error) {
	if r.account == nil {
		return nil, service.ErrAccountNotFound
	}
	return r.account, nil
}

func (r *upstreamBillingRateHistoryHandlerRepo) ListUpstreamBillingRateHistory(
	_ context.Context,
	_ int64,
	_ time.Time,
	limit int,
) ([]service.UpstreamBillingRateHistoryEvent, bool, error) {
	r.limit = limit
	return append([]service.UpstreamBillingRateHistoryEvent(nil), r.events...), false, nil
}

func setupUpstreamBillingRateHistoryRouter(repo *upstreamBillingRateHistoryHandlerRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetUpstreamBillingProbeService(service.NewUpstreamBillingProbeService(repo, nil, nil))
	router := gin.New()
	router.GET("/admin/accounts/:id/upstream-billing-rate-history", handler.GetUpstreamBillingRateHistory)
	return router
}

func TestAccountHandlerGetUpstreamBillingRateHistoryReturnsETag(t *testing.T) {
	detectedAt := time.Date(2026, 7, 20, 1, 2, 3, 0, time.UTC)
	repo := &upstreamBillingRateHistoryHandlerRepo{
		account: &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		events: []service.UpstreamBillingRateHistoryEvent{{
			ID: 1, DetectedAt: detectedAt, GroupRateMultiplier: 1,
			ResolvedRateMultiplier: 1, EffectiveRateMultiplier: 1,
		}},
	}
	router := setupUpstreamBillingRateHistoryRouter(repo)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/accounts/7/upstream-billing-rate-history", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "private, no-cache", recorder.Header().Get("Cache-Control"))
	require.Equal(t, service.UpstreamBillingRateHistoryMaxEvents, repo.limit)
	etag := recorder.Header().Get("ETag")
	require.NotEmpty(t, etag)
	var envelope struct {
		Data service.UpstreamBillingRateHistory `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, int64(7), envelope.Data.AccountID)
	require.Equal(t, 90, envelope.Data.RangeDays)
	require.Len(t, envelope.Data.Events, 1)

	notModified := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/accounts/7/upstream-billing-rate-history", nil)
	request.Header.Set("If-None-Match", etag)
	router.ServeHTTP(notModified, request)
	require.Equal(t, http.StatusNotModified, notModified.Code)
}

func TestAccountHandlerGetUpstreamBillingRateHistoryValidatesInput(t *testing.T) {
	repo := &upstreamBillingRateHistoryHandlerRepo{
		account: &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
	}
	router := setupUpstreamBillingRateHistoryRouter(repo)

	for _, path := range []string{
		"/admin/accounts/nope/upstream-billing-rate-history",
		"/admin/accounts/7/upstream-billing-rate-history?days=8",
		"/admin/accounts/7/upstream-billing-rate-history?limit=501",
		"/admin/accounts/7/upstream-billing-rate-history?limit=nope",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusBadRequest, recorder.Code, path)
	}
}
