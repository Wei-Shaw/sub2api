//go:build unit

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminUsageViewerScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		role       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "admin_allows_any_admin_path", role: service.RoleAdmin, method: http.MethodDelete, path: "/api/v1/admin/accounts/1", wantStatus: http.StatusOK},
		{name: "usage_viewer_allows_account_list", role: service.RoleUsageViewer, method: http.MethodGet, path: "/api/v1/admin/accounts", wantStatus: http.StatusOK},
		{name: "usage_viewer_allows_account_summary", role: service.RoleUsageViewer, method: http.MethodGet, path: "/api/v1/admin/accounts/usage-viewer-summary", wantStatus: http.StatusOK},
		{name: "usage_viewer_allows_account_usage", role: service.RoleUsageViewer, method: http.MethodGet, path: "/api/v1/admin/accounts/123/usage", wantStatus: http.StatusOK},
		{name: "usage_viewer_allows_batch_today_stats", role: service.RoleUsageViewer, method: http.MethodPost, path: "/api/v1/admin/accounts/today-stats/batch", wantStatus: http.StatusOK},
		{name: "usage_viewer_allows_compliance_status", role: service.RoleUsageViewer, method: http.MethodGet, path: "/api/v1/admin/compliance", wantStatus: http.StatusOK},
		{name: "usage_viewer_allows_compliance_accept", role: service.RoleUsageViewer, method: http.MethodPost, path: "/api/v1/admin/compliance/accept", wantStatus: http.StatusOK},
		{name: "usage_viewer_rejects_account_update", role: service.RoleUsageViewer, method: http.MethodPut, path: "/api/v1/admin/accounts/123", wantStatus: http.StatusForbidden},
		{name: "usage_viewer_rejects_group_list", role: service.RoleUsageViewer, method: http.MethodGet, path: "/api/v1/admin/groups", wantStatus: http.StatusForbidden},
		{name: "usage_viewer_rejects_similar_prefix", role: service.RoleUsageViewer, method: http.MethodGet, path: "/api/v1/admin/accounts/data", wantStatus: http.StatusForbidden},
		{name: "regular_user_rejected", role: service.RoleUser, method: http.MethodGet, path: "/api/v1/admin/accounts", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyUserRole), tt.role)
				c.Next()
			})
			router.Use(AdminUsageViewerScope())
			router.Any("/api/v1/admin/*path", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
