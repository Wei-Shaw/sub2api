//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDecisionsBypassesEmbeddedFrontend(t *testing.T) {
	server, err := NewFrontendServer(&mockSettingsProvider{settings: map[string]string{}})
	require.NoError(t, err)
	for name, middleware := range map[string]gin.HandlerFunc{
		"settings frontend": server.Middleware(),
		"legacy frontend":   ServeEmbeddedFrontend(),
	} {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"/decisions", "/v1/decisions"} {
				router := gin.New()
				router.Use(middleware)
				router.POST(path, func(c *gin.Context) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "questions required"})
				})
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
				require.Equal(t, http.StatusBadRequest, response.Code)
				require.JSONEq(t, `{"error":"questions required"}`, response.Body.String())
			}
		})
	}
	require.True(t, shouldBypassEmbeddedFrontend("/decisions/"))
	require.False(t, shouldBypassEmbeddedFrontend("/decisions-guide"))
}
