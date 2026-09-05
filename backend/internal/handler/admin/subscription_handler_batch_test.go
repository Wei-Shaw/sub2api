//go:build unit

package admin

import (
	"bytes"
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

type subscriptionHandlerRepoStub struct {
	service.UserSubscriptionRepository
	subs            map[int64]*service.UserSubscription
	hardDeleteCalls []int64
}

func newSubscriptionHandlerRepoStub(subs ...*service.UserSubscription) *subscriptionHandlerRepoStub {
	repo := &subscriptionHandlerRepoStub{subs: make(map[int64]*service.UserSubscription, len(subs))}
	for _, sub := range subs {
		copy := *sub
		repo.subs[sub.ID] = &copy
	}
	return repo
}

func (r *subscriptionHandlerRepoStub) GetByID(_ context.Context, id int64) (*service.UserSubscription, error) {
	sub := r.subs[id]
	if sub == nil || sub.DeletedAt != nil {
		return nil, service.ErrSubscriptionNotFound
	}
	copy := *sub
	return &copy, nil
}

func (r *subscriptionHandlerRepoStub) GetByIDIncludeDeleted(_ context.Context, id int64) (*service.UserSubscription, error) {
	sub := r.subs[id]
	if sub == nil {
		return nil, service.ErrSubscriptionNotFound
	}
	copy := *sub
	return &copy, nil
}

func (r *subscriptionHandlerRepoStub) Delete(_ context.Context, id int64) error {
	sub := r.subs[id]
	if sub == nil || sub.DeletedAt != nil {
		return service.ErrSubscriptionNotFound
	}
	now := time.Now()
	sub.DeletedAt = &now
	return nil
}

func (r *subscriptionHandlerRepoStub) HardDelete(_ context.Context, id int64) error {
	if r.subs[id] == nil {
		return service.ErrSubscriptionNotFound
	}
	delete(r.subs, id)
	r.hardDeleteCalls = append(r.hardDeleteCalls, id)
	return nil
}

func setupSubscriptionHandlerRouter(t *testing.T, repo service.UserSubscriptionRepository) *gin.Engine {
	t.Helper()
	previousCoordinator := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previousCoordinator) })

	svc := service.NewSubscriptionService(nil, repo, nil, nil, nil)
	t.Cleanup(svc.Stop)
	handler := NewSubscriptionHandler(svc)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/admin/subscriptions/batch-action", handler.BatchAction)
	router.DELETE("/api/v1/admin/subscriptions/:id/permanent", handler.PermanentDelete)
	return router
}

func performSubscriptionHandlerRequest(
	t *testing.T,
	router *gin.Engine,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "subscription-handler-test")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestSubscriptionHandlerBatchActionReturnsPerItemResults(t *testing.T) {
	now := time.Now()
	revokedAt := now.Add(-time.Hour)
	repo := newSubscriptionHandlerRepoStub(
		&service.UserSubscription{ID: 1, UserID: 10, GroupID: 20, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour)},
		&service.UserSubscription{ID: 2, UserID: 11, GroupID: 20, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DeletedAt: &revokedAt},
	)
	recorder := performSubscriptionHandlerRequest(
		t,
		setupSubscriptionHandlerRouter(t, repo),
		http.MethodPost,
		"/api/v1/admin/subscriptions/batch-action",
		`{"subscription_ids":[1,2],"action":"revoke"}`,
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			TotalCount     int `json:"total_count"`
			SucceededCount int `json:"succeeded_count"`
			SkippedCount   int `json:"skipped_count"`
			FailedCount    int `json:"failed_count"`
			Items          []struct {
				SubscriptionID int64  `json:"subscription_id"`
				Status         string `json:"status"`
				Reason         string `json:"reason"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 2, response.Data.TotalCount)
	require.Equal(t, 1, response.Data.SucceededCount)
	require.Equal(t, 1, response.Data.SkippedCount)
	require.Zero(t, response.Data.FailedCount)
	require.Equal(t, int64(1), response.Data.Items[0].SubscriptionID)
	require.Equal(t, service.SubscriptionBatchItemSucceeded, response.Data.Items[0].Status)
	require.Equal(t, service.SubscriptionBatchItemSkipped, response.Data.Items[1].Status)
	require.Equal(t, "SUBSCRIPTION_BATCH_ACTION_INELIGIBLE", response.Data.Items[1].Reason)
}

func TestSubscriptionHandlerPermanentDeleteRequiresRevokedSubscription(t *testing.T) {
	now := time.Now()
	revokedAt := now.Add(-time.Hour)
	repo := newSubscriptionHandlerRepoStub(
		&service.UserSubscription{ID: 1, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour)},
		&service.UserSubscription{ID: 2, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DeletedAt: &revokedAt},
	)
	router := setupSubscriptionHandlerRouter(t, repo)

	activeResponse := performSubscriptionHandlerRequest(
		t, router, http.MethodDelete, "/api/v1/admin/subscriptions/1/permanent", "",
	)
	require.Equal(t, http.StatusConflict, activeResponse.Code)
	require.Contains(t, activeResponse.Body.String(), `"reason":"SUBSCRIPTION_NOT_REVOKED"`)

	revokedResponse := performSubscriptionHandlerRequest(
		t, router, http.MethodDelete, "/api/v1/admin/subscriptions/2/permanent", "",
	)
	require.Equal(t, http.StatusOK, revokedResponse.Code)
	require.Equal(t, []int64{2}, repo.hardDeleteCalls)
	require.NotContains(t, repo.subs, int64(2))
}

func TestSubscriptionHandlerBatchActionRejectsInvalidOptions(t *testing.T) {
	repo := newSubscriptionHandlerRepoStub()
	recorder := performSubscriptionHandlerRequest(
		t,
		setupSubscriptionHandlerRouter(t, repo),
		http.MethodPost,
		"/api/v1/admin/subscriptions/batch-action",
		`{"subscription_ids":[1],"action":"reset_quota","reset_quota":{}}`,
	)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"reason":"INVALID_SUBSCRIPTION_BATCH"`)
}
