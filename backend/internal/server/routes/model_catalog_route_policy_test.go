package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelCatalogRoutesRequireAdminButNotStepUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	catalogService := service.NewModelCatalogAdminService(nil, nil, nil, nil, service.CatalogScopeGlobal)
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		ModelCatalog: adminhandler.NewModelCatalogHandler(catalogService),
	}}
	admin := router.Group("/api/v1/admin")
	admin.Use(func(c *gin.Context) {
		switch c.GetHeader("Authorization") {
		case "Bearer admin-token":
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
			c.Set(string(servermiddleware.ContextKeyUserRole), "admin")
			c.Next()
		case "Bearer user-token":
			servermiddleware.AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Admin access required")
		default:
			servermiddleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
		}
	})
	registerModelCatalogRoutes(admin, handlers)

	routes := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "sync", method: http.MethodPost, path: "/api/v1/admin/model-catalog/sync"},
		{name: "state", method: http.MethodPatch, path: "/api/v1/admin/model-catalog/models/1/state", body: `{"expected_version":1,"state":"disabled","reason":"test"}`},
		{name: "bulk-state", method: http.MethodPatch, path: "/api/v1/admin/model-catalog/models/bulk-state", body: `{"expected_epoch":1,"expected_revision_id":1,"state":"disabled","models":[{"model_id":1,"expected_version":1}],"reason":"test"}`},
		{name: "pricing", method: http.MethodPatch, path: "/api/v1/admin/model-catalog/models/1/pricing", body: `{"expected_epoch":1,"expected_revision_id":1,"expected_source_hash":"hash","input_cost_per_token":0,"reason":"test"}`},
	}

	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			for _, authCase := range []struct {
				name       string
				auth       string
				wantStatus int
			}{
				{name: "unauthenticated", wantStatus: http.StatusUnauthorized},
				{name: "non-admin", auth: "Bearer user-token", wantStatus: http.StatusForbidden},
				{name: "admin-without-step-up", auth: "Bearer admin-token", wantStatus: http.StatusInternalServerError},
			} {
				t.Run(authCase.name, func(t *testing.T) {
					recorder := httptest.NewRecorder()
					request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("Idempotency-Key", route.name+"-"+authCase.name)
					if authCase.auth != "" {
						request.Header.Set("Authorization", authCase.auth)
					}

					router.ServeHTTP(recorder, request)

					require.Equal(t, authCase.wantStatus, recorder.Code)
					require.NotContains(t, recorder.Body.String(), "STEP_UP")
				})
			}
		})
	}
}

func TestModelCatalogRoutesStayUnderAdminMiddlewareWithoutStepUp(t *testing.T) {
	source, err := os.ReadFile("admin.go")
	require.NoError(t, err)
	text := string(source)
	require.Contains(t, text, `admin.Use(gin.HandlerFunc(adminAuth))`)
	require.Contains(t, text, `registerModelCatalogRoutes(admin, h)`)

	start := strings.Index(text, "func registerModelCatalogRoutes")
	require.NotEqual(t, -1, start)
	remainder := text[start+1:]
	end := strings.Index(remainder, "\nfunc ")
	require.NotEqual(t, -1, end)
	block := remainder[:end]

	require.NotContains(t, block, "stepUpAuth")
	require.Contains(t, block, `catalog.POST("/sync", h.Admin.ModelCatalog.Sync)`)
	require.Contains(t, block, `catalog.PATCH("/models/:id/state", h.Admin.ModelCatalog.UpdateState)`)
	require.Contains(t, block, `catalog.PATCH("/models/:id/pricing", h.Admin.ModelCatalog.UpdatePricing)`)
}
