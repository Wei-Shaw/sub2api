//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokUsageResetHandlersValidateWithoutExposingSSO(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &GrokOAuthHandler{quotaService: &service.GrokQuotaService{}}
	router := gin.New()
	router.POST("/accounts/:id/reset-cards/query", h.QueryUsageResetCards)
	router.POST("/accounts/:id/reset-cards/redeem", h.RedeemUsageResetCard)
	for _, input := range []struct{ path, body string }{
		{"/accounts/invalid/reset-cards/query", `{"sso_token":"web-session-secret"}`},
		{"/accounts/12/reset-cards/query", `{}`},
		{"/accounts/12/reset-cards/query", `{"sso_token":"web-session-secret`},
		{"/accounts/12/reset-cards/query", `{"sso_token":"` + strings.Repeat("web-session-secret", 1000) + `"}`},
		{"/accounts/12/reset-cards/redeem", `{"sso_token":"web-session-secret"}`},
	} {
		request := httptest.NewRequest(http.MethodPost, input.path, strings.NewReader(input.body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.NotContains(t, recorder.Body.String(), "web-session-secret")
	}
}
