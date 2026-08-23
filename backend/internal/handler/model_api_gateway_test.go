//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type modelAPINoAccountRepo struct {
	service.AccountRepository
}

func (r *modelAPINoAccountRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	return nil, nil
}

type modelAPIDisabledSettingRepo struct {
	service.SettingRepository
}

func (r *modelAPIDisabledSettingRepo) GetValue(context.Context, string) (string, error) {
	return "false", nil
}

func newModelAPITestGatewayService() *service.GatewayService {
	return service.NewGatewayService(
		&modelAPINoAccountRepo{}, nil, nil, nil, nil, nil, nil, nil, &config.Config{}, nil, nil,
		&service.BillingService{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
}

func performModelAPISubmit(t *testing.T, handler *ModelAPIGatewayHandler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/model/*path", func(c *gin.Context) {
		groupID := int64(9)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      17,
			GroupID: &groupID,
			Group: &service.Group{
				ID:                   groupID,
				Platform:             service.PlatformComposite,
				AllowImageGeneration: true,
			},
		})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 23})
		handler.Native(c)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestModelAPIGatewayKnownImageNeverFallsThroughToVideoValidation(t *testing.T) {
	handler := NewModelAPIGatewayHandler(newModelAPITestGatewayService(), nil, nil, nil, nil, nil)

	recorder := performModelAPISubmit(t, handler, "/api/v1/model/openai/gpt-image-2", `{"prompt":"studio photo"}`)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "no available image account")
	require.NotContains(t, recorder.Body.String(), "Missing 'resolution'")
}

func TestModelAPIGatewayCompositeEditRejectsNonURLImageParameter(t *testing.T) {
	handler := NewModelAPIGatewayHandler(newModelAPITestGatewayService(), nil, nil, nil, nil, nil)

	recorder := performModelAPISubmit(t, handler, "/api/v1/model/gpt-image-2/edit", `{"image_urls":["https://example.test/reference.png","data:image/png;base64,aW1hZ2U="]}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{
		"error": {
			"type": "invalid_request_error",
			"message": "invalid parameter 'image_urls[1]': must be a valid HTTP or HTTPS URL"
		}
	}`, recorder.Body.String())
}

func TestModelAPIGatewayVideoFeatureGateOnlyAppliesAfterMediaRouting(t *testing.T) {
	settingService := service.NewSettingService(&modelAPIDisabledSettingRepo{}, &config.Config{})
	handler := NewModelAPIGatewayHandler(newModelAPITestGatewayService(), nil, nil, nil, nil, settingService)

	recorder := performModelAPISubmit(t, handler, "/api/v1/model/bytedance/seedance-2.5/image-to-video", `{"resolution":"720p"}`)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"type":"feature_disabled"`)
}

func TestMediaFalStatusFromTaskMapsTerminalFailureToFailed(t *testing.T) {
	for _, status := range []string{
		service.AsyncMediaStatusFailed,
		service.AsyncMediaStatusRefunded,
		service.AsyncMediaStatusExpired,
	} {
		t.Run(status, func(t *testing.T) {
			require.Equal(t, fal.StatusFailed, imageStatusFromTask(&service.AsyncMediaTask{Status: status}))
		})
	}
}
