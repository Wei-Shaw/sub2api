package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newUsageBalanceContext(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, nil)
	return c, recorder
}

func TestUsageBalanceUnauthorizedWithoutAPIKey(t *testing.T) {
	c, recorder := newUsageBalanceContext(http.MethodPost, "/v1/user/balance")
	handler := &GatewayHandler{}
	handler.UsageBalance(c)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	var resp apiUsageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.False(t, resp.IsValid)
	require.NotEmpty(t, resp.Error)
	require.Equal(t, "USD", resp.Unit)
}

func TestUsageBalanceQuotaLimited(t *testing.T) {
	c, recorder := newUsageBalanceContext(http.MethodPost, "/v1/user/balance")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		Name:      "my-key",
		Status:    service.StatusAPIKeyActive,
		Quota:     100,
		QuotaUsed: 40,
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})

	handler := &GatewayHandler{}
	handler.UsageBalance(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp apiUsageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.IsValid)
	require.Empty(t, resp.Error)
	require.Equal(t, 60.0, resp.Balance)
	require.Equal(t, 60.0, resp.Remaining)
	require.Equal(t, "USD", resp.Unit)
	require.Equal(t, "my-key", resp.PlanName)
	require.Equal(t, "quota_limited", resp.Mode)
	require.NotNil(t, resp.Total)
	require.Equal(t, 100.0, *resp.Total)
	require.NotNil(t, resp.Used)
	require.Equal(t, 40.0, *resp.Used)
}

func TestUsageBalanceSubscription(t *testing.T) {
	c, recorder := newUsageBalanceContext(http.MethodPost, "/v1/user/balance")
	dailyLimit := 20.0
	weeklyLimit := 50.0
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		Status: service.StatusAPIKeyActive,
		Group: &service.Group{
			Name:             "Pro",
			SubscriptionType: service.SubscriptionTypeSubscription,
			DailyLimitUSD:    &dailyLimit,
			WeeklyLimitUSD:   &weeklyLimit,
		},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2})
	c.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{
		DailyUsageUSD:  5,
		WeeklyUsageUSD: 10,
	})

	handler := &GatewayHandler{}
	handler.UsageBalance(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp apiUsageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.IsValid)
	require.Empty(t, resp.Error)
	// min(20-5, 50-10) = 15
	require.Equal(t, 15.0, resp.Balance)
	require.Equal(t, "Pro", resp.PlanName)
	require.Equal(t, "subscription", resp.Mode)
}

func TestUsageBalanceSubscriptionMissing(t *testing.T) {
	c, recorder := newUsageBalanceContext(http.MethodGet, "/v1/user/balance")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		Status: service.StatusAPIKeyActive,
		Group: &service.Group{
			Name:             "Pro",
			SubscriptionType: service.SubscriptionTypeSubscription,
		},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2})

	handler := &GatewayHandler{}
	handler.UsageBalance(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp apiUsageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.False(t, resp.IsValid)
	require.Equal(t, "No active subscription", resp.Error)
	require.Equal(t, 0.0, resp.Balance)
}

func TestUsageBalanceWalletWithoutUserService(t *testing.T) {
	c, recorder := newUsageBalanceContext(http.MethodPost, "/v1/user/balance")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		Status: service.StatusAPIKeyActive,
		Group:  &service.Group{Name: "default"},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9})

	handler := &GatewayHandler{}
	handler.UsageBalance(c)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	var resp apiUsageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.False(t, resp.IsValid)
	require.NotEmpty(t, resp.Error)
}
