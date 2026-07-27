//go:build unit

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

type settingEmailDeliveryRepo struct {
	service.NotificationEmailDeliveryRepository
	filter    service.NotificationEmailDeliveryListFilter
	retryID   int64
	retryable bool
}

func (r *settingEmailDeliveryRepo) List(_ context.Context, filter service.NotificationEmailDeliveryListFilter) (service.NotificationEmailDeliveryListResult, error) {
	r.filter = filter
	now := time.Now().UTC()
	return service.NotificationEmailDeliveryListResult{
		Items: []service.NotificationEmailDelivery{{
			ID: 7, Event: service.NotificationEmailEventOpsAlert, Channel: service.NotificationEmailChannelOpsAlert,
			RecipientEmail: "alice@example.com", SourceType: "ops_incident", SourceID: "incident-7",
			Status: service.NotificationEmailDeliveryStatusFailed, AttemptCount: 5, MaxAttempts: 5,
			Variables: map[string]string{"secret": "must-not-leak"}, LastErrorCategory: "transport",
			LastError: "smtp timeout", CreatedAt: now, UpdatedAt: now,
		}},
		Total: 1,
	}, nil
}

func (r *settingEmailDeliveryRepo) Retry(_ context.Context, id int64) (bool, error) {
	r.retryID = id
	return r.retryable, nil
}

func newSettingEmailDeliveryRouter(repo *settingEmailDeliveryRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := &SettingHandler{}
	handler.SetNotificationEmailDispatcher(service.NewNotificationEmailDispatcher(repo, nil))
	router := gin.New()
	router.GET("/deliveries", handler.ListNotificationEmailDeliveries)
	router.POST("/deliveries/:id/retry", handler.RetryNotificationEmailDelivery)
	return router
}

func TestListNotificationEmailDeliveriesRedactsPayloadAndRecipient(t *testing.T) {
	repo := &settingEmailDeliveryRepo{}
	router := newSettingEmailDeliveryRouter(repo)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/deliveries?page=2&page_size=10&status=failed&source_type=ops_incident", nil)
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, 2, repo.filter.Page)
	require.Equal(t, 10, repo.filter.PageSize)
	require.Equal(t, "failed", repo.filter.Status)
	require.Equal(t, "ops_incident", repo.filter.SourceType)
	require.NotContains(t, response.Body.String(), "alice@example.com")
	require.NotContains(t, response.Body.String(), "must-not-leak")
	require.Contains(t, response.Body.String(), "a***e@e***.com")
}

func TestRetryNotificationEmailDeliveryRejectsNonRetryableState(t *testing.T) {
	repo := &settingEmailDeliveryRepo{retryable: false}
	router := newSettingEmailDeliveryRouter(repo)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/deliveries/7/retry", nil)
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Equal(t, int64(7), repo.retryID)
	var envelope responseEnvelope
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Contains(t, envelope.Message, "not retryable")
}
