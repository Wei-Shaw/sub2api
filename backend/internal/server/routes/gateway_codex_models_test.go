package routes

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesCodexModelsManifestPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet {
			registered[route.Path] = true
		}
	}

	require.True(t, registered["/backend-api/codex/models"], "GET /backend-api/codex/models should be registered")
	require.True(t, registered["/v1/models"], "GET /v1/models should be registered")
}
