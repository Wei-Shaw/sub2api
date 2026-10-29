//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedSubscriptionRepo struct {
	service.UserSubscriptionRepository
	writes int
}

func (*scopedSubscriptionRepo) GetByID(context.Context, int64) (*service.UserSubscription, error) {
	return &service.UserSubscription{ID: 1, UserID: 2, GroupID: 3, DailyUsageUSD: 4, ExpiresAt: time.Now().Add(time.Hour),
		User:           &service.User{ID: 2, Email: "private-user@example.com", Balance: 987},
		AssignedByUser: &service.User{ID: 8, Email: "private-admin@example.com", Balance: 654},
		Group:          &service.Group{ID: 3, Name: "group", RateMultiplier: 42}}, nil
}
func TestScopedAdminSubscriptionDoesNotExposeAssociatedUserAndPricing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := service.NewSubscriptionService(nil, &scopedSubscriptionRepo{}, nil, nil, nil)
	defer s.Stop()
	h := NewSubscriptionHandler(s)
	for _, scoped := range []bool{false, true} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/subscriptions/1", nil)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		if scoped {
			c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: []string{"subscriptions.read"}}}))
		}
		h.GetByID(c)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"daily_usage_usd":4`)
		for _, field := range []string{"private-user@example.com", "private-admin@example.com", `"balance"`, `"rate_multiplier"`} {
			if scoped {
				require.NotContains(t, w.Body.String(), field)
			} else {
				require.Contains(t, w.Body.String(), field)
			}
		}
	}
}
func TestScopedAdminQuotaWriteDoesNotGrantQuotaRead(t *testing.T) {
	repo := &upsertCapturingQuotaRepo{listRecords: []service.UserPlatformQuotaRecord{{UserID: 42, Platform: "anthropic"}}}
	h := buildTestHandler(repo, &billingCacheStub{})
	c, w := putReq(t, `{"quotas":[{"platform":"anthropic","daily_limit_usd":10}]}`)
	c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: []string{"users.quotas.write"}}}))
	h.UpdateUserPlatformQuotas(c)
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), "platform_quotas")
	require.NotContains(t, w.Body.String(), "usage")
	require.Len(t, repo.upsertCalls, 1)
}

func (r *scopedSubscriptionRepo) GetByIDForUpdate(ctx context.Context, id int64) (*service.UserSubscription, error) {
	return r.GetByID(ctx, id)
}
func (r *scopedSubscriptionRepo) ExtendExpiry(context.Context, int64, time.Time) error {
	r.writes++
	return nil
}

func TestScopedAdminSubscriptionExtendReplayRespectsReadPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := service.DefaultIdempotencyCoordinator()
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), service.DefaultIdempotencyConfig()))
	repo := &scopedSubscriptionRepo{}
	svc := service.NewSubscriptionService(nil, repo, nil, nil, nil)
	defer svc.Stop()
	h := NewSubscriptionHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Mode") != "admin" {
			permissions := []string{"subscriptions.extend"}
			if c.GetHeader("X-Test-Mode") == "read" {
				permissions = append(permissions, "subscriptions.read")
			}
			c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: permissions}}))
		}
	})
	r.POST("/subscriptions/:id/extend", h.Extend)
	for i, mode := range []string{"admin", "write", "read"} {
		req := httptest.NewRequest("POST", "/subscriptions/1/extend", strings.NewReader(`{"days":1}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "same-extension")
		req.Header.Set("X-Test-Mode", mode)
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
		if mode == "admin" {
			require.Contains(t, result.Data, "user")
		} else {
			require.NotContains(t, result.Data, "user")
			require.NotContains(t, result.Data, "group")
			require.NotContains(t, result.Data, "assigned_by_user")
			if mode == "write" {
				require.Len(t, result.Data, 1)
			} else {
				require.Contains(t, result.Data, "daily_usage_usd")
			}
		}
	}
	require.Equal(t, 1, repo.writes)
}

func TestScopedAdminSubscriptionBulkProjectionPreservesIDs(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{}))
	sub, _ := (&scopedSubscriptionRepo{}).GetByID(context.Background(), 1)
	sub.ID = 9007199254740993
	data := &dto.BulkAssignResult{SuccessCount: 1, Subscriptions: []dto.AdminUserSubscription{*dto.UserSubscriptionFromServiceAdmin(sub)}}
	raw, err := json.Marshal(adminSubscriptionResponse(c, data))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"id":9007199254740993`)
	require.Contains(t, string(raw), `"success_count":1`)
	for _, field := range []string{"daily_usage_usd", "user_id", "balance", "group", "private-user"} {
		require.NotContains(t, string(raw), field)
	}
}

func TestScopedAdminQuotaResetRequiresReadToReturnUsage(t *testing.T) {
	for _, readable := range []bool{false, true} {
		repo := &upsertCapturingQuotaRepo{listRecords: []service.UserPlatformQuotaRecord{{UserID: 42, Platform: "anthropic"}}}
		cache := &billingCacheStub{}
		h := buildTestHandler(repo, cache)
		c, w := postReq(t, `{"platform":"anthropic","window":"daily"}`)
		permissions := []string{"users.quotas.write"}
		if readable {
			permissions = append(permissions, "users.quotas.read")
		}
		c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: permissions}}))
		h.ResetUserPlatformQuotaWindow(c)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Len(t, repo.resetCalls, 1)
		require.Len(t, cache.deleteCalls, 1)
		if readable {
			require.Contains(t, w.Body.String(), "platform_quotas")
		} else {
			require.NotContains(t, w.Body.String(), "platform_quotas")
		}
	}
}
