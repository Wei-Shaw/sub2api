package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
)

// ContextKey 定义上下文键类型
type ContextKey string

const (
	// ContextKeyUser 用户上下文键
	ContextKeyUser ContextKey = "user"
	// ContextKeyUserRole 当前用户角色（string）
	ContextKeyUserRole ContextKey = "user_role"
	// ContextKeyApiKey API密钥上下文键
	ContextKeyApiKey ContextKey = "api_key"
	// ContextKeySubscription 订阅上下文键
	ContextKeySubscription ContextKey = "subscription"
	// ContextKeyForcePlatform 强制平台（用于 /antigravity 路由）
	ContextKeyForcePlatform ContextKey = "force_platform"
)

// ctxKeyForcePlatformStr 用于 request.Context 的字符串 key（供 Service 读取）
// 注意：service 包中也需要使用相同的字符串 "ctx_force_platform"
const ctxKeyForcePlatformStr = "ctx_force_platform"

// ForcePlatform 返回设置强制平台的中间件
// 同时设置 request.Context（供 Service 使用）和 gin.Context（供 Handler 快速检查）
func ForcePlatform(platform string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置到 request.Context，使用字符串 key 供 Service 层读取
		ctx := context.WithValue(c.Request.Context(), ctxKeyForcePlatformStr, platform)
		c.Request = c.Request.WithContext(ctx)
		// 同时设置到 gin.Context，供 Handler 快速检查
		c.Set(string(ContextKeyForcePlatform), platform)
		c.Next()
REDACTED
REDACTED

// HasForcePlatform 检查是否有强制平台（用于 Handler 跳过分组检查）
func HasForcePlatform(c *gin.Context) bool {
	_, exists := c.Get(string(ContextKeyForcePlatform))
	return exists
REDACTED

// GetForcePlatformFromContext 从 gin.Context 获取强制平台
func GetForcePlatformFromContext(c *gin.Context) (string, bool) {
	value, exists := c.Get(string(ContextKeyForcePlatform))
	if !exists {
		return "", false
REDACTED
	platform, ok := value.(string)
	return platform, ok
REDACTED

// ErrorResponse 标准错误响应结构
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
REDACTED

// NewErrorResponse 创建错误响应
func NewErrorResponse(code, message string) ErrorResponse {
	return ErrorResponse{
		Code:    code,
		Message: message,
REDACTED
REDACTED

// AbortWithError 中断请求并返回JSON错误
func AbortWithError(c *gin.Context, statusCode int, code, message string) {
	c.JSON(statusCode, NewErrorResponse(code, message))
	c.Abort()
REDACTED
