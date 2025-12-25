package middleware

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ApiKeyAuthService 定义API Key认证服务需要的接口
type ApiKeyAuthService interface {
	GetByKey(ctx context.Context, key string) (*model.ApiKey, error)
}

// SubscriptionAuthService 定义订阅认证服务需要的接口
type SubscriptionAuthService interface {
	GetActiveSubscription(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error)
	ValidateSubscription(ctx context.Context, sub *model.UserSubscription) error
	CheckAndActivateWindow(ctx context.Context, sub *model.UserSubscription) error
	CheckAndResetWindows(ctx context.Context, sub *model.UserSubscription) error
	CheckUsageLimits(ctx context.Context, sub *model.UserSubscription, group *model.Group, additionalCost float64) error
}

// ApiKeyAuth API Key认证中间件
func ApiKeyAuth(apiKeyRepo ApiKeyAuthService) gin.HandlerFunc {
	return ApiKeyAuthWithSubscription(apiKeyRepo, nil)
}

// ApiKeyAuthWithSubscription API Key认证中间件（支持订阅验证）
func ApiKeyAuthWithSubscription(apiKeyRepo ApiKeyAuthService, subscriptionService SubscriptionAuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 尝试从Authorization header中提取API key (Bearer scheme)
		authHeader := c.GetHeader("Authorization")
		var apiKeyString string

		if authHeader != "" {
			// 验证Bearer scheme
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				apiKeyString = parts[1]
			}
		}

		// 如果Authorization header中没有，尝试从x-api-key header中提取
		if apiKeyString == "" {
			apiKeyString = c.GetHeader("x-api-key")
		}

		// 如果两个header都没有API key
		if apiKeyString == "" {
			AbortWithError(c, 401, "API_KEY_REQUIRED", "API key is required in Authorization header (Bearer scheme) or x-api-key header")
			return
		}

		// 从数据库验证API key
		apiKey, err := apiKeyRepo.GetByKey(c.Request.Context(), apiKeyString)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				AbortWithError(c, 401, "INVALID_API_KEY", "Invalid API key")
				return
			}
			AbortWithError(c, 500, "INTERNAL_ERROR", "Failed to validate API key")
			return
		}

		// 检查API key是否激活
		if !apiKey.IsActive() {
			AbortWithError(c, 401, "API_KEY_DISABLED", "API key is disabled")
			return
		}

		// 检查关联的用户
		if apiKey.User == nil {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User associated with API key not found")
			return
		}

		// 检查用户状态
		if !apiKey.User.IsActive() {
			AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
			return
		}

		// 判断计费方式：订阅模式 vs 余额模式
		isSubscriptionType := apiKey.Group != nil && apiKey.Group.IsSubscriptionType()

		if isSubscriptionType && subscriptionService != nil {
			// 订阅模式：验证订阅
			subscription, err := subscriptionService.GetActiveSubscription(
				c.Request.Context(),
				apiKey.User.ID,
				apiKey.Group.ID,
			)
			if err != nil {
				AbortWithError(c, 403, "SUBSCRIPTION_NOT_FOUND", "No active subscription found for this group")
				return
			}

			// 验证订阅状态（是否过期、暂停等）
			if err := subscriptionService.ValidateSubscription(c.Request.Context(), subscription); err != nil {
				AbortWithError(c, 403, "SUBSCRIPTION_INVALID", err.Error())
				return
			}

			// 激活滑动窗口（首次使用时）
			if err := subscriptionService.CheckAndActivateWindow(c.Request.Context(), subscription); err != nil {
				log.Printf("Failed to activate subscription windows: %v", err)
			}

			// 检查并重置过期窗口
			if err := subscriptionService.CheckAndResetWindows(c.Request.Context(), subscription); err != nil {
				log.Printf("Failed to reset subscription windows: %v", err)
			}

			// 预检查用量限制（使用0作为额外费用进行预检查）
			if err := subscriptionService.CheckUsageLimits(c.Request.Context(), subscription, apiKey.Group, 0); err != nil {
				AbortWithError(c, 429, "USAGE_LIMIT_EXCEEDED", err.Error())
				return
			}

			// 将订阅信息存入上下文
			c.Set(string(ContextKeySubscription), subscription)
		} else {
			// 余额模式：检查用户余额
			if apiKey.User.Balance <= 0 {
				AbortWithError(c, 403, "INSUFFICIENT_BALANCE", "Insufficient account balance")
				return
			}
		}

		// 将API key和用户信息存入上下文
		c.Set(string(ContextKeyApiKey), apiKey)
		c.Set(string(ContextKeyUser), apiKey.User)

		c.Next()
	}
}

// GetApiKeyFromContext 从上下文中获取API key
func GetApiKeyFromContext(c *gin.Context) (*model.ApiKey, bool) {
	value, exists := c.Get(string(ContextKeyApiKey))
	if !exists {
		return nil, false
	}
	apiKey, ok := value.(*model.ApiKey)
	return apiKey, ok
}

// GetSubscriptionFromContext 从上下文中获取订阅信息
func GetSubscriptionFromContext(c *gin.Context) (*model.UserSubscription, bool) {
	value, exists := c.Get(string(ContextKeySubscription))
	if !exists {
		return nil, false
	}
	subscription, ok := value.(*model.UserSubscription)
	return subscription, ok
}
