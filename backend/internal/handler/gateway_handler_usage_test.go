package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUsageUnrestrictedIncludesWeeklyWindowStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	weeklyWindowStart := time.Date(2026, time.July, 13, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	c.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{
		WeeklyWindowStart: &weeklyWindowStart,
	})

	handler := &GatewayHandler{}
	handler.usageUnrestricted(
		c,
		context.Background(),
		&service.APIKey{Group: &service.Group{
			Name:             "Weekly plan",
			SubscriptionType: service.SubscriptionTypeSubscription,
		}},
		middleware.AuthSubject{},
		nil,
		nil,
		nil,
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Subscription struct {
			WeeklyWindowStart *time.Time `json:"weekly_window_start"`
		} `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotNil(t, response.Subscription.WeeklyWindowStart)
	require.True(t, weeklyWindowStart.Equal(*response.Subscription.WeeklyWindowStart))
}

// TestUsageUnrestrictedTokenPlanShowsTokenDimensions 验证 token 型订阅在 /v1/usage
// 下展示 token 维度（已用/限额/单位/剩余），且不回吐 USD 维度。
func TestUsageUnrestrictedTokenPlanShowsTokenDimensions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	dailyLimit := int64(100000)
	c.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{
		DailyUsageTokens: 30000,
	})

	handler := &GatewayHandler{}
	handler.usageUnrestricted(
		c,
		context.Background(),
		&service.APIKey{Group: &service.Group{
			Name:             "Token plan",
			SubscriptionType: service.SubscriptionTypeSubscriptionToken,
			DailyLimitTokens: &dailyLimit,
		}},
		middleware.AuthSubject{},
		nil,
		nil,
		nil,
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Unit         string  `json:"unit"`
		Remaining    float64 `json:"remaining"`
		Subscription struct {
			DailyUsedTokens  int64    `json:"daily_usage_tokens"`
			DailyLimitTokens *int64   `json:"daily_limit_tokens"`
			DailyUsageUSD    *float64 `json:"daily_usage_usd"`
		} `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "tokens", response.Unit)
	require.Equal(t, int64(30000), response.Subscription.DailyUsedTokens)
	require.NotNil(t, response.Subscription.DailyLimitTokens)
	require.Equal(t, int64(100000), *response.Subscription.DailyLimitTokens)
	require.Equal(t, float64(70000), response.Remaining)
	// token 型订阅不应回吐 USD 维度（证明走的是 token 分支而非两套都返回）
	require.Nil(t, response.Subscription.DailyUsageUSD)
}
