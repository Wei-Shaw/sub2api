package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountAdminBoundariesRejectMalformedOpenAILongContextBillingValue(t *testing.T) {
	const malformedExtra = `"extra":{"openai_long_context_billing_enabled":"true"}`

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		mount  func(*gin.Engine, *AccountHandler)
		setup  func(*stubAdminService)
	}{
		{
			name:   "create",
			method: http.MethodPost,
			path:   "/accounts",
			body:   `{"name":"account","platform":"openai","type":"apikey","credentials":{"api_key":"test"},` + malformedExtra + `}`,
			mount:  func(router *gin.Engine, handler *AccountHandler) { router.POST("/accounts", handler.Create) },
		},
		{
			name:   "update",
			method: http.MethodPut,
			path:   "/accounts/1",
			body:   `{` + malformedExtra + `}`,
			mount:  func(router *gin.Engine, handler *AccountHandler) { router.PUT("/accounts/:id", handler.Update) },
			setup: func(stub *stubAdminService) {
				stub.updateAccountErr = infraerrors.BadRequest("OPENAI_LONG_CONTEXT_BILLING_INVALID", "invalid")
			},
		},
		{
			name:   "bulk update",
			method: http.MethodPost,
			path:   "/accounts/bulk-update",
			body:   `{"account_ids":[1],` + malformedExtra + `}`,
			mount: func(router *gin.Engine, handler *AccountHandler) {
				router.POST("/accounts/bulk-update", handler.BulkUpdate)
			},
			setup: func(stub *stubAdminService) {
				stub.bulkUpdateAccountErr = infraerrors.BadRequest("OPENAI_LONG_CONTEXT_BILLING_INVALID", "invalid")
			},
		},
		{
			name:   "batch create",
			method: http.MethodPost,
			path:   "/accounts/batch",
			body:   `{"accounts":[{"name":"account","platform":"openai","type":"apikey","credentials":{"api_key":"test"},` + malformedExtra + `}]}`,
			mount:  func(router *gin.Engine, handler *AccountHandler) { router.POST("/accounts/batch", handler.BatchCreate) },
		},
		{
			name:   "Codex session import",
			method: http.MethodPost,
			path:   "/accounts/import-codex-session",
			body:   `{"content":"token",` + malformedExtra + `}`,
			mount: func(router *gin.Engine, handler *AccountHandler) {
				router.POST("/accounts/import-codex-session", handler.ImportCodexSession)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			stub := newStubAdminService()
			if tt.setup != nil {
				tt.setup(stub)
			}
			handler := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			router := gin.New()
			tt.mount(router, handler)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			request.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			var responseBody struct {
				Reason string `json:"reason"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
			require.Equal(t, "OPENAI_LONG_CONTEXT_BILLING_INVALID", responseBody.Reason)
		})
	}
}

func TestAccountCreateBoundaryDoesNotApplyOpenAIValidationToOtherPlatforms(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(newStubAdminService(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/accounts", handler.Create)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(
		`{"name":"account","platform":"anthropic","type":"apikey","credentials":{"api_key":"test"},"extra":{"openai_long_context_billing_enabled":"provider-owned"}}`,
	))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestApplyOAuthCredentialsRejectsMalformedOpenAILongContextBillingBeforeMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := newStubAdminService()
	stub.getAccountResult = &service.Account{
		ID:       1,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
	}
	handler := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/accounts/:id/apply-oauth-credentials", handler.ApplyOAuthCredentials)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/accounts/1/apply-oauth-credentials", bytes.NewBufferString(
		`{"type":"oauth","credentials":{"access_token":"new-token"},"extra":{"openai_long_context_billing_enabled":"true"}}`,
	))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var responseBody struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
	require.Equal(t, "OPENAI_LONG_CONTEXT_BILLING_INVALID", responseBody.Reason)
	require.Zero(t, stub.updateAccountCalls)
	require.Zero(t, stub.updateAccountExtraCalls)
}

func TestOpenAIOAuthCodexPATBoundaryRejectsMalformedOpenAILongContextBillingValueBeforeTokenValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewOpenAIOAuthHandler(nil, newStubAdminService(), nil)
	router := gin.New()
	router.Use(gin.Recovery())
	router.POST("/openai/create-from-codex-pat", handler.CreateAccountFromCodexPAT)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/openai/create-from-codex-pat", bytes.NewBufferString(
		`{"access_token":"token","extra":{"openai_long_context_billing_enabled":1}}`,
	))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var responseBody struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
	require.Equal(t, "OPENAI_LONG_CONTEXT_BILLING_INVALID", responseBody.Reason)
}
