package middleware

import (
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ApiKeyAuthGoogle is a Google-style error wrapper for API key auth.
func ApiKeyAuthGoogle(apiKeyService *service.ApiKeyService) gin.HandlerFunc {
	return ApiKeyAuthWithSubscriptionGoogle(apiKeyService, nil)
REDACTED

// ApiKeyAuthWithSubscriptionGoogle behaves like ApiKeyAuthWithSubscription but returns Google-style errors:
// {"error":{"code":401,"message":"...","status":"UNAUTHENTICATED"REDACTEDREDACTED
//
// It is intended for Gemini native endpoints (/v1beta) to match Gemini SDK expectations.
func ApiKeyAuthWithSubscriptionGoogle(apiKeyService *service.ApiKeyService, subscriptionService *service.SubscriptionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKeyString := extractAPIKeyFromRequest(c)
		if apiKeyString == "" {
			abortWithGoogleError(c, 401, "API key is required")
			return
	REDACTED

		apiKey, err := apiKeyService.GetByKey(c.Request.Context(), apiKeyString)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				abortWithGoogleError(c, 401, "Invalid API key")
				return
		REDACTED
			abortWithGoogleError(c, 500, "Failed to validate API key")
			return
	REDACTED

		if !apiKey.IsActive() {
			abortWithGoogleError(c, 401, "API key is disabled")
			return
	REDACTED
		if apiKey.User == nil {
			abortWithGoogleError(c, 401, "User associated with API key not found")
			return
	REDACTED
		if !apiKey.User.IsActive() {
			abortWithGoogleError(c, 401, "User account is not active")
			return
	REDACTED

		isSubscriptionType := apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
		if isSubscriptionType && subscriptionService != nil {
			subscription, err := subscriptionService.GetActiveSubscription(
				c.Request.Context(),
				apiKey.User.ID,
				apiKey.Group.ID,
			)
			if err != nil {
				abortWithGoogleError(c, 403, "No active subscription found for this group")
				return
		REDACTED
			if err := subscriptionService.ValidateSubscription(c.Request.Context(), subscription); err != nil {
				abortWithGoogleError(c, 403, err.Error())
				return
		REDACTED
			_ = subscriptionService.CheckAndActivateWindow(c.Request.Context(), subscription)
			_ = subscriptionService.CheckAndResetWindows(c.Request.Context(), subscription)
			if err := subscriptionService.CheckUsageLimits(c.Request.Context(), subscription, apiKey.Group, 0); err != nil {
				abortWithGoogleError(c, 429, err.Error())
				return
		REDACTED
			c.Set(string(ContextKeySubscription), subscription)
	REDACTED else {
			if apiKey.User.Balance <= 0 {
				abortWithGoogleError(c, 403, "Insufficient account balance")
				return
		REDACTED
	REDACTED

		c.Set(string(ContextKeyApiKey), apiKey)
		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      apiKey.User.ID,
			Concurrency: apiKey.User.Concurrency,
	REDACTED)
		c.Set(string(ContextKeyUserRole), apiKey.User.Role)
		c.Next()
REDACTED
REDACTED

func extractAPIKeyFromRequest(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" && strings.TrimSpace(parts[1]) != "" {
			return strings.TrimSpace(parts[1])
	REDACTED
REDACTED
	if v := strings.TrimSpace(c.GetHeader("x-api-key")); v != "" {
		return v
REDACTED
	if v := strings.TrimSpace(c.GetHeader("x-goog-api-key")); v != "" {
		return v
REDACTED
	if v := strings.TrimSpace(c.Query("key")); v != "" {
		return v
REDACTED
	if v := strings.TrimSpace(c.Query("api_key")); v != "" {
		return v
REDACTED
	return ""
REDACTED

func abortWithGoogleError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
	REDACTED,
REDACTED)
	c.Abort()
REDACTED
