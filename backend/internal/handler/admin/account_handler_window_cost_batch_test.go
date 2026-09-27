//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type windowCostCountingUsageRepo struct {
	service.UsageLogRepository

	batchCalls  atomic.Int64
	singleCalls atomic.Int64
}

func (r *windowCostCountingUsageRepo) GetAccountWindowStats(context.Context, int64, time.Time) (*usagestats.AccountStats, error) {
	r.singleCalls.Add(1)
	return &usagestats.AccountStats{StandardCost: 1.5}, nil
}

func (r *windowCostCountingUsageRepo) GetAccountWindowStatsBatch(_ context.Context, accountIDs []int64, _ time.Time) (map[int64]*usagestats.AccountStats, error) {
	r.batchCalls.Add(1)
	out := make(map[int64]*usagestats.AccountStats, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = &usagestats.AccountStats{StandardCost: 1.5}
	}
	return out, nil
}

func TestAccountHandlerListBatchesWindowCostQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	windowStart := time.Now().Add(-time.Hour).Truncate(time.Hour)
	windowEnd := windowStart.Add(5 * time.Hour)
	adminSvc.accounts = make([]service.Account, 0, 50)
	for id := int64(1); id <= 50; id++ {
		adminSvc.accounts = append(adminSvc.accounts, service.Account{
			ID: id, Name: "claude", Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth,
			Status: service.StatusActive, Extra: map[string]any{"window_cost_limit": 100.0},
			SessionWindowStart: &windowStart, SessionWindowEnd: &windowEnd,
		})
	}
	repo := &windowCostCountingUsageRepo{}
	usageSvc := service.NewAccountUsageService(nil, repo, nil, nil, nil, nil, nil, nil, service.NewUsageCache(), nil, nil)
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, usageSvc, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/accounts", handler.List)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=50", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	t.Logf("window cost queries for a 50-account page: batch=%d single=%d", repo.batchCalls.Load(), repo.singleCalls.Load())
	require.EqualValues(t, 1, repo.batchCalls.Load())
	require.Zero(t, repo.singleCalls.Load())

	var payload struct {
		Data struct {
			Items []struct {
				ID                int64    `json:"id"`
				CurrentWindowCost *float64 `json:"current_window_cost"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 50)
	for _, item := range payload.Data.Items {
		require.NotNil(t, item.CurrentWindowCost, "account %d", item.ID)
		require.InDelta(t, 1.5, *item.CurrentWindowCost, 1e-9)
	}
}
