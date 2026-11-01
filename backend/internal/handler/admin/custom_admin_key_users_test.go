//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedUserResponseService struct {
	service.AdminService
	filters service.UserListFilters
	writes  int
}

func (*scopedUserResponseService) user() *service.User {
	return &service.User{ID: 7, Email: "user@example.com", Notes: "内部备注", Balance: 123,
		APIKeys: []service.APIKey{{Key: "内部密钥"}}, Subscriptions: []service.UserSubscription{{ID: 8, DailyUsageUSD: 10}}}
}
func (s *scopedUserResponseService) ListUsers(_ context.Context, _, _ int, filters service.UserListFilters, _, _ string) ([]service.User, int64, error) {
	s.filters = filters
	return []service.User{*s.user()}, 1, nil
}
func (s *scopedUserResponseService) GetUser(context.Context, int64) (*service.User, error) {
	return s.user(), nil
}
func (s *scopedUserResponseService) UpdateUserBalance(context.Context, int64, float64, string, string) (*service.User, error) {
	s.writes++
	return s.user(), nil
}

func TestScopedAdminUserResponsePreservesFullAdminResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &scopedUserResponseService{}
	h := NewUserHandler(svc, nil, nil, nil, nil, nil, nil)
	r := gin.New()
	r.GET("/users", h.List)
	r.GET("/users/:id", h.GetByID)
	for _, path := range []string{"/users", "/users?include_subscriptions=true", "/users/7"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 200, w.Code)
		for _, field := range []string{`"notes":"内部备注"`, `"balance":123`, `"subscriptions":`, `"daily_usage_usd":10`, `"api_keys":`} {
			require.Contains(t, w.Body.String(), field)
		}
		if path == "/users" {
			require.Nil(t, svc.filters.IncludeSubscriptions)
		}
		if strings.Contains(path, "include_subscriptions") {
			require.NotNil(t, svc.filters.IncludeSubscriptions)
			require.True(t, *svc.filters.IncludeSubscriptions)
		}
	}
}

func TestScopedAdminUserBalanceResponseRedactsCachedFullAdminResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := service.DefaultIdempotencyCoordinator()
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
	for _, scopedFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "完整管理员先执行", true: "受限Key先执行"}[scopedFirst], func(t *testing.T) {
			service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
			svc := &scopedUserResponseService{}
			h := NewUserHandler(svc, nil, nil, nil, nil, nil, nil)
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9})
				if c.GetHeader("X-Test-Scoped") == "true" {
					c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{
						KeyID: "test-key", Policy: adminaccess.Policy{GlobalPermissions: []string{"users.balance.write"}},
					}))
				}
				c.Next()
			})
			r.POST("/users/:id/balance", h.UpdateBalance)
			for i, scoped := range []bool{scopedFirst, !scopedFirst, true} {
				req := httptest.NewRequest("POST", "/users/7/balance", strings.NewReader(`{"balance":1,"operation":"add"}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", "same-operation")
				if scoped {
					req.Header.Set("X-Test-Scoped", "true")
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				require.Equal(t, 200, w.Code, w.Body.String())
				if i > 0 {
					require.Equal(t, "true", w.Header().Get("X-Idempotency-Replayed"))
				}
				var result struct {
					Data map[string]json.RawMessage `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
				if scoped {
					require.Len(t, result.Data, 1)
					require.JSONEq(t, `7`, string(result.Data["id"]))
				} else {
					require.Contains(t, result.Data, "notes")
					require.Contains(t, result.Data, "balance")
				}
			}
			require.Equal(t, 1, svc.writes, "重放只改变当前响应的权限投影，不能重复修改余额")
		})
	}
}
