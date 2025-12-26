package middleware

import (
	"errors"
	"log"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewApiKeyAuthMiddleware 创建 API Key 认证中间件
func NewApiKeyAuthMiddleware(apiKeyService *service.ApiKeyService, subscriptionService *service.SubscriptionService) ApiKeyAuthMiddleware {
	return ApiKeyAuthMiddleware(apiKeyAuthWithSubscription(apiKeyService, subscriptionService))
REDACTED

// apiKeyAuthWithSubscription API Key认证中间件（支持订阅验证）
func apiKeyAuthWithSubscription(apiKeyService *service.ApiKeyService, subscriptionService *service.SubscriptionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 尝试从Authorization header中提取API key (Bearer scheme)
		authHeader := c.GetHeader("Authorization")
		var apiKeyString string

		if authHeader != "" {
			// 验证Bearer scheme
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				apiKeyString = parts[1]
		REDACTED
	REDACTED

		// 如果Authorization header中没有，尝试从x-api-key header中提取
		if apiKeyString == "" {
			apiKeyString = c.GetHeader("x-api-key")
	REDACTED

		// 如果两个header都没有API key
		if apiKeyString == "" {
			AbortWithError(c, 401, "API_KEY_REQUIRED", "API key is required in Authorization header (Bearer scheme) or x-api-key header")
			return
	REDACTED

		// 从数据库验证API key
		apiKey, err := apiKeyService.GetByKey(c.Request.Context(), apiKeyString)
		if err != nil {
			if errors.Is(err, service.ErrApiKeyNotFound) {
				AbortWithError(c, 401, "INVALID_API_KEY", "Invalid API key")
				return
		REDACTED
			AbortWithError(c, 500, "INTERNAL_ERROR", "Failed to validate API key")
			return
	REDACTED

		// 检查API key是否激活
		if !apiKey.IsActive() {
			AbortWithError(c, 401, "API_KEY_DISABLED", "API key is disabled")
			return
	REDACTED

		// 检查关联的用户
		if apiKey.User == nil {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User associated with API key not found")
			return
	REDACTED

		// 检查用户状态
		if !apiKey.User.IsActive() {
			AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
			return
	REDACTED

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
		REDACTED

			// 验证订阅状态（是否过期、暂停等）
			if err := subscriptionService.ValidateSubscription(c.Request.Context(), subscription); err != nil {
				AbortWithError(c, 403, "SUBSCRIPTION_INVALID", err.Error())
				return
		REDACTED

			// 激活滑动窗口（首次使用时）
			if err := subscriptionService.CheckAndActivateWindow(c.Request.Context(), subscription); err != nil {
				log.Printf("Failed to activate subscription windows: %v", err)
		REDACTED

			// 检查并重置过期窗口
			if err := subscriptionService.CheckAndResetWindows(c.Request.Context(), subscription); err != nil {
				log.Printf("Failed to reset subscription windows: %v", err)
		REDACTED

			// 预检查用量限制（使用0作为额外费用进行预检查）
			if err := subscriptionService.CheckUsageLimits(c.Request.Context(), subscription, apiKey.Group, 0); err != nil {
				AbortWithError(c, 429, "USAGE_LIMIT_EXCEEDED", err.Error())
				return
		REDACTED

			// 将订阅信息存入上下文
			c.Set(string(ContextKeySubscription), subscription)
	REDACTED else {
			// 余额模式：检查用户余额
			if apiKey.User.Balance <= 0 {
				AbortWithError(c, 403, "INSUFFICIENT_BALANCE", "Insufficient account balance")
				return
		REDACTED
	REDACTED

		// 将API key和用户信息存入上下文
		c.Set(string(ContextKeyApiKey), apiKey)
		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      apiKey.User.ID,
			Concurrency: apiKey.User.Concurrency,
	REDACTED)
		c.Set(string(ContextKeyUserRole), apiKey.User.Role)

		c.Next()
REDACTED
REDACTED

// GetApiKeyFromContext 从上下文中获取API key
func GetApiKeyFromContext(c *gin.Context) (*service.ApiKey, bool) {
	value, exists := c.Get(string(ContextKeyApiKey))
	if !exists {
		return nil, false
REDACTED
	apiKey, ok := value.(*service.ApiKey)
	return apiKey, ok
REDACTED

// GetSubscriptionFromContext 从上下文中获取订阅信息
func GetSubscriptionFromContext(c *gin.Context) (*service.UserSubscription, bool) {
	value, exists := c.Get(string(ContextKeySubscription))
	if !exists {
		return nil, false
REDACTED
	subscription, ok := value.(*service.UserSubscription)
	return subscription, ok
REDACTED
