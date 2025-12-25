package infrastructure

import (
	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ProviderSet 提供基础设施层的依赖
var ProviderSet = wire.NewSet(
	ProvideDB,
	ProvideRedis,
)

// ProvideDB 提供数据库连接
func ProvideDB(cfg *config.Config) (*gorm.DB, error) {
	return InitDB(cfg)
}

// ProvideRedis 提供 Redis 客户端
func ProvideRedis(cfg *config.Config) *redis.Client {
	return InitRedis(cfg)
}
