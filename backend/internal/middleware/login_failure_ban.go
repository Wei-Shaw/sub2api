package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	ippkg "github.com/Wei-Shaw/sub2api/internal/pkg/ip"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// loginBanFailScript 原子累加失败计数并维护窗口 TTL：
// 首次失败设置窗口过期；TTL 丢失（key 存活但无过期，异常路径遗留）时修复，
// 与 rateLimitScript 的 ttl-repair 策略一致。
var loginBanFailScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// LoginBanOptions 登录失败自动封禁配置。
type LoginBanOptions struct {
	// Enabled 是否启用。
	Enabled bool
	// Threshold 窗口内触发封禁的失败次数。
	Threshold int
	// Window 失败计数统计窗口。
	Window time.Duration
	// BanDuration 封禁时长（自最近一次触发起算，持续失败会续期）。
	BanDuration time.Duration
	// IPv6PrefixBits IPv6 地址归并到该前缀长度的网段再计数/封禁；0 表示不归并。
	IPv6PrefixBits int
	// ExemptIPs 豁免的客户端地址（精确 IP 或 CIDR），不参与失败计数也不受封禁影响。
	ExemptIPs []string
}

// LoginFailureBanGuard 登录失败自动封禁。
//
// 行为：
//   - 前置：命中封禁 key 的客户端 IP 直接 429（带 Retry-After），不进入登录链路；
//   - 后置：响应状态码 >= 400 视为一次登录失败，按 IP 累计到固定窗口计数器；
//     达到阈值后写入带 TTL 的封禁 key。
//
// 设计要点：
//   - 复用限流的客户端 IP 解析（GetSecurityClientIP，兼容反代可信链），仅对
//     公网单播地址生效：回环/内网/链路本地地址跳过，反代内部转发与本地健康
//     检查不会被封，共享出口（所有请求源 IP 坍缩为网关地址）同样豁免；
//   - Redis 故障一律 fail-open：封禁是保护措施，不能反过来打挂登录入口；
//   - 计数与封禁 key 均带 TTL：攻击持续时封禁自动续期，攻击停止后窗口计数
//     与封禁依次过期，无残留状态、无需人工清理。
type LoginFailureBanGuard struct {
	redis       *redis.Client
	options     LoginBanOptions
	exemptRules *ippkg.CompiledIPRules
}

// NewLoginFailureBanGuard 创建登录失败自动封禁中间件。
func NewLoginFailureBanGuard(redisClient *redis.Client, options LoginBanOptions) *LoginFailureBanGuard {
	return &LoginFailureBanGuard{
		redis:       redisClient,
		options:     options,
		exemptRules: ippkg.CompileIPRules(options.ExemptIPs),
	}
}

// Guard 返回登录失败自动封禁中间件。挂在登录类路由链最外层。
func (g *LoginFailureBanGuard) Guard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil || g.redis == nil || !g.options.Enabled || g.options.Threshold <= 0 {
			c.Next()
			return
		}

		rawIP := clientIPForRateLimit(c)
		if !isPubliclyRoutableIP(rawIP) {
			c.Next()
			return
		}
		if ippkg.MatchesIPRules(rawIP, g.exemptRules) {
			c.Next()
			return
		}
		clientKey := rateLimitClientKey(rawIP, g.options.IPv6PrefixBits)
		ctx := c.Request.Context()

		if g.abortIfBanned(ctx, c, clientKey) {
			return
		}

		c.Next()

		// 4xx/5xx 视为失败（401 凭证错误、400 参数非法等），但 429 除外：
		// 内层限流器的节流响应不是凭证校验失败，正常用户忘记密码时的
		// 快速重试不应因此加速触发封禁。
		status := c.Writer.Status()
		if status < http.StatusBadRequest || status == http.StatusTooManyRequests {
			return
		}
		if err := g.recordFailure(ctx, clientKey); err != nil {
			slog.Warn("login failure ban: redis error", "error", err, "client_ip", rawIP)
		}
	}
}

func (g *LoginFailureBanGuard) failKey(clientKey string) string {
	return "login_ban:fail:" + clientKey
}

func (g *LoginFailureBanGuard) banKey(clientKey string) string {
	return "login_ban:blocked:" + clientKey
}

// abortIfBanned 命中封禁则中断请求并返回 true。
func (g *LoginFailureBanGuard) abortIfBanned(ctx context.Context, c *gin.Context, clientKey string) bool {
	ttl, err := g.redis.PTTL(ctx, g.banKey(clientKey)).Result()
	if err != nil {
		slog.Warn("login failure ban: redis error on ban check", "error", err, "client_ip", clientKey)
		return false
	}
	if ttl <= 0 {
		return false
	}
	abortLoginBanned(c, ttl)
	return true
}

// recordFailure 累计失败次数，达到阈值则写入封禁 key。
func (g *LoginFailureBanGuard) recordFailure(ctx context.Context, clientKey string) error {
	windowMillis := windowTTLMillis(g.options.Window)
	count, err := loginBanFailScript.Run(ctx, g.redis, []string{g.failKey(clientKey)}, windowMillis).Int64()
	if err != nil {
		return err
	}
	if count < int64(g.options.Threshold) {
		return nil
	}
	if err := g.redis.Set(ctx, g.banKey(clientKey), strconv.FormatInt(count, 10), g.options.BanDuration).Err(); err != nil {
		return err
	}
	slog.Warn("login failure ban: client banned",
		"client_ip", clientKey, "failures", count,
		"window", g.options.Window.String(), "ban_duration", g.options.BanDuration.String())
	return nil
}

// isPubliclyRoutableIP 判断地址是否为可作为封禁依据的全局单播地址。
// 与面板限流的 isPubliclyRoutableClientIP 判定保持一致。
func isPubliclyRoutableIP(clientIP string) bool {
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

func abortLoginBanned(c *gin.Context, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = time.Minute
	}
	seconds := int64(retryAfter / time.Second)
	if retryAfter%time.Second > 0 {
		seconds++
	}
	c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error":   "login failure ban",
		"message": "Too many failed login attempts from this address, please try again later",
	})
}
