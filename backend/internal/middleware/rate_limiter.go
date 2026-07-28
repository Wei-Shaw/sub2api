package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	clientip "github.com/Wei-Shaw/sub2api/internal/pkg/ip"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RateLimitFailureMode Redis 故障策略
type RateLimitFailureMode int

const (
	RateLimitFailOpen RateLimitFailureMode = iota
	RateLimitFailClose
)

// RateLimitOptions 限流可选配置
type RateLimitOptions struct {
	FailureMode RateLimitFailureMode
}

var rateLimitScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
local repaired = 0
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
elseif ttl == -1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  repaired = 1
end
return {current, repaired}
`)

var slidingLimitScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
local count = redis.call('ZCARD', KEYS[1])
if count >= limit then
  return {0, count}
end
redis.call('ZADD', KEYS[1], now, ARGV[4])
redis.call('PEXPIRE', KEYS[1], window + 60000)
return {1, count + 1}
`)

var successfulLimitReserveScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local pending_until = tonumber(ARGV[5])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now)
local count = redis.call('ZCARD', KEYS[1]) + redis.call('ZCARD', KEYS[2])
if count >= limit then
  return {0, count}
end
redis.call('ZADD', KEYS[2], pending_until, ARGV[4])
redis.call('PEXPIRE', KEYS[1], window + 60000)
redis.call('PEXPIRE', KEYS[2], window + 60000)
return {1, count + 1}
`)

var successfulLimitCommitScript = redis.NewScript(`
local removed = redis.call('ZREM', KEYS[2], ARGV[1])
if removed == 0 then
  return 0
