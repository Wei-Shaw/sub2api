package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type restoreSnapshotUserSubRepo struct {
	service.UserSubscriptionRepository

	created *service.UserSubscription
}

func (r *restoreSnapshotUserSubRepo) Create(_ context.Context, sub *service.UserSubscription) error {
	cp := *sub
	cp.ID = 42
	cp.CreatedAt = time.Date(2026, 5, 7, 10, 30, 0, 0, time.UTC)
	cp.UpdatedAt = cp.CreatedAt
	sub.ID = cp.ID
	sub.CreatedAt = cp.CreatedAt
	sub.UpdatedAt = cp.UpdatedAt
	r.created = &cp
	return nil
}

func TestSubscriptionHandlerRestoreSnapshotMapsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &restoreSnapshotUserSubRepo{}
	svc := service.NewSubscriptionService(nil, repo, nil, nil, nil)
	handler := NewSubscriptionHandler(svc)
	router := gin.New()
	router.POST("/api/v1/admin/subscriptions/restore", handler.RestoreSnapshot)

	body := []byte(`{
		"user_id": 31,
		"group_id": 7,
		"starts_at": "2026-05-01T10:00:00Z",
		"expires_at": "2026-05-31T10:00:00Z",
		"status": "active",
		"daily_window_start": "2026-05-07T00:00:00Z",
		"weekly_window_start": "2026-05-04T00:00:00Z",
		"monthly_window_start": "2026-05-01T00:00:00Z",
		"daily_usage_usd": 1.25,
		"weekly_usage_usd": 2.5,
		"monthly_usage_usd": 8.75,
		"assigned_by": 99,
		"assigned_at": "2026-05-01T10:01:00Z",
		"notes": "restore by sales-man after-sales operation op-1"
	}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, repo.created)
	require.Equal(t, int64(31), repo.created.UserID)
	require.Equal(t, int64(7), repo.created.GroupID)
	require.Equal(t, service.SubscriptionStatusActive, repo.created.Status)
	require.Equal(t, 8.75, repo.created.MonthlyUsageUSD)
	require.NotNil(t, repo.created.DailyWindowStart)
	require.Equal(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), *repo.created.DailyWindowStart)
	require.Contains(t, rec.Body.String(), `"id":42`)
	require.Contains(t, rec.Body.String(), `"monthly_usage_usd":8.75`)
}

func TestSubscriptionHandlerRestoreSnapshotRejectsInvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &restoreSnapshotUserSubRepo{}
	svc := service.NewSubscriptionService(nil, repo, nil, nil, nil)
	handler := NewSubscriptionHandler(svc)
	router := gin.New()
	router.POST("/api/v1/admin/subscriptions/restore", handler.RestoreSnapshot)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscriptions/restore", bytes.NewReader([]byte(`{"group_id":7}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Nil(t, repo.created)
}
