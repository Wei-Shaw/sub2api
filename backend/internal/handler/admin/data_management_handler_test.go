package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Reason  string          `json:"reason"`
	Data    json.RawMessage `json:"data"`
}

func TestDataManagementHandler_AgentHealthAlways200(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := service.NewDataManagementServiceWithOptions(filepath.Join(t.TempDir(), "missing.sock"), 50*time.Millisecond)
	h := NewDataManagementHandler(svc)

	r := gin.New()
	r.GET("/api/v1/admin/data-management/agent/health", h.GetAgentHealth)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/data-management/agent/health", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var envelope apiEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 0, envelope.Code)

	var data struct {
		Enabled    bool   `json:"enabled"`
		Reason     string `json:"reason"`
		SocketPath string `json:"socket_path"`
	}
	require.NoError(t, json.Unmarshal(envelope.Data, &data))
	require.False(t, data.Enabled)
	require.Equal(t, service.DataManagementDeprecatedReason, data.Reason)
	require.Equal(t, svc.SocketPath(), data.SocketPath)
}

func TestDataManagementHandler_NonHealthRouteReturns503WhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := service.NewDataManagementServiceWithOptions(filepath.Join(t.TempDir(), "missing.sock"), 50*time.Millisecond)
	h := NewDataManagementHandler(svc)

	r := gin.New()
	r.GET("/api/v1/admin/data-management/config", h.GetConfig)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/data-management/config", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var envelope apiEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, http.StatusServiceUnavailable, envelope.Code)
	require.Equal(t, service.DataManagementDeprecatedReason, envelope.Reason)
}

func TestNormalizeBackupIdempotencyKey(t *testing.T) {
	require.Equal(t, "from-header", normalizeBackupIdempotencyKey("from-header", "from-body"))
	require.Equal(t, "from-body", normalizeBackupIdempotencyKey(" ", " from-body "))
	require.Equal(t, "", normalizeBackupIdempotencyKey("", ""))
}
