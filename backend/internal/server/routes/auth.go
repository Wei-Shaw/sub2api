package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterAuthRoutes 注册认证相关路由
func RegisterAuthRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth servermiddleware.JWTAuthMiddleware,
	redisClient *redis.Client,
	settingService *service.SettingService,
) {
	// 创建速率限制器
	rateLimiter := middleware.NewRateLimiter(redisClient)

	// 公开接口
	auth := v1.Group("/auth")
	auth.Use(servermiddleware.BackendModeAuthGuard(settingService))
	{
		// 注册/登录/2FA/验证码发送均属于高风险入口，增加服务端兜底限流（Redis 故障时 fail-close）
		auth.POST("/register", rateLimiter.LimitWithOptions("auth-register", 5, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.Register)
		auth.POST("/login", rateLimiter.LimitWithOptions("auth-login", 20, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.Login)
		auth.POST("/login/2fa", rateLimiter.LimitWithOptions("auth-login-2fa", 20, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.Login2FA)
		auth.POST("/send-verify-code", rateLimiter.LimitWithOptions("auth-send-verify-code", 5, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.SendVerifyCode)
		// Token刷新接口添加速率限制：每分钟最多 30 次（Redis 故障时 fail-close）
		auth.POST("/refresh", rateLimiter.LimitWithOptions("refresh-token", 30, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.RefreshToken)
		// 登出接口（公开，允许未认证用户调用以撤销Refresh Token）
		auth.POST("/logout", h.Auth.Logout)
		// 优惠码验证接口添加速率限制：每分钟最多 10 次（Redis 故障时 fail-close）
		auth.POST("/validate-promo-code", rateLimiter.LimitWithOptions("validate-promo", 10, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.ValidatePromoCode)
		// 邀请码验证接口添加速率限制：每分钟最多 10 次（Redis 故障时 fail-close）
		auth.POST("/validate-invitation-code", rateLimiter.LimitWithOptions("validate-invitation", 10, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.ValidateInvitationCode)
		// 忘记密码接口添加速率限制：每分钟最多 5 次（Redis 故障时 fail-close）
		auth.POST("/forgot-password", rateLimiter.LimitWithOptions("forgot-password", 5, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.ForgotPassword)
		// 重置密码接口添加速率限制：每分钟最多 10 次（Redis 故障时 fail-close）
		auth.POST("/reset-password", rateLimiter.LimitWithOptions("reset-password", 10, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.ResetPassword)
		auth.GET("/oauth/linuxdo/start", h.Auth.LinuxDoOAuthStart)
		auth.GET("/oauth/linuxdo/bind/start", func(c *gin.Context) {
			query := c.Request.URL.Query()
			query.Set("intent", "bind_current_user")
			c.Request.URL.RawQuery = query.Encode()
			h.Auth.LinuxDoOAuthStart(c)
		})
		auth.GET("/oauth/linuxdo/callback", h.Auth.LinuxDoOAuthCallback)
		auth.GET("/oauth/wechat/start", h.Auth.WeChatOAuthStart)
		auth.GET("/oauth/wechat/bind/start", func(c *gin.Context) {
			query := c.Request.URL.Query()
			query.Set("intent", "bind_current_user")
			c.Request.URL.RawQuery = query.Encode()
			h.Auth.WeChatOAuthStart(c)
		})
		auth.GET("/oauth/wechat/callback", h.Auth.WeChatOAuthCallback)
		auth.GET("/oauth/wechat/payment/start", h.Auth.WeChatPaymentOAuthStart)
		auth.GET("/oauth/wechat/payment/callback", h.Auth.WeChatPaymentOAuthCallback)
		auth.POST("/oauth/pending/exchange",
			rateLimiter.LimitWithOptions("oauth-pending-exchange", 20, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.ExchangePendingOAuthCompletion,
		)
		auth.POST("/oauth/pending/send-verify-code",
			rateLimiter.LimitWithOptions("oauth-pending-send-verify-code", 5, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.SendPendingOAuthVerifyCode,
		)
		auth.POST("/oauth/pending/create-account",
			rateLimiter.LimitWithOptions("oauth-pending-create-account", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.CreatePendingOAuthAccount,
		)
		auth.POST("/oauth/pending/bind-login",
			rateLimiter.LimitWithOptions("oauth-pending-bind-login", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.BindPendingOAuthLogin,
		)
		auth.POST("/oauth/linuxdo/complete-registration",
			rateLimiter.LimitWithOptions("oauth-linuxdo-complete", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.CompleteLinuxDoOAuthRegistration,
		)
		auth.POST("/oauth/linuxdo/bind-login",
			rateLimiter.LimitWithOptions("oauth-linuxdo-bind-login", 20, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.BindLinuxDoOAuthLogin,
		)
		auth.POST("/oauth/linuxdo/create-account",
			rateLimiter.LimitWithOptions("oauth-linuxdo-create-account", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.CreateLinuxDoOAuthAccount,
		)
		auth.POST("/oauth/wechat/complete-registration",
			rateLimiter.LimitWithOptions("oauth-wechat-complete", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.CompleteWeChatOAuthRegistration,
		)
		auth.POST("/oauth/wechat/bind-login",
			rateLimiter.LimitWithOptions("oauth-wechat-bind-login", 20, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.BindWeChatOAuthLogin,
		)
		auth.POST("/oauth/wechat/create-account",
			rateLimiter.LimitWithOptions("oauth-wechat-create-account", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.CreateWeChatOAuthAccount,
		)
		auth.GET("/oauth/oidc/start", h.Auth.OIDCOAuthStart)
		auth.GET("/oauth/oidc/bind/start", func(c *gin.Context) {
			query := c.Request.URL.Query()
			query.Set("intent", "bind_current_user")
			c.Request.URL.RawQuery = query.Encode()
			h.Auth.OIDCOAuthStart(c)
		})
		auth.GET("/oauth/oidc/callback", h.Auth.OIDCOAuthCallback)
		auth.POST("/oauth/oidc/complete-registration",
			rateLimiter.LimitWithOptions("oauth-oidc-complete", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.CompleteOIDCOAuthRegistration,
		)
		auth.POST("/oauth/oidc/bind-login",
			rateLimiter.LimitWithOptions("oauth-oidc-bind-login", 20, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.BindOIDCOAuthLogin,
		)
		auth.POST("/oauth/oidc/create-account",
			rateLimiter.LimitWithOptions("oauth-oidc-create-account", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.CreateOIDCOAuthAccount,
		)
	}

	// 公开设置（无需认证）
	settings := v1.Group("/settings")
	{
		settings.GET("/public", h.Setting.GetPublicSettings)
	}

	// 需要认证的当前用户信息
	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(servermiddleware.BackendModeUserGuard(settingService))
	{
		authenticated.GET("/auth/me", h.Auth.GetCurrentUser)
		// 撤销所有会话（需要认证）
		authenticated.POST("/auth/revoke-all-sessions", h.Auth.RevokeAllSessions)
		authenticated.POST("/auth/oauth/bind-token", h.Auth.PrepareOAuthBindAccessTokenCookie)
	}
}
