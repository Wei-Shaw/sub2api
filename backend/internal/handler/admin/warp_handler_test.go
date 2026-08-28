package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBindOptionalJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		body      string
		wantGroup string
		wantErr   bool
	}{
		{name: "empty"},
		{name: "whitespace", body: "  \n\t"},
		{name: "valid", body: `{"group_name":"warp-pool"}`, wantGroup: "warp-pool"},
		{name: "malformed", body: `{"group_name":`, wantErr: true},
		{name: "trailing value", body: `{} {}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			var got struct {
				GroupName string `json:"group_name"`
			}
			err := bindOptionalJSON(ctx, &got)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantGroup, got.GroupName)
		})
	}
}

func TestWarpHandlerMalformedOptionalJSONDoesNotCallGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected gateway call", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	client := service.NewWarpGatewayClient(service.WarpGatewayConfig{Enabled: true, BaseURL: server.URL})
	handler := NewWarpHandler(service.NewWarpSyncService(nil, client, nil, nil, nil))

	tests := []struct {
		name   string
		method string
		path   string
		invoke func(*gin.Context)
	}{
		{name: "sync", method: http.MethodPost, path: "/sync", invoke: handler.Sync},
		{name: "health sync", method: http.MethodPost, path: "/health-sync", invoke: handler.HealthSync},
		{name: "rotate", method: http.MethodPost, path: "/instances/i-1/rotate", invoke: handler.Rotate},
		{name: "delete", method: http.MethodDelete, path: "/instances/i-1", invoke: handler.DeleteInstance},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := calls.Load()
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{"deregister_cloudflare":`))
			ctx.Params = gin.Params{{Key: "id", Value: "i-1"}}

			tt.invoke(ctx)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Equal(t, before, calls.Load(), "malformed body must not reach warp-gateway")
		})
	}
}
