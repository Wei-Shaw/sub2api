package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ContextKey 定义上下文键类型
type ContextKey string

const (
	// ContextKeyUser 用户上下文键
	ContextKeyUser ContextKey = "user"
	// ContextKeyUserRole 当前用户角色（string）
	ContextKeyUserRole ContextKey = "user_role"
	// ContextKeyAPIKey API密钥上下文键
	ContextKeyAPIKey ContextKey = "api_key"
	// ContextKeySubscription 订阅上下文键
	ContextKeySubscription ContextKey = "subscription"
	// ContextKeyForcePlatform 强制平台（用于 /antigravity 路由）
	ContextKeyForcePlatform ContextKey = "force_platform"
	// ContextKeyEffectiveGroup 跨组模型路由命中后的目标分组（*service.Group）。
	// 只在命中路由时设置；未命中则不存在，下游回落 apiKey.Group。
	ContextKeyEffectiveGroup ContextKey = "effective_group"
	// ContextKeyOpsFallbackAPIKey 运维错误日志专用回退键。
	// 鉴权早退（分组停用/删除、Key 停用/过期/额度、用户停用、IP 限制等）时，
	// apiKey 已加载但尚未写入 ContextKeyAPIKey；该键让 Ops 错误日志仍能取到
	// user/group/platform。仅供 Ops 错误日志读取，不代表请求已通过鉴权。
	ContextKeyOpsFallbackAPIKey ContextKey = "ops_fallback_api_key"
)

// ForcePlatform 返回设置强制平台的中间件
// 同时设置 request.Context（供 Service 使用）和 gin.Context（供 Handler 快速检查）
func ForcePlatform(platform string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置到 request.Context，使用 ctxkey.ForcePlatform 供 Service 层读取
		ctx := context.WithValue(c.Request.Context(), ctxkey.ForcePlatform, platform)
		c.Request = c.Request.WithContext(ctx)
		// 同时设置到 gin.Context，供 Handler 快速检查
		c.Set(string(ContextKeyForcePlatform), platform)
		c.Next()
	}
}

// HasForcePlatform 检查是否有强制平台（用于 Handler 跳过分组检查）
func HasForcePlatform(c *gin.Context) bool {
	_, exists := c.Get(string(ContextKeyForcePlatform))
	return exists
}

// GetForcePlatformFromContext 从 gin.Context 获取强制平台
func GetForcePlatformFromContext(c *gin.Context) (string, bool) {
	value, exists := c.Get(string(ContextKeyForcePlatform))
	if !exists {
		return "", false
	}
	platform, ok := value.(string)
	return platform, ok
}

// ErrorResponse 标准错误响应结构
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewErrorResponse 创建错误响应
func NewErrorResponse(code, message string) ErrorResponse {
	return ErrorResponse{
		Code:    code,
		Message: message,
	}
}

// AbortWithError 中断请求并返回JSON错误
func AbortWithError(c *gin.Context, statusCode int, code, message string) {
	c.JSON(statusCode, NewErrorResponse(code, message))
	c.Abort()
}

// abortWithOpenAIQuotaError writes the OpenAI-compatible insufficient quota response.
func abortWithOpenAIQuotaError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "insufficient_quota",
			"param":   nil,
			"code":    "insufficient_quota",
		},
	})
	c.Abort()
}

// ──────────────────────────────────────────────────────────
// RequireGroupAssignment — 未分组 Key 拦截中间件
// ──────────────────────────────────────────────────────────

// GatewayErrorWriter 定义网关错误响应格式（不同协议使用不同格式）
type GatewayErrorWriter func(c *gin.Context, status int, message string)

// AnthropicErrorWriter 按 Anthropic API 规范输出错误
func AnthropicErrorWriter(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"type":  "error",
		"error": gin.H{"type": "permission_error", "message": message},
	})
}

// GoogleErrorWriter 按 Google API 规范输出错误
func GoogleErrorWriter(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
}

// WriteGatewayRateLimitError 按请求协议输出与上游一致的 429 限流错误（rate_limit_error）。
// 用于订阅额度/份额用尽——对用户呈现为标准的上游限流，不泄露"share/额度"等内部概念，
// 客户端(如 Claude Code)据此正常退避重试。
func WriteGatewayRateLimitError(c *gin.Context, message string) {
	path := c.Request.URL.Path
	switch {
	case strings.Contains(path, "/v1beta") || strings.Contains(path, "/v1internal"):
		// Gemini 形态
		GoogleErrorWriter(c, http.StatusTooManyRequests, message)
	case strings.Contains(path, "/chat/completions") ||
		strings.Contains(path, "/responses") ||
		strings.Contains(path, "/embeddings") ||
		strings.Contains(path, "/images"):
		// OpenAI 形态
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": gin.H{"type": "rate_limit_error", "message": message},
		})
	default:
		// Anthropic 形态（/v1/messages 等，默认）
		c.JSON(http.StatusTooManyRequests, gin.H{
			"type":  "error",
			"error": gin.H{"type": "rate_limit_error", "message": message},
		})
	}
	c.Abort()
}

// RequireGroupAssignment 检查 API Key 是否已分配到分组，
// 如果未分组且系统设置不允许未分组 Key 调度则返回 403。
func RequireGroupAssignment(settingService *service.SettingService, writeError GatewayErrorWriter) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		if !ok || apiKey.GroupID != nil {
			c.Next()
			return
		}
		// 未分组 Key — 检查系统设置
		if settingService.IsUngroupedKeySchedulingAllowed(c.Request.Context()) {
			c.Next()
			return
		}
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnassigned)
		MarkIngressRejected(c, IngressRejectGroupUnassigned)
		writeError(c, http.StatusForbidden, "API Key is not assigned to any group and cannot be used. Please contact the administrator to assign it to a group.")
		c.Abort()
	}
}
