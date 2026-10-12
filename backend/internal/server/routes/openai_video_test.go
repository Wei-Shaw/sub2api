package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesOpenAIVideoCreateAndLookup(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformOpenAI)
	for _, prefix := range []string{"/v1", ""} {
		for _, tc := range []struct{ method, path string }{
			{http.MethodPost, "/videos"},
			{http.MethodPost, "/videos/generations"},
			{http.MethodGet, "/videos/task-1"},
			{http.MethodGet, "/videos/task-1/content"},
			{http.MethodGet, "/videos/generations/task-1"},
			{http.MethodGet, "/videos/generations/task-1/content"},
			{http.MethodGet, "/videos/edits/task-1"},
			{http.MethodGet, "/videos/edits/task-1/content"},
			{http.MethodGet, "/videos/extensions/task-1"},
			{http.MethodGet, "/videos/extensions/task-1/content"},
		} {
			req := httptest.NewRequest(tc.method, prefix+tc.path, strings.NewReader(`{"model":"grok-imagine-video-1.5（zj）","seconds":"6","size":"1280x720"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			// The stub has no auth subject: reaching that check proves dispatch
			// entered the handler rather than silently accepting a missing route.
			require.Equal(t, http.StatusInternalServerError, w.Code, "%s %s", tc.method, req.URL.Path)
			require.Contains(t, w.Body.String(), "User context not found")
		}
		for _, path := range []string{"/videos/edits", "/videos/extensions"} {
			req := httptest.NewRequest(http.MethodPost, prefix+path, strings.NewReader(`{"model":"video"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
			require.Contains(t, w.Body.String(), "Videos API is not supported")
		}
	}
}
