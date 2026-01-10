package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimiter Redis 速率限制器
type RateLimiter struct {
	redis  *redis.Client
	prefix string
REDACTED

// NewRateLimiter 创建速率限制器实例
func NewRateLimiter(redisClient *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis:  redisClient,
		prefix: "rate_limit:",
REDACTED
REDACTED

// Limit 返回速率限制中间件
// key: 限制类型标识
// limit: 时间窗口内最大请求数
// window: 时间窗口
func (r *RateLimiter) Limit(key string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		redisKey := r.prefix + key + ":" + ip

		ctx := c.Request.Context()

		// 使用 INCR 原子操作增加计数
		count, err := r.redis.Incr(ctx, redisKey).Result()
		if err != nil {
			// Redis 错误时放行，避免影响正常服务
			c.Next()
			return
	REDACTED

		// 首次访问时设置过期时间
		if count == 1 {
			r.redis.Expire(ctx, redisKey, window)
	REDACTED

		// 超过限制
		if count > int64(limit) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate limit exceeded",
				"message": "Too many requests, please try again later",
		REDACTED)
			return
	REDACTED

		c.Next()
REDACTED
REDACTED
