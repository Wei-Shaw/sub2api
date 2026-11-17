//go:build unit

package routes

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedUserResponses struct {
	service.AdminService
	filters service.UserListFilters
}

func scopedUserFixture() *service.User {
	return &service.User{
		ID: 7, Email: "user@example.com", Username: "测试用户", Role: "user", Status: "active",
		Balance: 123, FrozenBalance: 12, TotalRecharged: 456, Notes: "内部敏感备注",
		AllowedGroups: []int64{3}, RestrictPublicGroups: true, GroupRates: map[int64]float64{3: 0.8},
		APIKeys: []service.APIKey{{ID: 8, Key: "绝不返回的用户密钥"}},
		Subscriptions: []service.UserSubscription{{ID: 2, DailyUsageUSD: 11, WeeklyUsageUSD: 22, MonthlyUsageUSD: 33,
			Group: &service.Group{ID: 3, Name: "内部定价分组", RateMultiplier: 2}}},
	}
}

func (s *scopedUserResponses) ListUsers(_ context.Context, _, _ int, filters service.UserListFilters, _, _ string) ([]service.User, int64, error) {
	s.filters = filters
	// 故意保留关联数据，验证输出边界本身也能防止泄露。
	return []service.User{*scopedUserFixture()}, 1, nil
}
func (*scopedUserResponses) GetUser(context.Context, int64) (*service.User, error) {
	return scopedUserFixture(), nil
}
func (*scopedUserResponses) GetUserIncludeDeleted(context.Context, int64) (*service.User, error) {
	return scopedUserFixture(), nil
}
func (*scopedUserResponses) UpdateUserBalance(context.Context, int64, float64, string, string) (*service.User, error) {
	return scopedUserFixture(), nil
}

func TestScopedAdminUserResponsesRequireIndependentPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		permissions []string
	}{
		{"仅用户资料", []string{"users.read"}},
		{"同时授权余额", []string{"users.read", "users.balance.read"}},
		{"同时授权分组", []string{"users.read", "groups.read"}},
		{"同时授权订阅", []string{"users.read", "subscriptions.read"}},
		{"仅余额写入", []string{"users.balance.write"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := &scopedRouteKeys{}
			settings := service.NewSettingService(nil, nil)
			settings.SetScopedAdminKeyRepository(keys)
			key, _, err := settings.CreateScopedAdminKey(context.Background(), service.ScopedAdminKeyInput{
				Name: "用户字段权限", Enabled: true, Policy: adminaccess.Policy{GlobalPermissions: tc.permissions},
			}, 9)
			require.NoError(t, err)
			auth := middleware.NewAdminAuthMiddleware(nil, service.NewUserService(&scopedRouteUsers{}, nil, nil, nil), settings, nil)
			stub := &scopedUserResponses{}
			r := gin.New()
			RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{
				Account: &adminhandler.AccountHandler{}, User: adminhandler.NewUserHandler(stub, nil, nil, nil, nil, nil, nil),
				Setting: adminhandler.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil),
			}}, auth, middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() }), middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
			paths := []string{"/users", "/users?include_subscriptions=true", "/users/7", "/users/7?include_deleted=true"}
			balanceWrite := tc.name == "仅余额写入"
			if balanceWrite {
				paths = []string{"/users/7/balance"}
			}
			for _, path := range paths {
				t.Run(path, func(t *testing.T) {
					method, body := "GET", ""
					if balanceWrite {
						method, body = "POST", `{"balance":1,"operation":"add"}`
					}
					req := httptest.NewRequest(method, "/api/v1/admin"+path, strings.NewReader(body))
					req.Header.Set("x-api-key", key)
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					require.Equal(t, 200, w.Code, w.Body.String())
					var result struct {
						Data map[string]json.RawMessage `json:"data"`
					}
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
					data := result.Data
					if strings.HasPrefix(path, "/users?") || path == "/users" {
						var items []map[string]json.RawMessage
						require.NoError(t, json.Unmarshal(data["items"], &items))
						require.Len(t, items, 1)
						data = items[0]
						require.NotNil(t, stub.filters.IncludeSubscriptions)
						require.False(t, *stub.filters.IncludeSubscriptions)
					}
					for _, field := range []string{"subscriptions", "api_keys", "notes", "balance_notify_threshold", "balance_notify_extra_emails"} {
						require.NotContains(t, data, field)
					}
					for _, field := range []string{"balance", "frozen_balance", "total_recharged"} {
						require.Equal(t, tc.name == "同时授权余额", data[field] != nil, field)
					}
					for _, field := range []string{"allowed_groups", "restrict_public_groups", "group_rates"} {
						require.Equal(t, tc.name == "同时授权分组", data[field] != nil, field)
					}
					require.Equal(t, !balanceWrite, data["email"] != nil)
					require.NotContains(t, w.Body.String(), "daily_usage_usd")
					require.NotContains(t, w.Body.String(), "内部定价分组")
				})
			}
			if tc.name == "仅用户资料" {
				for _, path := range []string{"/users/7/subscriptions", "/users/7/api-keys"} {
					req := httptest.NewRequest("GET", "/api/v1/admin"+path, nil)
					req.Header.Set("x-api-key", key)
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					require.Equal(t, 403, w.Code, path)
				}
			}
		})
	}
}
