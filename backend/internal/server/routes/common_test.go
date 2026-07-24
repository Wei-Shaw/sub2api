package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/backgroundruntime"
	"github.com/gin-gonic/gin"
)

func TestHealthIsNotCacheableAndReportsDeploymentSlot(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "active-slot")
	if err := os.WriteFile(statePath, []byte("green\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEPLOYMENT_STANDBY", "true")
	t.Setenv("DEPLOYMENT_SLOT", "green")
	t.Setenv("DEPLOYMENT_STATE_FILE", statePath)
	if err := backgroundruntime.ConfigureFromEnv(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("DEPLOYMENT_STANDBY", "false")
		_ = backgroundruntime.ConfigureFromEnv()
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCommonRoutes(router)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"state":"active"`) || !strings.Contains(body, `"slot":"green"`) {
		t.Fatalf("health response lost deployment identity: %s", body)
	}
	if !strings.Contains(body, `"drain":{"supported":true`) {
		t.Fatalf("health response lost drain capability: %s", body)
	}
}