end
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[1])
redis.call('PEXPIRE', KEYS[1], ARGV[3])
return 1
`)

// rateLimitRun 允许测试覆写脚本执行逻辑
var rateLimitRun = func(ctx context.Context, client *redis.Client, key string, windowMillis int64) (int64, bool, error) {
	values, err := rateLimitScript.Run(ctx, client, []string{key}, windowMillis).Slice()
	if err != nil {
		return 0, false, err
	}
	if len(values) < 2 {
		return 0, false, fmt.Errorf("rate limit script returned %d values", len(values))
	}
	count, err := parseInt64(values[0])
	if err != nil {
		return 0, false, err
	}
	repaired, err := parseInt64(values[1])
	if err != nil {
		return 0, false, err
	}
	return count, repaired == 1, nil
}

var slidingLimitRun = func(ctx context.Context, client *redis.Client, key string, nowMillis, windowMillis int64, limit int, member string) (bool, int64, error) {
	values, err := slidingLimitScript.Run(ctx, client, []string{key}, nowMillis, windowMillis, limit, member).Slice()
	if err != nil {
		return false, 0, err
	}
	return parseAllowedAndCount(values)
}

var successfulLimitReserveRun = func(ctx context.Context, client *redis.Client, committedKey, pendingKey string, nowMillis, windowMillis int64, limit int, member string, pendingUntilMillis int64) (bool, int64, error) {
	values, err := successfulLimitReserveScript.Run(ctx, client, []string{committedKey, pendingKey}, nowMillis, windowMillis, limit, member, pendingUntilMillis).Slice()
	if err != nil {
		return false, 0, err
	}
	return parseAllowedAndCount(values)
}

var successfulLimitCommitRun = func(ctx context.Context, client *redis.Client, committedKey, pendingKey, member string, nowMillis, ttlMillis int64) (bool, error) {
	value, err := successfulLimitCommitScript.Run(ctx, client, []string{committedKey, pendingKey}, member, nowMillis, ttlMillis).Int64()
	return value == 1, err
}

var successfulLimitReleaseRun = func(ctx context.Context, client *redis.Client, pendingKey, member string) error {
	return client.ZRem(ctx, pendingKey, member).Err()
}

// RateLimiter Redis 速率限制器
type RateLimiter struct {
	redis  *redis.Client
	prefix string
}

// NewRateLimiter 创建速率限制器实例
func NewRateLimiter(redisClient *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis:  redisClient,
		prefix: "rate_limit:",
	}
}

// AllowResult 单次固定窗口限流判定结果。
type AllowResult struct {
	// Allowed 是否放行
	Allowed bool
	// Count 当前窗口内累计请求数（含本次）
	Count int64
	// RetryAfter 超限时距窗口重置的剩余时间（尽力而为；PTTL 不可用时回退为完整窗口）
	RetryAfter time.Duration
}

// Allow 对给定 key（不含 "rate_limit:" 前缀）执行一次固定窗口计数判定。
// 供需要自定义限流维度（如按用户 ID）的调用方使用；Redis 错误由调用方决定 fail-open/close。
func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (AllowResult, error) {
	redisKey := r.prefix + key
	windowMillis := windowTTLMillis(window)

	count, repaired, err := rateLimitRun(ctx, r.redis, redisKey, windowMillis)
	if err != nil {
		return AllowResult{}, err
	}
	if repaired {
		log.Printf("[RateLimit] ttl repaired: key=%s window_ms=%d", redisKey, windowMillis)
	}

	result := AllowResult{Allowed: count <= int64(limit), Count: count}
	if !result.Allowed {
		result.RetryAfter = window
		if ttl, ttlErr := r.redis.PTTL(ctx, redisKey).Result(); ttlErr == nil && ttl > 0 {
			result.RetryAfter = ttl
		}
	}
	return result, nil
}

// Limit 返回速率限制中间件
// key: 限制类型标识
// limit: 时间窗口内最大请求数
// window: 时间窗口
func (r *RateLimiter) Limit(key string, limit int, window time.Duration) gin.HandlerFunc {
	return r.LimitWithOptions(key, limit, window, RateLimitOptions{})
}

// LimitWithOptions 返回速率限制中间件（带可选配置）
func (r *RateLimiter) LimitWithOptions(key string, limit int, window time.Duration, opts RateLimitOptions) gin.HandlerFunc {
	failureMode := opts.FailureMode
	if failureMode != RateLimitFailClose {
		failureMode = RateLimitFailOpen
	}

	return func(c *gin.Context) {
		prefix := clientip.AbuseClientPrefix(c)
		if prefix == "" {
			log.Printf("[RateLimit] client IP unavailable: key=%s mode=%s", key, failureModeLabel(failureMode))
			if failureMode == RateLimitFailClose {
				abortRateLimit(c, window)
				return
			}
			c.Next()
			return
		}
		redisKey := r.prefix + key + ":" + prefix
		result, err := r.Allow(c.Request.Context(), key+":"+prefix, limit, window)
		if err != nil {
			log.Printf("[RateLimit] redis error: key=%s mode=%s err=%v", redisKey, failureModeLabel(failureMode), err)
			if failureMode == RateLimitFailClose {
				abortRateLimit(c, window)
				return
			}
			// Redis 错误时放行，避免影响正常服务
			c.Next()
			return
		}

		// 超过限制
		if !result.Allowed {
			abortRateLimit(c, result.RetryAfter)
			return
		}

		c.Next()
	}
}

// LimitSlidingWithOptions limits attempts using an atomic rolling window.
func (r *RateLimiter) LimitSlidingWithOptions(key string, limit int, window time.Duration, opts RateLimitOptions) gin.HandlerFunc {
	failureMode := normalizeFailureMode(opts.FailureMode)
	return func(c *gin.Context) {
		prefix := clientip.AbuseClientPrefix(c)
		if prefix == "" {
			handleLimiterFailure(c, key, failureMode, fmt.Errorf("client IP unavailable"))
			return
		}

		r.limitSliding(c, key+":"+prefix, limit, window, failureMode)
	}
}

// LimitEmailSlidingWithOptions adds a rolling quota for the normalized email
// in a JSON request body. Redis keys contain only a truncated SHA-256 digest.
func (r *RateLimiter) LimitEmailSlidingWithOptions(key string, limit int, window time.Duration, opts RateLimitOptions) gin.HandlerFunc {
	failureMode := normalizeFailureMode(opts.FailureMode)
	return func(c *gin.Context) {
		emailDigest, ok := requestEmailDigest(c)
		if !ok {
			c.Next()
			return
		}
		r.limitSliding(c, key+":email:"+emailDigest, limit, window, failureMode)
	}
}

func (r *RateLimiter) limitSliding(c *gin.Context, redisKeySuffix string, limit int, window time.Duration, failureMode RateLimitFailureMode) {
	redisKey := r.prefix + "v2:" + redisKeySuffix
	allowed, _, err := slidingLimitRun(c.Request.Context(), r.redis, redisKey, time.Now().UnixMilli(), windowTTLMillis(window), limit, uuid.NewString())
	if err != nil {
		handleLimiterFailure(c, redisKey, failureMode, err)
		return
	}
	if !allowed {
		abortRateLimit(c, window)
		return
	}
	c.Next()
}

func requestEmailDigest(c *gin.Context) (string, bool) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return "", false
	}
	const maxBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return "", false
	}
	if len(body) > maxBody {
		return "", false
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	if email == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(email))
	return hex.EncodeToString(sum[:16]), true
}

// LimitSuccessfulWithOptions atomically reserves capacity before a handler and
// commits it only when the handler returns a successful response. Pending
// reservations expire so crashes cannot consume a full-window quota.
func (r *RateLimiter) LimitSuccessfulWithOptions(key string, limit int, window time.Duration, opts RateLimitOptions) gin.HandlerFunc {
	failureMode := normalizeFailureMode(opts.FailureMode)
	const pendingTTL = 5 * time.Minute

	return func(c *gin.Context) {
		prefix := clientip.AbuseClientPrefix(c)
		if prefix == "" {
			handleLimiterFailure(c, key, failureMode, fmt.Errorf("client IP unavailable"))
			return
		}

		baseKey := r.prefix + "v2:" + key + ":" + prefix
		committedKey := baseKey + ":committed"
		pendingKey := baseKey + ":pending"
		member := uuid.NewString()
		now := time.Now()
		windowMillis := windowTTLMillis(window)
		allowed, _, err := successfulLimitReserveRun(
			c.Request.Context(),
			r.redis,
			committedKey,
			pendingKey,
			now.UnixMilli(),
			windowMillis,
			limit,
			member,
			now.Add(pendingTTL).UnixMilli(),
		)
		if err != nil {
			handleLimiterFailure(c, baseKey, failureMode, err)
			return
		}
		if !allowed {
			abortRateLimit(c, window)
			return
		}

		c.Next()

		updateCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		status := c.Writer.Status()
		if status >= http.StatusOK && status < http.StatusBadRequest {
			committed, commitErr := successfulLimitCommitRun(updateCtx, r.redis, committedKey, pendingKey, member, time.Now().UnixMilli(), windowMillis+int64(time.Minute/time.Millisecond))
			if commitErr != nil || !committed {
				log.Printf("[RateLimit] successful quota commit failed: key=%s committed=%t err=%v", baseKey, committed, commitErr)
			}
			return
		}
		if releaseErr := successfulLimitReleaseRun(updateCtx, r.redis, pendingKey, member); releaseErr != nil {
			log.Printf("[RateLimit] successful quota release failed: key=%s err=%v", baseKey, releaseErr)
		}
	}
}

func normalizeFailureMode(mode RateLimitFailureMode) RateLimitFailureMode {
	if mode == RateLimitFailClose {
		return RateLimitFailClose
	}
	return RateLimitFailOpen
}

func handleLimiterFailure(c *gin.Context, key string, mode RateLimitFailureMode, err error) {
	log.Printf("[RateLimit] redis error: key=%s mode=%s err=%v", key, failureModeLabel(mode), err)
	if mode == RateLimitFailClose {
		abortRateLimit(c, 0)
		return
	}
	c.Next()
}

func parseAllowedAndCount(values []any) (bool, int64, error) {
	if len(values) < 2 {
		return false, 0, fmt.Errorf("rate limit script returned %d values", len(values))
	}
	allowed, err := parseInt64(values[0])
	if err != nil {
		return false, 0, err
	}
	count, err := parseInt64(values[1])
	if err != nil {
		return false, 0, err
	}
	return allowed == 1, count, nil
}

func windowTTLMillis(window time.Duration) int64 {
	ttl := window.Milliseconds()
	if ttl < 1 {
		return 1
	}
	return ttl
}

func abortRateLimit(c *gin.Context, retryAfter time.Duration) {
	if retryAfter > 0 {
		seconds := int64(retryAfter / time.Second)
		if retryAfter%time.Second > 0 {
			seconds++
		}
		c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	}
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error":   "rate limit exceeded",
		"message": "Too many requests, please try again later",
	})
}

func failureModeLabel(mode RateLimitFailureMode) string {
	if mode == RateLimitFailClose {
		return "fail-close"
	}
	return "fail-open"
}

func parseInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected value type %T", value)
	}
}
