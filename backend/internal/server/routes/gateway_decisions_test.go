package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayDecisionsRoutesAndPlatformGate(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformAnthropic, service.PlatformGrok} {
		router := newGatewayRoutesTestRouter(platform)
		for _, path := range []string{"/v1/decisions", "/decisions"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-6-luna","input":"x","questions":[]}`)))
			if platform == service.PlatformOpenAI {
				require.NotEqual(t, http.StatusNotFound, response.Code)
			} else {
				require.Equal(t, http.StatusNotFound, response.Code)
				require.Contains(t, response.Body.String(), "Decisions API is not supported")
			}
		}
	}
	require.Equal(t, service.CompositeRouteEndpointDecisions, compositeRouteEndpointForPath("/v1/decisions"))
}
