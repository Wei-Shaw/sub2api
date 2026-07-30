package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// panelRateLimitWindow 面板限流固定窗口时长（所有档位均按每分钟计数）。
const panelRateLimitWindow = time.Minute

// panelRateLimitAllower 抽象底层限流原语，便于单测注入。
type panelRateLimitAllower interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (middleware.AllowResult, error)
}

// PanelRateLimiter 面板（管理面 /api/v1）API 限流器。
//
// 设计要点：
//   - 认证接口按「用户 ID」维度计数：与客户端 IP 完全无关，反向代理/共享出口
//     （所有请求源 IP 坍缩为 127.0.0.1 等）不会互相误伤。
//   - 公开接口按安全客户端 IP 计数：仅统计全局单播地址，回环/内网/链路本地
//     地址（反代内部转发地址）直接跳过，避免误拦整条反代链路的流量。
//   - 配置走进程内缓存（60s TTL），热路径零 DB 访问。
//   - Redis 异常一律 fail-open：限流是保护措施，不能反过来把面板打挂。
type PanelRateLimiter struct {
	limiter        panelRateLimitAllower
	settingService *service.SettingService
}

// NewPanelRateLimiter 创建面板限流器。
func NewPanelRateLimiter(redisClient *redis.Client, settingService *service.SettingService) *PanelRateLimiter {
	return &PanelRateLimiter{
		limiter:        middleware.NewRateLimiter(redisClient),
		settingService: settingService,
	}
}

// Global 认证面板接口的全局按用户限流（宽松档，覆盖所有登录后端点）。
func (p *PanelRateLimiter) Global() gin.HandlerFunc {
	return p.userScoped("global", func(s service.PanelRateLimitSettings) int { return s.UserRPM })
}

// Heavy 重查询接口的按用户限流（严格档，覆盖 usage/dashboard 等聚合统计端点）。
// 与 Global 叠加计数：一次重查询同时消耗两档额度。
func (p *PanelRateLimiter) Heavy() gin.HandlerFunc {
	return p.userScoped("heavy", func(s service.PanelRateLimitSettings) int { return s.HeavyRPM })
}

func (p *PanelRateLimiter) userScoped(scope string, limitOf func(service.PanelRateLimitSettings) int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p == nil || p.limiter == nil || p.settingService == nil {
			c.Next()
			return
		}
		settings := p.settingService.GetPanelRateLimitSettingsCached(c.Request.Context())
		if !settings.Enabled {
			c.Next()
			return
		}
		limit := limitOf(settings)
		if limit <= 0 {
			c.Next()
			return
		}
		subject, ok := GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			// 无认证主体（认证中间件缺位时的防御分支）：放行，避免误伤
			c.Next()
			return
		}
		if settings.ExemptAdmin {
			if role, hasRole := GetUserRoleFromContext(c); hasRole && role == service.RoleAdmin {
				c.Next()
				return
			}
		}

		key := "panel:" + scope + ":user:" + strconv.FormatInt(subject.UserID, 10)
		result, err := p.limiter.Allow(c.Request.Context(), key, limit, panelRateLimitWindow)
		if err != nil {
			// fail-open：Redis 异常不阻断面板访问
			slog.Warn("panel rate limit check failed, allowing request", "scope", scope, "error", err)
			c.Next()
			return
		}
		if !result.Allowed {
			abortPanelRateLimited(c, result.RetryAfter)
			return
		}
		c.Next()
	}
}

// PublicIP 无需认证的公开接口按客户端 IP 限流。
// 使用与审计日志/会话绑定一致的安全客户端 IP 解析；解析结果为回环/内网/
// 链路本地地址时跳过计数（这类地址通常是反代内部转发地址，按它计数会把
// 整条反代链路的所有真实用户合并进同一个桶造成大面积误拦截）。
func (p *PanelRateLimiter) PublicIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p == nil || p.limiter == nil || p.settingService == nil {
			c.Next()
			return
		}
		settings := p.settingService.GetPanelRateLimitSettingsCached(c.Request.Context())
		if !settings.Enabled || settings.PublicIPRPM <= 0 {
			c.Next()
			return
		}
		clientIP := SecurityClientIP(c)
		if !isPubliclyRoutableClientIP(clientIP) {
			c.Next()
			return
		}

		result, err := p.limiter.Allow(c.Request.Context(), "panel:public:ip:"+clientIP, settings.PublicIPRPM, panelRateLimitWindow)
		if err != nil {
			slog.Warn("panel public rate limit check failed, allowing request", "error", err)
			c.Next()
			return
		}
		if !result.Allowed {
			abortPanelRateLimited(c, result.RetryAfter)
			return
		}
		c.Next()
	}
}

// isPubliclyRoutableClientIP 判断地址是否为可作为限流依据的全局单播地址。
// 回环、RFC1918/ULA 内网、链路本地与未指定地址返回 false。
func isPubliclyRoutableClientIP(clientIP string) bool {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return ip.IsGlobalUnicast()
}

func abortPanelRateLimited(c *gin.Context, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = panelRateLimitWindow
	}
	seconds := int64(retryAfter / time.Second)
	if retryAfter%time.Second > 0 {
		seconds++
	}
	c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	AbortWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests, please slow down and try again later")
}
