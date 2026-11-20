//go:build unit

package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepseekToolsLimitsBeforeCompositePreread(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxBodySize: 2048, TextMaxBodySize: 32}}
	router := newGatewayRoutesTestRouterWithConfig(cfg, service.PlatformComposite)
	for _, path := range []string{"/v1/tools/search", "/v1/tools/fetch", "/v1/files"} {
		bodySize := 33
		if path == "/v1/files" {
			bodySize = 2049
		}
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Repeat("x", bodySize)))
		req.ContentLength = -1
		req.Header.Set("anthropic-beta", "files-api-2025-04-14")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 413, w.Code, path)
		require.Equal(t, path == "/v1/files", gjson.GetBytes(w.Body.Bytes(), "type").String() == "error")
	}
}
