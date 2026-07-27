//go:build unit

package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type opsNotificationEmailHealthRepo struct {
	service.NotificationEmailDeliveryRepository
}

func (r *opsNotificationEmailHealthRepo) Stats(context.Context) (service.NotificationEmailDeliveryStats, error) {
	oldest := time.Now().UTC().Add(-2 * time.Minute)
	return service.NotificationEmailDeliveryStats{Pending: 3, OldestCreatedAt: &oldest, MaxAttempts: 4}, nil
}

func TestOpsNotificationEmailDeliveryHealth(t *testing.T) {
	opsService := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	opsService.SetNotificationEmailDeliveryWorker(service.NewNotificationEmailDeliveryWorker(&opsNotificationEmailHealthRepo{}, nil))
	handler := NewOpsHandler(opsService)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", handler.GetNotificationEmailDeliveryHealth)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"pending":3`)
	require.Contains(t, response.Body.String(), `"oldest_lag":`)
}
