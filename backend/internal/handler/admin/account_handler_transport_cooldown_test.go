package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyOAuthCredentialsTransportCooldownValidationBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra string
		valid bool
	}{
		{"zero duration", `{"transport_error_cooldown_minutes":0}`, false},
		{"negative duration", `{"transport_error_cooldown_minutes":-1}`, false},
		{"fractional duration", `{"transport_error_cooldown_minutes":1.5}`, false},
		{"excessive duration", `{"transport_error_cooldown_minutes":1441}`, false},
		{"string duration", `{"transport_error_cooldown_minutes":"2"}`, false},
		{"null duration", `{"transport_error_cooldown_minutes":null}`, false},
		{"string switch", `{"transport_error_cooldown_enabled":"false"}`, false},
		{"null switch", `{"transport_error_cooldown_enabled":null}`, false},
		{"opt out", `{"transport_error_cooldown_enabled":false}`, true},
		{"custom duration", `{"transport_error_cooldown_enabled":true,"transport_error_cooldown_minutes":2}`, true},
		{"legacy request", `{}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			stub := newStubAdminService()
			stub.getAccountResult = &service.Account{ID: 1, Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth}
			handler := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			router := gin.New()
			router.POST("/accounts/:id/apply-oauth-credentials", handler.ApplyOAuthCredentials)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/accounts/1/apply-oauth-credentials", bytes.NewBufferString(
				`{"type":"oauth","credentials":{"access_token":"test-token"},"extra":`+tc.extra+`}`,
			))
			request.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, request)

			if tc.valid {
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				require.Equal(t, 1, stub.updateAccountCalls)
			} else {
				require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
				require.Contains(t, recorder.Body.String(), "TRANSPORT_ERROR_COOLDOWN_INVALID")
				require.Zero(t, stub.updateAccountCalls, "invalid settings must not save new credentials")
				require.Zero(t, stub.updateAccountExtraCalls, "invalid settings must not write extra")
			}
		})
	}
}
