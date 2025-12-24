package handler

import (
	"net/http"
	"strings"
	"sub2api/internal/model"
	"sub2api/internal/pkg/response"
	"sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// RegisterRequest represents the registration request payload
type RegisterRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required,min=6"`
	VerifyCode     string `json:"verify_code"`
	TurnstileToken string `json:"turnstile_token"`
}

// SendVerifyCodeRequest 发送验证码请求
type SendVerifyCodeRequest struct {
	Email          string `json:"email" binding:"required,email"`
	TurnstileToken string `json:"turnstile_token"`
}

// SendVerifyCodeResponse 发送验证码响应
type SendVerifyCodeResponse struct {
	Message   string `json:"message"`
	Countdown int    `json:"countdown"` // 倒计时秒数
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required"`
	TurnstileToken string `json:"turnstile_token"`
}

// AuthResponse 认证响应格式（匹配前端期望）
type AuthResponse struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type"`
	User        *model.User `json:"user"`
}

// Register handles user registration
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Turnstile 验证（当提供了邮箱验证码时跳过，因为发送验证码时已验证过）
	if req.VerifyCode == "" {
		if err := h.authService.VerifyTurnstile(c.Request.Context(), req.TurnstileToken, c.ClientIP()); err != nil {
			response.BadRequest(c, "Turnstile verification failed: "+err.Error())
			return
		}
	}

	token, user, err := h.authService.RegisterWithVerification(c.Request.Context(), req.Email, req.Password, req.VerifyCode)
	if err != nil {
		response.BadRequest(c, "Registration failed: "+err.Error())
		return
	}

	// 写入 HttpOnly Cookie，便于浏览器基于 Cookie 的会话鉴权。
	setAuthCookie(c, h.authService, token)
	response.Success(c, AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		User:        user,
	})
}

// SendVerifyCode 发送邮箱验证码
// POST /api/v1/auth/send-verify-code
func (h *AuthHandler) SendVerifyCode(c *gin.Context) {
	var req SendVerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Turnstile 验证
	if err := h.authService.VerifyTurnstile(c.Request.Context(), req.TurnstileToken, c.ClientIP()); err != nil {
		response.BadRequest(c, "Turnstile verification failed: "+err.Error())
		return
	}

	result, err := h.authService.SendVerifyCodeAsync(c.Request.Context(), req.Email)
	if err != nil {
		response.BadRequest(c, "Failed to send verification code: "+err.Error())
		return
	}

	response.Success(c, SendVerifyCodeResponse{
		Message:   "Verification code sent successfully",
		Countdown: result.Countdown,
	})
}

// Login handles user login
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Turnstile 验证
	if err := h.authService.VerifyTurnstile(c.Request.Context(), req.TurnstileToken, c.ClientIP()); err != nil {
		response.BadRequest(c, "Turnstile verification failed: "+err.Error())
		return
	}

	token, user, err := h.authService.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		response.Unauthorized(c, "Login failed: "+err.Error())
		return
	}

	// 写入 HttpOnly Cookie，避免前端持久化 token。
	setAuthCookie(c, h.authService, token)
	response.Success(c, AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		User:        user,
	})
}

// Logout 用户退出登录
// 接口: POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	clearAuthCookie(c, h.authService)
	response.Success(c, gin.H{"message": "Logged out"})
}

// GetCurrentUser 获取当前登录用户信息
// GET /api/v1/auth/me
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userValue, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, ok := userValue.(*model.User)
	if !ok {
		response.InternalError(c, "Invalid user context")
		return
	}

	response.Success(c, user)
}

// setAuthCookie 写入 HttpOnly Cookie，让浏览器自动携带会话令牌。
func setAuthCookie(c *gin.Context, authService *service.AuthService, token string) {
	// Cookie 的 SameSite/Secure 策略由配置决定。
	cookie := &http.Cookie{
		Name:     authService.AuthCookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: authService.AuthCookieSameSite(),
		Secure:   authService.AuthCookieSecure(isSecureRequest(c)),
		MaxAge:   authService.AuthCookieMaxAgeSeconds(),
	}
	http.SetCookie(c.Writer, cookie)
}

// clearAuthCookie 清理 Cookie，触发客户端登出。
func clearAuthCookie(c *gin.Context, authService *service.AuthService) {
	// 清理 Cookie 时保持一致的 SameSite/Secure，确保覆盖成功。
	cookie := &http.Cookie{
		Name:     authService.AuthCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: authService.AuthCookieSameSite(),
		Secure:   authService.AuthCookieSecure(isSecureRequest(c)),
		MaxAge:   -1,
	}
	http.SetCookie(c.Writer, cookie)
}

// isSecureRequest 用于判断当前请求是否走 HTTPS（含反向代理场景）。
func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		parts := strings.Split(proto, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) == "https" {
			return true
		}
	}
	return false
}
