package infrastructure

import (
	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/redis/go-redis/v9"
)

// InitRedis 初始化 Redis 客户端
func InitRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}
