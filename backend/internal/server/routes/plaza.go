package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterPlazaRoutes 注册公开计费广场路由（无需认证）。
//
// 包含两个端点：
//   - GET /api/v1/plaza/models  匿名查看模型定价
//   - GET /api/v1/plaza/plans   匿名查看在售套餐
//
// 与 /api/v1/settings/public 在同一公开链路上，但增加了一层 Redis 边界限流
// （每分钟 60 次/IP，Redis 不可用时降级 fail-open，不阻断匿名访问）。
func RegisterPlazaRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	redisClient *redis.Client,
) {
	rateLimiter := middleware.NewRateLimiter(redisClient)

	plaza := v1.Group("/plaza")
	{
		plaza.GET("/models",
			rateLimiter.LimitWithOptions("plaza-models", 60, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailOpen,
			}),
			h.Plaza.ListModels,
		)
		plaza.GET("/plans",
			rateLimiter.LimitWithOptions("plaza-plans", 60, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailOpen,
			}),
			h.Plaza.ListPlans,
		)
	}
}
