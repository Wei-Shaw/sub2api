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

func TestAdminRoleAccessMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		role       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "admin_has_full_access_to_admin_resources",
			role:       service.RoleAdmin,
			method:     http.MethodDelete,
			path:       "/api/v1/admin/accounts/42",
			wantStatus: http.StatusOK,
		},
		{
			name:       "marketing_can_view_dashboard",
			role:       service.RoleMarketing,
			method:     http.MethodGet,
			path:       "/api/v1/admin/dashboard/summary",
			wantStatus: http.StatusOK,
		},
		{
			name:       "marketing_can_view_users_for_customer_care",
			role:       service.RoleMarketing,
			method:     http.MethodGet,
			path:       "/api/v1/admin/users",
			wantStatus: http.StatusOK,
		},
		{
			name:       "marketing_can_activate_affiliate_scoped_user_devices",
			role:       service.RoleMarketing,
			method:     http.MethodPost,
			path:       "/api/v1/admin/users/7/activate-devices",
			wantStatus: http.StatusOK,
		},
		{
			name:       "marketing_can_update_affiliate_scoped_user_balance",
			role:       service.RoleMarketing,
			method:     http.MethodPost,
			path:       "/api/v1/admin/users/7/balance",
			wantStatus: http.StatusOK,
		},
		{
			name:       "marketing_can_manage_subscriptions",
			role:       service.RoleMarketing,
			method:     http.MethodPost,
			path:       "/api/v1/admin/subscriptions/grant",
			wantStatus: http.StatusOK,
		},
		{
			name:       "marketing_can_manage_sales_campaign_vouchers",
			role:       service.RoleMarketing,
			method:     http.MethodPut,
			path:       "/api/v1/admin/promo-codes/7",
			wantStatus: http.StatusOK,
		},
		{
			name:       "marketing_can_manage_subscription_plans",
			role:       service.RoleMarketing,
			method:     http.MethodPut,
			path:       "/api/v1/admin/payment/plans/3",
			wantStatus: http.StatusOK,
		},
		{
			name:       "marketing_can_handle_payment_order_support_actions",
			role:       service.RoleMarketing,
			method:     http.MethodPost,
			path:       "/api/v1/admin/payment/orders/9/refund",
			wantStatus: http.StatusOK,
		},
		{
			name:       "marketing_can_view_ops_but_cannot_change_ops_settings",
			role:       service.RoleMarketing,
			method:     http.MethodPut,
			path:       "/api/v1/admin/ops/runtime/logging",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "marketing_cannot_manage_accounts",
			role:       service.RoleMarketing,
			method:     http.MethodGet,
			path:       "/api/v1/admin/accounts",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "user_cannot_access_admin_resources",
			role:       service.RoleUser,
			method:     http.MethodGet,
			path:       "/api/v1/admin/users",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing_role_is_unauthorized",
			method:     http.MethodGet,
			path:       "/api/v1/admin/users",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			if tc.role != "" {
				r.Use(func(c *gin.Context) {
					c.Set(string(ContextKeyUserRole), tc.role)
					c.Next()
				})
			}
			r.Use(AdminRoleAccessMiddleware())
			r.Any("/*path", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
		})
	}
}
