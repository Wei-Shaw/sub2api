package middleware

import "github.com/gin-gonic/gin"

// ContextKey 定义上下文键类型
type ContextKey string

const (
	// ContextKeyUser 用户上下文键
	ContextKeyUser ContextKey = "user"
	// ContextKeyApiKey API密钥上下文键
	ContextKeyApiKey ContextKey = "api_key"
	// ContextKeySubscription 订阅上下文键
	ContextKeySubscription ContextKey = "subscription"
)

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
