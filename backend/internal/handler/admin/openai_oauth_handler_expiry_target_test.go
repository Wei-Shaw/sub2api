package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type expiryTargetHandlerService struct {
	account     *service.Account
	creditID    string
	leadTime    int
	setCalls    int
	cancelCalls int
}

func (s *expiryTargetHandlerService) SetResetCreditExpiryTarget(_ context.Context, _ int64, creditID string, leadTimeMinutes int) (*service.Account, error) {
	s.setCalls++
	s.creditID = creditID
	s.leadTime = leadTimeMinutes
	return s.account, nil
}

func (s *expiryTargetHandlerService) CancelResetCreditExpiryTarget(context.Context, int64) (*service.Account, error) {
	s.cancelCalls++
	return s.account, nil
}

func TestOpenAIResetCreditExpiryTargetHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &expiryTargetHandlerService{account: &service.Account{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}}
	handler := &OpenAIOAuthHandler{expiryTargetService: stub}
	router := gin.New()
	router.PUT("/openai/accounts/:id/reset-credit-expiry-target", handler.SetResetCreditExpiryTarget)
	router.DELETE("/openai/accounts/:id/reset-credit-expiry-target", handler.CancelResetCreditExpiryTarget)

	for index, test := range []struct {
		body, creditID string
		leadTime       int
	}{
		{`{"credit_id":"credit-one"}`, "credit-one", service.OpenAIResetCreditExpiryTargetDefaultLeadTimeMinutes},
		{`{"credit_id":"credit-two","lead_time_minutes":30}`, "credit-two", 30},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/openai/accounts/42/reset-credit-expiry-target", bytes.NewBufferString(test.body))
		request.Header.Set("content-type", "application/json")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, index+1, stub.setCalls)
		require.Equal(t, test.creditID, stub.creditID)
		require.Equal(t, test.leadTime, stub.leadTime)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/openai/accounts/42/reset-credit-expiry-target", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, stub.cancelCalls)
}
