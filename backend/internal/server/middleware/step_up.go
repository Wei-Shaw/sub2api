package middleware

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// StepUpAuthMiddleware 敏感操作 step-up 2FA 门控中间件类型。
type StepUpAuthMiddleware gin.HandlerFunc

// RequiredStepUpAuthMiddleware 是不受 step_up_enabled 功能开关影响的强制门控。
// 它用于上游凭证导入、OAuth token 交换等一旦被滥用便可持久化接管资源的操作。
type RequiredStepUpAuthMiddleware gin.HandlerFunc

const contextKeyRequiredStepUp = "sub2api.required_step_up"

// stepUpGrantChecker 抽象 TOTP step-up 授权检查能力（由 TotpService 实现）。
type stepUpGrantChecker interface {
	HasStepUpGrant(ctx context.Context, userID int64, sessionKey string) (bool, error)
}

// stepUpUserReader 抽象用户读取能力（检查 TOTP 是否启用）。
type stepUpUserReader interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

// stepUpSettingReader 抽象 step-up 功能开关读取能力（由 SettingService 实现）。
type stepUpSettingReader interface {
	IsStepUpEnabled(ctx context.Context) bool
}

// StepUpSessionKey 计算 step-up 授权的会话键：
// 优先绑定当前会话（refresh token family），无会话 ID 的旧 token 退化为用户级键。
func StepUpSessionKey(c *gin.Context, userID int64) string {
	if sid := c.GetString(ContextKeySessionID); sid != "" {
		return sid
	}
	return fmt.Sprintf("u%d", userID)
}

// NewStepUpAuthMiddleware 创建敏感操作 step-up 2FA 门控中间件。
//
// 功能开关 step_up_enabled（默认关闭）关闭时中间件直接放行，行为与门控引入前一致。
// 开启时的通过条件（全部满足）：
//  1. 必须是 JWT 认证的真人会话——admin API key（机器凭证）一律拒绝
//  2. 当前用户已启用 TOTP（未启用则拒绝并提示先启用 2FA）
//  3. 当前会话在有效期内完成过 TOTP step-up 验证（POST /api/v1/user/totp/step-up）
//
// 失败响应使用可区分的错误码，前端据此弹出 TOTP 验证对话框后重试。
func NewStepUpAuthMiddleware(
	totpService *service.TotpService,
	userService *service.UserService,
	settingService *service.SettingService,
) StepUpAuthMiddleware {
	return StepUpAuthMiddleware(stepUpAuth(
		stepUpGrantCheckerOrNil(totpService),
		stepUpUserReaderOrNil(userService),
		stepUpSettingsOrNil(settingService),
	))
}

// NewRequiredStepUpAuthMiddleware 将既有 step-up 门控提升为强制门控。
// 强制门控跳过功能开关读取，因此设置关闭或读取异常都不能放行敏感请求。
func NewRequiredStepUpAuthMiddleware(stepUpAuth StepUpAuthMiddleware) RequiredStepUpAuthMiddleware {
	return RequiredStepUpAuthMiddleware(func(c *gin.Context) {
		if stepUpAuth == nil {
			AbortWithError(c, 503, "STEP_UP_UNAVAILABLE", "Step-up verification service unavailable")
			return
		}
		c.Set(contextKeyRequiredStepUp, true)
		gin.HandlerFunc(stepUpAuth)(c)
	})
}

// IsStepUpRequired reports whether the current route requires step-up regardless of the feature flag.
func IsStepUpRequired(c *gin.Context) bool {
	if c == nil {
		return false
	}
	required, ok := c.Get(contextKeyRequiredStepUp)
	if !ok {
		return false
	}
	value, ok := required.(bool)
	return ok && value
}

// stepUpSettingsOrNil 将可能为 nil 的具体指针归一化为接口，
// 避免 typed-nil 装箱后绕过 enforceStepUp 内的 nil 判断。
func stepUpSettingsOrNil(settingService *service.SettingService) stepUpSettingReader {
	if settingService == nil {
		return nil
	}
	return settingService
}

func stepUpGrantCheckerOrNil(totpService *service.TotpService) stepUpGrantChecker {
	if totpService == nil {
		return nil
	}
	return totpService
}

func stepUpUserReaderOrNil(userService *service.UserService) stepUpUserReader {
	if userService == nil {
		return nil
	}
	return userService
}

func stepUpAuth(grantChecker stepUpGrantChecker, userReader stepUpUserReader, settings stepUpSettingReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enforceStepUp(c, grantChecker, userReader, settings) {
			return
		}
		c.Next()
	}
}

// EnforceStepUp 对当前请求执行与 StepUpAuthMiddleware 相同语义的 step-up 门控，
// 供 handler 在需要按请求内容条件触发时调用（如仅当把用户角色提升为管理员时）。
// 校验失败时写入错误响应并中止请求，返回 false；通过返回 true。
func EnforceStepUp(
	c *gin.Context,
	totpService *service.TotpService,
	userService *service.UserService,
	settingService *service.SettingService,
) bool {
	return enforceStepUp(
		c,
		stepUpGrantCheckerOrNil(totpService),
		stepUpUserReaderOrNil(userService),
		stepUpSettingsOrNil(settingService),
	)
}

// EnforceStepUpAlways 与 EnforceStepUp 语义相同但不读取功能开关，无条件执行门控。
// 供调用方已确知门控必须生效的场景使用（如"关闭 step-up 开关"本身：调用方刚从
// 持久化设置读到开关为开启状态，不应依赖二次读取——读取失败会导致门控被跳过）。
func EnforceStepUpAlways(
	c *gin.Context,
	totpService *service.TotpService,
	userService *service.UserService,
) bool {
	return enforceStepUp(c, stepUpGrantCheckerOrNil(totpService), stepUpUserReaderOrNil(userService), nil)
}

func enforceStepUp(c *gin.Context, grantChecker stepUpGrantChecker, userReader stepUpUserReader, settings stepUpSettingReader) bool {
	// 功能开关关闭时直接放行（含 admin API key），恢复门控引入前的行为。
	// 强制门控路由不读取功能开关；settings 为 nil 时也保持门控（fail-closed）。
	if !IsStepUpRequired(c) && settings != nil && !settings.IsStepUpEnabled(c.Request.Context()) {
		return true
	}

	if c.GetString("auth_method") == service.AuditAuthMethodAdminAPIKey {
		AbortWithError(c, 403, "STEP_UP_ADMIN_API_KEY_FORBIDDEN",
			"Admin API key cannot access this endpoint; a two-factor verified admin session is required")
		return false
	}
	subject, ok := GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
		return false
	}
	if grantChecker == nil || userReader == nil {
		AbortWithError(c, 503, "STEP_UP_UNAVAILABLE", "Step-up verification service unavailable")
		return false
	}

	user, err := userReader.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		AbortWithError(c, 500, "INTERNAL_ERROR", "Failed to load user")
		return false
	}
	if !user.TotpEnabled {
		AbortWithError(c, 403, "STEP_UP_TOTP_NOT_ENABLED",
			"This operation requires two-factor authentication; please enable TOTP first")
		return false
	}

	sessionKey := StepUpSessionKey(c, subject.UserID)
	granted, err := grantChecker.HasStepUpGrant(c.Request.Context(), subject.UserID, sessionKey)
	if err != nil {
		// 安全门控故障时选择 fail-closed。
		AbortWithError(c, 503, "STEP_UP_UNAVAILABLE", "Step-up verification service unavailable")
		return false
	}
	if !granted {
		AbortWithError(c, 403, "STEP_UP_REQUIRED",
			"This operation requires recent two-factor verification")
		return false
	}

	return true
}
