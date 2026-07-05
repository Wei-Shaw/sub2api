package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type imageControlsStatusStore struct {
	status *service.ResponsesImageStatus
}

func (s *imageControlsStatusStore) GetResponsesImageStatus(context.Context, string) (*service.ResponsesImageStatus, error) {
	if s.status == nil {
		return nil, service.ErrResponsesImageStatusNotFound
	}
	return s.status, nil
}

func (s *imageControlsStatusStore) GetResponsesImageStatuses(context.Context, []string) (map[string]*service.ResponsesImageStatus, error) {
	if s.status == nil {
		return map[string]*service.ResponsesImageStatus{}, nil
	}
	return map[string]*service.ResponsesImageStatus{s.status.RequestID: s.status}, nil
}

func (s *imageControlsStatusStore) SetResponsesImageStatus(_ context.Context, status *service.ResponsesImageStatus, _ time.Duration) error {
	s.status = status
	return nil
}

func TestOpenAIGatewayHandlerImages_DisabledGroupRejectsBeforeScheduling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-image-2","prompt":"draw","size":"1024x1024"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	groupID := int64(111)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      222,
		GroupID: &groupID,
		Group: &service.Group{
			ID:                   groupID,
			AllowImageGeneration: false,
		},
		User: &service.User{ID: 333},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 333, Concurrency: 1})

	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: &service.ConcurrencyService{}},
	}

	h.Images(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "permission_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}

func TestOpenAIGatewayHandlerImages_EditsTrackImageStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-image-2","prompt":"edit","images":[{"image_url":"https://example.com/input.png"}],"size":"1024x1024"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-client-request-id", "edit-req-1")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	groupID := int64(111)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      222,
		GroupID: &groupID,
		Group: &service.Group{
			ID:                   groupID,
			AllowImageGeneration: false,
		},
		User: &service.User{ID: 333},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 333, Concurrency: 1})

	store := &imageControlsStatusStore{}
	h := &OpenAIGatewayHandler{
		gatewayService: service.NewOpenAIGatewayService(
			nil, nil, nil, nil, nil, nil, nil, store, &config.Config{},
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		),
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: &service.ConcurrencyService{}},
	}

	h.Images(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.NotNil(t, store.status)
	require.Equal(t, "edit-req-1", store.status.RequestID)
	require.Equal(t, service.ResponsesImageStatusFailed, store.status.Status)
	require.Equal(t, 100, store.status.Progress)
	require.NotNil(t, store.status.Error)
	require.Contains(t, store.status.Error.Message, service.ImageGenerationPermissionMessage())
}
