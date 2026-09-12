package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHTTPSubscriptionQuotaFreezesAfterWaitAndSurvivesRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	subscription := &service.UserSubscription{ID: 1, UserID: 2, GroupID: 3, Quota: &service.SubscriptionQuotaSnapshot{DailyBucketID: 10}}
	group := &service.Group{ID: 3, SubscriptionType: service.SubscriptionTypeSubscription}
	c.Set(string(middleware.ContextKeySubscription), subscription)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{User: &service.User{ID: 2}, Group: group})
	calls := 0
	billing := &service.BillingCacheService{}
	billing.SetSubscriptionQuotaAdmission(func(context.Context, int64, *service.Group) (*service.UserSubscription, error) {
		calls++
		return &service.UserSubscription{ID: 1, UserID: 2, GroupID: 3, Quota: &service.SubscriptionQuotaSnapshot{DailyBucketID: int64(20 + calls)}}, nil
	})
	h := &OpenAIGatewayHandler{billingCacheService: billing}
	require.NoError(t, h.freezeHTTPSubscriptionQuota(c))
	require.Equal(t, int64(21), subscription.Quota.DailyBucketID, "refresh the pre-wait token")
	require.NoError(t, h.freezeHTTPSubscriptionQuota(c))
	require.Equal(t, 1, calls, "failover retries retain the admitted bucket")
}
