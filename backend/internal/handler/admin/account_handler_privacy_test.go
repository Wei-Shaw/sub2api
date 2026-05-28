package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountPrivacyRouter(adminSvc *stubAdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/:id/set-privacy", handler.SetPrivacy)
	return router
}

func TestAccountHandlerSetPrivacyNotExecutableReturnsBadRequest(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.getAccountValue = &service.Account{
		ID:       11,
		Name:     "openai-oauth",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
	}
	adminSvc.forceOpenAIPrivacy = service.PrivacySetResult{
		Reason:  "PRIVACY_SET_NOT_EXECUTABLE",
		Message: "Cannot set privacy: missing access_token",
		Stage:   "precheck",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/11/set-privacy", nil)
	setupAccountPrivacyRouter(adminSvc).ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, http.StatusBadRequest, body.Code)
	require.Equal(t, "Cannot set privacy: missing access_token", body.Message)
	require.Equal(t, "PRIVACY_SET_NOT_EXECUTABLE", body.Reason)
	require.Equal(t, map[string]string{
		"account_id": "11",
		"platform":   service.PlatformOpenAI,
		"stage":      "precheck",
	}, body.Metadata)
}

func TestAccountHandlerSetPrivacyFailureReturnsBadGatewayWithMetadata(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.getAccountValue = &service.Account{
		ID:       12,
		Name:     "openai-oauth",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Extra: map[string]any{
			"privacy_mode": service.PrivacyModeFailed,
		},
	}
	adminSvc.forceOpenAIPrivacy = service.PrivacySetResult{
		Mode:       service.PrivacyModeFailed,
		Reason:     "PRIVACY_UPSTREAM_FAILED",
		Message:    "Privacy API returned a non-success response",
		StatusCode: http.StatusUnauthorized,
		Stage:      "upstream_response",
		Detail:     "upstream said no",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/12/set-privacy", nil)
	setupAccountPrivacyRouter(adminSvc).ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, http.StatusBadGateway, body.Code)
	require.Equal(t, "Privacy API returned a non-success response", body.Message)
	require.Equal(t, "PRIVACY_UPSTREAM_FAILED", body.Reason)
	require.Equal(t, map[string]string{
		"account_id":   "12",
		"detail":       "upstream said no",
		"platform":     service.PlatformOpenAI,
		"privacy_mode": service.PrivacyModeFailed,
		"stage":        "upstream_response",
		"status_code":  "401",
	}, body.Metadata)
}

func TestAccountHandlerSetPrivacySuccessReturnsAccount(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.getAccountValue = &service.Account{
		ID:       13,
		Name:     "openai-oauth",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Extra: map[string]any{
			"privacy_mode": service.PrivacyModeTrainingOff,
		},
	}
	adminSvc.forceOpenAIPrivacy = service.PrivacySetResult{
		Mode:    service.PrivacyModeTrainingOff,
		Success: true,
		Message: "Training data sharing disabled",
		Stage:   "upstream_response",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/13/set-privacy", nil)
	setupAccountPrivacyRouter(adminSvc).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "success", body.Message)
	require.Empty(t, body.Reason)
	require.Nil(t, body.Metadata)
}
