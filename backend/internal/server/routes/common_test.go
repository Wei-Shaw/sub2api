package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCommonRoutesTestRouter(t *testing.T, token string) *gin.Engine {
	t.Helper()
	t.Setenv("SUB2API_WORKER_SHARED_TOKEN", token)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCommonRoutes(router)
	return router
}

func TestCommonRoutesInternalHealthRequiresToken(t *testing.T) {
	router := newCommonRoutesTestRouter(t, "secret")

	req := httptest.NewRequest(http.MethodGet, "/internal/health", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestCommonRoutesInternalExecutePathIsRegistered(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/responses", r.URL.Path)
		require.Equal(t, "Bearer worker-local-key", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_worker",
			"object": "response",
		})
	}))
	defer upstream.Close()
	t.Setenv("SUB2API_WORKER_PUBLIC_BASE_URL", upstream.URL)
	t.Setenv("SUB2API_WORKER_EXECUTION_API_KEY", "worker-local-key")
	router := newCommonRoutesTestRouter(t, "secret")

	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/execute", strings.NewReader(`{"job_id":"job-1","capability":"text.basic","input":{"prompt":"hello"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(service.WorkerJobsTokenHeader, "secret")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.NotEqual(t, http.StatusNotFound, resp.Code)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"executor":"py-worker"`)
}
