//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountWindowHistoryQueryValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &AccountHandler{}
	router.GET("/accounts/:id/window-history", handler.GetWindowHistory)
	for _, path := range []string{"/accounts/no/window-history", "/accounts/-1/window-history", "/accounts/0/window-history", "/accounts/1/window-history?days=0", "/accounts/1/window-history?days=91", "/accounts/1/window-history?days=oops"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/accounts/1/window-history?days=90", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}
